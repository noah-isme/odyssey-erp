package leave

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
	"github.com/odyssey-erp/odyssey-erp/internal/hr/schedule"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

type LeaveType struct {
	ID          int64
	Code, Name  string
	DefaultDays float64
}

type Request struct {
	ID                int64
	EmployeeID        int64
	EmployeeName      string
	EmployeeNumber    string
	Department        string
	LeaveTypeID       int64
	TypeName          string
	StartDate         time.Time
	EndDate           time.Time
	Days              float64
	StartHalfDay      bool
	EndHalfDay        bool
	Reason            string
	Status            string
	ApprovalRequestID *int64
	CreatedAt         time.Time
}

type Balance struct {
	LeaveTypeID int64
	TypeCode    string
	TypeName    string
	Year        int
	Entitled    float64
	Used        float64
	Pending     float64
	Available   float64
}

type EmployeeBalanceSummary struct {
	EmployeeID     int64
	EmployeeNumber string
	EmployeeName   string
	Department     string
	LeaveTypeID    int64
	TypeName       string
	Year           int
	Entitled       float64
	Used           float64
	Pending        float64
	Available      float64
}

type LeaveFilter struct {
	Status      string
	LeaveTypeID *int64
	Search      string
}

type EmployeeOption struct {
	ID             int64
	EmployeeNumber string
	Name           string
	Department     string
}

type CreateInput struct {
	CompanyID, UserID, LeaveTypeID int64
	StartDate, EndDate             time.Time
	Reason                         string
	StartHalfDay, EndHalfDay       bool
}

// WorkdayCalculator supplies the calendar data needed to count working days.
// schedule.Service satisfies it.
type WorkdayCalculator interface {
	HolidaysBetween(ctx context.Context, companyID int64, from, to time.Time) ([]time.Time, error)
	WorkDaysFor(ctx context.Context, companyID, employeeID int64, date time.Time) ([]time.Weekday, error)
}

var errCompanyRequired = errors.New("hr: company required")

type Audit interface {
	Record(context.Context, shared.AuditLog) error
}

type Service struct {
	pool      leaveDB
	approvals *approvals.Service
	audit     Audit
	workdays  WorkdayCalculator
}

