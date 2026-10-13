// Package profilechanges manages employee self-service profile change
// requests (address, bank details, emergency contact) routed through the
// approvals engine (module "HR_PROFILE").
//
// A request is stored PENDING with its payload; the change is applied only
// when the approval completes as APPROVED, in one transaction:
//
//   - ADDRESS updates the address/phone columns present in the payload on
//     hr_employees;
//   - EMERGENCY_CONTACT creates or updates (contact_id) an emergency contact;
//   - BANK calls compensation.ChangeBankTx effective from the approval's
//     decision date (approval_requests.completed_at).
//
// The finalizer is idempotent (non-PENDING rows are left untouched) and
// verifies the approval's company. A failed apply leaves the request PENDING
// with apply_error recorded; an HR admin re-runs it with RetryApply.
//
// Audit metadata carries the change type only, never payload values.
package profilechanges

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/approvals"
	"github.com/odyssey-erp/odyssey-erp/internal/hr/compensation"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// Module is the approvals-engine module name for profile change requests.
const Module = "HR_PROFILE"

// Change types (hr_profile_change_requests.change_type).
const (
	TypeAddress          = "ADDRESS"
	TypeBank             = "BANK"
	TypeEmergencyContact = "EMERGENCY_CONTACT"
)

// Request statuses (hr_profile_change_requests.status).
const (
	StatusPending   = "PENDING"
	StatusApproved  = "APPROVED"
	StatusRejected  = "REJECTED"
	StatusCancelled = "CANCELLED"
)

// Audit actions recorded by this package.
const (
	ActionSubmitted    = "PROFILE_CHANGE_SUBMITTED"
	ActionSubmitFailed = "PROFILE_CHANGE_SUBMIT_FAILED"
	ActionCancelled    = "PROFILE_CHANGE_CANCELLED"
	ActionApplied      = "PROFILE_CHANGE_APPLIED"
	ActionRejected     = "PROFILE_CHANGE_REJECTED"

	entity = "hr_profile_change_request"
)

// BankChangeReason is the compensation assignment reason written for an
// approved BANK request.
const BankChangeReason = "Employee bank change"

// MaskedValue replaces sensitive payload values for callers without
// hr.employee.sensitive.
const MaskedValue = "••••"

// maxValueLen bounds every payload value (runes).
const maxValueLen = 200

var (
	ErrNotFound               = errors.New("hr: not found")
	ErrEmployeeNotFound       = fmt.Errorf("%w: employee", ErrNotFound)
	ErrRequestNotFound        = fmt.Errorf("%w: profile change request", ErrNotFound)
	ErrContactNotFound        = fmt.Errorf("%w: emergency contact", ErrNotFound)
	ErrCompanyRequired        = errors.New("hr: company required")
	ErrInvalidRequest         = errors.New("hr: invalid profile change request")
	ErrInvalidChangeType      = errors.New("hr: change type must be ADDRESS, BANK or EMERGENCY_CONTACT")
	ErrInvalidPayload         = errors.New("hr: invalid profile change payload")
	ErrNoPolicy               = errors.New("hr: no approval policy configured for profile changes")
	ErrSubmitFailed           = errors.New("hr: could not submit profile change for approval")
	ErrNotPending             = errors.New("hr: only pending profile change requests can be cancelled")
	ErrNotRetryable           = errors.New("hr: only pending profile change requests can be retried")
	ErrNoApproval             = errors.New("hr: profile change request has no approval")
	ErrApprovalNotDecided     = errors.New("hr: approval is not decided yet")
	ErrCompanyMismatch        = errors.New("hr: approval company does not match profile change request")
	ErrInvalidDecision        = errors.New("hr: invalid approval decision")
	errAuditNotConfigured     = errors.New("hr: profile change audit not configured")
	errApprovalsNotConfigured = errors.New("hr: profile change approvals not configured")
	errBankNotConfigured      = errors.New("hr: profile change compensation not configured")
)

// Payload keys accepted per change type.
var allowedKeys = map[string][]string{
	TypeAddress:          {"address_line1", "address_line2", "city", "province", "postal_code", "country", "phone"},
	TypeBank:             {"bank_code", "bank_account_number", "bank_account_name"},
	TypeEmergencyContact: {"contact_id", "name", "relationship", "phone", "email", "is_primary"},
}

