package leave

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/approvals"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"strconv"
	"time"
)

type LeaveType struct {
	ID          int64
	Code, Name  string
	DefaultDays float64
}
type Request struct {
	ID, EmployeeID, LeaveTypeID int64
	TypeName                    string
	StartDate, EndDate          time.Time
	Days                        float64
	Reason, Status              string
	ApprovalRequestID           *int64
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
	defer tx.Rollback(ctx)
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
func (s *Service) FinalizeApproval(ctx context.Context, a approvals.Request, status string, actorID int64, note string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
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
func (s *Service) SeedBalance(ctx context.Context, employeeID, typeID int64, year int, days float64) error {
	if days < 0 {
		return fmt.Errorf("hr: invalid balance")
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO hr_leave_balances(employee_id,leave_type_id,year,entitled) VALUES($1,$2,$3,$4) ON CONFLICT(employee_id,leave_type_id,year) DO UPDATE SET entitled=EXCLUDED.entitled,updated_at=NOW()`, employeeID, typeID, year, days)
	return err
}