type leaveDB interface {
	Begin(context.Context) (pgx.Tx, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func NewService(pool *pgxpool.Pool, a *approvals.Service, audit Audit, workdays WorkdayCalculator) *Service {
	return &Service{pool: pool, approvals: a, audit: audit, workdays: workdays}
}

func (s *Service) Types(ctx context.Context, companyID int64) ([]LeaveType, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,code,name,default_days FROM hr_leave_types WHERE is_active AND (company_id=$1 OR company_id IS NULL) ORDER BY name`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LeaveType
	for rows.Next() {
		var x LeaveType
		if err := rows.Scan(&x.ID, &x.Code, &x.Name, &x.DefaultDays); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Service) CreateType(ctx context.Context, companyID int64, code, name string, defaultDays float64) (LeaveType, error) {
	code = strings.TrimSpace(code)
	name = strings.TrimSpace(name)
	if companyID <= 0 || code == "" || name == "" || defaultDays < 0 {
		return LeaveType{}, errors.New("hr: invalid leave type parameters")
	}
	var t LeaveType
	err := s.pool.QueryRow(ctx, `INSERT INTO hr_leave_types(company_id, code, name, default_days, is_active) VALUES($1, $2, $3, $4, TRUE) RETURNING id, code, name, default_days`, companyID, code, name, defaultDays).Scan(&t.ID, &t.Code, &t.Name, &t.DefaultDays)
	return t, err
}

func (s *Service) ListOwn(ctx context.Context, userID int64) ([]Request, error) {
	rows, err := s.pool.Query(ctx, `SELECT r.id,r.employee_id,r.leave_type_id,t.name,r.start_date,r.end_date,r.days,r.start_half_day,r.end_half_day,r.reason,r.status,r.approval_request_id FROM hr_leave_requests r JOIN hr_employees e ON e.id=r.employee_id JOIN hr_leave_types t ON t.id=r.leave_type_id WHERE e.user_id=$1 ORDER BY r.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Request
	for rows.Next() {
		var x Request
		if err := rows.Scan(&x.ID, &x.EmployeeID, &x.LeaveTypeID, &x.TypeName, &x.StartDate, &x.EndDate, &x.Days, &x.StartHalfDay, &x.EndHalfDay, &x.Reason, &x.Status, &x.ApprovalRequestID); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Service) GetOwnBalances(ctx context.Context, companyID, userID int64, year int) ([]Balance, error) {
	if companyID <= 0 {
		return nil, errCompanyRequired
	}
	rows, err := s.pool.Query(ctx, `SELECT b.leave_type_id, t.code, t.name, b.year, b.entitled, b.used, b.pending, GREATEST(0, b.entitled - b.used - b.pending) FROM hr_leave_balances b JOIN hr_leave_types t ON t.id=b.leave_type_id JOIN hr_employees e ON e.id=b.employee_id WHERE e.user_id=$1 AND e.company_id=$2 AND b.year=$3 ORDER BY t.name`, userID, companyID, year)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Balance
	for rows.Next() {
		var b Balance
		if err := rows.Scan(&b.LeaveTypeID, &b.TypeCode, &b.TypeName, &b.Year, &b.Entitled, &b.Used, &b.Pending, &b.Available); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Service) ListAll(ctx context.Context, companyID int64, filter LeaveFilter) ([]Request, error) {
	if companyID <= 0 {
		return nil, errCompanyRequired
	}
	query := `SELECT r.id, r.employee_id, r.leave_type_id, t.name, r.start_date, r.end_date, r.days, r.start_half_day, r.end_half_day, r.reason, r.status, r.approval_request_id, e.name, e.employee_number, COALESCE(d.name,''), r.created_at FROM hr_leave_requests r JOIN hr_employees e ON e.id=r.employee_id LEFT JOIN hr_departments d ON d.id=e.department_id JOIN hr_leave_types t ON t.id=r.leave_type_id WHERE e.company_id=$1`
	args := []any{companyID}

	if filter.Status != "" && filter.Status != "ALL" {
		args = append(args, strings.ToUpper(strings.TrimSpace(filter.Status)))
		query += fmt.Sprintf(" AND r.status=$%d", len(args))
	}
	if filter.LeaveTypeID != nil && *filter.LeaveTypeID > 0 {
		args = append(args, *filter.LeaveTypeID)
		query += fmt.Sprintf(" AND r.leave_type_id=$%d", len(args))
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		pattern := "%" + strings.ToLower(search) + "%"
		args = append(args, pattern)
		query += fmt.Sprintf(" AND (LOWER(e.name) LIKE $%d OR LOWER(e.employee_number) LIKE $%d)", len(args), len(args))
	}

	query += ` ORDER BY r.created_at DESC`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Request
	for rows.Next() {
		var r Request
		if err := rows.Scan(&r.ID, &r.EmployeeID, &r.LeaveTypeID, &r.TypeName, &r.StartDate, &r.EndDate, &r.Days, &r.StartHalfDay, &r.EndHalfDay, &r.Reason, &r.Status, &r.ApprovalRequestID, &r.EmployeeName, &r.EmployeeNumber, &r.Department, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Service) ListAllBalances(ctx context.Context, companyID int64, year int) ([]EmployeeBalanceSummary, error) {
	if companyID <= 0 {
		return nil, errCompanyRequired
	}
	rows, err := s.pool.Query(ctx, `SELECT b.employee_id, e.employee_number, e.name, COALESCE(d.name,''), b.leave_type_id, t.name, b.year, b.entitled, b.used, b.pending, GREATEST(0, b.entitled - b.used - b.pending) FROM hr_leave_balances b JOIN hr_employees e ON e.id=b.employee_id LEFT JOIN hr_departments d ON d.id=e.department_id JOIN hr_leave_types t ON t.id=b.leave_type_id WHERE e.company_id=$1 AND b.year=$2 ORDER BY e.name, t.name`, companyID, year)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EmployeeBalanceSummary
	for rows.Next() {
		var s EmployeeBalanceSummary
		if err := rows.Scan(&s.EmployeeID, &s.EmployeeNumber, &s.EmployeeName, &s.Department, &s.LeaveTypeID, &s.TypeName, &s.Year, &s.Entitled, &s.Used, &s.Pending, &s.Available); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (s *Service) ListActiveEmployees(ctx context.Context, companyID int64) ([]EmployeeOption, error) {
	if companyID <= 0 {
		return nil, errCompanyRequired
	}
	rows, err := s.pool.Query(ctx, `SELECT e.id, e.employee_number, e.name, COALESCE(d.name,'') FROM hr_employees e LEFT JOIN hr_departments d ON d.id=e.department_id WHERE e.company_id=$1 AND e.status='ACTIVE' ORDER BY e.name`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EmployeeOption
	for rows.Next() {
		var o EmployeeOption
		if err := rows.Scan(&o.ID, &o.EmployeeNumber, &o.Name, &o.Department); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Service) Submit(ctx context.Context, in CreateInput) (Request, error) {
	if in.UserID <= 0 || in.LeaveTypeID <= 0 || in.StartDate.IsZero() || in.EndDate.Before(in.StartDate) {
		return Request{}, errors.New("hr: invalid leave request")
	}
	if in.CompanyID <= 0 {
		return Request{}, errCompanyRequired
	}
	if in.StartDate.Year() != in.EndDate.Year() {
		return Request{}, errors.New("hr: split requests at year end")
	}
	year := in.StartDate.Year()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Request{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var employeeID, companyID int64
	var managerUserID *int64
	err = tx.QueryRow(ctx, `SELECT e.id,e.company_id,m.user_id FROM hr_employees e LEFT JOIN hr_employees m ON m.id=e.manager_id WHERE e.user_id=$1 AND e.company_id=$2 AND e.status='ACTIVE' FOR UPDATE OF e`, in.UserID, in.CompanyID).Scan(&employeeID, &companyID, &managerUserID)
	if err != nil {
		return Request{}, err
	}
	if managerUserID == nil {
		return Request{}, errors.New("hr: employee manager must have a user account")
	}
	days, err := s.workingDays(ctx, in, employeeID)
	if err != nil {
		return Request{}, err
	}
	if days <= 0 {
		return Request{}, errors.New("hr: no working days in range")
	}
	var available float64
	err = tx.QueryRow(ctx, `SELECT entitled-used-pending FROM hr_leave_balances WHERE employee_id=$1 AND leave_type_id=$2 AND year=$3 FOR UPDATE`, employeeID, in.LeaveTypeID, year).Scan(&available)
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, errors.New("hr: leave balance not configured")
	}
	if err != nil {
		return Request{}, err
	}
	if available < days {
		return Request{}, errors.New("hr: insufficient leave balance")
	}
	var overlap int
	err = tx.QueryRow(ctx, `SELECT 1 FROM hr_leave_requests WHERE employee_id=$1 AND status IN ('PENDING','APPROVED') AND start_date<=$3 AND end_date>=$2 LIMIT 1`, employeeID, in.StartDate, in.EndDate).Scan(&overlap)
	if err == nil {
		return Request{}, errors.New("hr: overlaps an existing leave request")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Request{}, err
	}
	var req Request
	// The leave type name is returned with the row so the response matches the
	// list endpoint (type_name).
	err = tx.QueryRow(ctx, `WITH ins AS (INSERT INTO hr_leave_requests(employee_id,leave_type_id,start_date,end_date,days,start_half_day,end_half_day,reason,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'PENDING') RETURNING id,employee_id,leave_type_id,start_date,end_date,days,start_half_day,end_half_day,reason,status)
SELECT ins.id,ins.employee_id,ins.leave_type_id,ins.start_date,ins.end_date,ins.days,ins.start_half_day,ins.end_half_day,ins.reason,ins.status,COALESCE(t.name,'') FROM ins LEFT JOIN hr_leave_types t ON t.id=ins.leave_type_id`, employeeID, in.LeaveTypeID, in.StartDate, in.EndDate, days, in.StartHalfDay, in.EndHalfDay, in.Reason).Scan(&req.ID, &req.EmployeeID, &req.LeaveTypeID, &req.StartDate, &req.EndDate, &req.Days, &req.StartHalfDay, &req.EndHalfDay, &req.Reason, &req.Status, &req.TypeName)
	if err != nil {
		return Request{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE hr_leave_balances SET pending=pending+$4,updated_at=NOW() WHERE employee_id=$1 AND leave_type_id=$2 AND year=$3`, employeeID, in.LeaveTypeID, year, days); err != nil {
		return Request{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Request{}, err
	}

	approvalReq, err := s.approvals.Submit(ctx, approvals.Submission{Module: "LEAVE", DocumentID: req.ID, RequesterID: in.UserID, CompanyID: &companyID, Amount: days, ManagerID: *managerUserID})
	if err != nil {
		class := "approval_error"
		userErr := errors.New("hr: could not submit leave request for approval")
		if errors.Is(err, approvals.ErrNoPolicy) {
			class = "no_policy"
			userErr = errors.New("hr: no approval policy configured for leave")
		}
		if cerr := s.compensate(ctx, in.UserID, req.ID, employeeID, in.LeaveTypeID, year, days, class); cerr != nil {
			return Request{}, fmt.Errorf("%w (cleanup failed: %v)", userErr, cerr)
		}
		return Request{}, userErr
	}
	if _, err = s.pool.Exec(ctx, `UPDATE hr_leave_requests SET approval_request_id=$2,updated_at=NOW() WHERE id=$1`, req.ID, approvalReq.ID); err != nil {
		return Request{}, fmt.Errorf("hr: linking approval request: %w", err)
	}
	req.ApprovalRequestID = &approvalReq.ID
	s.record(ctx, in.UserID, "LEAVE_REQUEST", req.ID, map[string]any{"days": days})
	return req, nil
}

// workingDays counts the employee's working days in the request range using
// their shift week (Mon-Fri fallback) and the company holiday calendar.
func (s *Service) workingDays(ctx context.Context, in CreateInput, employeeID int64) (float64, error) {
	if s.workdays == nil {
		return 0, errors.New("hr: working-day calculator not configured")
	}
	weekDays, err := s.workdays.WorkDaysFor(ctx, in.CompanyID, employeeID, in.StartDate)
	if err != nil {
		return 0, fmt.Errorf("hr: loading work schedule: %w", err)
	}
	holidays, err := s.workdays.HolidaysBetween(ctx, in.CompanyID, in.StartDate, in.EndDate)
	if err != nil {
		return 0, fmt.Errorf("hr: loading holidays: %w", err)
	}
	return schedule.WorkingDays(in.StartDate, in.EndDate, weekDays, holidays, in.StartHalfDay, in.EndHalfDay), nil
}

// compensate undoes a committed submission whose approval could not be created:
// the request becomes CANCELLED and its reserved pending days are released.
func (s *Service) compensate(ctx context.Context, userID, reqID, employeeID, typeID int64, year int, days float64, class string) error {
	ctx = context.WithoutCancel(ctx)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `UPDATE hr_leave_requests SET status='CANCELLED',updated_at=NOW() WHERE id=$1 AND status='PENDING'`, reqID)
	if err != nil {
		return err
	}
	// Release the reserved days only when this call performed the PENDING ->
	// CANCELLED transition; if the request was already cancelled or decided
	// elsewhere, its pending days were released (or consumed) there.
	released := tag.RowsAffected() == 1
	if released {
		if _, err = tx.Exec(ctx, `UPDATE hr_leave_balances SET pending=GREATEST(0,pending-$4),updated_at=NOW() WHERE employee_id=$1 AND leave_type_id=$2 AND year=$3`, employeeID, typeID, year, days); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	s.record(ctx, userID, "LEAVE_SUBMIT_FAILED", reqID, map[string]any{"days": days, "error_class": class, "released": released})
	return nil
}

func (s *Service) Cancel(ctx context.Context, companyID, requestID, userID int64) error {
	if companyID <= 0 {
		return errCompanyRequired
	}
	if requestID <= 0 || userID <= 0 {
		return errors.New("hr: invalid request cancellation parameters")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var employeeID, typeID int64
	var days float64
	var status string
	var start time.Time
	var approvalReqID *int64

	err = tx.QueryRow(ctx, `
		SELECT r.employee_id, r.leave_type_id, r.days, r.status, r.start_date, r.approval_request_id
		FROM hr_leave_requests r
		JOIN hr_employees e ON e.id=r.employee_id
		WHERE r.id=$1 AND e.user_id=$2 AND e.company_id=$3
		FOR UPDATE OF r
	`, requestID, userID, companyID).Scan(&employeeID, &typeID, &days, &status, &start, &approvalReqID)
	if err != nil {
		return errors.New("hr: leave request not found or unauthorized")
	}

	if status != "PENDING" && status != "DRAFT" {
		return errors.New("hr: only pending leave requests can be cancelled")
	}

	_, err = tx.Exec(ctx, `UPDATE hr_leave_requests SET status='CANCELLED', updated_at=NOW() WHERE id=$1`, requestID)
	if err != nil {
		return err
	}

	year := start.Year()
	_, err = tx.Exec(ctx, `UPDATE hr_leave_balances SET pending=GREATEST(0, pending-$4), updated_at=NOW() WHERE employee_id=$1 AND leave_type_id=$2 AND year=$3`, employeeID, typeID, year, days)
	if err != nil {
		return err
	}

	if approvalReqID != nil {
		if _, err = tx.Exec(ctx, `UPDATE approval_requests SET status='CANCELLED', updated_at=NOW() WHERE id=$1 AND status='PENDING'`, *approvalReqID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE approval_assignments SET status='CANCELLED', updated_at=NOW() WHERE request_id=$1 AND status='PENDING'`, *approvalReqID); err != nil {
			return err
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return err
	}

	s.record(ctx, userID, "LEAVE_CANCELLED", requestID, map[string]any{"days": days})
	return nil
}

func (s *Service) FinalizeApproval(ctx context.Context, a approvals.Request, status string, actorID int64, note string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var employeeID, typeID, employeeCompanyID int64
	var start time.Time
	var days float64
	var current string
	err = tx.QueryRow(ctx, `SELECT r.employee_id,r.leave_type_id,r.start_date,r.days,r.status,e.company_id FROM hr_leave_requests r JOIN hr_employees e ON e.id=r.employee_id WHERE r.id=$1 FOR UPDATE OF r`, a.DocumentID).Scan(&employeeID, &typeID, &start, &days, &current, &employeeCompanyID)
	if err != nil {
		return err
	}
	if a.CompanyID == nil || *a.CompanyID != employeeCompanyID {
		return errors.New("hr: approval company does not match leave request")
	}
	if current != "PENDING" {
		// Idempotent: a repeated finalization is a no-op without balance changes.
		s.record(ctx, actorID, "LEAVE_FINALIZE_SKIPPED", a.DocumentID, map[string]any{"status": current})
		return nil
	}
	newStatus := "REJECTED"
	usedDelta := 0.0
	if status == approvals.StatusApproved {
		newStatus = "APPROVED"
		usedDelta = days
	}
	_, err = tx.Exec(ctx, `UPDATE hr_leave_requests SET status=$2,updated_at=NOW() WHERE id=$1`, a.DocumentID, newStatus)
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE hr_leave_balances SET pending=GREATEST(0,pending-$4),used=used+$5,updated_at=NOW() WHERE employee_id=$1 AND leave_type_id=$2 AND year=$3`, employeeID, typeID, start.Year(), days, usedDelta)
	}
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	s.record(ctx, actorID, "LEAVE_"+newStatus, a.DocumentID, map[string]any{"note": note, "days": days})
	return nil
}

func (s *Service) record(ctx context.Context, actor int64, action string, id int64, meta map[string]any) {
	if s.audit != nil {
		_ = s.audit.Record(ctx, shared.AuditLog{ActorID: actor, Action: action, Entity: "hr_leave_request", EntityID: strconv.FormatInt(id, 10), Meta: meta})
	}
}

func (s *Service) SeedBalance(ctx context.Context, companyID, employeeID, typeID int64, year int, days float64) error {
	if companyID <= 0 || employeeID <= 0 || typeID <= 0 || year <= 0 || days < 0 {
		return fmt.Errorf("hr: invalid balance")
	}
	tag, err := s.pool.Exec(ctx, `INSERT INTO hr_leave_balances(employee_id, leave_type_id, year, entitled)
		SELECT e.id, t.id, $3, $4
		FROM hr_employees e
		CROSS JOIN hr_leave_types t
		WHERE e.id = $1 AND e.company_id = $5 AND t.id = $2 AND (t.company_id IS NULL OR t.company_id = $5)
		ON CONFLICT(employee_id, leave_type_id, year) DO UPDATE SET entitled=EXCLUDED.entitled, updated_at=NOW()`,
		employeeID, typeID, year, days, companyID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("hr: employee or leave type not found in company")
	}
	return nil
}