// sensitiveKeys are masked in ListAll for callers without hr.employee.sensitive.
var sensitiveKeys = map[string]map[string]bool{
	TypeAddress: {"address_line1": true, "address_line2": true, "city": true, "province": true, "postal_code": true, "country": true},
	TypeBank:    {"bank_code": true, "bank_account_number": true, "bank_account_name": true},
}

// Request maps an hr_profile_change_requests row (plus employee display
// fields on list queries).
type Request struct {
	ID                int64
	CompanyID         int64
	EmployeeID        int64
	EmployeeName      string
	EmployeeNumber    string
	ChangeType        string
	Payload           map[string]string
	Masked            bool
	Status            string
	ApprovalRequestID *int64
	ApplyError        string
	AppliedAt         *time.Time
	DecidedBy         *int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// SubmitInput is an employee's profile change request.
type SubmitInput struct {
	UserID     int64
	CompanyID  int64
	ChangeType string
	Payload    map[string]string
}

// Filter narrows ListAll. ShowSensitive must be true only for callers holding
// hr.employee.sensitive; otherwise address and bank payload values are masked.
type Filter struct {
	Status        string
	ChangeType    string
	EmployeeID    *int64
	Search        string
	ShowSensitive bool
}

// Audit is satisfied by *shared.AuditLogger.
type Audit interface {
	RecordTx(context.Context, pgx.Tx, shared.AuditLog) error
}

// Approvals is satisfied by *approvals.Service.
type Approvals interface {
	Submit(context.Context, approvals.Submission) (approvals.Request, error)
}

// BankChanger is satisfied by *compensation.Service.
type BankChanger interface {
	ChangeBankTx(ctx context.Context, tx compensation.DBTX, companyID, employeeID int64, bank compensation.BankDetails, effectiveFrom time.Time, reason string, actorID int64) (compensation.Assignment, error)
}

type db interface {
	Begin(context.Context) (pgx.Tx, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Service manages profile change requests and is the HR_PROFILE approvals
// finalizer.
type Service struct {
	pool      db
	approvals Approvals
	audit     Audit
	bank      BankChanger
}

// NewService builds a Service.
func NewService(pool *pgxpool.Pool, a Approvals, audit Audit, bank BankChanger) *Service {
	return &Service{pool: pool, approvals: a, audit: audit, bank: bank}
}

// NormalizePayload validates the payload for the change type and returns a
// trimmed copy. Unknown keys are rejected.
func NormalizePayload(changeType string, payload map[string]string) (map[string]string, error) {
	keys, ok := allowedKeys[changeType]
	if !ok {
		return nil, ErrInvalidChangeType
	}
	allowed := make(map[string]bool, len(keys))
	for _, k := range keys {
		allowed[k] = true
	}
	out := make(map[string]string, len(payload))
	for k, v := range payload {
		if !allowed[k] {
			return nil, fmt.Errorf("%w: unknown field %q", ErrInvalidPayload, k)
		}
		v = strings.TrimSpace(v)
		if utf8.RuneCountInString(v) > maxValueLen {
			return nil, fmt.Errorf("%w: %s is too long", ErrInvalidPayload, k)
		}
		out[k] = v
	}
	switch changeType {
	case TypeAddress:
		if len(out) == 0 {
			return nil, fmt.Errorf("%w: no address fields", ErrInvalidPayload)
		}
		if v, ok := out["address_line1"]; ok && v == "" {
			return nil, fmt.Errorf("%w: address_line1 required", ErrInvalidPayload)
		}
	case TypeBank:
		if out["bank_code"] == "" || out["bank_account_number"] == "" {
			return nil, fmt.Errorf("%w: bank_code and bank_account_number required", ErrInvalidPayload)
		}
	case TypeEmergencyContact:
		if out["name"] == "" {
			return nil, fmt.Errorf("%w: name required", ErrInvalidPayload)
		}
		if v, ok := out["contact_id"]; ok {
			if v == "" {
				delete(out, "contact_id")
			} else if id, err := strconv.ParseInt(v, 10, 64); err != nil || id <= 0 {
				return nil, fmt.Errorf("%w: contact_id", ErrInvalidPayload)
			} else {
				out["contact_id"] = strconv.FormatInt(id, 10)
			}
		}
		if v := out["email"]; v != "" {
			if _, err := mail.ParseAddress(v); err != nil {
				return nil, fmt.Errorf("%w: email", ErrInvalidPayload)
			}
		}
		if v, ok := out["is_primary"]; ok {
			b, err := parseBool(v)
			if err != nil {
				return nil, fmt.Errorf("%w: is_primary", ErrInvalidPayload)
			}
			out["is_primary"] = strconv.FormatBool(b)
		}
	}
	return out, nil
}

func parseBool(v string) (bool, error) {
	switch strings.ToLower(v) {
	case "", "0", "false", "off", "no":
		return false, nil
	case "1", "true", "on", "yes":
		return true, nil
	}
	return false, errors.New("invalid bool")
}

func contactID(payload map[string]string) int64 {
	id, _ := strconv.ParseInt(payload["contact_id"], 10, 64)
	return id
}

// Submit records a PENDING profile change request and routes it to the
// approvals engine. When the engine refuses (including no matching policy)
// the request is cancelled by a compensating transaction.
func (s *Service) Submit(ctx context.Context, in SubmitInput) (Request, error) {
	if in.CompanyID <= 0 {
		return Request{}, ErrCompanyRequired
	}
	if in.UserID <= 0 {
		return Request{}, ErrInvalidRequest
	}
	changeType := strings.ToUpper(strings.TrimSpace(in.ChangeType))
	payload, err := NormalizePayload(changeType, in.Payload)
	if err != nil {
		return Request{}, err
	}
	if s.audit == nil {
		return Request{}, errAuditNotConfigured
	}
	if s.approvals == nil {
		return Request{}, errApprovalsNotConfigured
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return Request{}, fmt.Errorf("submit profile change: payload: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Request{}, fmt.Errorf("submit profile change: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var employeeID int64
	var managerUserID *int64
	err = tx.QueryRow(ctx, `SELECT e.id, m.user_id FROM hr_employees e LEFT JOIN hr_employees m ON m.id=e.manager_id AND m.company_id=e.company_id WHERE e.user_id=$1 AND e.company_id=$2 AND e.status='ACTIVE'`, in.UserID, in.CompanyID).Scan(&employeeID, &managerUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, ErrEmployeeNotFound
	}
	if err != nil {
		return Request{}, fmt.Errorf("submit profile change: employee: %w", err)
	}
	if id := contactID(payload); changeType == TypeEmergencyContact && id > 0 {
		if err := checkContact(ctx, tx, in.CompanyID, employeeID, id); err != nil {
			return Request{}, err
		}
	}

	req, err := scanRequest(tx.QueryRow(ctx, `INSERT INTO hr_profile_change_requests(company_id, employee_id, change_type, payload, status) VALUES($1,$2,$3,$4::jsonb,'PENDING') RETURNING `+requestCols, in.CompanyID, employeeID, changeType, string(raw)))
	if err != nil {
		return Request{}, fmt.Errorf("submit profile change: insert: %w", err)
	}
	if err := s.record(ctx, tx, in.UserID, ActionSubmitted, req.ID, changeType, nil); err != nil {
		return Request{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Request{}, fmt.Errorf("submit profile change: commit: %w", err)
	}

	var manager int64
	if managerUserID != nil {
		manager = *managerUserID
	}
	companyID := in.CompanyID
	approvalReq, err := s.approvals.Submit(ctx, approvals.Submission{
		Module:      Module,
		DocumentID:  req.ID,
		RequesterID: in.UserID,
		CompanyID:   &companyID,
		Amount:      0,
		ManagerID:   manager,
	})
	if err != nil {
		class, userErr := "approval_error", ErrSubmitFailed
		if errors.Is(err, approvals.ErrNoPolicy) {
			class, userErr = "no_policy", ErrNoPolicy
		}
		if cerr := s.compensate(ctx, in.UserID, req.ID, changeType, class); cerr != nil {
			return Request{}, fmt.Errorf("%w (cleanup failed: %v)", userErr, cerr)
		}
		return Request{}, userErr
	}
	if _, err := s.pool.Exec(ctx, `UPDATE hr_profile_change_requests SET approval_request_id=$2, updated_at=NOW() WHERE id=$1 AND company_id=$3`, req.ID, approvalReq.ID, in.CompanyID); err != nil {
		return Request{}, fmt.Errorf("hr: linking approval request: %w", err)
	}
	req.ApprovalRequestID = &approvalReq.ID
	return req, nil
}

// checkContact verifies the contact belongs to the employee in the company.
func checkContact(ctx context.Context, q rowQuerier, companyID, employeeID, contactID int64) error {
	var id int64
	err := q.QueryRow(ctx, `SELECT c.id FROM hr_employee_emergency_contacts c JOIN hr_employees e ON e.id=c.employee_id AND e.company_id=$3 WHERE c.id=$1 AND c.employee_id=$2`, contactID, employeeID, companyID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrContactNotFound
	}
	if err != nil {
		return fmt.Errorf("check emergency contact: %w", err)
	}
	return nil
}

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// compensate cancels a committed submission whose approval could not be created.
func (s *Service) compensate(ctx context.Context, userID, requestID int64, changeType, class string) error {
	ctx = context.WithoutCancel(ctx)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `UPDATE hr_profile_change_requests SET status='CANCELLED', updated_at=NOW() WHERE id=$1 AND status='PENDING'`, requestID); err != nil {
		return err
	}
	if err := s.record(ctx, tx, userID, ActionSubmitFailed, requestID, changeType, map[string]any{"error_class": class}); err != nil {
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
		return fmt.Errorf("cancel profile change: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status, changeType string
	var approvalReqID *int64
	err = tx.QueryRow(ctx, `SELECT p.status, p.change_type, p.approval_request_id FROM hr_profile_change_requests p JOIN hr_employees e ON e.id=p.employee_id WHERE p.id=$1 AND e.user_id=$2 AND p.company_id=$3 AND e.company_id=$3 FOR UPDATE OF p`, requestID, userID, companyID).Scan(&status, &changeType, &approvalReqID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRequestNotFound
	}
	if err != nil {
		return fmt.Errorf("cancel profile change: %w", err)
	}
	if status != StatusPending {
		return ErrNotPending
	}
	if _, err := tx.Exec(ctx, `UPDATE hr_profile_change_requests SET status='CANCELLED', updated_at=NOW() WHERE id=$1`, requestID); err != nil {
		return fmt.Errorf("cancel profile change: %w", err)
	}
	if approvalReqID != nil {
		if _, err := tx.Exec(ctx, `UPDATE approval_requests SET status='CANCELLED', updated_at=NOW() WHERE id=$1 AND status='PENDING'`, *approvalReqID); err != nil {
			return fmt.Errorf("cancel profile change: approval request: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE approval_assignments SET status='CANCELLED', updated_at=NOW() WHERE request_id=$1 AND status='PENDING'`, *approvalReqID); err != nil {
			return fmt.Errorf("cancel profile change: approval assignments: %w", err)
		}
	}
	if err := s.record(ctx, tx, userID, ActionCancelled, requestID, changeType, nil); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("cancel profile change: commit: %w", err)
	}
	return nil
}

// FinalizeApproval implements approvals.Finalizer. It is idempotent (a
// non-PENDING row is left untouched) and verifies the approval's company.
func (s *Service) FinalizeApproval(ctx context.Context, a approvals.Request, status string, actorID int64, note string) error {
	return s.finalize(ctx, a, status, actorID, note)
}

type lockedRow struct {
	companyID  int64
	employeeID int64
	changeType string
	status     string
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
		return fmt.Errorf("finalize profile change: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var row lockedRow
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT p.company_id, p.employee_id, p.change_type, p.payload, p.status FROM hr_profile_change_requests p WHERE p.id=$1 FOR UPDATE OF p`, a.DocumentID).
		Scan(&row.companyID, &row.employeeID, &row.changeType, &raw, &row.status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRequestNotFound
	}
	if err != nil {
		return fmt.Errorf("finalize profile change: %w", err)
	}
	if a.CompanyID == nil || *a.CompanyID != row.companyID {
		return ErrCompanyMismatch
	}
	if row.status != StatusPending {
		return nil // idempotent no-op
	}

	err = s.apply(ctx, tx, a, &row, raw, status, actorID, note)
	if err == nil {
		if cerr := tx.Commit(ctx); cerr != nil {
			err = fmt.Errorf("finalize profile change: commit: %w", cerr)
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
func (s *Service) apply(ctx context.Context, tx pgx.Tx, a approvals.Request, row *lockedRow, raw []byte, status string, actorID int64, note string) error {
	// The approver's note is free text and may echo payload values, so it is
	// never copied into audit meta.
	_ = note
	id := a.DocumentID
	meta := map[string]any{"approval_request_id": a.ID}
	if status == approvals.StatusRejected {
		if _, err := tx.Exec(ctx, `UPDATE hr_profile_change_requests SET status='REJECTED', apply_error='', decided_by=$2, updated_at=NOW() WHERE id=$1`, id, actorID); err != nil {
			return fmt.Errorf("finalize profile change: reject: %w", err)
		}
		return s.record(ctx, tx, actorID, ActionRejected, id, row.changeType, meta)
	}

	var stored map[string]string
	if err := json.Unmarshal(raw, &stored); err != nil {
		return fmt.Errorf("finalize profile change: payload: %w", err)
	}
	payload, err := NormalizePayload(row.changeType, stored)
	if err != nil {
		return err
	}
	switch row.changeType {
	case TypeAddress:
		err = applyAddress(ctx, tx, row.companyID, row.employeeID, payload)
	case TypeEmergencyContact:
		err = applyContact(ctx, tx, row.companyID, row.employeeID, payload)
	case TypeBank:
		err = s.applyBank(ctx, tx, a, row.companyID, row.employeeID, payload, actorID)
	default:
		err = ErrInvalidChangeType
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE hr_profile_change_requests SET status='APPROVED', apply_error='', applied_at=NOW(), decided_by=$2, updated_at=NOW() WHERE id=$1`, id, actorID); err != nil {
		return fmt.Errorf("finalize profile change: approve: %w", err)
	}
	return s.record(ctx, tx, actorID, ActionApplied, id, row.changeType, meta)
}

// addressColumns maps ADDRESS payload keys to hr_employees columns in a fixed order.
var addressColumns = []string{"address_line1", "address_line2", "city", "province", "postal_code", "country", "phone"}

func applyAddress(ctx context.Context, tx pgx.Tx, companyID, employeeID int64, payload map[string]string) error {
	args := []any{employeeID, companyID}
	sets := make([]string, 0, len(payload)+1)
	for _, col := range addressColumns {
		// Only fields present and non-empty overwrite; a partial change keeps the
		// current value of every other address field.
		v, ok := payload[col]
		if !ok || strings.TrimSpace(v) == "" {
			continue
		}
		args = append(args, v)
		sets = append(sets, fmt.Sprintf("%s=$%d", col, len(args)))
	}
	if len(sets) == 0 {
		return fmt.Errorf("%w: no address fields", ErrInvalidPayload)
	}
	sets = append(sets, "updated_at=NOW()")
	tag, err := tx.Exec(ctx, `UPDATE hr_employees SET `+strings.Join(sets, ", ")+` WHERE id=$1 AND company_id=$2`, args...)
	if err != nil {
		return fmt.Errorf("apply address change: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEmployeeNotFound
	}
	return nil
}

func applyContact(ctx context.Context, tx pgx.Tx, companyID, employeeID int64, payload map[string]string) error {
	var locked int64
	err := tx.QueryRow(ctx, `SELECT id FROM hr_employees WHERE id=$1 AND company_id=$2 FOR UPDATE`, employeeID, companyID).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrEmployeeNotFound
	}
	if err != nil {
		return fmt.Errorf("lock employee: %w", err)
	}
	primary, _ := parseBool(payload["is_primary"])
	name, relationship, phone, email := payload["name"], payload["relationship"], payload["phone"], payload["email"]
	var savedID int64
	if cid := contactID(payload); cid > 0 {
		err = tx.QueryRow(ctx, `UPDATE hr_employee_emergency_contacts c SET name=$1, relationship=$2, phone=$3, email=$4, is_primary=$5, updated_at=NOW()
FROM hr_employees e WHERE e.id=c.employee_id AND e.company_id=$6 AND c.employee_id=$7 AND c.id=$8 RETURNING c.id`,
			name, relationship, phone, email, primary, companyID, employeeID, cid).Scan(&savedID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrContactNotFound
		}
	} else {
		err = tx.QueryRow(ctx, `INSERT INTO hr_employee_emergency_contacts(company_id,employee_id,name,relationship,phone,email,is_primary)
VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, companyID, employeeID, name, relationship, phone, email, primary).Scan(&savedID)
	}
	if err != nil {
		return fmt.Errorf("apply emergency contact change: %w", err)
	}
	if primary {
		if _, err := tx.Exec(ctx, `UPDATE hr_employee_emergency_contacts SET is_primary=FALSE, updated_at=NOW() WHERE employee_id=$1 AND id<>$2 AND is_primary`, employeeID, savedID); err != nil {
			return fmt.Errorf("clear primary contact: %w", err)
		}
	}
	return nil
}

// decisionDateSQL returns the approval's decision date (completed_at, or now
// when the engine has not stamped it).
//
// The date is the calendar date in Asia/Jakarta (the business timezone, as for
// job changes): a decision made at 02:00 Jakarta time on the 1st is still the
// 31st in UTC, which would backdate the bank change by a day.
const decisionDateExpr = `(COALESCE(completed_at, NOW()) AT TIME ZONE 'Asia/Jakarta')::date`

const decisionDateSQL = `SELECT ` + decisionDateExpr + ` FROM approval_requests WHERE id=$1`

func (s *Service) applyBank(ctx context.Context, tx pgx.Tx, a approvals.Request, companyID, employeeID int64, payload map[string]string, actorID int64) error {
	if s.bank == nil {
		return errBankNotConfigured
	}
	var decided time.Time
	if err := tx.QueryRow(ctx, decisionDateSQL, a.ID).Scan(&decided); err != nil {
		return fmt.Errorf("apply bank change: decision date: %w", err)
	}
	_, err := s.bank.ChangeBankTx(ctx, tx, companyID, employeeID, compensation.BankDetails{
		BankCode:          payload["bank_code"],
		BankAccountNumber: payload["bank_account_number"],
		BankAccountName:   payload["bank_account_name"],
	}, dateOnly(decided), BankChangeReason, actorID)
	if err != nil {
		return fmt.Errorf("apply bank change: %w", err)
	}
	return nil
}

// markApplyError records a failed apply on the still-PENDING row (best effort).
func (s *Service) markApplyError(ctx context.Context, requestID int64, cause error) {
	msg := cause.Error()
	if len(msg) > 500 {
		msg = msg[:500]
	}
	_, _ = s.pool.Exec(context.WithoutCancel(ctx), `UPDATE hr_profile_change_requests SET apply_error=$2, updated_at=NOW() WHERE id=$1 AND status='PENDING'`, requestID, msg)
}

// RetryApply re-runs the finalizer for a PENDING request whose approval is
// already decided (for example after an apply error).
func (s *Service) RetryApply(ctx context.Context, companyID, requestID, actorID int64) error {
	if companyID <= 0 {
		return ErrCompanyRequired
	}
	if requestID <= 0 || actorID <= 0 {
		return ErrInvalidRequest
	}
	var status string
	var approvalReqID *int64
	err := s.pool.QueryRow(ctx, `SELECT p.status, p.approval_request_id FROM hr_profile_change_requests p WHERE p.id=$1 AND p.company_id=$2`, requestID, companyID).Scan(&status, &approvalReqID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRequestNotFound
	}
	if err != nil {
		return fmt.Errorf("retry profile change: %w", err)
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
		return fmt.Errorf("retry profile change: approval: %w", err)
	}
	if a.Status != approvals.StatusApproved && a.Status != approvals.StatusRejected {
		return ErrApprovalNotDecided
	}
	return s.finalize(ctx, a, a.Status, actorID, "retry apply")
}

const requestCols = `id, company_id, employee_id, change_type, payload, status, approval_request_id, apply_error, applied_at, decided_by, created_at, updated_at`

func scanRequest(row pgx.Row) (Request, error) {
	var r Request
	var raw []byte
	if err := row.Scan(&r.ID, &r.CompanyID, &r.EmployeeID, &r.ChangeType, &raw, &r.Status, &r.ApprovalRequestID, &r.ApplyError, &r.AppliedAt, &r.DecidedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return Request{}, err
	}
	if err := decodePayload(raw, &r); err != nil {
		return Request{}, err
	}
	return r, nil
}

func decodePayload(raw []byte, r *Request) error {
	r.Payload = map[string]string{}
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, &r.Payload); err != nil {
		return fmt.Errorf("decode profile change payload: %w", err)
	}
	return nil
}

const listCols = `p.id, p.company_id, p.employee_id, p.change_type, p.payload, p.status, p.approval_request_id, p.apply_error, p.applied_at, p.decided_by, p.created_at, p.updated_at, e.name, e.employee_number`

const listFrom = ` FROM hr_profile_change_requests p JOIN hr_employees e ON e.id=p.employee_id AND e.company_id=p.company_id`

func (s *Service) list(ctx context.Context, query string, args ...any) ([]Request, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list profile changes: %w", err)
	}
	defer rows.Close()
	var out []Request
	for rows.Next() {
		var r Request
		var raw []byte
		if err := rows.Scan(&r.ID, &r.CompanyID, &r.EmployeeID, &r.ChangeType, &raw, &r.Status, &r.ApprovalRequestID, &r.ApplyError, &r.AppliedAt, &r.DecidedBy, &r.CreatedAt, &r.UpdatedAt, &r.EmployeeName, &r.EmployeeNumber); err != nil {
			return nil, fmt.Errorf("list profile changes: %w", err)
		}
		if err := decodePayload(raw, &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list profile changes: %w", err)
	}
	return out, nil
}

// ListOwn returns the session user's requests in the company, newest first.
// The owner sees their own payload unmasked.
func (s *Service) ListOwn(ctx context.Context, companyID, userID int64) ([]Request, error) {
	if companyID <= 0 {
		return nil, ErrCompanyRequired
	}
	if userID <= 0 {
		return nil, ErrInvalidRequest
	}
	return s.list(ctx, `SELECT `+listCols+listFrom+` WHERE p.company_id=$1 AND e.user_id=$2 ORDER BY p.created_at DESC, p.id DESC`, companyID, userID)
}

// ListAll returns the company's requests matching the filter, newest first.
// Address and bank payload values are masked unless f.ShowSensitive is set.
func (s *Service) ListAll(ctx context.Context, companyID int64, f Filter) ([]Request, error) {
	if companyID <= 0 {
		return nil, ErrCompanyRequired
	}
	query := `SELECT ` + listCols + listFrom + ` WHERE p.company_id=$1`
	args := []any{companyID}
	if st := strings.ToUpper(strings.TrimSpace(f.Status)); st != "" && st != "ALL" {
		args = append(args, st)
		query += fmt.Sprintf(" AND p.status=$%d", len(args))
	}
	if ct := strings.ToUpper(strings.TrimSpace(f.ChangeType)); ct != "" && ct != "ALL" {
		args = append(args, ct)
		query += fmt.Sprintf(" AND p.change_type=$%d", len(args))
	}
	if f.EmployeeID != nil && *f.EmployeeID > 0 {
		args = append(args, *f.EmployeeID)
		query += fmt.Sprintf(" AND p.employee_id=$%d", len(args))
	}
	if search := strings.TrimSpace(f.Search); search != "" {
		args = append(args, "%"+strings.ToLower(search)+"%")
		query += fmt.Sprintf(" AND (LOWER(e.name) LIKE $%d OR LOWER(e.employee_number) LIKE $%d)", len(args), len(args))
	}
	query += ` ORDER BY p.created_at DESC, p.id DESC`
	out, err := s.list(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	if !f.ShowSensitive {
		for i := range out {
			MaskPayload(&out[i])
		}
	}
	return out, nil
}

// MaskPayload blanks sensitive payload values (address fields and bank
// details) in place. Callers without hr.employee.sensitive must call it
// before rendering a Request.
func MaskPayload(r *Request) {
	if r == nil {
		return
	}
	keys := sensitiveKeys[r.ChangeType]
	if len(keys) == 0 {
		return
	}
	masked := make(map[string]string, len(r.Payload))
	for k, v := range r.Payload {
		if keys[k] && v != "" {
			v = MaskedValue
		}
		masked[k] = v
	}
	r.Payload = masked
	r.Masked = true
}

// PayloadKeys returns the payload keys in a stable order for rendering.
func (r Request) PayloadKeys() []string {
	keys := make([]string, 0, len(r.Payload))
	for k := range r.Payload {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (s *Service) record(ctx context.Context, tx pgx.Tx, actorID int64, action string, id int64, changeType string, extra map[string]any) error {
	if s.audit == nil {
		return errAuditNotConfigured
	}
	meta := map[string]any{"change_type": changeType}
	for k, v := range extra {
		meta[k] = v
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
