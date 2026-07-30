package crm

import (
	"context"
	"fmt"

	"github.com/odyssey-erp/odyssey-erp/internal/notifications"
)

type NotificationAdapter struct{ dispatcher *notifications.Dispatcher }

func NewNotificationAdapter(dispatcher *notifications.Dispatcher) *NotificationAdapter {
	return &NotificationAdapter{dispatcher: dispatcher}
}
func (n *NotificationAdapter) Reminder(ctx context.Context, a Activity, escalated bool) error {
	if n == nil || n.dispatcher == nil {
		return nil
	}
	kind := notifications.TypeCRMActivityReminder
	title := "CRM activity reminder"
	key := fmt.Sprintf("activity:%d:reminder", a.ID)
	if escalated {
		kind = notifications.TypeCRMActivityEscalated
		title = "Overdue CRM activity"
		key = fmt.Sprintf("activity:%d:escalated", a.ID)
	}
	recipientID := a.OwnerID
	if escalated && a.EscalationRecipientID > 0 {
		recipientID = a.EscalationRecipientID
	}
	return n.dispatcher.Dispatch(ctx, notifications.Message{RecipientID: recipientID, DedupeKey: key, Type: kind, Title: title, Body: a.Subject, URL: "/crm"})
}
func (n *NotificationAdapter) Reassigned(ctx context.Context, userID int64, entity string, id int64) error {
	if n == nil || n.dispatcher == nil {
		return nil
	}
	return n.dispatcher.Dispatch(ctx, notifications.Message{RecipientID: userID, DedupeKey: fmt.Sprintf("%s:%d:owner:%d", entity, id, userID), Type: notifications.TypeCRMOwnerReassigned, Title: "CRM ownership assigned", Body: fmt.Sprintf("%s #%d was assigned to you.", entity, id), URL: "/crm"})
}
