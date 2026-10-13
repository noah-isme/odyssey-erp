// Package compensation manages effective-dated employee compensation
// assignments (payroll_compensation_assignments) and keeps their recurring
// components (payroll_recurring_components) consistent across changes.
//
// Money values are carried as NUMERIC text strings and are never converted to
// floating point. Audit metadata never contains salary or bank values.
package compensation

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// Audit actions recorded by this package.
const (
	ActionCompensationChanged = "EMPLOYEE_COMPENSATION_CHANGED"
	ActionBankChanged         = "EMPLOYEE_BANK_CHANGED"
	auditEntity               = "hr_employee"
)

var (
	ErrCompanyRequired     = errors.New("hr: company required")
	ErrEmployeeRequired    = errors.New("hr: employee required")
	ErrEmployeeNotFound    = errors.New("hr: employee not found")
	ErrInvalidSalary       = errors.New("hr: base salary must be a non-negative amount with at most 2 decimals")
	ErrPTKPRequired        = errors.New("hr: PTKP code required")
	ErrEffectiveFromNeeded = errors.New("hr: effective date required")
	ErrBankRequired        = errors.New("hr: bank code and account number required")
	ErrLaterChangeExists   = errors.New("hr: a later compensation change already exists; edit that one")
	ErrBeforeCurrent       = errors.New("hr: effective date is before the current compensation assignment")
	ErrNoAssignment        = errors.New("hr: no compensation assignment as of the effective date")
	ErrAssignmentExists    = errors.New("hr: compensation already exists; use a compensation change")
	errAuditNotConfigured  = errors.New("hr: compensation audit not configured")
)

// NUMERIC(18,2): up to 16 integer digits and 2 fraction digits, non-negative.
var salaryPattern = regexp.MustCompile(`^[0-9]{1,16}(\.[0-9]{1,2})?$`)

// Assignment maps a payroll_compensation_assignments row.
type Assignment struct {
	ID                int64
	EmployeeID        int64
	BaseSalary        string // NUMERIC text, e.g. "8500000.00"
	PTKPCode          string
	BPJSHealth        bool
	BPJSEmployment    bool
	BankCode          string
	BankAccountNumber string
	BankAccountName   string
	EffectiveFrom     time.Time
	EffectiveTo       *time.Time
	Reason            string
	CreatedBy         *int64
	CreatedAt         time.Time
}

// ChangeInput describes a salary/tax/BPJS change. Bank fields are only used
// when the employee has no assignment yet (initial assignment); a salary
// change on an existing assignment never writes bank fields.
type ChangeInput struct {
	EffectiveFrom     time.Time
	BaseSalary        string
	PTKPCode          string
	BPJSHealth        bool
	BPJSEmployment    bool
	BankCode          string
	BankAccountNumber string
	BankAccountName   string
	Reason            string
	ActorID           int64
}

// BankDetails holds the bank fields changed by ChangeBankTx.
type BankDetails struct {
	BankCode          string
	BankAccountNumber string
	BankAccountName   string
}

// Audit is satisfied by *shared.AuditLogger.
type Audit interface {
	RecordTx(context.Context, pgx.Tx, shared.AuditLog) error
}

// DBTX is the minimal transaction surface used by the *Tx variants. Both
// pgx.Tx and pgxmock transactions satisfy it.
type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type db interface {
	DBTX
	Begin(context.Context) (pgx.Tx, error)
}

// Service manages compensation assignments.
type Service struct {
	pool  db
	audit Audit
}

// NewService builds a Service backed by the pool.
func NewService(pool *pgxpool.Pool, audit Audit) *Service {
	return &Service{pool: pool, audit: audit}
}

const assignmentCols = `ca.id, ca.employee_id, ca.base_salary::text, ca.ptkp_code, ca.bpjs_health, ca.bpjs_employment,
	ca.bank_code, ca.bank_account_number, ca.bank_account_name, ca.effective_from, ca.effective_to,
	ca.reason, ca.created_by, ca.created_at`

func scanAssignment(row pgx.Row) (Assignment, error) {
	var a Assignment
	err := row.Scan(&a.ID, &a.EmployeeID, &a.BaseSalary, &a.PTKPCode, &a.BPJSHealth, &a.BPJSEmployment,
		&a.BankCode, &a.BankAccountNumber, &a.BankAccountName, &a.EffectiveFrom, &a.EffectiveTo,
		&a.Reason, &a.CreatedBy, &a.CreatedAt)
	return a, err
}

