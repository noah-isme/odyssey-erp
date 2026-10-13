package checklists

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/notifications"
)

// NotificationTypeTaskOverdue is the notification type sent to the assignee
// of an overdue checklist task.
const NotificationTypeTaskOverdue = "hr_checklist_task_overdue"

// DefaultReminderLimit is the per-run cap used by the reminder job.
const DefaultReminderLimit = 200

const overdueTasksQuery = `SELECT k.id, k.checklist_id, k.title, k.assignee_user_id, k.due_date, e.name
FROM hr_employee_checklist_tasks k
JOIN hr_employee_checklists c ON c.id = k.checklist_id AND c.company_id = k.company_id
JOIN hr_employees e ON e.id = c.employee_id AND e.company_id = c.company_id
JOIN users u ON u.id = k.assignee_user_id
WHERE k.status = 'OPEN'
  AND c.status = 'OPEN'
  AND k.assignee_user_id IS NOT NULL
  AND u.is_active = TRUE
  AND k.due_date < $1
ORDER BY k.due_date, k.id
LIMIT $2`

// overdueDedupeKey is unique per task per calendar day, so concurrent worker
// replicas and reruns produce one notification per task per day.
func overdueDedupeKey(taskID int64, day time.Time) string {
	return fmt.Sprintf("hr-checklist-overdue-%d-%s", taskID, day.Format("2006-01-02"))
}

// DispatchOverdueReminders notifies the assignee of up to limit OPEN tasks
// (on OPEN checklists) whose due date is before today, oldest first. Delivery
// failures do not stop the run; they are joined and returned so the job is
// retried (dedupe keys make the retry safe).
func (s *Service) DispatchOverdueReminders(ctx context.Context, limit int) error {
	if s.notifier == nil {
		return errors.New("hr: checklist notifier not configured")
	}
	if limit <= 0 {
		limit = DefaultReminderLimit
	}
	today := s.today()
	rows, err := s.pool.Query(ctx, overdueTasksQuery, today, limit)
	if err != nil {
		return fmt.Errorf("list overdue checklist tasks: %w", err)
	}
	type overdue struct {
		taskID, checklistID, assignee int64
		title, employee               string
		due                           time.Time
	}
	var list []overdue
	for rows.Next() {
		var o overdue
		if err := rows.Scan(&o.taskID, &o.checklistID, &o.title, &o.assignee, &o.due, &o.employee); err != nil {
			rows.Close()
			return fmt.Errorf("scan overdue checklist task: %w", err)
		}
		list = append(list, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("list overdue checklist tasks: %w", err)
	}
	var errs []error
	for _, o := range list {
		if err := s.notifier.Dispatch(ctx, notifications.Message{
			RecipientID: o.assignee,
			DedupeKey:   overdueDedupeKey(o.taskID, today),
			Type:        NotificationTypeTaskOverdue,
			Title:       "Checklist task overdue",
			Body:        fmt.Sprintf("%q for %s was due on %s.", o.title, o.employee, o.due.Format("2006-01-02")),
			URL:         fmt.Sprintf("/hr/checklists/%d", o.checklistID),
		}); err != nil {
			errs = append(errs, fmt.Errorf("task %d: %w", o.taskID, err))
		}
	}
	return errors.Join(errs...)
}
