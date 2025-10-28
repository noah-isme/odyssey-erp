package accounting

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// AccountType enumerates CoA categories.
type AccountType string

const (
	AccountTypeAsset     AccountType = "ASSET"
	AccountTypeLiability AccountType = "LIABILITY"
	AccountTypeEquity    AccountType = "EQUITY"
	AccountTypeRevenue   AccountType = "REVENUE"
	AccountTypeExpense   AccountType = "EXPENSE"
)

// PeriodStatus enumerates valid period states.
type PeriodStatus string

const (
	PeriodStatusOpen   PeriodStatus = "OPEN"
	PeriodStatusClosed PeriodStatus = "CLOSED"
	PeriodStatusLocked PeriodStatus = "LOCKED"
)

// JournalStatus enumerates journal lifecycle values.
type JournalStatus string

const (
	JournalStatusPosted JournalStatus = "POSTED"
	JournalStatusVoid   JournalStatus = "VOID"
)

// Account models a chart of accounts node.
type Account struct {
	ID        int64
	Code      string
	Name      string
	Type      AccountType
	ParentID  *int64
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

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

// AccountMapping links integration keys to ledger accounts.
type AccountMapping struct {
	Module    string
	Key       string
	AccountID int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// PostingLineInput describes a journal line for posting request.
type PostingLineInput struct {
	AccountID int64
	Debit     float64
	Credit    float64
	CompanyID *int64
	BranchID  *int64
	Warehouse *int64
}

// PostingInput groups fields required to create a journal entry.
type PostingInput struct {
	PeriodID     int64
	Date         time.Time
	SourceModule string
	SourceID     uuid.UUID
	Memo         string
	PostedBy     int64
	Lines        []PostingLineInput
}

// VoidInput wraps parameters for voiding.
type VoidInput struct {
	EntryID int64
	ActorID int64
	Reason  string
}

// ReverseInput wraps parameters for reversal.
type ReverseInput struct {
	EntryID    int64
	ActorID    int64
	Memo       string
	Override   bool
	TargetDate *time.Time
}

var (
	// ErrUnbalanced indicates debit != credit.
	ErrUnbalanced = errors.New("accounting: journal lines must balance")
	// ErrTooFewLines indicates less than two lines.
	ErrTooFewLines = errors.New("accounting: journal requires at least two lines")
	// ErrInvalidPeriod indicates missing or locked period.
	ErrInvalidPeriod = errors.New("accounting: period is not open")
	// ErrSourceAlreadyLinked indicates idempotency conflict.
	ErrSourceAlreadyLinked = errors.New("accounting: source already linked")
	// ErrJournalNotFound indicates missing entry.
	ErrJournalNotFound = errors.New("accounting: journal entry not found")
	// ErrPeriodLocked indicates locked period.
	ErrPeriodLocked = errors.New("accounting: period locked")
	// ErrInvalidStatus indicates action can't proceed.
	ErrInvalidStatus = errors.New("accounting: invalid status transition")
	// ErrDateOutOfRange indicates journal date mismatch.
	ErrDateOutOfRange = errors.New("accounting: date outside period")
	// ErrMappingNotFound indicates account mapping missing.
	ErrMappingNotFound = errors.New("accounting: account mapping not found")
)

// Validate ensures posting input meets minimum criteria.
func (in PostingInput) Validate() error {
	if in.PeriodID == 0 {
		return errors.New("accounting: period required")
	}
	if len(in.Lines) < 2 {
		return ErrTooFewLines
	}
	var debit, credit float64
	for idx, line := range in.Lines {
		if line.AccountID == 0 {
			return fmt.Errorf("accounting: line %d missing account", idx)
		}
		if line.Debit < 0 || line.Credit < 0 {
			return fmt.Errorf("accounting: line %d negative amount", idx)
		}
		if line.Debit > 0 && line.Credit > 0 {
			return fmt.Errorf("accounting: line %d cannot be both debit and credit", idx)
		}
		debit += line.Debit
		credit += line.Credit
	}
	if fmt.Sprintf("%.2f", debit) != fmt.Sprintf("%.2f", credit) {
		return ErrUnbalanced
	}
	if in.SourceModule == "" {
		return errors.New("accounting: source module required")
	}
	if in.SourceID == uuid.Nil {
		return errors.New("accounting: source id required")
	}
	return nil
}
