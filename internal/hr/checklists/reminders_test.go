package checklists

import (
	"context"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

var reminderCls = []string{"id", "checklist_id", "title", "assignee_user_id", "due_date", "name"}

func TestDispatchOverdueRemindersDedupeKeyAndURL(t *testing.T) {
	db, svc, _, notifier, _ := newService(t)
	due := today.AddDate(0, 0, -3)
	db.ExpectQuery(q(overdueTasksQuery)).WithArgs(today, 200).
		WillReturnRows(pgxmock.NewRows(reminderCls).
			AddRow(int64(5), checklistID, "Return laptop", int64(300), due, "Rina").
			AddRow(int64(6), checklistID, "Close badge", int64(301), due.AddDate(0, 0, 1), "Rina"))
	require.NoError(t, svc.DispatchOverdueReminders(context.Background(), 200))
	require.Len(t, notifier.msgs, 2)
	require.Equal(t, "hr-checklist-overdue-5-2026-10-12", notifier.msgs[0].DedupeKey)
	require.Equal(t, "hr-checklist-overdue-6-2026-10-12", notifier.msgs[1].DedupeKey)
	require.Equal(t, int64(300), notifier.msgs[0].RecipientID)
	require.Equal(t, NotificationTypeTaskOverdue, notifier.msgs[0].Type)
	require.Equal(t, "/hr/checklists/31", notifier.msgs[0].URL)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestOverdueDedupeKeyChangesDaily(t *testing.T) {
	d1 := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	require.NotEqual(t, overdueDedupeKey(5, d1), overdueDedupeKey(5, d1.AddDate(0, 0, 1)))
	require.Equal(t, overdueDedupeKey(5, d1), overdueDedupeKey(5, d1.Add(15*time.Hour)))
}

func TestDispatchOverdueRemindersAppliesCap(t *testing.T) {
	db, svc, _, notifier, _ := newService(t)
	// Non-positive limit falls back to the default cap passed to the query.
	db.ExpectQuery(q(overdueTasksQuery)).WithArgs(today, DefaultReminderLimit).
		WillReturnRows(pgxmock.NewRows(reminderCls))
	require.NoError(t, svc.DispatchOverdueReminders(context.Background(), 0))
	require.Empty(t, notifier.msgs)

	db.ExpectQuery(q(overdueTasksQuery)).WithArgs(today, 1).
		WillReturnRows(pgxmock.NewRows(reminderCls).AddRow(int64(5), checklistID, "T", int64(300), today.AddDate(0, 0, -1), "Rina"))
	require.NoError(t, svc.DispatchOverdueReminders(context.Background(), 1))
	require.Len(t, notifier.msgs, 1)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestDispatchOverdueRemindersRequiresNotifier(t *testing.T) {
	_, svc, _, _, _ := newService(t)
	svc.notifier = nil
	require.Error(t, svc.DispatchOverdueReminders(context.Background(), 10))
}
