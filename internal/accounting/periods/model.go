package periods

import "time"

// PeriodStatus enumerates valid period states.
type PeriodStatus string

const (
	PeriodStatusOpen   PeriodStatus = "OPEN"
	PeriodStatusClosed PeriodStatus = "CLOSED"
	PeriodStatusLocked PeriodStatus = "LOCKED"
)

// Period represents a fiscal period window.
type Period struct {
	ID        int64
	Code      string
	StartDate time.Time
	EndDate   time.Time
	Status    PeriodStatus
	ClosedAt  *time.Time
	LockedBy  *int64
	CreatedAt time.Time
	UpdatedAt time.Time
}
