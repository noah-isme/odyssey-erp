package schedules

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &repository{pool: pool}
}

func (r *repository) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	wrapper := &txRepo{tx: tx}
	if err := fn(ctx, wrapper); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *repository) List(ctx context.Context, companyID int64) ([]Schedule, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, report_type, recipients, frequency, is_active, last_sent_at, period_offset_months, department_id, cost_center_id FROM report_schedules WHERE company_id=$1 ORDER BY created_at DESC`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []Schedule
	for rows.Next() {
		var s Schedule
		var recipients []string
		var deptID, ccID pgtype.Int8
		if err := rows.Scan(&s.ID, &s.Type, &recipients, &s.Frequency, &s.Active, &s.LastSentAt, &s.PeriodOffset, &deptID, &ccID); err != nil {
			return nil, err
		}
		s.Recipients = recipients
		if deptID.Valid {
			s.DepartmentID = deptID.Int64
		}
		if ccID.Valid {
			s.CostCenterID = ccID.Int64
		}
		schedules = append(schedules, s)
	}
	return schedules, rows.Err()
}

func (r *repository) Create(ctx context.Context, input CreateScheduleInput) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO report_schedules (company_id, report_type, recipients, frequency, department_id, cost_center_id, period_offset_months) VALUES ($1,$2,$3,$4,NULLIF($5,0),NULLIF($6,0),$7)`, input.CompanyID, input.Type, input.Recipients, input.Frequency, input.DepartmentID, input.CostCenterID, input.PeriodOffset)
	return err
}

func (r *repository) Toggle(ctx context.Context, id, companyID int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE report_schedules SET is_active=NOT is_active, updated_at=NOW() WHERE id=$1 AND company_id=$2 AND is_active=true`, id, companyID)
	return err
}

func (r *repository) Retry(ctx context.Context, id, companyID int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE report_schedules SET last_sent_at=NULL, updated_at=NOW() WHERE id=$1 AND company_id=$2 AND is_active=true`, id, companyID)
	return err
}

type txRepo struct {
	tx pgx.Tx
}

func (r *txRepo) Create(ctx context.Context, input CreateScheduleInput) error {
	_, err := r.tx.Exec(ctx, `INSERT INTO report_schedules (company_id, report_type, recipients, frequency, department_id, cost_center_id, period_offset_months) VALUES ($1,$2,$3,$4,NULLIF($5,0),NULLIF($6,0),$7)`, input.CompanyID, input.Type, input.Recipients, input.Frequency, input.DepartmentID, input.CostCenterID, input.PeriodOffset)
	return err
}

func (r *txRepo) Toggle(ctx context.Context, id, companyID int64) error {
	_, err := r.tx.Exec(ctx, `UPDATE report_schedules SET is_active=NOT is_active, updated_at=NOW() WHERE id=$1 AND company_id=$2 AND is_active=true`, id, companyID)
	return err
}

func (r *txRepo) Retry(ctx context.Context, id, companyID int64) error {
	_, err := r.tx.Exec(ctx, `UPDATE report_schedules SET last_sent_at=NULL, updated_at=NOW() WHERE id=$1 AND company_id=$2 AND is_active=true`, id, companyID)
	return err
}