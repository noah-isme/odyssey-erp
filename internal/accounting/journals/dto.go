package journals

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/fx"
)

// PostingLineInput describes a journal line for posting request.
type PostingLineInput struct {
	AccountID int64
	Debit     float64
	Credit    float64
	// DebitDecimal/CreditDecimal are the exact NUMERIC boundary for new
	// accounting flows. Debit/Credit remain legacy UI/API compatibility fields.
	DebitDecimal  fx.Decimal
	CreditDecimal fx.Decimal
	CompanyID     *int64
	BranchID      *int64
	Warehouse     *int64
	DepartmentID  *int64
	CostCenterID  *int64
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
	var debit, credit fx.Decimal
	for idx, line := range in.Lines {
		if line.AccountID == 0 {
			return fmt.Errorf("accounting: line %d missing account", idx)
		}
		lineDebit, lineCredit := line.DebitDecimal, line.CreditDecimal
		if lineDebit.IsZero() {
			lineDebit = fx.MustDecimal(fmt.Sprintf("%.2f", line.Debit))
		}
		if lineCredit.IsZero() {
			lineCredit = fx.MustDecimal(fmt.Sprintf("%.2f", line.Credit))
		}
		if lineDebit.Cmp(fx.MustDecimal("0")) < 0 || lineCredit.Cmp(fx.MustDecimal("0")) < 0 {
			return fmt.Errorf("accounting: line %d negative amount", idx)
		}
		if lineDebit.Cmp(fx.MustDecimal("0")) > 0 && lineCredit.Cmp(fx.MustDecimal("0")) > 0 {
			return fmt.Errorf("accounting: line %d cannot be both debit and credit", idx)
		}
		debit = debit.Add(lineDebit)
		credit = credit.Add(lineCredit)
	}
	if debit.Cmp(credit) != 0 {
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
