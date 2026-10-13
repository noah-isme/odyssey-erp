// Package overtime manages employee overtime requests routed through the
// approvals engine (module "OVERTIME").
//
// Approved requests flow into payroll by date range (the payroll draft query
// sums APPROVED minutes whose worked_on falls inside the period). A payroll
// run's lines freeze once it leaves DRAFT, so overtime on a day covered by a
// REGULAR run in APPROVAL or POSTED is refused at submission, and the
// finalizer re-checks the lock when the approval completes:
//
//   - covering POSTED run: the request becomes REJECTED with
//     apply_error 'payroll already posted' and the requester is notified
//     explicitly ("rejected: payroll locked");
//   - covering run in APPROVAL: the request stays PENDING with
//     apply_error 'payroll run awaiting approval'; an HR admin later uses
//     RetryApply, which rejects (run POSTED) or approves (run back in DRAFT).
//
// Every query is company-scoped and every state change is audited with
// RecordTx inside the same transaction.
package overtime

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/approvals"
	"github.com/odyssey-erp/odyssey-erp/internal/notifications"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// Module is the approvals-engine module name for overtime requests.
const Module = "OVERTIME"

// Request statuses (hr_overtime_requests.status).
const (
	StatusPending   = "PENDING"
	StatusApproved  = "APPROVED"
	StatusRejected  = "REJECTED"
	StatusCancelled = "CANCELLED"
)

// apply_error values written by the payroll-lock rule.
const (
	ApplyErrorPayrollPosted   = "payroll already posted"
	ApplyErrorPayrollApproval = "payroll run awaiting approval"
)

// Audit actions recorded by this package.
const (
	ActionSubmitted             = "OVERTIME_SUBMITTED"
	ActionSubmitFailed          = "OVERTIME_SUBMIT_FAILED"
	ActionCancelled             = "OVERTIME_CANCELLED"
	ActionApproved              = "OVERTIME_APPROVED"
	ActionRejected              = "OVERTIME_REJECTED"
	ActionRejectedPostedPeriod  = "OVERTIME_REJECTED_POSTED_PERIOD"
	ActionApplyDeferredApproval = "OVERTIME_APPLY_DEFERRED"

	entity = "hr_overtime_request"
)

// MaxMinutes mirrors the CHECK (minutes > 0 AND minutes <= 720) constraint.
const MaxMinutes = 720

var (
	ErrNotFound                = errors.New("hr: not found")
	ErrEmployeeNotFound        = fmt.Errorf("%w: employee", ErrNotFound)
	ErrRequestNotFound         = fmt.Errorf("%w: overtime request", ErrNotFound)
	ErrCompanyRequired         = errors.New("hr: company required")
	ErrInvalidRequest          = errors.New("hr: invalid overtime request")
	ErrInvalidMinutes          = errors.New("hr: overtime minutes must be between 1 and 720")
	ErrFutureOvertime          = errors.New("hr: overtime can only be requested for days already worked")
	ErrManagerRequired         = errors.New("hr: employee manager must have a user account")
	ErrAlreadyRequested        = errors.New("hr: overtime already requested for that day")
	ErrPayrollPosted           = errors.New("hr: payroll for that date is already posted")
	ErrPayrollAwaitingApproval = errors.New("hr: payroll run awaiting approval")
	ErrNoPolicy                = errors.New("hr: no approval policy configured for overtime")
	ErrSubmitFailed            = errors.New("hr: could not submit overtime request for approval")
	ErrNotPending              = errors.New("hr: only pending overtime requests can be cancelled")
	ErrNotRetryable            = errors.New("hr: only pending overtime requests can be retried")
	ErrNoApproval              = errors.New("hr: overtime request has no approval")
	ErrApprovalNotDecided      = errors.New("hr: approval is not decided yet")
	ErrCompanyMismatch         = errors.New("hr: approval company does not match overtime request")
	ErrInvalidDecision         = errors.New("hr: invalid approval decision")
	errAuditNotConfigured      = errors.New("hr: overtime audit not configured")
	errApprovalsNotConfigured  = errors.New("hr: overtime approvals not configured")
)

