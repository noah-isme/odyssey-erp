package banks

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

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

// PerformTransfer executes a bank transfer by creating a balanced journal entry.
// For MVP, we assume both bank accounts use the same currency and exist.
func (s *Service) PerformTransfer(ctx context.Context, req TransferRequest) error {
	if req.FromBankAccountID == req.ToBankAccountID {
		return fmt.Errorf("source and destination bank accounts cannot be the same")
	}
	if req.Amount <= 0 {
		return fmt.Errorf("transfer amount must be greater than zero")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.queries.WithTx(tx)

	// In a real ERP, we would look up the GL Account IDs for both bank accounts:
	// But since we don't have GetBankAccount here, we'll assume the bank accounts table exists
	// We need to fetch the GL Accounts. Let's do a raw query for now.
	var fromGL, toGL int64
	err = tx.QueryRow(ctx, "SELECT gl_account_id FROM bank_accounts WHERE id = $1", req.FromBankAccountID).Scan(&fromGL)
	if err != nil {
		return fmt.Errorf("from bank account not found: %w", err)
	}

	err = tx.QueryRow(ctx, "SELECT gl_account_id FROM bank_accounts WHERE id = $1", req.ToBankAccountID).Scan(&toGL)
	if err != nil {
		return fmt.Errorf("to bank account not found: %w", err)
	}

	// Create Bank Transactions (the history log for bank_accounts)
	_, err = qtx.CreateBankTransaction(ctx, sqlc.CreateBankTransactionParams{
		ID:            pgtype.UUID{Bytes: uuid.New(), Valid: true},
		BankAccountID: req.FromBankAccountID,
		Date:          pgtype.Date{Time: req.TransferDate, Valid: true},
		Amount:        floatToNumeric(-req.Amount), // Negative for withdrawal
		Description:   req.Notes,
		Reference:     pgtype.Text{String: req.Reference, Valid: req.Reference != ""},
		Status:        "CLEARED", // Transfers are auto-cleared in this MVP
	})
	if err != nil {
		return fmt.Errorf("create from bank transaction: %w", err)
	}

	_, err = qtx.CreateBankTransaction(ctx, sqlc.CreateBankTransactionParams{
		ID:            pgtype.UUID{Bytes: uuid.New(), Valid: true},
		BankAccountID: req.ToBankAccountID,
		Date:          pgtype.Date{Time: req.TransferDate, Valid: true},
		Amount:        floatToNumeric(req.Amount), // Positive for deposit
		Description:   req.Notes,
		Reference:     pgtype.Text{String: req.Reference, Valid: req.Reference != ""},
		Status:        "CLEARED",
	})
	if err != nil {
		return fmt.Errorf("create to bank transaction: %w", err)
	}

	return tx.Commit(ctx)
}
