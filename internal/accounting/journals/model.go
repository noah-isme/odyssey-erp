package journals

import (
	"time"

	"github.com/google/uuid"
)

// JournalStatus enumerates journal lifecycle values.
type JournalStatus string

const (
	JournalStatusPosted JournalStatus = "POSTED"
	JournalStatusVoid   JournalStatus = "VOID"
)

// JournalEntry captures posting metadata.
type JournalEntry struct {
	ID           int64
	Number       int64
	PeriodID     int64
	Date         time.Time
	SourceModule string
	SourceID     uuid.UUID
	Memo         string
	PostedBy     int64
	PostedAt     time.Time
	Status       JournalStatus
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Lines        []JournalLine
}

// JournalLine stores debit or credit amount for an account.
type JournalLine struct {
	ID             int64
	JournalID      int64
	AccountID      int64
	Debit          float64
	Credit         float64
	DimCompanyID   *int64
	DimBranchID    *int64
	DimWarehouseID *int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