// Request maps an hr_overtime_requests row (plus employee display fields on
// list queries).
type Request struct {
	ID                int64
	CompanyID         int64
	EmployeeID        int64
	EmployeeName      string
	EmployeeNumber    string
	Department        string
	WorkedOn          time.Time
	Minutes           int
	Reason            string
	Status            string
	ApprovalRequestID *int64
	ApplyError        string
	DecidedBy         *int64
	DecidedAt         *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Hours returns the requested duration in hours.
func (r Request) Hours() float64 { return float64(r.Minutes) / 60.0 }

// SubmitInput is an employee's overtime request.
type SubmitInput struct {
	UserID    int64
	CompanyID int64
	WorkedOn  time.Time
	Minutes   int
	Reason    string
}

// Filter narrows ListAll.
type Filter struct {
	Status     string
	EmployeeID *int64
	From, To   *time.Time
	Search     string
}

// Audit is satisfied by *shared.AuditLogger.
type Audit interface {
	RecordTx(context.Context, pgx.Tx, shared.AuditLog) error
}

// Approvals is satisfied by *approvals.Service.
type Approvals interface {
	Submit(context.Context, approvals.Submission) (approvals.Request, error)
}

// Notifier is satisfied by *notifications.Dispatcher.
type Notifier interface {
	Dispatch(context.Context, notifications.Message) error
}

type db interface {
	Begin(context.Context) (pgx.Tx, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Service manages overtime requests and is the OVERTIME approvals finalizer.
type Service struct {
	pool      db
	approvals Approvals
	audit     Audit
	notifier  Notifier
}

// NewService builds a Service. notifier may be nil (no payroll-lock
// notifications are sent then).
func NewService(pool *pgxpool.Pool, a Approvals, audit Audit, notifier Notifier) *Service {
	return &Service{pool: pool, approvals: a, audit: audit, notifier: notifier}
}

// payrollLockSQL finds a REGULAR payroll run whose lines are frozen
// (APPROVAL or POSTED) and whose period covers the date; POSTED wins.
const payrollLockSQL = `SELECT r.status FROM payroll_runs r JOIN payroll_periods p ON p.id=r.period_id WHERE p.company_id=$1 AND r.status IN ('APPROVAL','POSTED') AND r.run_type='REGULAR' AND $2 BETWEEN p.starts_on AND p.ends_on ORDER BY (r.status='POSTED') DESC LIMIT 1`

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// payrollLock returns "POSTED", "APPROVAL", or "" when no frozen run covers the day.
func payrollLock(ctx context.Context, q rowQuerier, companyID int64, day time.Time) (string, error) {
	var status string
	err := q.QueryRow(ctx, payrollLockSQL, companyID, day).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("payroll lock check: %w", err)
	}
	return status, nil
}

// Submit records a PENDING overtime request and routes it to the approvals
// engine. When the engine refuses (including no matching policy) the request
// is cancelled by a compensating transaction.
func (s *Service) Submit(ctx context.Context, in SubmitInput) (Request, error) {
	if in.CompanyID <= 0 {
		return Request{}, ErrCompanyRequired
	}
	if in.UserID <= 0 || in.WorkedOn.IsZero() {
		return Request{}, ErrInvalidRequest
	}
	if in.Minutes <= 0 || in.Minutes > MaxMinutes {
		return Request{}, ErrInvalidMinutes
	}
	// Overtime is claimed for work already done; "today" is the Asia/Jakarta date.
	if dateOnly(in.WorkedOn).After(shared.BusinessToday()) {
		return Request{}, ErrFutureOvertime
	}
	if s.audit == nil {
		return Request{}, errAuditNotConfigured
	}
	if s.approvals == nil {
		return Request{}, errApprovalsNotConfigured
	}
	day := dateOnly(in.WorkedOn)
	reason := strings.TrimSpace(in.Reason)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Request{}, fmt.Errorf("submit overtime: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var employeeID int64
	var managerUserID *int64
	err = tx.QueryRow(ctx, `SELECT e.id, m.user_id FROM hr_employees e LEFT JOIN hr_employees m ON m.id=e.manager_id AND m.company_id=e.company_id WHERE e.user_id=$1 AND e.company_id=$2 AND e.status='ACTIVE'`, in.UserID, in.CompanyID).Scan(&employeeID, &managerUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, ErrEmployeeNotFound
	}
	if err != nil {
		return Request{}, fmt.Errorf("submit overtime: employee: %w", err)
	}
	if managerUserID == nil {
		return Request{}, ErrManagerRequired
	}

	switch lock, err := payrollLock(ctx, tx, in.CompanyID, day); {
	case err != nil:
		return Request{}, err
	case lock == "POSTED":
		return Request{}, ErrPayrollPosted
	case lock == "APPROVAL":
		return Request{}, ErrPayrollAwaitingApproval
	}

	req, err := scanRequest(tx.QueryRow(ctx, `INSERT INTO hr_overtime_requests(company_id, employee_id, worked_on, minutes, reason, status) VALUES($1,$2,$3,$4,$5,'PENDING') RETURNING `+requestCols, in.CompanyID, employeeID, day, in.Minutes, reason))
	if isUniqueViolation(err) {
		return Request{}, ErrAlreadyRequested
	}
	if err != nil {
		return Request{}, fmt.Errorf("submit overtime: insert: %w", err)
	}
	if err := s.record(ctx, tx, in.UserID, ActionSubmitted, req.ID, map[string]any{"worked_on": day.Format("2006-01-02"), "minutes": in.Minutes}); err != nil {
		return Request{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Request{}, fmt.Errorf("submit overtime: commit: %w", err)
	}

	companyID := in.CompanyID
	approvalReq, err := s.approvals.Submit(ctx, approvals.Submission{
		Module:      Module,
		DocumentID:  req.ID,
		RequesterID: in.UserID,
		CompanyID:   &companyID,
		Amount:      float64(in.Minutes) / 60.0,
		ManagerID:   *managerUserID,
	})
	if err != nil {
		class, userErr := "approval_error", ErrSubmitFailed
		if errors.Is(err, approvals.ErrNoPolicy) {
			class, userErr = "no_policy", ErrNoPolicy
		}
		if cerr := s.compensate(ctx, in.UserID, req.ID, class); cerr != nil {
			return Request{}, fmt.Errorf("%w (cleanup failed: %v)", userErr, cerr)
		}
		return Request{}, userErr
	}
	if _, err := s.pool.Exec(ctx, `UPDATE hr_overtime_requests SET approval_request_id=$2, updated_at=NOW() WHERE id=$1 AND company_id=$3`, req.ID, approvalReq.ID, in.CompanyID); err != nil {
		return Request{}, fmt.Errorf("hr: linking approval request: %w", err)
	}
	req.ApprovalRequestID = &approvalReq.ID
	return req, nil
}

// compensate cancels a committed submission whose approval could not be created.
func (s *Service) compensate(ctx context.Context, userID, requestID int64, class string) error {
	ctx = context.WithoutCancel(ctx)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `UPDATE hr_overtime_requests SET status='CANCELLED', updated_at=NOW() WHERE id=$1 AND status='PENDING'`, requestID); err != nil {
		return err
	}
	if err := s.record(ctx, tx, userID, ActionSubmitFailed, requestID, map[string]any{"error_class": class}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Cancel withdraws the caller's own PENDING request together with its
// approval request and pending approval assignments.
func (s *Service) Cancel(ctx context.Context, companyID, requestID, userID int64) error {
	if companyID <= 0 {
		return ErrCompanyRequired
	}
	if requestID <= 0 || userID <= 0 {
		return ErrInvalidRequest
	}
	if s.audit == nil {
		return errAuditNotConfigured
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cancel overtime: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status string
	var approvalReqID *int64
	err = tx.QueryRow(ctx, `SELECT o.status, o.approval_request_id FROM hr_overtime_requests o JOIN hr_employees e ON e.id=o.employee_id WHERE o.id=$1 AND e.user_id=$2 AND o.company_id=$3 AND e.company_id=$3 FOR UPDATE OF o`, requestID, userID, companyID).Scan(&status, &approvalReqID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRequestNotFound
	}
	if err != nil {
		return fmt.Errorf("cancel overtime: %w", err)
	}
	if status != StatusPending {
		return ErrNotPending
	}
	if _, err := tx.Exec(ctx, `UPDATE hr_overtime_requests SET status='CANCELLED', updated_at=NOW() WHERE id=$1`, requestID); err != nil {
		return fmt.Errorf("cancel overtime: %w", err)
	}
	if approvalReqID != nil {
		if _, err := tx.Exec(ctx, `UPDATE approval_requests SET status='CANCELLED', updated_at=NOW() WHERE id=$1 AND status='PENDING'`, *approvalReqID); err != nil {
			return fmt.Errorf("cancel overtime: approval request: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE approval_assignments SET status='CANCELLED', updated_at=NOW() WHERE request_id=$1 AND status='PENDING'`, *approvalReqID); err != nil {
			return fmt.Errorf("cancel overtime: approval assignments: %w", err)
		}
	}
	if err := s.record(ctx, tx, userID, ActionCancelled, requestID, nil); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("cancel overtime: commit: %w", err)
	}
	return nil
}

// FinalizeApproval implements approvals.Finalizer. It is idempotent (a
// non-PENDING row is left untouched) and verifies the approval's company.
func (s *Service) FinalizeApproval(ctx context.Context, a approvals.Request, status string, actorID int64, note string) error {
	return s.finalize(ctx, a, status, actorID, note)
}

type lockedRow struct {
	companyID     int64
	workedOn      time.Time
	minutes       int
	status        string
	employeeUser  *int64
	approvalReqID *int64
}

func (s *Service) finalize(ctx context.Context, a approvals.Request, status string, actorID int64, note string) error {
	if status != approvals.StatusApproved && status != approvals.StatusRejected {
		return ErrInvalidDecision
	}
	if s.audit == nil {
		return errAuditNotConfigured
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("finalize overtime: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var row lockedRow
	err = tx.QueryRow(ctx, `SELECT o.company_id, o.worked_on, o.minutes, o.status, e.user_id, o.approval_request_id FROM hr_overtime_requests o JOIN hr_employees e ON e.id=o.employee_id WHERE o.id=$1 FOR UPDATE OF o`, a.DocumentID).
		Scan(&row.companyID, &row.workedOn, &row.minutes, &row.status, &row.employeeUser, &row.approvalReqID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRequestNotFound
	}
	if err != nil {
		return fmt.Errorf("finalize overtime: %w", err)
	}
	if a.CompanyID == nil || *a.CompanyID != row.companyID {
		return ErrCompanyMismatch
	}
	if row.status != StatusPending {
		return nil // idempotent no-op
	}

	outcome, err := s.apply(ctx, tx, a, row, status, actorID, note)
	if err == nil {
		if cerr := tx.Commit(ctx); cerr != nil {
			err = fmt.Errorf("finalize overtime: commit: %w", cerr)
		}
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		s.markApplyError(ctx, a.DocumentID, err)
		return err
	}
	switch outcome {
	case outcomeRejectedPosted:
		s.notifyPayrollLocked(ctx, a, row)
	case outcomeDeferred:
		return fmt.Errorf("%w: overtime request #%d stays pending; retry apply once the payroll run is posted or returned to draft", ErrPayrollAwaitingApproval, a.DocumentID)
	}
	return nil
}

type outcome int

const (
	outcomeApplied outcome = iota
	outcomeRejectedPosted
	outcomeDeferred
)

// apply performs the state change inside tx; the caller commits.
func (s *Service) apply(ctx context.Context, tx pgx.Tx, a approvals.Request, row lockedRow, status string, actorID int64, note string) (outcome, error) {
	id := a.DocumentID
	meta := map[string]any{"worked_on": dateOnly(row.workedOn).Format("2006-01-02"), "minutes": row.minutes, "approval_request_id": a.ID}
	if note = strings.TrimSpace(note); note != "" {
		meta["note"] = note
	}
	if status == approvals.StatusRejected {
		if _, err := tx.Exec(ctx, `UPDATE hr_overtime_requests SET status='REJECTED', apply_error='', decided_by=$2, decided_at=NOW(), updated_at=NOW() WHERE id=$1`, id, actorID); err != nil {
			return 0, fmt.Errorf("finalize overtime: reject: %w", err)
		}
		return outcomeApplied, s.record(ctx, tx, actorID, ActionRejected, id, meta)
	}

	lock, err := payrollLock(ctx, tx, row.companyID, dateOnly(row.workedOn))
	if err != nil {
		return 0, err
	}
	switch lock {
	case "POSTED":
		if _, err := tx.Exec(ctx, `UPDATE hr_overtime_requests SET status='REJECTED', apply_error=$3, decided_by=$2, decided_at=NOW(), updated_at=NOW() WHERE id=$1`, id, actorID, ApplyErrorPayrollPosted); err != nil {
			return 0, fmt.Errorf("finalize overtime: reject posted period: %w", err)
		}
		return outcomeRejectedPosted, s.record(ctx, tx, actorID, ActionRejectedPostedPeriod, id, meta)
	case "APPROVAL":
		if _, err := tx.Exec(ctx, `UPDATE hr_overtime_requests SET apply_error=$2, updated_at=NOW() WHERE id=$1`, id, ApplyErrorPayrollApproval); err != nil {
			return 0, fmt.Errorf("finalize overtime: defer: %w", err)
		}
		return outcomeDeferred, s.record(ctx, tx, actorID, ActionApplyDeferredApproval, id, meta)
	}
	if _, err := tx.Exec(ctx, `UPDATE hr_overtime_requests SET status='APPROVED', apply_error='', decided_by=$2, decided_at=NOW(), updated_at=NOW() WHERE id=$1`, id, actorID); err != nil {
		return 0, fmt.Errorf("finalize overtime: approve: %w", err)
	}
	return outcomeApplied, s.record(ctx, tx, actorID, ActionApproved, id, meta)
}

// markApplyError records a failed apply on the still-PENDING row (best effort).
func (s *Service) markApplyError(ctx context.Context, requestID int64, cause error) {
	msg := cause.Error()
	if len(msg) > 500 {
		msg = msg[:500]
	}
	_, _ = s.pool.Exec(context.WithoutCancel(ctx), `UPDATE hr_overtime_requests SET apply_error=$2, updated_at=NOW() WHERE id=$1 AND status='PENDING'`, requestID, msg)
}

// notifyPayrollLocked tells the requester that an approved request was
// rejected because payroll is locked; approvals.Service.Decide has already
// sent the generic "approved" notification.
func (s *Service) notifyPayrollLocked(ctx context.Context, a approvals.Request, row lockedRow) {
	if s.notifier == nil {
		return
	}
	recipient := a.RequesterID
	if recipient <= 0 && row.employeeUser != nil {
		recipient = *row.employeeUser
	}
	if recipient <= 0 {
		return
	}
	day := dateOnly(row.workedOn).Format("2006-01-02")
	_ = s.notifier.Dispatch(ctx, notifications.Message{
		RecipientID: recipient,
		DedupeKey:   fmt.Sprintf("overtime:%d:rejected:payroll_locked", a.DocumentID),
		Type:        notifications.TypeApprovalRejected,
		Title:       "Overtime rejected: payroll locked",
		Body:        fmt.Sprintf("Overtime #%d for %s was approved but rejected: payroll locked (payroll for that date is already posted).", a.DocumentID, day),
		URL:         "/hr/overtime",
	})
}

// RetryApply re-runs the finalizer for a PENDING request whose approval is
// already decided (for example after an apply error or while a payroll run
// was awaiting approval).
func (s *Service) RetryApply(ctx context.Context, companyID, requestID, actorID int64) error {
	if companyID <= 0 {
		return ErrCompanyRequired
	}
	if requestID <= 0 || actorID <= 0 {
		return ErrInvalidRequest
	}
	var status string
	var approvalReqID *int64
	err := s.pool.QueryRow(ctx, `SELECT o.status, o.approval_request_id FROM hr_overtime_requests o WHERE o.id=$1 AND o.company_id=$2`, requestID, companyID).Scan(&status, &approvalReqID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRequestNotFound
	}
	if err != nil {
		return fmt.Errorf("retry overtime: %w", err)
	}
	if status != StatusPending {
		return ErrNotRetryable
	}
	if approvalReqID == nil {
		return ErrNoApproval
	}
	var a approvals.Request
	err = s.pool.QueryRow(ctx, `SELECT id, policy_id, module, document_id, company_id, amount, requester_id, current_step, status, submitted_at FROM approval_requests WHERE id=$1 AND module=$2 AND document_id=$3`, *approvalReqID, Module, requestID).
		Scan(&a.ID, &a.PolicyID, &a.Module, &a.DocumentID, &a.CompanyID, &a.Amount, &a.RequesterID, &a.CurrentStep, &a.Status, &a.SubmittedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoApproval
	}
	if err != nil {
		return fmt.Errorf("retry overtime: approval: %w", err)
	}
	if a.Status != approvals.StatusApproved && a.Status != approvals.StatusRejected {
		return ErrApprovalNotDecided
	}
	return s.finalize(ctx, a, a.Status, actorID, "retry apply")
}

const requestCols = `id, company_id, employee_id, worked_on, minutes, reason, status, approval_request_id, apply_error, decided_by, decided_at, created_at, updated_at`

func scanRequest(row pgx.Row) (Request, error) {
	var r Request
	err := row.Scan(&r.ID, &r.CompanyID, &r.EmployeeID, &r.WorkedOn, &r.Minutes, &r.Reason, &r.Status, &r.ApprovalRequestID, &r.ApplyError, &r.DecidedBy, &r.DecidedAt, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

const listCols = `o.id, o.company_id, o.employee_id, o.worked_on, o.minutes, o.reason, o.status, o.approval_request_id, o.apply_error, o.decided_by, o.decided_at, o.created_at, o.updated_at, e.name, e.employee_number, COALESCE(d.name,'')`

const listFrom = ` FROM hr_overtime_requests o JOIN hr_employees e ON e.id=o.employee_id AND e.company_id=o.company_id LEFT JOIN hr_departments d ON d.id=e.department_id`

func (s *Service) list(ctx context.Context, query string, args ...any) ([]Request, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list overtime: %w", err)
	}
	defer rows.Close()
	var out []Request
	for rows.Next() {
		var r Request
		if err := rows.Scan(&r.ID, &r.CompanyID, &r.EmployeeID, &r.WorkedOn, &r.Minutes, &r.Reason, &r.Status, &r.ApprovalRequestID, &r.ApplyError, &r.DecidedBy, &r.DecidedAt, &r.CreatedAt, &r.UpdatedAt, &r.EmployeeName, &r.EmployeeNumber, &r.Department); err != nil {
			return nil, fmt.Errorf("list overtime: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list overtime: %w", err)
	}
	return out, nil
}

// ListOwn returns the session user's requests in the company, newest first.
func (s *Service) ListOwn(ctx context.Context, companyID, userID int64) ([]Request, error) {
	if companyID <= 0 {
		return nil, ErrCompanyRequired
	}
	if userID <= 0 {
		return nil, ErrInvalidRequest
	}
	return s.list(ctx, `SELECT `+listCols+listFrom+` WHERE o.company_id=$1 AND e.user_id=$2 ORDER BY o.worked_on DESC, o.id DESC`, companyID, userID)
}

// ListAll returns the company's requests matching the filter, newest first.
func (s *Service) ListAll(ctx context.Context, companyID int64, f Filter) ([]Request, error) {
	if companyID <= 0 {
		return nil, ErrCompanyRequired
	}
	query := `SELECT ` + listCols + listFrom + ` WHERE o.company_id=$1`
	args := []any{companyID}
	if st := strings.ToUpper(strings.TrimSpace(f.Status)); st != "" && st != "ALL" {
		args = append(args, st)
		query += fmt.Sprintf(" AND o.status=$%d", len(args))
	}
	if f.EmployeeID != nil && *f.EmployeeID > 0 {
		args = append(args, *f.EmployeeID)
		query += fmt.Sprintf(" AND o.employee_id=$%d", len(args))
	}
	if f.From != nil && !f.From.IsZero() {
		args = append(args, dateOnly(*f.From))
		query += fmt.Sprintf(" AND o.worked_on>=$%d", len(args))
	}
	if f.To != nil && !f.To.IsZero() {
		args = append(args, dateOnly(*f.To))
		query += fmt.Sprintf(" AND o.worked_on<=$%d", len(args))
	}
	if search := strings.TrimSpace(f.Search); search != "" {
		args = append(args, "%"+strings.ToLower(search)+"%")
		query += fmt.Sprintf(" AND (LOWER(e.name) LIKE $%d OR LOWER(e.employee_number) LIKE $%d)", len(args), len(args))
	}
	query += ` ORDER BY o.worked_on DESC, o.id DESC`
	return s.list(ctx, query, args...)
}

// ApprovedMinutesBetween sums an employee's APPROVED overtime minutes with
// worked_on in [from, to].
func (s *Service) ApprovedMinutesBetween(ctx context.Context, companyID, employeeID int64, from, to time.Time) (int64, error) {
	if companyID <= 0 {
		return 0, ErrCompanyRequired
	}
	if employeeID <= 0 || from.IsZero() || to.IsZero() || to.Before(from) {
		return 0, ErrInvalidRequest
	}
	var total int64
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(SUM(o.minutes),0) FROM hr_overtime_requests o WHERE o.company_id=$1 AND o.employee_id=$2 AND o.status='APPROVED' AND o.worked_on BETWEEN $3 AND $4`, companyID, employeeID, dateOnly(from), dateOnly(to)).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("approved overtime minutes: %w", err)
	}
	return total, nil
}

func (s *Service) record(ctx context.Context, tx pgx.Tx, actorID int64, action string, id int64, meta map[string]any) error {
	if s.audit == nil {
		return errAuditNotConfigured
	}
	if err := s.audit.RecordTx(ctx, tx, shared.AuditLog{ActorID: actorID, Action: action, Entity: entity, EntityID: strconv.FormatInt(id, 10), Meta: meta}); err != nil {
		return fmt.Errorf("audit %s: %w", action, err)
	}
	return nil
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
