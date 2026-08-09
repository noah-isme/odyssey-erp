package banking

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// PGRepository implements Repository using PostgreSQL and keeps generated SQL
// types confined to this adapter.
type PGRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

func NewRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{pool: pool, queries: sqlc.New(pool)}
}

func (r *PGRepository) CreateBankAccount(ctx context.Context, input BankAccountCreate) (BankAccount, error) {
	row, err := r.queries.CreateBankAccount(ctx, sqlc.CreateBankAccountParams{
		CompanyID:      input.CompanyID,
		Name:           input.Name,
		AccountNumber:  input.AccountNumber,
		Currency:       input.Currency,
		GlAccountID:    input.GLAccountID,
		InitialBalance: numericOf(input.InitialBalance),
		IsActive:       input.IsActive,
	})
	if err != nil {
		return BankAccount{}, err
	}
	return mapBankAccount(row)
}

func (r *PGRepository) GetBankAccount(ctx context.Context, id int64) (BankAccount, error) {
	row, err := r.queries.GetBankAccount(ctx, id)
	if err != nil {
		return BankAccount{}, err
	}
	return mapBankAccount(row)
}

func (r *PGRepository) ListBankAccounts(ctx context.Context, companyID int64) ([]BankAccount, error) {
	rows, err := r.queries.ListBankAccounts(ctx, companyID)
	if err != nil {
		return nil, err
	}
	accounts := make([]BankAccount, 0, len(rows))
	for _, row := range rows {
		account, err := mapBankAccount(row)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, nil
}

func (r *PGRepository) CreateBankTransaction(ctx context.Context, input BankTransactionCreate) (BankTransaction, error) {
	row, err := r.queries.CreateBankTransaction(ctx, sqlc.CreateBankTransactionParams{
		ID:                pgtype.UUID{Bytes: input.ID, Valid: true},
		BankAccountID:     input.BankAccountID,
		Date:              pgtype.Date{Time: input.Date, Valid: true},
		Amount:            numericOf(input.Amount),
		Description:       input.Description,
		Reference:         optionalText(input.Reference),
		Status:            input.Status,
		GlJournalID:       optionalInt8(input.GLJournalID),
		ImportRunID:       optionalInt8(input.ImportRunID),
		ExternalReference: optionalText(input.ExternalReference),
		Fingerprint:       optionalText(input.Fingerprint),
		SkipReason:        optionalText(input.SkipReason),
	})
	if err != nil {
		return BankTransaction{}, err
	}
	return mapBankTransaction(row)
}

func (r *PGRepository) ListBankTransactions(ctx context.Context, bankAccountID int64) ([]BankTransaction, error) {
	rows, err := r.queries.ListBankTransactions(ctx, bankAccountID)
	if err != nil {
		return nil, err
	}
	transactions := make([]BankTransaction, 0, len(rows))
	for _, row := range rows {
		transaction, err := mapBankTransaction(row)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, transaction)
	}
	return transactions, nil
}

func (r *PGRepository) UpdateBankTransactionStatus(ctx context.Context, update BankTransactionStatusUpdate) error {
	return r.queries.UpdateBankTransactionStatus(ctx, sqlc.UpdateBankTransactionStatusParams{
		ID:          pgtype.UUID{Bytes: update.ID, Valid: true},
		Status:      update.Status,
		GlJournalID: optionalInt8(update.GLJournalID),
	})
}

func (r *PGRepository) CreateStatementImportRun(ctx context.Context, input StatementImportRunCreate) (StatementImportRun, error) {
	row, err := r.queries.CreateStatementImportRun(ctx, sqlc.CreateStatementImportRunParams{
		CompanyID:     input.CompanyID,
		BankAccountID: input.BankAccountID,
		Filename:      input.Filename,
		ContentHash:   input.ContentHash,
		ImportedBy:    optionalInt8(input.ImportedBy),
	})
	if err != nil {
		return StatementImportRun{}, err
	}
	return StatementImportRun{ID: row.ID, CompanyID: row.CompanyID, BankAccountID: row.BankAccountID}, nil
}

