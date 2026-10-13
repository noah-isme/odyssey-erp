package jobs

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
)

// TaskHRChecklistReminders notifies assignees of overdue HR checklist tasks.
const TaskHRChecklistReminders = "hr:checklist_reminders"

// HRChecklistReminderLimit caps the tasks notified per run.
const HRChecklistReminderLimit = 200

type HRChecklistReminderDispatcher interface {
	DispatchOverdueReminders(context.Context, int) error
}

func HandleHRChecklistReminders(dispatcher HRChecklistReminderDispatcher) asynq.HandlerFunc {
	return func(ctx context.Context, _ *asynq.Task) error {
		if dispatcher == nil {
			return fmt.Errorf("HR checklist reminder dispatcher not configured: %w", asynq.SkipRetry)
		}
		return dispatcher.DispatchOverdueReminders(ctx, HRChecklistReminderLimit)
	}
}
