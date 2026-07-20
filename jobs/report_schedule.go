package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const TypeReportScheduleScan = "reports:scan_schedules"

// HandleReportScheduleScanTask queues email delivery for schedules due at the scan time.
func HandleReportScheduleScanTask(logger *slog.Logger, db *pgxpool.Pool, client *asynq.Client) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, _ *asynq.Task) error {
		now := time.Now().UTC()
		rows, err := db.Query(ctx, `SELECT id, report_type, recipients, department_id, cost_center_id, period_offset_months
			FROM report_schedules WHERE is_active = true AND (
				last_sent_at IS NULL OR (frequency = 'DAILY' AND last_sent_at < $1 - INTERVAL '1 day')
				OR (frequency = 'WEEKLY' AND last_sent_at < $1 - INTERVAL '7 days')
				OR (frequency = 'MONTHLY' AND last_sent_at < $1 - INTERVAL '1 month')) FOR UPDATE SKIP LOCKED`, now)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var departmentID, costCenterID pgtype.Int8
			var reportType string
			var recipients []string
			var periodOffset int
			if err := rows.Scan(&id, &reportType, &recipients, &departmentID, &costCenterID, &periodOffset); err != nil {
				return err
			}
			path := "/accounting/pnl"
			title := "Profit and Loss"
			if reportType == "BUDGET_VS_ACTUAL" {
				path = "/accounting/budget"
				title = "Budget vs Actual"
			}
			period := now.AddDate(0, periodOffset, 0).Format("2006-01")
			url := fmt.Sprintf("%s?period=%s&department_id=%d&cost_center_id=%d", path, period, departmentID.Int64, costCenterID.Int64)
			task, err := NewEmailDeliveryTask(EmailDeliveryPayload{To: recipients, Subject: fmt.Sprintf("%s — %s", title, period), BodyHTML: fmt.Sprintf("<p>Your scheduled %s report is ready for %s.</p><p><a href=\"%s\">Open report</a></p>", title, period, url)})
			if err != nil {
				return err
			}
			if _, err := client.EnqueueContext(ctx, task, asynq.Queue(QueueDefault), asynq.MaxRetry(3)); err != nil {
				return err
			}
			if _, err := db.Exec(ctx, `UPDATE report_schedules SET last_sent_at = $1, updated_at = $1 WHERE id = $2`, now, id); err != nil {
				return err
			}
			if _, err := db.Exec(ctx, `INSERT INTO report_schedule_deliveries (schedule_id, status, detail) VALUES ($1, 'QUEUED', $2)`, id, "Email queued"); err != nil {
				return err
			}
		}
		if err := rows.Err(); err != nil {
			return err
		}
		logger.Info("processed report schedules")
		return nil
	}
}
