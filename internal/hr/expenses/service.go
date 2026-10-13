// Package expenses manages employee expense claims routed through the
// approvals engine (module "EXPENSE").
//
// Amounts are exact decimals: they are validated as strings, stored as
// NUMERIC(18,2) and returned as NUMERIC text. A float64 is derived only for
// the approvals engine's policy thresholds. Approved claims stop at APPROVED;
// nothing is posted to AP or payroll.
//
// Every query is company-scoped and every state change is audited with
// RecordTx inside the same transaction.
package expenses

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/approvals"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// Module is the approvals-engine module name for expense claims.
const Module = "EXPENSE"

// Claim statuses (hr_expense_claims.status).
const (
	StatusPending   = "PENDING"
	StatusApproved  = "APPROVED"
	StatusRejected  = "REJECTED"
	StatusCancelled = "CANCELLED"
)

// Audit actions recorded by this package.
const (
	ActionSubmitted    = "EXPENSE_SUBMITTED"
	ActionSubmitFailed = "EXPENSE_SUBMIT_FAILED"
	ActionCancelled    = "EXPENSE_CANCELLED"
	ActionApproved     = "EXPENSE_APPROVED"
	ActionRejected     = "EXPENSE_REJECTED"

	entity = "hr_expense_claim"
)

// Field limits enforced before touching the database.
const (
	MaxCategoryLen    = 100
	MaxDescriptionLen = 2000
)

// amountPattern accepts a positive decimal that fits NUMERIC(18,2): up to 16
// integer digits and at most two fraction digits, no sign, exponent, or
// thousands separators.
var amountPattern = regexp.MustCompile(`^[0-9]{1,16}(\.[0-9]{1,2})?$`)

var (
	ErrNotFound               = errors.New("hr: not found")
	ErrEmployeeNotFound       = fmt.Errorf("%w: employee", ErrNotFound)
	ErrClaimNotFound          = fmt.Errorf("%w: expense claim", ErrNotFound)
	ErrDocumentNotFound       = fmt.Errorf("%w: document", ErrNotFound)
	ErrCompanyRequired        = errors.New("hr: company required")
	ErrInvalidRequest         = errors.New("hr: invalid expense claim")
	ErrInvalidAmount          = errors.New("hr: expense amount must be a positive decimal with at most 2 decimal places")
	ErrCategoryRequired       = errors.New("hr: expense category required")
	ErrManagerRequired        = errors.New("hr: employee manager must have a user account")
	ErrNoPolicy               = errors.New("hr: no approval policy configured for expense claims")
	ErrSubmitFailed           = errors.New("hr: could not submit expense claim for approval")
	ErrNotPending             = errors.New("hr: only pending expense claims can be cancelled")
	ErrNotRetryable           = errors.New("hr: only pending expense claims can be retried")
	ErrNoApproval             = errors.New("hr: expense claim has no approval")
	ErrApprovalNotDecided     = errors.New("hr: approval is not decided yet")
	ErrCompanyMismatch        = errors.New("hr: approval company does not match expense claim")
	ErrInvalidDecision        = errors.New("hr: invalid approval decision")
	errAuditNotConfigured     = errors.New("hr: expense audit not configured")
	errApprovalsNotConfigured = errors.New("hr: expense approvals not configured")
)

