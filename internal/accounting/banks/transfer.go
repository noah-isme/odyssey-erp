package banks

import (
	"errors"
	"time"
)

// ErrSameTransferAccount indicates that a transfer uses the same account on
// both sides.
var ErrSameTransferAccount = errors.New("source and destination bank accounts cannot be the same")

// ErrInvalidTransferAmount indicates a non-positive transfer amount.
var ErrInvalidTransferAmount = errors.New("transfer amount must be greater than zero")

// TransferRequest contains data to perform a bank transfer.
type TransferRequest struct {
	FromBankAccountID int64
	ToBankAccountID   int64
	Amount            float64
	TransferDate      time.Time
	Reference         string
	Notes             string
	CreatedBy         int64
}