func (r *PGRepository) CreateBankStatement(ctx context.Context, input BankStatementCreate) (BankStatement, error) {
	row, err := r.queries.CreateBankStatement(ctx, sqlc.CreateBankStatementParams{
		BankAccountID:   input.BankAccountID,
		StatementDate:   pgtype.Date{Time: input.StatementDate, Valid: true},
		StartingBalance: numericOf(input.StartingBalance),
		EndingBalance:   numericOf(input.EndingBalance),
		Status:          sqlc.BankStatementStatus(input.Status),
	})
	if err != nil {
		return BankStatement{}, err
	}
	return BankStatement{
		ID:            row.ID,
		BankAccountID: row.BankAccountID,
		StatementDate: row.StatementDate.Time,
		Status:        string(row.Status),
	}, nil
}

func (r *PGRepository) CreateBankStatementLine(ctx context.Context, input BankStatementLineCreate) error {
	_, err := r.queries.CreateBankStatementLine(ctx, sqlc.CreateBankStatementLineParams{
		StatementID:     input.StatementID,
		TrxDate:         pgtype.Date{Time: input.Date, Valid: true},
		Description:     input.Description,
		Amount:          numericOf(input.Amount),
		ReferenceNumber: optionalText(input.ReferenceNumber),
		Status:          sqlc.BankLineStatus(input.Status),
		MatchedDocType:  optionalText(input.MatchedDocType),
		MatchedDocID:    optionalInt8(input.MatchedDocID),
	})
	return err
}

func (r *PGRepository) FindOpenPeriod(ctx context.Context, companyID int64, date time.Time) (int64, error) {
	var periodID int64
	err := r.pool.QueryRow(ctx, `
		SELECT period_id
		FROM accounting_periods
		WHERE (company_id = $1 OR company_id IS NULL)
		  AND start_date <= $2 AND end_date >= $2 AND status = 'OPEN'
		ORDER BY (company_id = $1) DESC, id DESC
		LIMIT 1`, companyID, date).Scan(&periodID)
	return periodID, err
}

func (r *PGRepository) BankTransactionExists(ctx context.Context, bankAccountID int64, externalRef, fingerprint string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM bank_transactions
			WHERE bank_account_id = $1
			  AND ((external_reference = $2 AND external_reference IS NOT NULL AND external_reference != '')
			       OR (fingerprint = $3 AND fingerprint IS NOT NULL AND fingerprint != ''))
		)`, bankAccountID, externalRef, fingerprint).Scan(&exists)
	return exists, err
}

func mapBankAccount(row sqlc.BankAccount) (BankAccount, error) {
	initialBalance, err := numericFloat(row.InitialBalance)
	if err != nil {
		return BankAccount{}, err
	}
	return BankAccount{
		ID:             row.ID,
		CompanyID:      row.CompanyID,
		Name:           row.Name,
		AccountNumber:  row.AccountNumber,
		Currency:       row.Currency,
		GLAccountID:    row.GlAccountID,
		InitialBalance: initialBalance,
		IsActive:       row.IsActive,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
	}, nil
}

func mapBankTransaction(row sqlc.BankTransaction) (BankTransaction, error) {
	amount, err := numericFloat(row.Amount)
	if err != nil {
		return BankTransaction{}, err
	}
	transaction := BankTransaction{
		ID:                uuidFromPG(row.ID),
		BankAccountID:     row.BankAccountID,
		Date:              row.Date.Time,
		Amount:            amount,
		Description:       row.Description,
		Reference:         row.Reference.String,
		Status:            row.Status,
		CreatedAt:         row.CreatedAt.Time,
		UpdatedAt:         row.UpdatedAt.Time,
		ExternalReference: row.ExternalReference.String,
		Fingerprint:       row.Fingerprint.String,
		SkipReason:        row.SkipReason.String,
	}
	if row.GlJournalID.Valid {
		transaction.GLJournalID = &row.GlJournalID.Int64
	}
	if row.ImportRunID.Valid {
		transaction.ImportRunID = &row.ImportRunID.Int64
	}
	return transaction, nil
}

func uuidFromPG(value pgtype.UUID) uuid.UUID {
	if !value.Valid {
		return uuid.Nil
	}
	return uuid.UUID(value.Bytes)
}

func numericOf(value float64) pgtype.Numeric {
	var number pgtype.Numeric
	_ = number.Scan(fmt.Sprintf("%.6f", value))
	return number
}

func numericFloat(value pgtype.Numeric) (float64, error) {
	converted, err := value.Float64Value()
	if err != nil || !converted.Valid {
		return 0, fmt.Errorf("invalid numeric value")
	}
	return converted.Float64, nil
}

func optionalText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func optionalInt8(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}