func collectAssignments(rows pgx.Rows) ([]Assignment, error) {
	defer rows.Close()
	var out []Assignment
	for rows.Next() {
		a, err := scanAssignment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListForEmployee returns the employee's assignments, newest first.
func (s *Service) ListForEmployee(ctx context.Context, companyID, employeeID int64) ([]Assignment, error) {
	if err := checkIDs(companyID, employeeID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+assignmentCols+`
FROM payroll_compensation_assignments ca
JOIN hr_employees e ON e.id=ca.employee_id AND e.company_id=$1
WHERE ca.employee_id=$2
ORDER BY ca.effective_from DESC`, companyID, employeeID)
	if err != nil {
		return nil, fmt.Errorf("list compensation: %w", err)
	}
	out, err := collectAssignments(rows)
	if err != nil {
		return nil, fmt.Errorf("list compensation: %w", err)
	}
	return out, nil
}

// AsOf returns the assignment effective on date, or ErrNoAssignment.
func (s *Service) AsOf(ctx context.Context, companyID, employeeID int64, date time.Time) (Assignment, error) {
	if err := checkIDs(companyID, employeeID); err != nil {
		return Assignment{}, err
	}
	a, err := scanAssignment(s.pool.QueryRow(ctx, `SELECT `+assignmentCols+`
FROM payroll_compensation_assignments ca
JOIN hr_employees e ON e.id=ca.employee_id AND e.company_id=$1
WHERE ca.employee_id=$2 AND ca.effective_from<=$3 AND (ca.effective_to IS NULL OR ca.effective_to>=$3)
ORDER BY ca.effective_from DESC
LIMIT 1`, companyID, employeeID, dateOnly(date)))
	if errors.Is(err, pgx.ErrNoRows) {
		return Assignment{}, ErrNoAssignment
	}
	if err != nil {
		return Assignment{}, fmt.Errorf("compensation as of: %w", err)
	}
	return a, nil
}

// Change applies a salary/PTKP/BPJS change in its own transaction. When the
// employee has no assignment yet it creates the initial one.
func (s *Service) Change(ctx context.Context, companyID, employeeID int64, in ChangeInput) (Assignment, error) {
	if err := validateChange(companyID, employeeID, in); err != nil {
		return Assignment{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Assignment{}, fmt.Errorf("begin compensation change: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	out, err := s.changeTx(ctx, tx, companyID, employeeID, in)
	if err != nil {
		return Assignment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Assignment{}, fmt.Errorf("commit compensation change: %w", err)
	}
	return out, nil
}

// ChangeBank applies a bank change in its own transaction.
func (s *Service) ChangeBank(ctx context.Context, companyID, employeeID int64, bank BankDetails, effectiveFrom time.Time, reason string, actorID int64) (Assignment, error) {
	if err := validateBank(companyID, employeeID, bank, effectiveFrom); err != nil {
		return Assignment{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Assignment{}, fmt.Errorf("begin bank change: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	out, err := s.ChangeBankTx(ctx, tx, companyID, employeeID, bank, effectiveFrom, reason, actorID)
	if err != nil {
		return Assignment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Assignment{}, fmt.Errorf("commit bank change: %w", err)
	}
	return out, nil
}

// CreateInitialTx inserts the employee's first assignment inside a
// caller-owned transaction (e.g. employee creation). It fails with
// ErrAssignmentExists when the employee already has an assignment.
func (s *Service) CreateInitialTx(ctx context.Context, tx DBTX, companyID, employeeID int64, in ChangeInput) (Assignment, error) {
	if err := validateChange(companyID, employeeID, in); err != nil {
		return Assignment{}, err
	}
	existing, err := lockAssignments(ctx, tx, companyID, employeeID)
	if err != nil {
		return Assignment{}, err
	}
	if len(existing) > 0 {
		return Assignment{}, ErrAssignmentExists
	}
	return s.insertInitial(ctx, tx, employeeID, in)
}

func (s *Service) insertInitial(ctx context.Context, tx DBTX, employeeID int64, in ChangeInput) (Assignment, error) {
	from := dateOnly(in.EffectiveFrom)
	a, err := insertAssignment(ctx, tx, Assignment{
		EmployeeID:        employeeID,
		BaseSalary:        strings.TrimSpace(in.BaseSalary),
		PTKPCode:          strings.TrimSpace(in.PTKPCode),
		BPJSHealth:        in.BPJSHealth,
		BPJSEmployment:    in.BPJSEmployment,
		BankCode:          strings.TrimSpace(in.BankCode),
		BankAccountNumber: strings.TrimSpace(in.BankAccountNumber),
		BankAccountName:   strings.TrimSpace(in.BankAccountName),
		EffectiveFrom:     from,
		Reason:            strings.TrimSpace(in.Reason),
	}, in.ActorID)
	if err != nil {
		return Assignment{}, err
	}
	if err := s.record(ctx, tx, in.ActorID, ActionCompensationChanged, employeeID, a.ID, from, in.Reason); err != nil {
		return Assignment{}, err
	}
	return a, nil
}

func (s *Service) changeTx(ctx context.Context, tx DBTX, companyID, employeeID int64, in ChangeInput) (Assignment, error) {
	existing, err := lockAssignments(ctx, tx, companyID, employeeID)
	if err != nil {
		return Assignment{}, err
	}
	if len(existing) == 0 {
		return s.insertInitial(ctx, tx, employeeID, in)
	}
	newFrom := dateOnly(in.EffectiveFrom)
	for _, a := range existing {
		if dateOnly(a.EffectiveFrom).After(newFrom) {
			return Assignment{}, ErrLaterChangeExists
		}
	}
	cur := latestStartingOnOrBefore(existing, newFrom)
	if cur == nil {
		return Assignment{}, ErrBeforeCurrent
	}
	reason := strings.TrimSpace(in.Reason)
	salary := strings.TrimSpace(in.BaseSalary)
	ptkp := strings.TrimSpace(in.PTKPCode)

	var out Assignment
	if dateOnly(cur.EffectiveFrom).Equal(newFrom) {
		if _, err := tx.Exec(ctx, `UPDATE payroll_compensation_assignments
SET base_salary=$2::text::numeric, ptkp_code=$3, bpjs_health=$4, bpjs_employment=$5, reason=$6
WHERE id=$1`, cur.ID, salary, ptkp, in.BPJSHealth, in.BPJSEmployment, reason); err != nil {
			return Assignment{}, fmt.Errorf("update compensation in place: %w", err)
		}
		out = *cur
		out.BaseSalary, out.PTKPCode, out.BPJSHealth, out.BPJSEmployment, out.Reason = salary, ptkp, in.BPJSHealth, in.BPJSEmployment, reason
	} else {
		next := *cur
		next.BaseSalary, next.PTKPCode, next.BPJSHealth, next.BPJSEmployment, next.Reason = salary, ptkp, in.BPJSHealth, in.BPJSEmployment, reason
		out, err = splitAssignment(ctx, tx, *cur, next, newFrom, in.ActorID)
		if err != nil {
			return Assignment{}, err
		}
	}
	if err := s.record(ctx, tx, in.ActorID, ActionCompensationChanged, employeeID, out.ID, newFrom, reason); err != nil {
		return Assignment{}, err
	}
	return out, nil
}

// ChangeBankTx changes bank details effective from effectiveFrom inside a
// caller-owned transaction. Salary, PTKP and BPJS are copied from the
// assignment current at effectiveFrom, and all later-dated assignments also
// receive the new bank details.
func (s *Service) ChangeBankTx(ctx context.Context, tx DBTX, companyID, employeeID int64, bank BankDetails, effectiveFrom time.Time, reason string, actorID int64) (Assignment, error) {
	if err := validateBank(companyID, employeeID, bank, effectiveFrom); err != nil {
		return Assignment{}, err
	}
	bank = BankDetails{
		BankCode:          strings.TrimSpace(bank.BankCode),
		BankAccountNumber: strings.TrimSpace(bank.BankAccountNumber),
		BankAccountName:   strings.TrimSpace(bank.BankAccountName),
	}
	reason = strings.TrimSpace(reason)
	existing, err := lockAssignments(ctx, tx, companyID, employeeID)
	if err != nil {
		return Assignment{}, err
	}
	newFrom := dateOnly(effectiveFrom)
	cur := activeOn(existing, newFrom)
	if cur == nil {
		return Assignment{}, ErrNoAssignment
	}

	var out Assignment
	if dateOnly(cur.EffectiveFrom).Equal(newFrom) {
		if _, err := tx.Exec(ctx, `UPDATE payroll_compensation_assignments
SET bank_code=$2, bank_account_number=$3, bank_account_name=$4, reason=$5
WHERE id=$1`, cur.ID, bank.BankCode, bank.BankAccountNumber, bank.BankAccountName, reason); err != nil {
			return Assignment{}, fmt.Errorf("update bank in place: %w", err)
		}
		out = *cur
		out.Reason = reason
	} else {
		next := *cur
		next.Reason = reason
		next.BankCode, next.BankAccountNumber, next.BankAccountName = bank.BankCode, bank.BankAccountNumber, bank.BankAccountName
		out, err = splitAssignment(ctx, tx, *cur, next, newFrom, actorID)
		if err != nil {
			return Assignment{}, err
		}
	}
	out.BankCode, out.BankAccountNumber, out.BankAccountName = bank.BankCode, bank.BankAccountNumber, bank.BankAccountName

	if _, err := tx.Exec(ctx, `UPDATE payroll_compensation_assignments
SET bank_code=$1, bank_account_number=$2, bank_account_name=$3
WHERE employee_id=$4 AND effective_from > $5`, bank.BankCode, bank.BankAccountNumber, bank.BankAccountName, employeeID, newFrom); err != nil {
		return Assignment{}, fmt.Errorf("propagate bank to future assignments: %w", err)
	}
	if err := s.record(ctx, tx, actorID, ActionBankChanged, employeeID, out.ID, newFrom, reason); err != nil {
		return Assignment{}, err
	}
	return out, nil
}

// lockAssignments verifies the employee belongs to the company, locks the
// employee row (serialising compensation changes even when no assignment
// exists yet) and locks and returns all assignments ordered by effective_from.
func lockAssignments(ctx context.Context, tx DBTX, companyID, employeeID int64) ([]Assignment, error) {
	var id int64
	err := tx.QueryRow(ctx, `SELECT e.id FROM hr_employees e WHERE e.id=$2 AND e.company_id=$1 FOR NO KEY UPDATE`, companyID, employeeID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEmployeeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock employee: %w", err)
	}
	rows, err := tx.Query(ctx, `SELECT `+assignmentCols+`
FROM payroll_compensation_assignments ca
JOIN hr_employees e ON e.id=ca.employee_id AND e.company_id=$1
WHERE ca.employee_id=$2
ORDER BY ca.effective_from
FOR UPDATE OF ca`, companyID, employeeID)
	if err != nil {
		return nil, fmt.Errorf("lock compensation assignments: %w", err)
	}
	out, err := collectAssignments(rows)
	if err != nil {
		return nil, fmt.Errorf("lock compensation assignments: %w", err)
	}
	return out, nil
}

// splitAssignment closes cur at newFrom-1, inserts next starting at newFrom
// (inheriting cur's previous effective_to so ranges never overlap) and moves
// cur's recurring components that are still effective on or after newFrom.
func splitAssignment(ctx context.Context, tx DBTX, cur, next Assignment, newFrom time.Time, actorID int64) (Assignment, error) {
	if cur.EffectiveTo != nil && dateOnly(*cur.EffectiveTo).Before(newFrom) {
		// cur already ended before newFrom: nothing to close and no
		// component still effective to move.
		next.EffectiveTo = nil
	} else {
		next.EffectiveTo = cur.EffectiveTo
		if _, err := tx.Exec(ctx, `UPDATE payroll_compensation_assignments SET effective_to=$2 WHERE id=$1`, cur.ID, newFrom.AddDate(0, 0, -1)); err != nil {
			return Assignment{}, fmt.Errorf("close compensation assignment: %w", err)
		}
	}
	next.EffectiveFrom = newFrom
	created, err := insertAssignment(ctx, tx, next, actorID)
	if err != nil {
		return Assignment{}, err
	}
	if err := moveRecurringComponents(ctx, tx, cur.ID, created.ID, newFrom); err != nil {
		return Assignment{}, err
	}
	return created, nil
}

func moveRecurringComponents(ctx context.Context, tx DBTX, oldID, newID int64, newFrom time.Time) error {
	if _, err := tx.Exec(ctx, `INSERT INTO payroll_recurring_components(assignment_id, component_id, amount, effective_from, effective_to)
SELECT $1, component_id, amount, GREATEST(effective_from, $3::date), effective_to
FROM payroll_recurring_components
WHERE assignment_id = $2 AND (effective_to IS NULL OR effective_to >= $3::date)`, newID, oldID, newFrom); err != nil {
		return fmt.Errorf("copy recurring components: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM payroll_recurring_components
WHERE assignment_id = $1 AND effective_from >= $2::date`, oldID, newFrom); err != nil {
		return fmt.Errorf("delete moved recurring components: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE payroll_recurring_components SET effective_to = $3::date
WHERE assignment_id = $1 AND effective_from < $2::date AND (effective_to IS NULL OR effective_to >= $2::date)`, oldID, newFrom, newFrom.AddDate(0, 0, -1)); err != nil {
		return fmt.Errorf("close recurring components: %w", err)
	}
	return nil
}

func insertAssignment(ctx context.Context, tx DBTX, a Assignment, actorID int64) (Assignment, error) {
	var createdBy *int64
	if actorID > 0 {
		createdBy = &actorID
	}
	out, err := scanAssignment(tx.QueryRow(ctx, `INSERT INTO payroll_compensation_assignments AS ca
(employee_id, base_salary, ptkp_code, bpjs_health, bpjs_employment, bank_code, bank_account_number, bank_account_name, effective_from, effective_to, reason, created_by)
VALUES ($1, $2::text::numeric, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING `+assignmentCols,
		a.EmployeeID, a.BaseSalary, a.PTKPCode, a.BPJSHealth, a.BPJSEmployment,
		a.BankCode, a.BankAccountNumber, a.BankAccountName, a.EffectiveFrom, a.EffectiveTo, a.Reason, createdBy))
	if err != nil {
		return Assignment{}, fmt.Errorf("insert compensation assignment: %w", err)
	}
	return out, nil
}

func (s *Service) record(ctx context.Context, tx DBTX, actorID int64, action string, employeeID, assignmentID int64, from time.Time, reason string) error {
	if s.audit == nil {
		return errAuditNotConfigured
	}
	ptx, ok := tx.(pgx.Tx)
	if !ok {
		ptx = execTx{inner: tx}
	}
	err := s.audit.RecordTx(ctx, ptx, shared.AuditLog{
		ActorID:  actorID,
		Action:   action,
		Entity:   auditEntity,
		EntityID: strconv.FormatInt(employeeID, 10),
		Meta: map[string]any{
			"assignment_id":  assignmentID,
			"effective_from": from.Format("2006-01-02"),
			"reason":         strings.TrimSpace(reason),
		},
	})
	if err != nil {
		return fmt.Errorf("audit %s: %w", action, err)
	}
	return nil
}

// execTx adapts a DBTX to pgx.Tx for audit writers that only use
// Exec/Query/QueryRow (shared.AuditLogger.RecordTx uses Exec). Other pgx.Tx
// methods are not available and panic if called.
type execTx struct {
	pgx.Tx
	inner DBTX
}

func (t execTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return t.inner.Exec(ctx, sql, args...)
}

func (t execTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return t.inner.Query(ctx, sql, args...)
}

func (t execTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return t.inner.QueryRow(ctx, sql, args...)
}

func latestStartingOnOrBefore(list []Assignment, d time.Time) *Assignment {
	var cur *Assignment
	for i := range list {
		if !dateOnly(list[i].EffectiveFrom).After(d) {
			if cur == nil || dateOnly(list[i].EffectiveFrom).After(dateOnly(cur.EffectiveFrom)) {
				cur = &list[i]
			}
		}
	}
	return cur
}

func activeOn(list []Assignment, d time.Time) *Assignment {
	cur := latestStartingOnOrBefore(list, d)
	if cur == nil || (cur.EffectiveTo != nil && dateOnly(*cur.EffectiveTo).Before(d)) {
		return nil
	}
	return cur
}

func checkIDs(companyID, employeeID int64) error {
	if companyID <= 0 {
		return ErrCompanyRequired
	}
	if employeeID <= 0 {
		return ErrEmployeeRequired
	}
	return nil
}

func validateChange(companyID, employeeID int64, in ChangeInput) error {
	if err := checkIDs(companyID, employeeID); err != nil {
		return err
	}
	if in.EffectiveFrom.IsZero() {
		return ErrEffectiveFromNeeded
	}
	if !salaryPattern.MatchString(strings.TrimSpace(in.BaseSalary)) {
		return ErrInvalidSalary
	}
	if strings.TrimSpace(in.PTKPCode) == "" {
		return ErrPTKPRequired
	}
	return nil
}

func validateBank(companyID, employeeID int64, bank BankDetails, effectiveFrom time.Time) error {
	if err := checkIDs(companyID, employeeID); err != nil {
		return err
	}
	if effectiveFrom.IsZero() {
		return ErrEffectiveFromNeeded
	}
	if strings.TrimSpace(bank.BankCode) == "" || strings.TrimSpace(bank.BankAccountNumber) == "" {
		return ErrBankRequired
	}
	return nil
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
