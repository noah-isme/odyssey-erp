package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Task names
const (
	TypeOverdueInvoicesScan = "invoices:scan_overdue"
)

// NewOverdueInvoicesScanTask creates a task to scan for overdue invoices.
func NewOverdueInvoicesScanTask() (*asynq.Task, error) {
	return asynq.NewTask(TypeOverdueInvoicesScan, nil), nil
}

// HandleOverdueInvoicesScanTask checks for overdue AR invoices and queues email deliveries.
func HandleOverdueInvoicesScanTask(logger *slog.Logger, db *pgxpool.Pool, client *asynq.Client) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		logger.Info("starting overdue invoices scan")

		// We do a direct query for MVP instead of importing AR service to avoid circular dependencies
		// Only fetch posted invoices that are overdue and unpaid
		query := `
			SELECT invoice_number, total, due_date
			FROM ar_invoices 
			WHERE status = 'POSTED' AND due_date < $1 AND amount_due > 0
		`

		rows, err := db.Query(ctx, query, time.Now())
		if err != nil {
			logger.Error("failed to query overdue invoices", slog.Any("error", err))
			return fmt.Errorf("db.Query failed: %w", err)
		}
		defer rows.Close()

		count := 0
		for rows.Next() {
			var invoiceNumber string
			var total float64
			var dueDate time.Time

			if err := rows.Scan(&invoiceNumber, &total, &dueDate); err != nil {
				logger.Error("failed to scan invoice row", slog.Any("error", err))
				continue
			}

			// In a real application, we would check a flag to see if the notification
			// has already been sent to avoid spamming the customer every day,
			// or we would use a last_reminded_at timestamp.

			customerEmail := "customer@example.com" // Dummy for MVP

			payload := fmt.Sprintf(`{"to": ["%s"], "subject": "OVERDUE: Invoice %s", "body_html": "<p>Your invoice %s for amount %.2f is overdue since %s. Please arrange payment.</p>"}`,
				customerEmail, invoiceNumber, invoiceNumber, total, dueDate.Format("2006-01-02"))

			task := asynq.NewTask(TypeEmailDelivery, []byte(payload))
			if _, err := client.EnqueueContext(ctx, task); err != nil {
				logger.Error("failed to enqueue email task for overdue invoice",
					slog.String("invoice", invoiceNumber),
					slog.Any("error", err))
			} else {
				count++
			}
		}

		logger.Info("overdue invoices scan completed", slog.Int("emails_queued", count))
		return nil
	}
}
