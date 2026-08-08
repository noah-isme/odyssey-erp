package approvals

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/notifications"
)

func TestNotificationAdapterDispatchesApprovalEvents(t *testing.T) {
	store := &notificationsTestStore{memoryStore: &notificationsMemoryStore{}}
	dispatcher := notifications.NewDispatcher(notifications.NewService(store.memoryStore), notificationsPreferenceFake{channels: notifications.Channels{InApp: true, Email: false}}, nil, nil)
	adapter := NewNotificationAdapter(dispatcher)
	req := Request{ID: 12, CurrentStep: 3, Module: "PO", DocumentID: 99}

	require.NoError(t, adapter.Assigned(context.Background(), 7, req))
	require.NoError(t, adapter.Escalated(context.Background(), 7, req))
	require.NoError(t, adapter.Completed(context.Background(), 7, req, StatusApproved))
	require.NoError(t, adapter.Completed(context.Background(), 7, req, StatusRejected))

	require.Len(t, store.memoryStore.items, 4)
	require.Equal(t, notifications.TypeApprovalAssigned, store.memoryStore.items[0].Type)
	require.Equal(t, "request:12:step:3:user:7", store.memoryStore.items[0].DedupeKey)
	require.Equal(t, notifications.TypeApprovalEscalated, store.memoryStore.items[1].Type)
	require.Equal(t, notifications.TypeApprovalApproved, store.memoryStore.items[2].Type)
	require.Equal(t, notifications.TypeApprovalRejected, store.memoryStore.items[3].Type)
}

type notificationsMemoryStore struct {
	items []notifications.Notification
}

func (m *notificationsMemoryStore) Create(_ context.Context, n notifications.Notification) (notifications.Notification, error) {
	n.ID = int64(len(m.items) + 1)
	m.items = append(m.items, n)
	return n, nil
}

func (m *notificationsMemoryStore) ListRecent(context.Context, int64, int) ([]notifications.Notification, error) {
	return nil, nil
}

func (m *notificationsMemoryStore) ListUnread(context.Context, int64, int) ([]notifications.Notification, error) {
	return nil, nil
}

func (m *notificationsMemoryStore) UnreadCount(context.Context, int64) (int64, error) {
	return 0, nil
}

func (m *notificationsMemoryStore) MarkRead(context.Context, int64, int64, time.Time) (bool, error) {
	return false, nil
}

func (m *notificationsMemoryStore) MarkAllRead(context.Context, int64, time.Time) (int64, error) {
	return 0, nil
}

type notificationsPreferenceFake struct {
	channels notifications.Channels
}

func (p notificationsPreferenceFake) Channels(context.Context, int64, string) (notifications.Channels, error) {
	return p.channels, nil
}

func (notificationsPreferenceFake) UserEmail(context.Context, int64) (string, error) {
	return "", nil
}

func (notificationsPreferenceFake) UserPhone(context.Context, int64) (string, error) {
	return "", nil
}

type notificationsTestStore struct {
	memoryStore *notificationsMemoryStore
}

func (s *notificationsTestStore) Create(ctx context.Context, n notifications.Notification) (notifications.Notification, error) {
	return s.memoryStore.Create(ctx, n)
}

func (s *notificationsTestStore) ListRecent(ctx context.Context, recipientID int64, limit int) ([]notifications.Notification, error) {
	return s.memoryStore.ListRecent(ctx, recipientID, limit)
}

func (s *notificationsTestStore) ListUnread(ctx context.Context, recipientID int64, limit int) ([]notifications.Notification, error) {
	return s.memoryStore.ListUnread(ctx, recipientID, limit)
}

func (s *notificationsTestStore) UnreadCount(ctx context.Context, recipientID int64) (int64, error) {
	return s.memoryStore.UnreadCount(ctx, recipientID)
}

func (s *notificationsTestStore) MarkRead(ctx context.Context, recipientID, id int64, at time.Time) (bool, error) {
	return s.memoryStore.MarkRead(ctx, recipientID, id, at)
}

func (s *notificationsTestStore) MarkAllRead(ctx context.Context, recipientID int64, at time.Time) (int64, error) {
	return s.memoryStore.MarkAllRead(ctx, recipientID, at)
}
