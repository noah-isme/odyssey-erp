package treasury

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// PGRepository adapts generated treasury rows to the storage-neutral treasury port.
type PGRepository struct {
	queries *sqlc.Queries
	pool    treasuryPool
}

// treasuryPool is the small database surface used by the repository's
// hand-written, transaction-sensitive checks. Keeping it as an interface lets
// those checks use the same pgxmock pool as the generated queries in tests.
type treasuryPool interface {
	Begin(context.Context) (pgx.Tx, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func NewPGRepository(db *pgxpool.Pool) *PGRepository {
	return &PGRepository{queries: sqlc.New(db), pool: db}
}

func (r *PGRepository) SupplierBelongsToCompany(ctx context.Context, supplierID, companyID int64) (bool, error) {
	return r.queries.SupplierBelongsToCompany(ctx, sqlc.SupplierBelongsToCompanyParams{
		ID:        supplierID,
		CompanyID: pgtype.Int8{Int64: companyID, Valid: companyID > 0},
	})
}

func (r *PGRepository) CreateSupplierBankAccount(ctx context.Context, input SupplierBankAccountCreate) (SupplierBankAccount, error) {
	row, err := r.queries.CreateTreasurySupplierBankAccount(ctx, sqlc.CreateTreasurySupplierBankAccountParams{
		CompanyID:     input.CompanyID,
		SupplierID:    input.SupplierID,
		BankName:      input.BankName,
		AccountNumber: input.AccountNumber,
		RoutingNumber: optionalText(input.RoutingNumber),
		Currency:      input.Currency,
		EffectiveFrom: pgtype.Timestamptz{Time: input.EffectiveFrom, Valid: !input.EffectiveFrom.IsZero()},
		EffectiveTo:   optionalTime(input.EffectiveTo),
		EvidenceRef:   optionalText(input.EvidenceRef),
		CreatedBy:     input.CreatedBy,
	})
	if err != nil {
		return SupplierBankAccount{}, err
	}
	return mapSupplierBankAccount(row), nil
}

func (r *PGRepository) UpdateSupplierBankAccountVerification(ctx context.Context, input SupplierBankAccountVerificationUpdate) (SupplierBankAccount, error) {
	row, err := r.queries.UpdateTreasurySupplierBankAccountVerification(ctx, sqlc.UpdateTreasurySupplierBankAccountVerificationParams{
		ID:                 input.ID,
		VerificationStatus: input.VerificationStatus,
		HoldPayments:       input.HoldPayments,
		ApprovedBy:         optionalInt(input.ApprovedBy),
	})
	if err != nil {
		return SupplierBankAccount{}, err
	}
	return mapSupplierBankAccount(row), nil
}

func (r *PGRepository) GetSupplierBankAccount(ctx context.Context, id int64) (SupplierBankAccount, error) {
	row, err := r.queries.GetTreasurySupplierBankAccount(ctx, id)
	if err != nil {
		return SupplierBankAccount{}, err
	}
	return mapSupplierBankAccount(row), nil
}

func (r *PGRepository) ListSupplierBankAccounts(ctx context.Context, filter SupplierBankAccountFilter) ([]SupplierBankAccount, error) {
	rows, err := r.queries.ListTreasurySupplierBankAccounts(ctx, sqlc.ListTreasurySupplierBankAccountsParams{
		SupplierID: filter.SupplierID,
		CompanyID:  filter.CompanyID,
	})
	if err != nil {
		return nil, err
	}
	accounts := make([]SupplierBankAccount, 0, len(rows))
	for _, row := range rows {
		accounts = append(accounts, mapSupplierBankAccount(row))
	}
	return accounts, nil
}

func (r *PGRepository) GetPaymentPolicy(ctx context.Context, companyID int64) (PaymentPolicy, error) {
	row, err := r.queries.GetTreasuryPaymentPolicy(ctx, companyID)
	if err != nil {
		return PaymentPolicy{}, err
	}
	if row.CompanyID != companyID {
		return PaymentPolicy{}, fmt.Errorf("treasury: payment policy company mismatch")
	}
	policy := PaymentPolicy{
		BankFormat:           strings.TrimSpace(row.BankFormat.String),
		RequiresMakerChecker: row.RequiresMakerChecker,
	}
	if row.CalendarID.Valid {
		policy.CalendarID = int64Ptr(row.CalendarID.Int64)
	}
	if row.MaxBatchAmount.Valid {
		policy.MaxBatchAmount, err = amountFromNumeric(row.MaxBatchAmount)
		if err != nil {
			return PaymentPolicy{}, fmt.Errorf("treasury: payment policy max batch amount: %w", err)
		}
	}
	if row.MaxItemAmount.Valid {
		policy.MaxItemAmount, err = amountFromNumeric(row.MaxItemAmount)
		if err != nil {
			return PaymentPolicy{}, fmt.Errorf("treasury: payment policy max item amount: %w", err)
		}
	}
	if row.CutOffTime.Valid {
		policy.CutOffTime = durationPtr(time.Duration(row.CutOffTime.Microseconds) * time.Microsecond)
	}
	if err := policy.Validate(); err != nil {
		return PaymentPolicy{}, err
	}
	return policy, nil
}

func (r *PGRepository) APInvoiceEligibleForPayment(ctx context.Context, invoiceID, supplierID, companyID int64, currency string) (bool, error) {
	return r.queries.APInvoiceEligibleForTreasuryPayment(ctx, sqlc.APInvoiceEligibleForTreasuryPaymentParams{
		ID:         invoiceID,
		SupplierID: supplierID,
		CompanyID:  pgtype.Int8{Int64: companyID, Valid: companyID > 0},
		Currency:   currency,
	})
}

// APInvoiceAllocationAvailable extends the posted/unpaid check with the exact
// amount being proposed and active treasury reservations from the same
// company. Active items in draft, pending, approved, exported, and processing
// batches reserve their amount; the current batch is excluded because its aggregate is passed as the
// requested amount by the approval path. A supplier ID of zero is accepted by
// the approval path after each item has already passed the supplier ownership
// check.
func (r *PGRepository) APInvoiceAllocationAvailable(ctx context.Context, invoiceID, supplierID, companyID int64, currency string, batchID int64, amount Amount) (bool, error) {
	if r == nil || r.pool == nil || invoiceID <= 0 || companyID <= 0 || batchID <= 0 {
		return false, nil
	}
	if err := amount.Validate(); err != nil || !amount.IsPositive() {
		return false, fmt.Errorf("treasury: invalid invoice allocation amount")
	}
	var available bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM ap_invoices i
			JOIN suppliers s ON s.id = i.supplier_id
			WHERE i.id = $1
			  AND ($2 = 0 OR i.supplier_id = $2)
			  AND s.company_id = $3
			  AND i.currency = $4
			  AND i.status = 'POSTED'
			  AND i.total
			      - COALESCE((
			          SELECT SUM(pa.amount)
			          FROM ap_payment_allocations pa
			          WHERE pa.ap_invoice_id = i.id
			        ), 0)
			      - COALESCE((
			          SELECT SUM(bi.amount)
			          FROM treasury_payment_batch_items bi
			          JOIN treasury_payment_batches b ON b.id = bi.batch_id
			          WHERE bi.ap_invoice_id = i.id
			            AND bi.status = 'ACTIVE'
			            AND b.company_id = $3
			            AND b.currency = $4
			            AND b.status IN ('DRAFT', 'PENDING_APPROVAL', 'APPROVED', 'EXPORTED', 'PROCESSING')
			            AND b.id <> $5
			        ), 0) >= $6::numeric
		)`, invoiceID, supplierID, companyID, strings.ToUpper(strings.TrimSpace(currency)), batchID, amount.String()).Scan(&available)
	return available, err
}

func (r *PGRepository) CreatePaymentBatch(ctx context.Context, input PaymentBatchCreate) (PaymentBatch, error) {
	row, err := r.queries.CreateTreasuryPaymentBatch(ctx, sqlc.CreateTreasuryPaymentBatchParams{
		CompanyID:           input.CompanyID,
		ReferenceCode:       input.ReferenceCode,
		Currency:            input.Currency,
		ProposedBy:          input.ProposedBy,
		PaymentConnectionID: optionalInt(input.PaymentConnectionID),
		SourceBankAccountID: optionalInt(input.SourceBankAccountID),
	})
	if err != nil {
		return PaymentBatch{}, err
	}
	return mapPaymentBatch(row)
}

func (r *PGRepository) GetPaymentBatch(ctx context.Context, id int64) (PaymentBatch, error) {
	row, err := r.queries.GetTreasuryPaymentBatch(ctx, id)
	if err != nil {
		return PaymentBatch{}, err
	}
	return mapPaymentBatch(row)
}

func (r *PGRepository) UpdatePaymentBatchStatus(ctx context.Context, input PaymentBatchStatusUpdate) (PaymentBatch, error) {
	row, err := r.queries.UpdateTreasuryPaymentBatchStatus(ctx, sqlc.UpdateTreasuryPaymentBatchStatusParams{
		ID:         input.ID,
		Status:     input.Status,
		ApprovedBy: optionalInt(input.ApprovedBy),
		ApprovedAt: optionalTime(input.ApprovedAt),
	})
	if err != nil {
		return PaymentBatch{}, err
	}
	return mapPaymentBatch(row)
}

func (r *PGRepository) UpdatePaymentBatchRevision(ctx context.Context, input PaymentBatchRevisionUpdate) (PaymentBatch, error) {
	row, err := r.queries.UpdateTreasuryPaymentBatchRevision(ctx, sqlc.UpdateTreasuryPaymentBatchRevisionParams{
		BatchID:     input.ID,
		TotalAmount: numericOf(input.TotalAmount),
	})
	if err != nil {
		return PaymentBatch{}, err
	}
	return mapPaymentBatch(row)
}

func (r *PGRepository) UpdatePaymentBatchTotal(ctx context.Context, input PaymentBatchTotalUpdate) (PaymentBatch, error) {
	row, err := r.queries.UpdateTreasuryPaymentBatchTotal(ctx, sqlc.UpdateTreasuryPaymentBatchTotalParams{
		BatchID:     input.ID,
		TotalAmount: numericOf(input.TotalAmount),
	})
	if err != nil {
		return PaymentBatch{}, err
	}
	return mapPaymentBatch(row)
}

func (r *PGRepository) UpdatePaymentBatchExport(ctx context.Context, input PaymentBatchExportUpdate) (PaymentBatch, error) {
	if input.ExpectedRevisionNumber <= 0 {
		return PaymentBatch{}, ErrPaymentBatchRevisionConflict
	}
	row, err := r.queries.UpdateTreasuryPaymentBatchExport(ctx, sqlc.UpdateTreasuryPaymentBatchExportParams{
		ID:                     input.ID,
		ExportedFileHash:       optionalText(input.ExportedFileHash),
		ExportedBy:             optionalInt(input.ExportedBy),
		ExpectedRevisionNumber: input.ExpectedRevisionNumber,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PaymentBatch{}, ErrPaymentBatchRevisionConflict
		}
		return PaymentBatch{}, err
	}
	return mapPaymentBatch(row)
}

func (r *PGRepository) UpdatePaymentBatchSettlement(ctx context.Context, input PaymentBatchSettlementUpdate) (PaymentBatch, error) {
	row, err := r.queries.UpdateTreasuryPaymentBatchSettlement(ctx, sqlc.UpdateTreasuryPaymentBatchSettlementParams{
		ID:        input.ID,
		SettledBy: optionalInt(input.SettledBy),
	})
	if err != nil {
		return PaymentBatch{}, err
	}
	return mapPaymentBatch(row)
}

func (r *PGRepository) CreatePaymentBatchItem(ctx context.Context, input PaymentBatchItemCreate) (PaymentBatchItem, error) {
	row, err := r.queries.CreateTreasuryPaymentBatchItem(ctx, sqlc.CreateTreasuryPaymentBatchItemParams{
		BatchID:       input.BatchID,
		SupplierID:    input.SupplierID,
		BankAccountID: input.BankAccountID,
		Amount:        numericOf(input.Amount),
		ApInvoiceID:   optionalInt(input.APInvoiceID),
	})
	if err != nil {
		return PaymentBatchItem{}, err
	}
	return mapPaymentBatchItem(row)
}

// CreatePaymentBatchItemAtomically serializes AP-backed proposal reservations
// on the invoice row. The caller's earlier eligibility read is retained for a
// fast rejection, but this transaction is authoritative: it rechecks the
// posted invoice balance and all active treasury reservations after acquiring
// the lock, then inserts the item before releasing it.
func (r *PGRepository) CreatePaymentBatchItemAtomically(ctx context.Context, input PaymentBatchItemCreate) (PaymentBatchItem, error) {
	if r == nil || r.pool == nil || input.BatchID <= 0 || input.SupplierID <= 0 || input.BankAccountID <= 0 {
		return PaymentBatchItem{}, errors.New("treasury: payment batch item repository is not configured")
	}
	if err := input.Amount.Validate(); err != nil || !input.Amount.IsPositive() {
		return PaymentBatchItem{}, errors.New("treasury: invalid payment batch item amount")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return PaymentBatchItem{}, fmt.Errorf("treasury: begin payment item transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var batchStatus string
	if err := tx.QueryRow(ctx, `
		SELECT status
		FROM treasury_payment_batches
		WHERE id = $1
		FOR UPDATE`, input.BatchID).Scan(&batchStatus); err != nil {
		return PaymentBatchItem{}, fmt.Errorf("treasury: lock payment batch: %w", err)
	}
	if batchStatus == "EXPORTED" || batchStatus == "SETTLED" || batchStatus == "CANCELLED" {
		return PaymentBatchItem{}, errors.New("treasury: cannot edit batch in terminal or exported state")
	}

	if input.APInvoiceID != nil {
		var totalText, paidText, reservedText string
		err := tx.QueryRow(ctx, `
			SELECT i.total::text,
			       COALESCE((
			           SELECT SUM(pa.amount)
			           FROM ap_payment_allocations pa
			           WHERE pa.ap_invoice_id = i.id
			       ), 0)::text,
			       COALESCE((
			           SELECT SUM(bi.amount)
			           FROM treasury_payment_batch_items bi
			           JOIN treasury_payment_batches b ON b.id = bi.batch_id
			           WHERE bi.ap_invoice_id = i.id
			             AND bi.status = 'ACTIVE'
			             AND b.currency = i.currency
			             AND b.status IN ('DRAFT', 'PENDING_APPROVAL', 'APPROVED', 'EXPORTED', 'PROCESSING')
			       ), 0)::text
			FROM ap_invoices i
			JOIN suppliers s ON s.id = i.supplier_id
			JOIN treasury_payment_batches batch ON batch.id = $2
			WHERE i.id = $1
			  AND i.supplier_id = $3
			  AND s.company_id = batch.company_id
			  AND COALESCE(i.company_id, s.company_id) = batch.company_id
			  AND i.currency = batch.currency
			  AND i.status = 'POSTED'
			FOR UPDATE OF i`, *input.APInvoiceID, input.BatchID, input.SupplierID).Scan(&totalText, &paidText, &reservedText)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return PaymentBatchItem{}, fmt.Errorf("%w: invoice %d", ErrPaymentInvoiceAllocationLimit, *input.APInvoiceID)
			}
			return PaymentBatchItem{}, fmt.Errorf("treasury: lock AP invoice %d: %w", *input.APInvoiceID, err)
		}
		total, err := ParseAmount(totalText)
		if err != nil {
			return PaymentBatchItem{}, fmt.Errorf("treasury: AP invoice %d total: %w", *input.APInvoiceID, err)
		}
		paid, err := ParseAmount(paidText)
		if err != nil {
			return PaymentBatchItem{}, fmt.Errorf("treasury: AP invoice %d paid amount: %w", *input.APInvoiceID, err)
		}
		reserved, err := ParseAmount(reservedText)
		if err != nil {
			return PaymentBatchItem{}, fmt.Errorf("treasury: AP invoice %d reserved amount: %w", *input.APInvoiceID, err)
		}
		remaining, err := total.Sub(paid)
		if err == nil {
			remaining, err = remaining.Sub(reserved)
		}
		if err != nil {
			return PaymentBatchItem{}, fmt.Errorf("treasury: AP invoice %d balance: %w", *input.APInvoiceID, err)
		}
		if exceeds, err := input.Amount.Cmp(remaining); err != nil {
			return PaymentBatchItem{}, fmt.Errorf("treasury: AP invoice %d balance comparison: %w", *input.APInvoiceID, err)
		} else if exceeds > 0 {
			return PaymentBatchItem{}, fmt.Errorf("%w: invoice %d", ErrPaymentInvoiceAllocationLimit, *input.APInvoiceID)
		}
	}

	row, err := sqlc.New(tx).CreateTreasuryPaymentBatchItem(ctx, sqlc.CreateTreasuryPaymentBatchItemParams{
		BatchID:       input.BatchID,
		SupplierID:    input.SupplierID,
		BankAccountID: input.BankAccountID,
		Amount:        numericOf(input.Amount),
		ApInvoiceID:   optionalInt(input.APInvoiceID),
	})
	if err != nil {
		return PaymentBatchItem{}, err
	}
	item, err := mapPaymentBatchItem(row)
	if err != nil {
		return PaymentBatchItem{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PaymentBatchItem{}, fmt.Errorf("treasury: commit payment item transaction: %w", err)
	}
	return item, nil
}

func (r *PGRepository) ListPaymentBatchItems(ctx context.Context, batchID int64) ([]PaymentBatchItem, error) {
	rows, err := r.queries.ListTreasuryPaymentBatchItems(ctx, batchID)
	if err != nil {
		return nil, err
	}
	items := make([]PaymentBatchItem, 0, len(rows))
	for _, row := range rows {
		item, err := mapPaymentBatchItem(row)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *PGRepository) RemovePaymentBatchItem(ctx context.Context, id int64) error {
	return r.queries.RemoveTreasuryPaymentBatchItem(ctx, id)
}

// PaymentConnectionBelongsToCompany validates the provider connection before
// a live batch can be executed. The company predicate is part of the query,
// not just an application-side check.
func (r *PGRepository) PaymentConnectionBelongsToCompany(ctx context.Context, connectionID, companyID int64) (bool, error) {
	if r == nil || r.pool == nil || connectionID <= 0 || companyID <= 0 {
		return false, nil
	}
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM connector_connections
			WHERE id = $1 AND company_id = $2 AND LOWER(status) <> 'disabled'
		)`, connectionID, companyID).Scan(&exists)
	return exists, err
}

