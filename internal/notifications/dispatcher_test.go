package notifications

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type preferenceFake struct {
	channels Channels
	email    string
}

func (p preferenceFake) Channels(context.Context, int64, string) (Channels, error) {
	return p.channels, nil
}
func (p preferenceFake) UserEmail(context.Context, int64) (string, error) { return p.email, nil }

type emailQueueFake struct{ messages []Email }

func (q *emailQueueFake) EnqueueEmail(_ context.Context, email Email) error {
	q.messages = append(q.messages, email)
	return nil
}

func TestDispatcherHonorsChannelPreferences(t *testing.T) {
	store := &memoryStore{}
	queue := &emailQueueFake{}
	dispatcher := NewDispatcher(NewService(store), preferenceFake{channels: Channels{InApp: false, Email: true}, email: "person@example.com"}, queue)
	require.NoError(t, dispatcher.Dispatch(context.Background(), InvoiceIssued(8, 12, "INV-12")))
	require.Empty(t, store.items)
	require.Len(t, queue.messages, 1)
	require.Equal(t, "person@example.com", queue.messages[0].To)

	queue.messages = nil
	dispatcher = NewDispatcher(NewService(store), preferenceFake{channels: Channels{InApp: true, Email: false}}, queue)
	require.NoError(t, dispatcher.Dispatch(context.Background(), PasswordReset(8)))
	require.Len(t, store.items, 1)
	require.Empty(t, queue.messages)
}
