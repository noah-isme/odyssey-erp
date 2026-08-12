package banks

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// PGRepository adapts generated persistence types to the banking boundary.
type PGRepository struct {
	db      *pgxpool.Pool
	queries *sqlc.Queries
}

func NewRepository(db *pgxpool.Pool) *PGRepository {
	return &PGRepository{db: db, queries: sqlc.New(db)}
}

func (r *PGRepository) ImportStatement(ctx context.Context, accountID int64, statementDate time.Time, lines []ParsedStatementLine) (int64, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.queries.WithTx(tx)
	var startingBalance, endingBalance float64
	for _, line := range lines {
		endingBalance += line.Amount
	}

	statement, err := qtx.CreateBankStatement(ctx, sqlc.CreateBankStatementParams{
		BankAccountID:   accountID,
		StatementDate:   pgtype.Date{Time: statementDate, Valid: true},
		StartingBalance: numericOf(startingBalance),
		EndingBalance:   numericOf(endingBalance),
		Status:          sqlc.BankStatementStatusDRAFT,
	})
	if err != nil {
		return 0, fmt.Errorf("create statement failed: %w", err)
	}

	for _, line := range lines {
		status := sqlc.BankLineStatusUNMATCHED
		var matchedType pgtype.Text
		var matchedID pgtype.Int8
		if line.Amount > 0 {
			invoices, matchErr := qtx.FindUnpaidARInvoicesForMatching(ctx, numericOf(line.Amount))
			if matchErr != nil {
				return 0, fmt.Errorf("find matching invoices failed: %w", matchErr)
			}
			if len(invoices) == 1 {
				status = sqlc.BankLineStatusSUGGESTED
				matchedType = pgtype.Text{String: "AR_INVOICE", Valid: true}
				matchedID = pgtype.Int8{Int64: invoices[0].ID, Valid: true}
			}
		}

		_, err = qtx.CreateBankStatementLine(ctx, sqlc.CreateBankStatementLineParams{
			StatementID:     statement.ID,
			TrxDate:         pgtype.Date{Time: line.Date, Valid: true},
			Description:     line.Description,
			Amount:          numericOf(line.Amount),
			ReferenceNumber: optionalText(line.Reference),
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
	return statement.ID, nil
}

func (r *PGRepository) ConfirmStatement(ctx context.Context, id int64) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.queries.WithTx(tx)
	lines, err := qtx.ListBankStatementLines(ctx, id)
	if err != nil {
		return err
	}
	for _, line := range lines {
		if line.Status != sqlc.BankLineStatusSUGGESTED {
			continue
		}
		if err := qtx.UpdateBankStatementLineStatus(ctx, sqlc.UpdateBankStatementLineStatusParams{
			ID:             line.ID,
			Status:         sqlc.BankLineStatusMATCHED,
			MatchedDocType: line.MatchedDocType,
			MatchedDocID:   line.MatchedDocID,
		}); err != nil {
			return err
		}
	}
	if err := qtx.UpdateBankStatementStatus(ctx, sqlc.UpdateBankStatementStatusParams{
		ID:     id,
		Status: sqlc.BankStatementStatusRECONCILED,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PGRepository) PerformTransfer(ctx context.Context, req TransferRequest) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var fromGL int64
	if err := tx.QueryRow(ctx, "SELECT gl_account_id FROM bank_accounts WHERE id = $1", req.FromBankAccountID).Scan(&fromGL); err != nil {
		return fmt.Errorf("from bank account not found: %w", err)
	}
	var toGL int64
	if err := tx.QueryRow(ctx, "SELECT gl_account_id FROM bank_accounts WHERE id = $1", req.ToBankAccountID).Scan(&toGL); err != nil {
		return fmt.Errorf("to bank account not found: %w", err)
	}

	qtx := r.queries.WithTx(tx)
	for _, entry := range []struct {
		accountID int64
		amount    float64
	}{
		{accountID: req.FromBankAccountID, amount: -req.Amount},
		{accountID: req.ToBankAccountID, amount: req.Amount},
	} {
		if _, err := qtx.CreateBankTransaction(ctx, sqlc.CreateBankTransactionParams{
			ID:            pgtype.UUID{Bytes: uuid.New(), Valid: true},
			BankAccountID: entry.accountID,
			Date:          pgtype.Date{Time: req.TransferDate, Valid: true},
			Amount:        numericOf(entry.amount),
			Description:   req.Notes,
			Reference:     optionalText(req.Reference),
			Status:        "CLEARED",
		}); err != nil {
			return fmt.Errorf("create bank transaction: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func (r *PGRepository) BankAccountIDForCompany(ctx context.Context, companyID int64) (int64, error) {
	accounts, err := r.queries.ListBankAccounts(ctx, companyID)
	if err != nil {
		return 0, err
	}
	if len(accounts) == 0 {
		return 0, ErrNoBankAccount
	}
	return accounts[0].ID, nil
}

func (r *PGRepository) ListStatements(ctx context.Context, accountID int64, limit, offset int32) ([]BankStatement, error) {
	rows, err := r.queries.ListBankStatements(ctx, sqlc.ListBankStatementsParams{
		BankAccountID: accountID,
		Limit:         limit,
		Offset:        offset,
	})
	if err != nil {
		return nil, err
	}
	items := make([]BankStatement, len(rows))
	for i, row := range rows {
		items[i], err = mapStatement(row)
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (r *PGRepository) GetStatement(ctx context.Context, id int64) (BankStatement, error) {
	row, err := r.queries.GetBankStatement(ctx, id)
	if err != nil {
		return BankStatement{}, err
	}
	return mapStatement(row)
}

func (r *PGRepository) ListStatementLines(ctx context.Context, statementID int64) ([]BankStatementLine, error) {
	rows, err := r.queries.ListBankStatementLines(ctx, statementID)
	if err != nil {
		return nil, err
	}
	items := make([]BankStatementLine, len(rows))
	for i, row := range rows {
		items[i], err = mapStatementLine(row)
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

func mapStatement(row sqlc.BankStatement) (BankStatement, error) {
	starting, err := numericFloat(row.StartingBalance)
	if err != nil {
		return BankStatement{}, fmt.Errorf("invalid starting balance: %w", err)
	}
	ending, err := numericFloat(row.EndingBalance)
	if err != nil {
		return BankStatement{}, fmt.Errorf("invalid ending balance: %w", err)
	}
	return BankStatement{
		ID:              row.ID,
		BankAccountID:   row.BankAccountID,
		StatementDate:   row.StatementDate.Time,
		StartingBalance: starting,
		EndingBalance:   ending,
		Status:          string(row.Status),
		ImportedAt:      row.ImportedAt.Time,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}, nil
}

func mapStatementLine(row sqlc.BankStatementLine) (BankStatementLine, error) {
	amount, err := numericFloat(row.Amount)
	if err != nil {
		return BankStatementLine{}, fmt.Errorf("invalid statement line amount: %w", err)
	}
	var matchedID *int64
	if row.MatchedDocID.Valid {
		value := row.MatchedDocID.Int64
		matchedID = &value
	}
	return BankStatementLine{
		ID:              row.ID,
		StatementID:     row.StatementID,
		TrxDate:         row.TrxDate.Time,
		Description:     row.Description,
		Amount:          amount,
		ReferenceNumber: row.ReferenceNumber.String,
		Status:          string(row.Status),
		MatchedDocType:  row.MatchedDocType.String,
		MatchedDocID:    matchedID,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}, nil
}

func numericOf(value float64) pgtype.Numeric {
	var numeric pgtype.Numeric
	_ = numeric.Scan(fmt.Sprintf("%.6f", value))
	return numeric
}

func numericFloat(value pgtype.Numeric) (float64, error) {
	converted, err := value.Float64Value()
	if err != nil {
		return 0, err
	}
	if !converted.Valid {
		return 0, fmt.Errorf("numeric value is null")
	}
	return converted.Float64, nil
}

func optionalText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}
