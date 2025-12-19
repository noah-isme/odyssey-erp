package journals

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/shared"
)

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

// Validate ensures posting input meets minimum criteria.
func (in PostingInput) Validate() error {
	if in.PeriodID == 0 {
		return errors.New("accounting: period required")
	}
	if len(in.Lines) < 2 {
		return shared.ErrTooFewLines
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
		return shared.ErrUnbalanced
	}
	if in.SourceModule == "" {
		return errors.New("accounting: source module required")
	}
	if in.SourceID == uuid.Nil {
		return errors.New("accounting: source id required")
	}
	return nil
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