// SourceBankAccountBelongsToCompany validates the source account and currency
// for the live payment boundary. Inactive accounts cannot fund new execution.
func (r *PGRepository) SourceBankAccountBelongsToCompany(ctx context.Context, accountID, companyID int64, currency string) (bool, error) {
	if r == nil || r.pool == nil || accountID <= 0 || companyID <= 0 {
		return false, nil
	}
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM bank_accounts ba
			JOIN accounts a ON a.id = ba.gl_account_id
			WHERE ba.id = $1 AND ba.company_id = $2 AND ba.is_active AND ba.currency = $3
			  AND a.is_active
			  AND (a.company_id = $2 OR a.company_id IS NULL)
		)`, accountID, companyID, strings.ToUpper(strings.TrimSpace(currency))).Scan(&exists)
	return exists, err
}

func mapSupplierBankAccount(row sqlc.TreasurySupplierBankAccount) SupplierBankAccount {
	account := SupplierBankAccount{
		ID:                 row.ID,
		CompanyID:          row.CompanyID,
		SupplierID:         row.SupplierID,
		BankName:           row.BankName,
		AccountNumber:      row.AccountNumber,
		RoutingNumber:      row.RoutingNumber.String,
		Currency:           row.Currency,
		EffectiveFrom:      row.EffectiveFrom.Time,
		VerificationStatus: row.VerificationStatus,
		EvidenceRef:        row.EvidenceRef.String,
		HoldPayments:       row.HoldPayments,
		CreatedBy:          row.CreatedBy,
		CreatedAt:          row.CreatedAt.Time,
		UpdatedAt:          row.UpdatedAt.Time,
	}
	if row.EffectiveTo.Valid {
		account.EffectiveTo = timePtr(row.EffectiveTo.Time)
	}
	if row.ApprovedBy.Valid {
		account.ApprovedBy = int64Ptr(row.ApprovedBy.Int64)
	}
	return account
}

func mapPaymentBatch(row sqlc.TreasuryPaymentBatch) (PaymentBatch, error) {
	amount, err := amountFromNumeric(row.TotalAmount)
	if err != nil {
		return PaymentBatch{}, err
	}
	batch := PaymentBatch{
		ID:               row.ID,
		CompanyID:        row.CompanyID,
		ReferenceCode:    row.ReferenceCode,
		Status:           row.Status,
		Currency:         row.Currency,
		TotalAmount:      amount,
		RevisionNumber:   row.RevisionNumber,
		ProposedBy:       row.ProposedBy,
		CreatedAt:        row.CreatedAt.Time,
		UpdatedAt:        row.UpdatedAt.Time,
		ExportedFileHash: row.ExportedFileHash.String,
	}
	if row.ApprovedBy.Valid {
		batch.ApprovedBy = int64Ptr(row.ApprovedBy.Int64)
	}
	if row.ApprovedAt.Valid {
		batch.ApprovedAt = timePtr(row.ApprovedAt.Time)
	}
	if row.ExportedAt.Valid {
		batch.ExportedAt = timePtr(row.ExportedAt.Time)
	}
	if row.ExportedBy.Valid {
		batch.ExportedBy = int64Ptr(row.ExportedBy.Int64)
	}
	if row.SettledAt.Valid {
		batch.SettledAt = timePtr(row.SettledAt.Time)
	}
	if row.SettledBy.Valid {
		batch.SettledBy = int64Ptr(row.SettledBy.Int64)
	}
	if row.PaymentConnectionID.Valid {
		batch.PaymentConnectionID = int64Ptr(row.PaymentConnectionID.Int64)
	}
	if row.SourceBankAccountID.Valid {
		batch.SourceBankAccountID = int64Ptr(row.SourceBankAccountID.Int64)
	}
	return batch, nil
}

func mapPaymentBatchItem(row sqlc.TreasuryPaymentBatchItem) (PaymentBatchItem, error) {
	amount, err := amountFromNumeric(row.Amount)
	if err != nil {
		return PaymentBatchItem{}, err
	}
	item := PaymentBatchItem{
		ID:            row.ID,
		BatchID:       row.BatchID,
		SupplierID:    row.SupplierID,
		BankAccountID: row.BankAccountID,
		Amount:        amount,
		Status:        row.Status,
		CreatedAt:     row.CreatedAt.Time,
	}
	if row.ApInvoiceID.Valid {
		item.APInvoiceID = int64Ptr(row.ApInvoiceID.Int64)
	}
	return item, nil
}

func numericOf(value Amount) pgtype.Numeric {
	var number pgtype.Numeric
	_ = number.Scan(value.String())
	return number
}

func durationPtr(value time.Duration) *time.Duration { return &value }

func amountFromNumeric(value pgtype.Numeric) (Amount, error) {
	if !value.Valid || value.NaN || value.InfinityModifier != pgtype.Finite {
		return "", fmt.Errorf("invalid numeric value")
	}
	encoded, err := value.Value()
	if err != nil {
		return "", fmt.Errorf("invalid numeric value: %w", err)
	}
	text, ok := encoded.(string)
	if !ok {
		return "", fmt.Errorf("invalid numeric value")
	}
	return ParseAmount(text)
}

func optionalText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func optionalTime(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *value, Valid: true}
}

func optionalInt(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}
