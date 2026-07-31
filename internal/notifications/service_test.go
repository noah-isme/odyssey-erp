package notifications

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type memoryStore struct {
	items []Notification
}

func (m *memoryStore) Create(_ context.Context, n Notification) (Notification, error) {
	for _, item := range m.items {
		if n.DedupeKey != "" && item.RecipientID == n.RecipientID && item.Type == n.Type && item.DedupeKey == n.DedupeKey {
			return item, nil
		}
	}
	n.ID = int64(len(m.items) + 1)
	m.items = append(m.items, n)
	return n, nil
}
func (m *memoryStore) ListRecent(_ context.Context, uid int64, limit int) ([]Notification, error) {
	return m.filtered(uid, false, limit), nil
}
func (m *memoryStore) ListUnread(_ context.Context, uid int64, limit int) ([]Notification, error) {
	return m.filtered(uid, true, limit), nil
}
func (m *memoryStore) UnreadCount(_ context.Context, uid int64) (int64, error) {
	return int64(len(m.filtered(uid, true, 100))), nil
}
func (m *memoryStore) MarkRead(_ context.Context, uid, id int64, at time.Time) (bool, error) {
	for i := range m.items {
		if m.items[i].ID == id && m.items[i].RecipientID == uid && m.items[i].ReadAt == nil {
			m.items[i].ReadAt = &at
			return true, nil
		}
	}
	return false, nil
}
func (m *memoryStore) MarkAllRead(_ context.Context, uid int64, at time.Time) (int64, error) {
	var count int64
	for i := range m.items {
		if m.items[i].RecipientID == uid && m.items[i].ReadAt == nil {
			m.items[i].ReadAt = &at
			count++
		}
	}
	return count, nil
}
func (m *memoryStore) filtered(uid int64, unread bool, limit int) []Notification {
	result := []Notification{}
	for _, item := range m.items {
		if item.RecipientID == uid && (!unread || item.ReadAt == nil) {
			result = append(result, item)
			if len(result) == limit {
				break
			}
		}
	}
	return result
}

func TestServiceMarkOneAndAllRead(t *testing.T) {
	store := &memoryStore{items: []Notification{{ID: 1, RecipientID: 9}, {ID: 2, RecipientID: 9}, {ID: 3, RecipientID: 10}}}
	service := NewService(store)
	fixed := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixed }

	require.NoError(t, service.MarkRead(context.Background(), 9, 1))
	count, err := service.UnreadCount(context.Background(), 9)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	updated, err := service.MarkAllRead(context.Background(), 9)
	require.NoError(t, err)
	require.Equal(t, int64(1), updated)
	require.ErrorIs(t, service.MarkRead(context.Background(), 9, 999), ErrNotFound)
}

func TestServiceValidatesCreateAndBoundsListLimit(t *testing.T) {
	store := &memoryStore{}
	service := NewService(store)
	_, err := service.Create(context.Background(), Notification{})
	require.ErrorIs(t, err, ErrInvalidNotification)
	created, err := service.Create(context.Background(), Notification{RecipientID: 1, Type: TypeInvoiceIssued, Title: "Issued"})
	require.NoError(t, err)
	require.Equal(t, int64(1), created.ID)
	items, err := service.ListRecent(context.Background(), 1, 0)
	require.NoError(t, err)
	require.Len(t, items, 1)
}

func TestServiceUnreadAndReadAllRespectRecipientIsolation(t *testing.T) {
	store := &memoryStore{items: []Notification{
		{ID: 1, RecipientID: 9, Type: TypeInvoiceIssued},
		{ID: 2, RecipientID: 10, Type: TypeInvoiceIssued},
		{ID: 3, RecipientID: 9, Type: TypePasswordReset, ReadAt: func() *time.Time { ts := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC); return &ts }()},
	}}
	service := NewService(store)

	items, err := service.ListUnread(context.Background(), 9, 50)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, int64(1), items[0].ID)

	count, err := service.UnreadCount(context.Background(), 9)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)

	updated, err := service.MarkAllRead(context.Background(), 9)
	require.NoError(t, err)
	require.Equal(t, int64(1), updated)

	count, err = service.UnreadCount(context.Background(), 9)
	require.NoError(t, err)
	require.Zero(t, count)

	count, err = service.UnreadCount(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}
