package ap

import (
	"errors"
	"fmt"

	accounting "github.com/odyssey-erp/odyssey-erp/internal/accounting/shared"
)

// LedgerPostError indicates the payment was recorded but journal posting failed.
type LedgerPostError struct {
	Err       error
	Retryable bool
	Message   string
}

func (e *LedgerPostError) Error() string {
	return e.Message
}

func (e *LedgerPostError) Unwrap() error {
	return e.Err
}

func wrapLedgerPostError(err error) *LedgerPostError {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, accounting.ErrPeriodLocked):
		return &LedgerPostError{
			Err:       err,
			Retryable: true,
			Message:   "Ledger period locked; payment recorded but journal posting pending",
		}
	case errors.Is(err, accounting.ErrInvalidPeriod):
		return &LedgerPostError{
			Err:       err,
			Retryable: true,
			Message:   "No posting period available for payment date; payment recorded but journal posting pending",
		}
	case errors.Is(err, accounting.ErrDateOutOfRange):
		return &LedgerPostError{
			Err:       err,
			Retryable: true,
			Message:   "Payment date outside ledger period; payment recorded but journal posting pending",
		}
	case errors.Is(err, accounting.ErrMappingNotFound):
		return &LedgerPostError{
			Err:       err,
			Retryable: true,
			Message:   "Account mapping missing for AP payment; payment recorded but journal posting pending",
		}
	default:
		return &LedgerPostError{
			Err:       err,
			Retryable: false,
			Message:   fmt.Sprintf("Failed to post payment to ledger; payment recorded but journal posting pending (%s)", err.Error()),
		}
	}
}
