package approvals

import (
	"context"
	"fmt"

	"github.com/odyssey-erp/odyssey-erp/internal/notifications"
)

type NotificationAdapter struct{ dispatcher *notifications.Dispatcher }

func NewNotificationAdapter(dispatcher *notifications.Dispatcher) *NotificationAdapter {
	return &NotificationAdapter{dispatcher: dispatcher}
}
func (n *NotificationAdapter) Assigned(ctx context.Context, userID int64, r Request) error {
	return n.dispatcher.Dispatch(ctx, notifications.Message{RecipientID: userID, Type: notifications.TypeApprovalAssigned, Title: "Approval assigned", Body: fmt.Sprintf("%s #%d is waiting for your decision.", r.Module, r.DocumentID), URL: "/approvals"})
}
func (n *NotificationAdapter) Escalated(ctx context.Context, userID int64, r Request) error {
	return n.dispatcher.Dispatch(ctx, notifications.Message{RecipientID: userID, Type: notifications.TypeApprovalEscalated, Title: "Approval overdue", Body: fmt.Sprintf("%s #%d approval is overdue.", r.Module, r.DocumentID), URL: "/approvals"})
}
func (n *NotificationAdapter) Completed(ctx context.Context, userID int64, r Request, status string) error {
	typ, title := notifications.TypeApprovalApproved, "Approval approved"
	if status == StatusRejected {
		typ, title = notifications.TypeApprovalRejected, "Approval rejected"
	}
	return n.dispatcher.Dispatch(ctx, notifications.Message{RecipientID: userID, Type: typ, Title: title, Body: fmt.Sprintf("%s #%d was %s.", r.Module, r.DocumentID, status), URL: "/approvals"})
}
