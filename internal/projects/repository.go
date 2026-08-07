package projects

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SQLRepository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *SQLRepository { return &SQLRepository{pool: pool} }

func (r *SQLRepository) GetProjectTask(ctx context.Context, projectID, taskID int64) (Project, Task, error) {
	var p Project
	var t Task
	err := r.pool.QueryRow(ctx, `SELECT p.id,p.company_id,COALESCE(p.manager_id,0),p.code,p.name,p.currency,p.status,t.id,t.project_id,t.code,t.name,t.status FROM projects p JOIN project_tasks t ON t.project_id=p.id WHERE p.id=$1 AND t.id=$2`, projectID, taskID).Scan(&p.ID, &p.CompanyID, &p.ManagerID, &p.Code, &p.Name, &p.Currency, &p.Status, &t.ID, &t.ProjectID, &t.Code, &t.Name, &t.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, Task{}, ErrNotFound
	}
	return p, t, err
}
func (r *SQLRepository) IsProjectMember(ctx context.Context, companyID, projectID, userID int64) (bool, error) {
	var member bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_members m JOIN projects p ON p.id=m.project_id AND p.company_id=$1 WHERE m.project_id=$2 AND m.user_id=$3)`, companyID, projectID, userID).Scan(&member)
	return member, err
}
func (r *SQLRepository) CreateTimesheet(ctx context.Context, s Timesheet) (Timesheet, error) {
	var out Timesheet
	var args []any
	query := `WITH owned AS (SELECT p.id AS project_id,t.id AS task_id FROM projects p JOIN project_tasks t ON t.project_id=p.id WHERE p.id=$2 AND t.id=$3 AND p.company_id=$1) INSERT INTO timesheets(company_id,project_id,task_id,employee_id,work_date,hours,description,billable,billable_rate,status) SELECT $1,owned.project_id,owned.task_id,$4,CURRENT_DATE,$5,$6,$7,$8,'DRAFT' FROM owned RETURNING id,company_id,project_id,task_id,employee_id,work_date::text,hours,COALESCE(description,''),billable,billable_rate,status`
	if s.WorkDate != "" {
		query = `WITH owned AS (SELECT p.id AS project_id,t.id AS task_id FROM projects p JOIN project_tasks t ON t.project_id=p.id WHERE p.id=$2 AND t.id=$3 AND p.company_id=$1) INSERT INTO timesheets(company_id,project_id,task_id,employee_id,work_date,hours,description,billable,billable_rate,status) SELECT $1,owned.project_id,owned.task_id,$4,$5,$6,$7,$8,$9,'DRAFT' FROM owned RETURNING id,company_id,project_id,task_id,employee_id,work_date::text,hours,COALESCE(description,''),billable,billable_rate,status`
		args = []any{s.CompanyID, s.ProjectID, s.TaskID, s.EmployeeID, s.WorkDate, s.Hours, s.Description, s.Billable, s.BillableRate}
	} else {
		args = []any{s.CompanyID, s.ProjectID, s.TaskID, s.EmployeeID, s.Hours, s.Description, s.Billable, s.BillableRate}
	}
	err := r.pool.QueryRow(ctx, query, args...).Scan(&out.ID, &out.CompanyID, &out.ProjectID, &out.TaskID, &out.EmployeeID, &out.WorkDate, &out.Hours, &out.Description, &out.Billable, &out.BillableRate, &out.Status)
	return out, err
}
func (r *SQLRepository) GetTimesheet(ctx context.Context, companyID, id int64) (Timesheet, error) {
	var s Timesheet
	err := r.pool.QueryRow(ctx, `SELECT t.id,t.company_id,t.project_id,t.task_id,t.employee_id,t.work_date::text,t.hours,COALESCE(t.description,''),t.billable,t.billable_rate,p.currency,c.base_currency,COALESCE(t.base_amount,0),COALESCE(t.fx_rate,0),COALESCE(t.fx_rate_source,''),COALESCE(t.fx_rate_locked_at::text,'') FROM timesheets t JOIN projects p ON p.id=t.project_id JOIN companies c ON c.id=t.company_id WHERE t.company_id=$1 AND t.id=$2`, companyID, id).Scan(&s.ID, &s.CompanyID, &s.ProjectID, &s.TaskID, &s.EmployeeID, &s.WorkDate, &s.Hours, &s.Description, &s.Billable, &s.BillableRate, &s.Currency, &s.BaseCurrency, &s.BaseAmount, &s.FXRate, &s.FXRateSource, &s.FXRateLockedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Timesheet{}, ErrNotFound
	}
	return s, err
}
func (r *SQLRepository) UpdateTimesheet(ctx context.Context, s Timesheet) error {
	var res pgconn.CommandTag
	var err error
	if s.Status == "APPROVED" {
		res, err = r.pool.Exec(ctx, `WITH locked AS (SELECT t.id,t.company_id,t.hours,t.billable_rate,p.currency,c.base_currency,fx.rate FROM timesheets t JOIN projects p ON p.id=t.project_id JOIN companies c ON c.id=t.company_id LEFT JOIN LATERAL (SELECT rate FROM fx_daily_rates WHERE base_currency=c.base_currency AND quote_currency=p.currency AND rate_date<=t.work_date ORDER BY rate_date DESC LIMIT 1) fx ON TRUE WHERE t.company_id=$2 AND t.id=$3 AND (p.currency=c.base_currency OR fx.rate IS NOT NULL)) UPDATE timesheets t SET status=$1,base_currency=locked.base_currency,fx_rate=CASE WHEN locked.currency=locked.base_currency THEN 1 ELSE locked.rate END,fx_rate_source=CASE WHEN locked.currency=locked.base_currency THEN 'INTERNAL' ELSE 'DAILY_RATE' END,base_amount=ROUND(locked.hours*locked.billable_rate*CASE WHEN locked.currency=locked.base_currency THEN 1 ELSE locked.rate END,6),fx_rate_locked_at=NOW() FROM locked WHERE t.id=locked.id`, s.Status, s.CompanyID, s.ID)
	} else {
		res, err = r.pool.Exec(ctx, `UPDATE timesheets SET status=$1 WHERE company_id=$2 AND id=$3`, s.Status, s.CompanyID, s.ID)
	}
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// =============================================================================
// Advanced Project Features (Milestones, Resource Allocation, Expenses)
// =============================================================================

func (r *SQLRepository) CreateMilestone(ctx context.Context, milestone ProjectMilestone) (ProjectMilestone, error) {
	err := r.pool.QueryRow(ctx, `INSERT INTO project_milestones (project_id, name, description, due_date, status) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		milestone.ProjectID, milestone.Name, milestone.Description, milestone.DueDate, milestone.Status).Scan(&milestone.ID)
	return milestone, err
}

func (r *SQLRepository) AllocateResource(ctx context.Context, allocation ResourceAllocation) (ResourceAllocation, error) {
	err := r.pool.QueryRow(ctx, `INSERT INTO project_resource_allocations (project_id, employee_id, allocated_hours, start_date, end_date) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		allocation.ProjectID, allocation.EmployeeID, allocation.AllocatedHours, allocation.StartDate, allocation.EndDate).Scan(&allocation.ID)
	return allocation, err
}

func (r *SQLRepository) SubmitExpense(ctx context.Context, expense ProjectExpense) (ProjectExpense, error) {
	err := r.pool.QueryRow(ctx, `INSERT INTO project_expenses (project_id, employee_id, amount, currency, description, receipt_url, status) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		expense.ProjectID, expense.EmployeeID, expense.Amount, expense.Currency, expense.Description, expense.ReceiptURL, expense.Status).Scan(&expense.ID)
	return expense, err
}

var _ AdvancedRepository = (*SQLRepository)(nil)
