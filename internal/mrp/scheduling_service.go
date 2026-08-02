package mrp

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type SchedulingService struct{ pool *pgxpool.Pool }

func NewSchedulingService(pool *pgxpool.Pool) *SchedulingService {
	return &SchedulingService{pool: pool}
}
func (s *SchedulingService) Run(ctx context.Context, companyID, actorID int64, asOf time.Time) ([]ScheduleException, error) {
	if s == nil || s.pool == nil || companyID <= 0 || actorID <= 0 {
		return nil, ErrInvalidSchedule
	}
	asOf = dayStart(asOf)
	if asOf.IsZero() {
		asOf = dayStart(time.Now())
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	caps, err := loadCapacity(ctx, tx, companyID, asOf, 60)
	if err != nil {
		return nil, err
	}
	ops, err := loadSchedulableOperations(ctx, tx, companyID)
	if err != nil {
		return nil, err
	}
	scheduled, issues, err := ScheduleFinite(asOf, caps, ops)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM mrp_schedule_exceptions WHERE company_id=$1 AND resolved_at IS NULL`, companyID); err != nil {
		return nil, err
	}
	for n, op := range scheduled {
		if op.Manual || op.Start.IsZero() {
			continue
		}
		if _, err = tx.Exec(ctx, `UPDATE mrp_work_order_operations SET scheduled_start_at=$1,scheduled_end_at=$2,schedule_sequence=$3,scheduled_by=$4,updated_at=NOW() WHERE id=$5 AND company_id=$6`, op.Start, op.End, n+1, actorID, op.ID, companyID); err != nil {
			return nil, err
		}
	}
	for _, issue := range issues {
		if _, err = tx.Exec(ctx, `INSERT INTO mrp_schedule_exceptions(company_id,operation_id,exception_type,detail) VALUES($1,$2,$3,$4)`, companyID, issue.OperationID, issue.Type, issue.Detail); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	for _, issue := range issues {
		if err := NewExceptionService(s.pool).Upsert(ctx, Exception{CompanyID: companyID, Fingerprint: fmt.Sprintf("schedule:%s:%d", issue.Type, issue.OperationID), Type: map[string]string{"LATE": "LATE_WORK_ORDER", "MISSING_CAPACITY": "MISSING_CAPACITY", "DEPENDENCY": "MISSING_CAPACITY"}[issue.Type], Severity: "HIGH", OperationID: &issue.OperationID, Explanation: []byte(fmt.Sprintf(`{"detail":%q}`, issue.Detail))}); err != nil {
			return nil, err
		}
	}
	return issues, nil
}
func (s *SchedulingService) Reschedule(ctx context.Context, companyID, actorID, operationID int64, start, end time.Time) error {
	if s == nil || s.pool == nil || companyID <= 0 || actorID <= 0 || operationID <= 0 || start.IsZero() || !end.After(start) {
		return ErrInvalidSchedule
	}
	res, err := s.pool.Exec(ctx, `UPDATE mrp_work_order_operations SET scheduled_start_at=$1,scheduled_end_at=$2,schedule_manual=TRUE,scheduled_by=$3,updated_at=NOW() WHERE id=$4 AND company_id=$5 AND status NOT IN ('COMPLETED')`, start, end, actorID, operationID, companyID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
func loadCapacity(ctx context.Context, tx pgx.Tx, companyID int64, start time.Time, days int) ([]CapacityDay, error) {
	var out []CapacityDay
	for i := 0; i < days; i++ {
		d := start.AddDate(0, 0, i)
		rows, err := tx.Query(ctx, `SELECT shift.work_center_id,COALESCE(exception.capacity_hours, SUM(shift.capacity_hours))::float8 FROM mrp_work_center_shifts shift LEFT JOIN mrp_work_center_calendar_exceptions exception ON exception.work_center_id=shift.work_center_id AND exception.exception_date=$2 WHERE shift.company_id=$1 AND shift.active AND shift.weekday=$3 GROUP BY shift.work_center_id,exception.capacity_hours`, companyID, d, int(d.Weekday()))
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var wc int64
			var hrs float64
			if err := rows.Scan(&wc, &hrs); err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, CapacityDay{wc, d, hrs})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return out, nil
}
func loadSchedulableOperations(ctx context.Context, tx pgx.Tx, companyID int64) ([]SchedulableOperation, error) {
	rows, err := tx.Query(ctx, `SELECT op.id,op.work_center_id,(op.planned_setup_minutes+op.planned_run_minutes)::float8,COALESCE(wo.planned_due_date::timestamptz,NOW()),op.schedule_manual,op.scheduled_start_at,op.scheduled_end_at,COALESCE(array_agg(pre.predecessor_operation_id) FILTER (WHERE pre.predecessor_operation_id IS NOT NULL),'{}') FROM mrp_work_order_operations op JOIN mrp_work_orders wo ON wo.id=op.work_order_id LEFT JOIN mrp_operation_dependencies pre ON pre.operation_id=op.id WHERE op.company_id=$1 AND op.status IN ('PENDING','READY','IN_PROGRESS') GROUP BY op.id,wo.planned_due_date ORDER BY wo.planned_due_date NULLS LAST,op.sequence`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SchedulableOperation
	for rows.Next() {
		var op SchedulableOperation
		var mins float64
		var predecessors []int64
		if err := rows.Scan(&op.ID, &op.WorkCenterID, &mins, &op.Due, &op.Manual, &op.Start, &op.End, &predecessors); err != nil {
			return nil, err
		}
		op.DurationHours = mins / 60
		if op.DurationHours <= 0 {
			op.DurationHours = .01
		}
		op.Predecessors = predecessors
		out = append(out, op)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
