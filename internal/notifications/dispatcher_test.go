package notifications

import (
	"context"
	"errors"
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

type preferenceMatrixFake struct {
	channels map[string]Channels
	email    string
}

func (p preferenceMatrixFake) Channels(_ context.Context, _ int64, typ string) (Channels, error) {
	if ch, ok := p.channels[typ]; ok {
		return ch, nil
	}
	return Channels{InApp: true, Email: true}, nil
}

func (p preferenceMatrixFake) UserEmail(context.Context, int64) (string, error) { return p.email, nil }

type emailQueueFake struct {
	messages []Email
	failures int
}

func (q *emailQueueFake) EnqueueEmail(_ context.Context, email Email) error {
	q.messages = append(q.messages, email)
	if q.failures > 0 {
		q.failures--
		return errors.New("queue unavailable")
	}
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
	require.NoError(t, dispatcher.Dispatch(context.Background(), PasswordReset(8, "request-1")))
	require.Len(t, store.items, 1)
	require.Empty(t, queue.messages)
}

func TestDispatcherRetryDoesNotDuplicateInAppNotification(t *testing.T) {
	store := &memoryStore{}
	queue := &emailQueueFake{failures: 1}
	dispatcher := NewDispatcher(NewService(store), preferenceFake{channels: Channels{InApp: true, Email: true}, email: "person@example.com"}, queue)
	message := InvoiceIssued(8, 12, "INV-12")

	require.Error(t, dispatcher.Dispatch(context.Background(), message))
	require.NoError(t, dispatcher.Dispatch(context.Background(), message))
	require.Len(t, store.items, 1)
	require.Len(t, queue.messages, 2)
	require.Equal(t, "notification-email-1", queue.messages[0].CorrelationID)
	require.Equal(t, queue.messages[0].CorrelationID, queue.messages[1].CorrelationID)
}

func TestDispatcherHonorsPerEventChannelPreferences(t *testing.T) {
	store := &memoryStore{}
	queue := &emailQueueFake{}
	dispatcher := NewDispatcher(NewService(store), preferenceMatrixFake{
		channels: map[string]Channels{
			TypeInvoiceIssued:   {InApp: false, Email: true},
			TypePasswordReset:   {InApp: true, Email: false},
			TypeReportDelivered: {InApp: true, Email: false},
		},
		email: "person@example.com",
	}, queue)

	require.NoError(t, dispatcher.Dispatch(context.Background(), InvoiceIssued(8, 12, "INV-12")))
	require.Empty(t, store.items)
	require.Len(t, queue.messages, 1)
	require.Equal(t, "person@example.com", queue.messages[0].To)

	require.NoError(t, dispatcher.Dispatch(context.Background(), PasswordReset(8, "request-2")))
	require.Len(t, store.items, 1)
	require.Len(t, queue.messages, 1)
	require.Equal(t, TypePasswordReset, store.items[0].Type)
}
