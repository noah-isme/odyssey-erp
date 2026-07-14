package banks

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

type Service struct {
	db      *pgxpool.Pool
	queries *sqlc.Queries
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{
		db:      db,
		queries: sqlc.New(db),
	}
}

// ImportStatement creates a bank statement from parsed CSV lines and runs auto-matching.
func (s *Service) ImportStatement(ctx context.Context, accountID int64, statementDate time.Time, lines []ParsedStatementLine) (int64, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.queries.WithTx(tx)

	// Calculate starting and ending balances roughly (assuming linear)
	var startingBalance, endingBalance float64 = 0, 0
	for _, l := range lines {
		endingBalance += l.Amount
	}

	// For MVP, using pgtype.Numeric mapping might require string parsing, but sqlc generates numeric types.
	// Since sqlc generated numeric type is pgtype.Numeric, we create a helper mapping
	stmt, err := qtx.CreateBankStatement(ctx, sqlc.CreateBankStatementParams{
		BankAccountID:   accountID,
		StatementDate:   pgtype.Date{Time: statementDate, Valid: true},
		StartingBalance: floatToNumeric(startingBalance),
		EndingBalance:   floatToNumeric(endingBalance),
		Status:          sqlc.BankStatementStatusDRAFT,
	})
	if err != nil {
		return 0, fmt.Errorf("create statement failed: %w", err)
	}

	for _, l := range lines {
		// Run Auto-matching logic
		status := sqlc.BankLineStatusUNMATCHED
		var matchedType pgtype.Text
		var matchedID pgtype.Int8

		// We attempt to match AR Invoices (since those receive payments via bank)
		if l.Amount > 0 { // Money in, possibly AR payment
			invoices, _ := qtx.FindUnpaidARInvoicesForMatching(ctx, floatToNumeric(l.Amount))

			// If exactly one unpaid invoice matches the exact amount, auto-match it
			if len(invoices) == 1 {
				status = sqlc.BankLineStatusSUGGESTED
				matchedType = pgtype.Text{String: "AR_INVOICE", Valid: true}
				matchedID = pgtype.Int8{Int64: invoices[0].ID, Valid: true}
			}
		}

		ref := pgtype.Text{}
		if l.Reference != "" {
			ref.String = l.Reference
			ref.Valid = true
		}

		_, err = qtx.CreateBankStatementLine(ctx, sqlc.CreateBankStatementLineParams{
			StatementID:     stmt.ID,
			TrxDate:         pgtype.Date{Time: l.Date, Valid: true},
			Description:     l.Description,
			Amount:          floatToNumeric(l.Amount),
			ReferenceNumber: ref,
			Status:          status,
			MatchedDocType:  matchedType,
			MatchedDocID:    matchedID,
		})
		if err != nil {
			return 0, fmt.Errorf("create statement line failed: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}

	return stmt.ID, nil
}

// ConfirmStatement marks a statement as reconciled and accepts all suggested matches.
func (s *Service) ConfirmStatement(ctx context.Context, id int64) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.queries.WithTx(tx)

	// Accept all SUGGESTED lines as MATCHED
	lines, err := qtx.ListBankStatementLines(ctx, id)
	if err != nil {
		return err
	}

	for _, l := range lines {
		if l.Status == sqlc.BankLineStatusSUGGESTED {
			err = qtx.UpdateBankStatementLineStatus(ctx, sqlc.UpdateBankStatementLineStatusParams{
				ID:             l.ID,
				Status:         sqlc.BankLineStatusMATCHED,
				MatchedDocType: l.MatchedDocType,
				MatchedDocID:   l.MatchedDocID,
			})
			if err != nil {
				return err
			}
			// In a real ERP, we would also generate a payment/journal entry here.
		}
	}

	err = qtx.UpdateBankStatementStatus(ctx, sqlc.UpdateBankStatementStatusParams{
		ID:     id,
		Status: sqlc.BankStatementStatusRECONCILED,
	})
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// floatToNumeric is a helper to convert float64 to pgtype.Numeric
func floatToNumeric(f float64) pgtype.Numeric {
	var num pgtype.Numeric
	_ = num.Scan(fmt.Sprintf("%f", f))
	return num
}