// Claim maps an hr_expense_claims row (plus employee display fields on list
// queries). Amount is the NUMERIC value as text (for example "125000.50").
type Claim struct {
	ID                int64
	CompanyID         int64
	EmployeeID        int64
	EmployeeName      string
	EmployeeNumber    string
	Department        string
	ClaimDate         time.Time
	Category          string
	Description       string
	Amount            string
	Currency          string
	DocumentID        *int64
	Status            string
	ApprovalRequestID *int64
	ApplyError        string
	DecidedBy         *int64
	DecidedAt         *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// SubmitInput is an employee's expense claim. Amount is a decimal string.
type SubmitInput struct {
	UserID      int64
	CompanyID   int64
	ClaimDate   time.Time
	Category    string
	Description string
	Amount      string
	DocumentID  *int64
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

type db interface {
	Begin(context.Context) (pgx.Tx, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Service manages expense claims and is the EXPENSE approvals finalizer.
type Service struct {
	pool      db
	approvals Approvals
	audit     Audit
}

// NewService builds a Service.
func NewService(pool *pgxpool.Pool, a Approvals, audit Audit) *Service {
	return &Service{pool: pool, approvals: a, audit: audit}
}

// NormalizeAmount validates a decimal amount string and returns it trimmed.
// It never parses through float, so the stored value is exact.
func NormalizeAmount(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if !amountPattern.MatchString(s) {
		return "", ErrInvalidAmount
	}
	if strings.Trim(strings.ReplaceAll(s, ".", ""), "0") == "" {
		return "", ErrInvalidAmount // zero
	}
	return s, nil
}

// thresholdAmount converts a validated amount to float64 for approval
// policy thresholds only; it is never stored.
func thresholdAmount(amount string) float64 {
	f, _ := strconv.ParseFloat(amount, 64)
	return f
}

// Submit records a PENDING expense claim and routes it to the approvals
// engine. When the engine refuses (including no matching policy) the claim
// is cancelled by a compensating transaction.
func (s *Service) Submit(ctx context.Context, in SubmitInput) (Claim, error) {
	if in.CompanyID <= 0 {
		return Claim{}, ErrCompanyRequired
	}
	if in.UserID <= 0 || in.ClaimDate.IsZero() {
		return Claim{}, ErrInvalidRequest
	}
	if in.DocumentID != nil && *in.DocumentID <= 0 {
		return Claim{}, ErrInvalidRequest
	}
	amount, err := NormalizeAmount(in.Amount)
	if err != nil {
		return Claim{}, err
	}
	category := strings.TrimSpace(in.Category)
	if category == "" {
		return Claim{}, ErrCategoryRequired
	}
	description := strings.TrimSpace(in.Description)
	if utf8.RuneCountInString(category) > MaxCategoryLen || utf8.RuneCountInString(description) > MaxDescriptionLen {
		return Claim{}, ErrInvalidRequest
	}
	if s.audit == nil {
		return Claim{}, errAuditNotConfigured
	}
	if s.approvals == nil {
		return Claim{}, errApprovalsNotConfigured
	}
	day := dateOnly(in.ClaimDate)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Claim{}, fmt.Errorf("submit expense: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var employeeID int64
	var managerUserID *int64
	err = tx.QueryRow(ctx, `SELECT e.id, m.user_id FROM hr_employees e LEFT JOIN hr_employees m ON m.id=e.manager_id AND m.company_id=e.company_id WHERE e.user_id=$1 AND e.company_id=$2 AND e.status='ACTIVE'`, in.UserID, in.CompanyID).Scan(&employeeID, &managerUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Claim{}, ErrEmployeeNotFound
	}
	if err != nil {
		return Claim{}, fmt.Errorf("submit expense: employee: %w", err)
	}
	if managerUserID == nil {
		return Claim{}, ErrManagerRequired
	}

	if in.DocumentID != nil {
		var docID int64
		err = tx.QueryRow(ctx, `SELECT d.id FROM documents d WHERE d.id=$1 AND d.company_id=$2`, *in.DocumentID, in.CompanyID).Scan(&docID)
		if errors.Is(err, pgx.ErrNoRows) {
			return Claim{}, ErrDocumentNotFound
		}
		if err != nil {
			return Claim{}, fmt.Errorf("submit expense: document: %w", err)
		}
	}

	claim, err := scanClaim(tx.QueryRow(ctx, `INSERT INTO hr_expense_claims(company_id, employee_id, claim_date, category, description, amount, document_id, status) VALUES($1,$2,$3,$4,$5,$6::numeric,$7,'PENDING') RETURNING `+claimCols, in.CompanyID, employeeID, day, category, description, amount, in.DocumentID))
	if err != nil {
		return Claim{}, fmt.Errorf("submit expense: insert: %w", err)
	}
	meta := map[string]any{"claim_date": day.Format("2006-01-02"), "category": category, "amount": claim.Amount, "currency": claim.Currency}
	if in.DocumentID != nil {
		meta["document_id"] = *in.DocumentID
	}
	if err := s.record(ctx, tx, in.UserID, ActionSubmitted, claim.ID, meta); err != nil {
		return Claim{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Claim{}, fmt.Errorf("submit expense: commit: %w", err)
	}

	companyID := in.CompanyID
	approvalReq, err := s.approvals.Submit(ctx, approvals.Submission{
		Module:      Module,
		DocumentID:  claim.ID,
		RequesterID: in.UserID,
		CompanyID:   &companyID,
		Amount:      thresholdAmount(amount),
		ManagerID:   *managerUserID,
	})
	if err != nil {
		class, userErr := "approval_error", ErrSubmitFailed
		if errors.Is(err, approvals.ErrNoPolicy) {
			class, userErr = "no_policy", ErrNoPolicy
		}
		if cerr := s.compensate(ctx, in.UserID, claim.ID, class); cerr != nil {
			return Claim{}, fmt.Errorf("%w (cleanup failed: %v)", userErr, cerr)
		}
		return Claim{}, userErr
	}
	if _, err := s.pool.Exec(ctx, `UPDATE hr_expense_claims SET approval_request_id=$2, updated_at=NOW() WHERE id=$1 AND company_id=$3`, claim.ID, approvalReq.ID, in.CompanyID); err != nil {
		return Claim{}, fmt.Errorf("hr: linking approval request: %w", err)
	}
	claim.ApprovalRequestID = &approvalReq.ID
	return claim, nil
}

// compensate cancels a committed submission whose approval could not be created.
func (s *Service) compensate(ctx context.Context, userID, claimID int64, class string) error {
	ctx = context.WithoutCancel(ctx)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `UPDATE hr_expense_claims SET status='CANCELLED', updated_at=NOW() WHERE id=$1 AND status='PENDING'`, claimID); err != nil {
		return err
	}
	if err := s.record(ctx, tx, userID, ActionSubmitFailed, claimID, map[string]any{"error_class": class}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Cancel withdraws the caller's own PENDING claim together with its approval
// request and pending approval assignments.
func (s *Service) Cancel(ctx context.Context, companyID, claimID, userID int64) error {
	if companyID <= 0 {
		return ErrCompanyRequired
	}
	if claimID <= 0 || userID <= 0 {
		return ErrInvalidRequest
	}
	if s.audit == nil {
		return errAuditNotConfigured
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cancel expense: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status string
	var approvalReqID *int64
	err = tx.QueryRow(ctx, `SELECT c.status, c.approval_request_id FROM hr_expense_claims c JOIN hr_employees e ON e.id=c.employee_id WHERE c.id=$1 AND e.user_id=$2 AND c.company_id=$3 AND e.company_id=$3 FOR UPDATE OF c`, claimID, userID, companyID).Scan(&status, &approvalReqID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrClaimNotFound
	}
	if err != nil {
		return fmt.Errorf("cancel expense: %w", err)
	}
	if status != StatusPending {
		return ErrNotPending
	}
	if _, err := tx.Exec(ctx, `UPDATE hr_expense_claims SET status='CANCELLED', updated_at=NOW() WHERE id=$1`, claimID); err != nil {
		return fmt.Errorf("cancel expense: %w", err)
	}
	if approvalReqID != nil {
		if _, err := tx.Exec(ctx, `UPDATE approval_requests SET status='CANCELLED', updated_at=NOW() WHERE id=$1 AND status='PENDING'`, *approvalReqID); err != nil {
			return fmt.Errorf("cancel expense: approval request: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE approval_assignments SET status='CANCELLED', updated_at=NOW() WHERE request_id=$1 AND status='PENDING'`, *approvalReqID); err != nil {
			return fmt.Errorf("cancel expense: approval assignments: %w", err)
		}
	}
	if err := s.record(ctx, tx, userID, ActionCancelled, claimID, nil); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("cancel expense: commit: %w", err)
	}
	return nil
}

// FinalizeApproval implements approvals.Finalizer. It is idempotent (a
// non-PENDING row is left untouched) and verifies the approval's company.
// An apply failure leaves the claim PENDING with apply_error recorded.
func (s *Service) FinalizeApproval(ctx context.Context, a approvals.Request, status string, actorID int64, note string) error {
	if status != approvals.StatusApproved && status != approvals.StatusRejected {
		return ErrInvalidDecision
	}
	if s.audit == nil {
		return errAuditNotConfigured
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("finalize expense: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var companyID int64
	var rowStatus, amount, currency string
	var claimDate time.Time
	err = tx.QueryRow(ctx, `SELECT c.company_id, c.status, c.claim_date, c.amount::text, c.currency FROM hr_expense_claims c WHERE c.id=$1 FOR UPDATE OF c`, a.DocumentID).
		Scan(&companyID, &rowStatus, &claimDate, &amount, &currency)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrClaimNotFound
	}
	if err != nil {
		return fmt.Errorf("finalize expense: %w", err)
	}
	if a.CompanyID == nil || *a.CompanyID != companyID {
		return ErrCompanyMismatch
	}
	if rowStatus != StatusPending {
		return nil // idempotent no-op
	}

	err = s.apply(ctx, tx, a, status, actorID, note, map[string]any{"claim_date": dateOnly(claimDate).Format("2006-01-02"), "amount": amount, "currency": currency, "approval_request_id": a.ID})
	if err == nil {
		if cerr := tx.Commit(ctx); cerr != nil {
			err = fmt.Errorf("finalize expense: commit: %w", cerr)
		}
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		s.markApplyError(ctx, a.DocumentID, err)
		return err
	}
	return nil
}

// apply performs the state change inside tx; the caller commits.
func (s *Service) apply(ctx context.Context, tx pgx.Tx, a approvals.Request, status string, actorID int64, note string, meta map[string]any) error {
	if note = strings.TrimSpace(note); note != "" {
		meta["note"] = note
	}
	newStatus, action := StatusApproved, ActionApproved
	if status == approvals.StatusRejected {
		newStatus, action = StatusRejected, ActionRejected
	}
	if _, err := tx.Exec(ctx, `UPDATE hr_expense_claims SET status=$3, apply_error='', decided_by=$2, decided_at=NOW(), updated_at=NOW() WHERE id=$1`, a.DocumentID, actorID, newStatus); err != nil {
		return fmt.Errorf("finalize expense: %s: %w", strings.ToLower(newStatus), err)
	}
	return s.record(ctx, tx, actorID, action, a.DocumentID, meta)
}

// markApplyError records a failed apply on the still-PENDING row (best effort).
func (s *Service) markApplyError(ctx context.Context, claimID int64, cause error) {
	msg := cause.Error()
	if len(msg) > 500 {
		msg = msg[:500]
	}
	_, _ = s.pool.Exec(context.WithoutCancel(ctx), `UPDATE hr_expense_claims SET apply_error=$2, updated_at=NOW() WHERE id=$1 AND status='PENDING'`, claimID, msg)
}

// RetryApply re-runs the finalizer for a PENDING claim whose approval is
// already decided (for example after an apply error).
func (s *Service) RetryApply(ctx context.Context, companyID, claimID, actorID int64) error {
	if companyID <= 0 {
		return ErrCompanyRequired
	}
	if claimID <= 0 || actorID <= 0 {
		return ErrInvalidRequest
	}
	var status string
	var approvalReqID *int64
	err := s.pool.QueryRow(ctx, `SELECT c.status, c.approval_request_id FROM hr_expense_claims c WHERE c.id=$1 AND c.company_id=$2`, claimID, companyID).Scan(&status, &approvalReqID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrClaimNotFound
	}
	if err != nil {
		return fmt.Errorf("retry expense: %w", err)
	}
	if status != StatusPending {
		return ErrNotRetryable
	}
	if approvalReqID == nil {
		return ErrNoApproval
	}
	var a approvals.Request
	err = s.pool.QueryRow(ctx, `SELECT id, policy_id, module, document_id, company_id, amount, requester_id, current_step, status, submitted_at FROM approval_requests WHERE id=$1 AND module=$2 AND document_id=$3`, *approvalReqID, Module, claimID).
		Scan(&a.ID, &a.PolicyID, &a.Module, &a.DocumentID, &a.CompanyID, &a.Amount, &a.RequesterID, &a.CurrentStep, &a.Status, &a.SubmittedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoApproval
	}
	if err != nil {
		return fmt.Errorf("retry expense: approval: %w", err)
	}
	if a.Status != approvals.StatusApproved && a.Status != approvals.StatusRejected {
		return ErrApprovalNotDecided
	}
	return s.FinalizeApproval(ctx, a, a.Status, actorID, "retry apply")
}

const claimCols = `id, company_id, employee_id, claim_date, category, description, amount::text, currency, document_id, status, approval_request_id, apply_error, decided_by, decided_at, created_at, updated_at`

func scanClaim(row pgx.Row) (Claim, error) {
	var c Claim
	err := row.Scan(&c.ID, &c.CompanyID, &c.EmployeeID, &c.ClaimDate, &c.Category, &c.Description, &c.Amount, &c.Currency, &c.DocumentID, &c.Status, &c.ApprovalRequestID, &c.ApplyError, &c.DecidedBy, &c.DecidedAt, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

const listCols = `c.id, c.company_id, c.employee_id, c.claim_date, c.category, c.description, c.amount::text, c.currency, c.document_id, c.status, c.approval_request_id, c.apply_error, c.decided_by, c.decided_at, c.created_at, c.updated_at, e.name, e.employee_number, COALESCE(d.name,'')`

const listFrom = ` FROM hr_expense_claims c JOIN hr_employees e ON e.id=c.employee_id AND e.company_id=c.company_id LEFT JOIN hr_departments d ON d.id=e.department_id`

func (s *Service) list(ctx context.Context, query string, args ...any) ([]Claim, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list expenses: %w", err)
	}
	defer rows.Close()
	var out []Claim
	for rows.Next() {
		var c Claim
		if err := rows.Scan(&c.ID, &c.CompanyID, &c.EmployeeID, &c.ClaimDate, &c.Category, &c.Description, &c.Amount, &c.Currency, &c.DocumentID, &c.Status, &c.ApprovalRequestID, &c.ApplyError, &c.DecidedBy, &c.DecidedAt, &c.CreatedAt, &c.UpdatedAt, &c.EmployeeName, &c.EmployeeNumber, &c.Department); err != nil {
			return nil, fmt.Errorf("list expenses: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list expenses: %w", err)
	}
	return out, nil
}

// ListOwn returns the session user's claims in the company, newest first.
func (s *Service) ListOwn(ctx context.Context, companyID, userID int64) ([]Claim, error) {
	if companyID <= 0 {
		return nil, ErrCompanyRequired
	}
	if userID <= 0 {
		return nil, ErrInvalidRequest
	}
	return s.list(ctx, `SELECT `+listCols+listFrom+` WHERE c.company_id=$1 AND e.user_id=$2 ORDER BY c.claim_date DESC, c.id DESC`, companyID, userID)
}

// ListAll returns the company's claims matching the filter, newest first.
func (s *Service) ListAll(ctx context.Context, companyID int64, f Filter) ([]Claim, error) {
	if companyID <= 0 {
		return nil, ErrCompanyRequired
	}
	query := `SELECT ` + listCols + listFrom + ` WHERE c.company_id=$1`
	args := []any{companyID}
	if st := strings.ToUpper(strings.TrimSpace(f.Status)); st != "" && st != "ALL" {
		args = append(args, st)
		query += fmt.Sprintf(" AND c.status=$%d", len(args))
	}
	if f.EmployeeID != nil && *f.EmployeeID > 0 {
		args = append(args, *f.EmployeeID)
		query += fmt.Sprintf(" AND c.employee_id=$%d", len(args))
	}
	if f.From != nil && !f.From.IsZero() {
		args = append(args, dateOnly(*f.From))
		query += fmt.Sprintf(" AND c.claim_date>=$%d", len(args))
	}
	if f.To != nil && !f.To.IsZero() {
		args = append(args, dateOnly(*f.To))
		query += fmt.Sprintf(" AND c.claim_date<=$%d", len(args))
	}
	if search := strings.TrimSpace(f.Search); search != "" {
		args = append(args, "%"+strings.ToLower(search)+"%")
		query += fmt.Sprintf(" AND (LOWER(e.name) LIKE $%d OR LOWER(e.employee_number) LIKE $%d OR LOWER(c.category) LIKE $%d)", len(args), len(args), len(args))
	}
	query += ` ORDER BY c.claim_date DESC, c.id DESC`
	return s.list(ctx, query, args...)
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
