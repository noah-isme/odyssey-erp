package treasury

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/payments"
	pgxmock "github.com/pashagolub/pgxmock/v4"
)

func TestSettlementSourceBankAccountQueryScopesLinkedGLAccount(t *testing.T) {
	for _, fragment := range []string{
		"JOIN accounts a ON a.id = ba.gl_account_id",
		"ba.company_id = $2",
		"ba.is_active",
		"a.is_active",
		"(a.company_id = $2 OR a.company_id IS NULL)",
	} {
		if !strings.Contains(settlementSourceBankAccountSQL, fragment) {
			t.Fatalf("settlement source-bank query is missing scope guard %q: %s", fragment, settlementSourceBankAccountSQL)
		}
	}
}

func TestSettlementBeneficiaryQueryRevalidatesCurrentPaymentControls(t *testing.T) {
	for _, fragment := range []string{
		"id = $1",
		"company_id = $2",
		"supplier_id = $3",
		"UPPER(TRIM(currency)) = $4",
		"verification_status = 'VERIFIED'",
		"hold_payments = FALSE",
		"effective_from <= $5",
		"effective_to IS NULL OR effective_to >= $5",
		"FOR UPDATE",
	} {
		if !strings.Contains(settlementBeneficiarySQL, fragment) {
			t.Fatalf("settlement beneficiary query is missing current-control guard %q: %s", fragment, settlementBeneficiarySQL)
		}
	}
}

func TestSettlementBeneficiaryGuardFailsBeforeAccountingWrites(t *testing.T) {
	db, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgx mock pool: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	result := payments.SettlementResult{
		CompanyID: 7,
		ResultID:  "result-beneficiary-guard",
		InstructionReference: automation.ExternalReference{
			Connection: automation.ConnectionRef{CompanyID: 7, ConnectionID: 11, Provider: "iris"},
			ObjectType: "treasury_payment_item",
			ObjectID:   "treasury-batch-10-item-42",
		},
		Status: payments.ResultStatusSettled,
		State:  payments.StateSettled,
		SettledAmount: automation.ExactAmount{
			Amount:   accountingmoney.Must("10.00", 2),
			Currency: "USD",
		},
		SettledAt: now,
	}
	request := payments.SettlementEffectRequest{
		CompanyID: 7,
		EffectKey: "result-beneficiary-guard",
		Result:    result,
	}

	db.ExpectBegin()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	db.ExpectQuery("SELECT b\\.company_id, b\\.id, i\\.supplier_id, i\\.bank_account_id").WithArgs(int64(42), int64(7)).WillReturnRows(
		pgxmock.NewRows([]string{
			"company_id", "id", "supplier_id", "bank_account_id", "source_bank_account_id",
			"amount", "currency", "ap_invoice_id", "base_currency",
		}).AddRow(int64(7), int64(10), int64(99), int64(55), int64(66), "10.00", "USD", int64(101), "USD"),
	)
	db.ExpectQuery("SELECT id\\s+FROM treasury_supplier_bank_accounts").WithArgs(
		int64(55), int64(7), int64(99), "USD", pgxmock.AnyArg(),
	).WillReturnError(pgx.ErrNoRows)
	db.ExpectRollback()

	effects := TreasurySettlementEffects{now: func() time.Time { return now }}
	_, err = effects.ApplySettlementEffectsTx(ctx, tx, request)
	if !errors.Is(err, ErrSettlementBeneficiaryNotPayable) {
		t.Fatalf("expected beneficiary guard error, got %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback transaction: %v", err)
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected database interaction: %v", err)
	}
}

func TestTreasuryBatchItemIDsRequireCanonicalBatchAndItemBinding(t *testing.T) {
	tests := []struct {
		name      string
		objectID  string
		batchID   int64
		itemID    int64
		wantValid bool
	}{
		{name: "canonical", objectID: "treasury-batch-10-item-42", batchID: 10, itemID: 42, wantValid: true},
		{name: "missing prefix", objectID: "item-42"},
		{name: "extra suffix", objectID: "treasury-batch-10-item-42-extra"},
		{name: "non numeric batch", objectID: "treasury-batch-x-item-42"},
		{name: "zero item", objectID: "treasury-batch-10-item-0"},
		{name: "leading zero", objectID: "treasury-batch-010-item-042"},
		{name: "trailing whitespace", objectID: "treasury-batch-10-item-42 "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batchID, itemID, err := treasuryBatchItemIDs(tt.objectID)
			if tt.wantValid {
				if err != nil {
					t.Fatalf("treasuryBatchItemIDs() error = %v", err)
				}
				if batchID != tt.batchID || itemID != tt.itemID {
					t.Fatalf("treasuryBatchItemIDs() = (%d, %d), want (%d, %d)", batchID, itemID, tt.batchID, tt.itemID)
				}
				return
			}
			if !errors.Is(err, payments.ErrSettlementResultReferenceMismatch) {
				t.Fatalf("treasuryBatchItemIDs() error = %v, want reference mismatch", err)
			}
		})
	}
}

func TestSettlementEffectsRejectsItemFromDifferentBatch(t *testing.T) {
	db, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgx mock pool: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	result := payments.SettlementResult{
		CompanyID: 7,
		ResultID:  "result-batch-binding",
		InstructionReference: automation.ExternalReference{
			Connection: automation.ConnectionRef{CompanyID: 7, ConnectionID: 11, Provider: "iris"},
			ObjectType: "treasury_payment_item",
			ObjectID:   "treasury-batch-10-item-42",
		},
		Status: payments.ResultStatusSettled,
		State:  payments.StateSettled,
		SettledAmount: automation.ExactAmount{
			Amount:   accountingmoney.Must("10.00", 2),
			Currency: "USD",
		},
		SettledAt: time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC),
	}
	request := payments.SettlementEffectRequest{CompanyID: 7, EffectKey: result.ResultID, Result: result}

	db.ExpectBegin()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	db.ExpectQuery("SELECT b\\.company_id, b\\.id, i\\.supplier_id, i\\.bank_account_id").WithArgs(int64(42), int64(7)).WillReturnRows(
		pgxmock.NewRows([]string{
			"company_id", "id", "supplier_id", "bank_account_id", "source_bank_account_id",
			"amount", "currency", "ap_invoice_id", "base_currency",
		}).AddRow(int64(7), int64(11), int64(99), int64(55), int64(66), "10.00", "USD", int64(101), "USD"),
	)
	db.ExpectRollback()

	effects := TreasurySettlementEffects{now: func() time.Time { return result.SettledAt }}
	_, err = effects.ApplySettlementEffectsTx(ctx, tx, request)
	if !errors.Is(err, payments.ErrSettlementResultReferenceMismatch) {
		t.Fatalf("expected batch binding mismatch, got %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback transaction: %v", err)
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected database interaction: %v", err)
	}
}
