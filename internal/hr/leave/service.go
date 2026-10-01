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
	UserID, LeaveTypeID int64
	StartDate, EndDate  time.Time
	Reason              string
}

type Audit interface {
	Record(context.Context, shared.AuditLog) error
}

type Service struct {
	pool      leaveDB
	approvals *approvals.Service
	audit     Audit
}

type leaveDB interface {
	Begin(context.Context) (pgx.Tx, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func NewService(pool *pgxpool.Pool, a *approvals.Service, audit Audit) *Service {
	return &Service{pool: pool, approvals: a, audit: audit}
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
	rows, err := s.pool.Query(ctx, `SELECT r.id,r.employee_id,r.leave_type_id,t.name,r.start_date,r.end_date,r.days,r.reason,r.status,r.approval_request_id FROM hr_leave_requests r JOIN hr_employees e ON e.id=r.employee_id JOIN hr_leave_types t ON t.id=r.leave_type_id WHERE e.user_id=$1 ORDER BY r.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Request
	for rows.Next() {
		var x Request
		if err := rows.Scan(&x.ID, &x.EmployeeID, &x.LeaveTypeID, &x.TypeName, &x.StartDate, &x.EndDate, &x.Days, &x.Reason, &x.Status, &x.ApprovalRequestID); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Service) GetOwnBalances(ctx context.Context, userID int64, year int) ([]Balance, error) {
	rows, err := s.pool.Query(ctx, `SELECT b.leave_type_id, t.code, t.name, b.year, b.entitled, b.used, b.pending, GREATEST(0, b.entitled - b.used - b.pending) FROM hr_leave_balances b JOIN hr_leave_types t ON t.id=b.leave_type_id JOIN hr_employees e ON e.id=b.employee_id WHERE e.user_id=$1 AND b.year=$2 ORDER BY t.name`, userID, year)
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
	query := `SELECT r.id, r.employee_id, r.leave_type_id, t.name, r.start_date, r.end_date, r.days, r.reason, r.status, r.approval_request_id, e.name, e.employee_number, COALESCE(d.name,''), r.created_at FROM hr_leave_requests r JOIN hr_employees e ON e.id=r.employee_id LEFT JOIN hr_departments d ON d.id=e.department_id JOIN hr_leave_types t ON t.id=r.leave_type_id WHERE ($1=0 OR e.company_id=$1)`
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
		if err := rows.Scan(&r.ID, &r.EmployeeID, &r.LeaveTypeID, &r.TypeName, &r.StartDate, &r.EndDate, &r.Days, &r.Reason, &r.Status, &r.ApprovalRequestID, &r.EmployeeName, &r.EmployeeNumber, &r.Department, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Service) ListAllBalances(ctx context.Context, companyID int64, year int) ([]EmployeeBalanceSummary, error) {
	rows, err := s.pool.Query(ctx, `SELECT b.employee_id, e.employee_number, e.name, COALESCE(d.name,''), b.leave_type_id, t.name, b.year, b.entitled, b.used, b.pending, GREATEST(0, b.entitled - b.used - b.pending) FROM hr_leave_balances b JOIN hr_employees e ON e.id=b.employee_id LEFT JOIN hr_departments d ON d.id=e.department_id JOIN hr_leave_types t ON t.id=b.leave_type_id WHERE ($1=0 OR e.company_id=$1) AND b.year=$2 ORDER BY e.name, t.name`, companyID, year)
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
	rows, err := s.pool.Query(ctx, `SELECT e.id, e.employee_number, e.name, COALESCE(d.name,'') FROM hr_employees e LEFT JOIN hr_departments d ON d.id=e.department_id WHERE ($1=0 OR e.company_id=$1) AND e.status='ACTIVE' ORDER BY e.name`, companyID)
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
	days := in.EndDate.Sub(in.StartDate).Hours()/24 + 1
	var employeeID, companyID int64
	var managerUserID *int64
	err := s.pool.QueryRow(ctx, `SELECT e.id,e.company_id,m.user_id FROM hr_employees e LEFT JOIN hr_employees m ON m.id=e.manager_id WHERE e.user_id=$1 AND e.status='ACTIVE'`, in.UserID).Scan(&employeeID, &companyID, &managerUserID)
	if err != nil {
		return Request{}, err
	}
	if managerUserID == nil {
		return Request{}, errors.New("hr: employee manager must have a user account")
	}
	year := in.StartDate.Year()
	var available float64
	err = s.pool.QueryRow(ctx, `SELECT entitled-used-pending FROM hr_leave_balances WHERE employee_id=$1 AND leave_type_id=$2 AND year=$3`, employeeID, in.LeaveTypeID, year).Scan(&available)
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, errors.New("hr: leave balance not configured")
	}
	if err != nil {
		return Request{}, err
	}
	if available < days {
		return Request{}, errors.New("hr: insufficient leave balance")
	}
	var req Request
	err = s.pool.QueryRow(ctx, `INSERT INTO hr_leave_requests(employee_id,leave_type_id,start_date,end_date,days,reason,status) VALUES($1,$2,$3,$4,$5,$6,'DRAFT') RETURNING id,employee_id,leave_type_id,start_date,end_date,days,reason,status`, employeeID, in.LeaveTypeID, in.StartDate, in.EndDate, days, in.Reason).Scan(&req.ID, &req.EmployeeID, &req.LeaveTypeID, &req.StartDate, &req.EndDate, &req.Days, &req.Reason, &req.Status)
	if err != nil {
		return Request{}, err
	}
	approvalReq, err := s.approvals.Submit(ctx, approvals.Submission{Module: "LEAVE", DocumentID: req.ID, RequesterID: in.UserID, CompanyID: &companyID, Amount: days, ManagerID: *managerUserID})
	if err != nil {
		return Request{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Request{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `UPDATE hr_leave_requests SET status='PENDING',approval_request_id=$2,updated_at=NOW() WHERE id=$1`, req.ID, approvalReq.ID)
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE hr_leave_balances SET pending=pending+$4,updated_at=NOW() WHERE employee_id=$1 AND leave_type_id=$2 AND year=$3`, employeeID, in.LeaveTypeID, year, days)
	}
	if err != nil {
		return Request{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Request{}, err
	}
	req.Status = "PENDING"
	req.ApprovalRequestID = &approvalReq.ID
	s.record(ctx, in.UserID, "LEAVE_REQUEST", req.ID, map[string]any{"days": days})
	return req, nil
}

func (s *Service) Cancel(ctx context.Context, requestID, userID int64) error {
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
		WHERE r.id=$1 AND e.user_id=$2
		FOR UPDATE
	`, requestID, userID).Scan(&employeeID, &typeID, &days, &status, &start, &approvalReqID)
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
	var employeeID, typeID int64
	var start time.Time
	var days float64
	var current string
	err = tx.QueryRow(ctx, `SELECT employee_id,leave_type_id,start_date,days,status FROM hr_leave_requests WHERE id=$1 FOR UPDATE`, a.DocumentID).Scan(&employeeID, &typeID, &start, &days, &current)
	if err != nil {
		return err
	}
	if current != "PENDING" {
		return errors.New("hr: leave request is not pending")
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
