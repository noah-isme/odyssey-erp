package outbox

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type memoryEventStore struct {
	events    []Event
	claimErr  error
	published []int64
	failed    map[int64]string
}

func (s *memoryEventStore) ClaimPending(context.Context, int) ([]Event, error) {
	if s.claimErr != nil {
		return nil, s.claimErr
	}
	return s.events, nil
}

func (s *memoryEventStore) MarkPublished(_ context.Context, id int64) error {
	s.published = append(s.published, id)
	return nil
}

func (s *memoryEventStore) MarkFailed(_ context.Context, id int64, errStr string) error {
	if s.failed == nil {
		s.failed = make(map[int64]string)
	}
	s.failed[id] = errStr
	return nil
}

func TestDispatcherPublishesHandledAndUnhandledEvents(t *testing.T) {
	store := &memoryEventStore{events: []Event{{ID: 1, EventType: "sales.order.created"}, {ID: 2, EventType: "ignored"}}}
	dispatcher := &Dispatcher{store: store, handlers: make(map[string][]Handler)}
	var handled int
	dispatcher.Register("sales.order.created", func(_ context.Context, event Event) error {
		handled += int(event.ID)
		return nil
	})

	if err := dispatcher.ProcessPending(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if handled != 1 || len(store.published) != 2 || store.published[0] != 1 || store.published[1] != 2 {
		t.Fatalf("handled=%d published=%v", handled, store.published)
	}
}

func TestDispatcherRecordsHandlerFailureWithBoundedError(t *testing.T) {
	store := &memoryEventStore{events: []Event{{ID: 3, EventType: "failing"}}}
	dispatcher := &Dispatcher{store: store, handlers: make(map[string][]Handler)}
	dispatcher.Register("failing", func(context.Context, Event) error {
		return errors.New(strings.Repeat("x", 2001))
	})

	if err := dispatcher.ProcessPending(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if len(store.published) != 0 || len(store.failed[3]) != 2000 {
		t.Fatalf("published=%v failedLength=%d", store.published, len(store.failed[3]))
	}
}

func TestDispatcherReturnsClaimErrors(t *testing.T) {
	dispatcher := &Dispatcher{store: &memoryEventStore{claimErr: errors.New("database unavailable")}, handlers: make(map[string][]Handler)}
	if err := dispatcher.ProcessPending(context.Background(), 10); err == nil || err.Error() != "database unavailable" {
		t.Fatalf("error=%v", err)
	}
}
