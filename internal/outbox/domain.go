package outbox

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Event represents a durable message emitted by a module.
type Event struct {
	ID               int64
	CompanyID        int64
	CorrelationID    uuid.UUID
	CausationID      *uuid.UUID
	EventType        string
	AggregateType    string
	AggregateID      int64
	AggregateVersion *int32
	Payload          json.RawMessage
	IdempotencyKey   *string
	CreatedAt        time.Time
	PublishedAt      *time.Time
	PublishAttempts  int32
	LastError        *string
}

// PublishRequest contains the data needed to append an event to the outbox.
type PublishRequest struct {
	CompanyID        int64
	CorrelationID    uuid.UUID
	CausationID      *uuid.UUID
	EventType        string
	AggregateType    string
	AggregateID      int64
	AggregateVersion *int32
	Payload          any
	IdempotencyKey   *string
}

// Validate ensures a publish request is structurally sound.
func (r PublishRequest) Validate() error {
	if r.CompanyID <= 0 {
		return errors.New("outbox: company id required")
	}
	if r.CorrelationID == uuid.Nil {
		return errors.New("outbox: correlation id required")
	}
	if r.EventType == "" {
		return errors.New("outbox: event type required")
	}
	if r.AggregateType == "" {
		return errors.New("outbox: aggregate type required")
	}
	if r.AggregateID <= 0 {
		return errors.New("outbox: aggregate id required")
	}
	if r.Payload == nil {
		return errors.New("outbox: payload required")
	}
	return nil
}
