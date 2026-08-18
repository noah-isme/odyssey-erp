package treasury

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/payments"
)

// TreasurySettlementEffects applies a confirmed Iris result to the bounded
// treasury accounting boundary. AP payment/allocation rows, the source-bank
// transaction, and the durable settlement-effect claim are committed by the
// same PostgreSQL transaction. The AP liability, provider-fee expense, and
// source-bank journal are posted in that transaction when mappings and an
// open accounting period are available.
type TreasurySettlementEffects struct{}

var _ payments.SettlementEffectsApplier = TreasurySettlementEffects{}

func NewTreasurySettlementEffects() TreasurySettlementEffects {
	return TreasurySettlementEffects{}
}

func (TreasurySettlementEffects) ApplySettlementEffectsTx(ctx context.Context, tx pgx.Tx, request payments.SettlementEffectRequest) ([]payments.SettlementEffectLink, error) {
	if tx == nil {
		return nil, fmt.Errorf("treasury: settlement effects transaction is required")
	}
	result := request.Result
	itemID, err := treasuryItemID(result.InstructionReference.ObjectID)
	if err != nil {
		return nil, err
	}
	if !result.SettledAmount.IsPositive() {
		return nil, fmt.Errorf("treasury: confirmed settlement amount must be positive")
	}
	if result.SettledAmount.Amount.Scale > 2 {
		return nil, fmt.Errorf("treasury: settlement amount scale %d exceeds AP/bank precision", result.SettledAmount.Amount.Scale)
	}
	if result.ProviderFee.IsPositive() && result.ProviderFee.Amount.Scale > 2 {
		return nil, fmt.Errorf("treasury: provider fee scale %d exceeds AP/bank precision", result.ProviderFee.Amount.Scale)
	}

	var (
		companyID     int64
		batchID       int64
		supplierID    int64
		beneficiaryID int64
		sourceBankID  int64
		itemAmount    string
		batchCurrency string
		invoiceID     pgtype.Int8
	)
	err = tx.QueryRow(ctx, `
		SELECT b.company_id, b.id, i.supplier_id, i.bank_account_id,
		       b.source_bank_account_id, i.amount::text, b.currency, i.ap_invoice_id
		FROM treasury_payment_batch_items i
		JOIN treasury_payment_batches b ON b.id = i.batch_id
		WHERE i.id = $1 AND i.status = 'ACTIVE' AND b.company_id = $2`, itemID, result.CompanyID).
		Scan(&companyID, &batchID, &supplierID, &beneficiaryID, &sourceBankID, &itemAmount, &batchCurrency, &invoiceID)
	if err != nil {
		return nil, fmt.Errorf("treasury: load settlement item %d: %w", itemID, err)
	}
	if companyID != result.CompanyID || sourceBankID <= 0 || beneficiaryID <= 0 {
		return nil, fmt.Errorf("treasury: settlement item is outside company scope")
	}
	if !invoiceID.Valid || invoiceID.Int64 <= 0 {
		return nil, fmt.Errorf("treasury: settlement item %d has no AP invoice", itemID)
	}
	if !strings.EqualFold(strings.TrimSpace(batchCurrency), strings.TrimSpace(result.SettledAmount.Currency)) {
		return nil, fmt.Errorf("treasury: settlement currency does not match batch currency")
	}
	itemMoney, err := exactTreasuryMoney(itemAmount)
	if err != nil {
		return nil, err
	}
	if result.SettledAmount.Amount.Cmp(itemMoney) > 0 {
		return nil, fmt.Errorf("treasury: settlement amount exceeds batch item amount")
	}

	var (
		sourceGLAccountID int64
		sourceCurrency    string
	)
	if err := tx.QueryRow(ctx, `
		SELECT gl_account_id, currency FROM bank_accounts
		WHERE id = $1 AND company_id = $2 AND is_active`, sourceBankID, result.CompanyID).Scan(&sourceGLAccountID, &sourceCurrency); err != nil {
		return nil, fmt.Errorf("treasury: source bank account is not company-scoped: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(sourceCurrency), strings.TrimSpace(result.SettledAmount.Currency)) {
		return nil, fmt.Errorf("treasury: source bank currency does not match settlement currency")
	}

	paidAt := result.SettledAt
	if paidAt.IsZero() {
		paidAt = time.Now().UTC()
	}
	number := settlementPaymentNumber(result.CompanyID, result.ResultID)
	amount := result.SettledAmount.Amount.String()
	var paymentID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO ap_payments (
			number, ap_invoice_id, supplier_id, amount, currency,
			original_currency_amount, base_currency, base_amount, fx_rate,
			fx_rate_date, fx_rate_source, fx_rate_locked_at,
			paid_at, method, note, created_at, updated_at
		) VALUES ($1, $2, $3, $4::numeric, $5, $4::numeric, $5, $4::numeric, 1,
		          $6::date, 'MIDTRANS_IRIS', $6, $6::date, 'MIDTRANS_IRIS', $7, NOW(), NOW())
		RETURNING id`, number, nullableInt8(invoiceID), supplierID, amount, strings.ToUpper(batchCurrency), paidAt, "settlement:"+result.ResultID).Scan(&paymentID)
	if err != nil {
		return nil, fmt.Errorf("treasury: create AP payment: %w", err)
	}

	links := []payments.SettlementEffectLink{{
		LinkType:   "settlement.ap_payment",
		EntityType: "ap_payment",
		EntityID:   strconv.FormatInt(paymentID, 10),
		Amount:     result.SettledAmount,
		Metadata: map[string]any{
			"batch_id":        batchID,
			"item_id":         itemID,
			"provider_result": result.ResultID,
		},
	}}

	if invoiceID.Valid {
		var invoiceCurrency, invoiceStatus string
		var invoiceBalance string
		var invoiceSupplier, invoiceCompany int64
		err = tx.QueryRow(ctx, `
			SELECT i.supplier_id, s.company_id, i.currency, i.status,
			       (i.total - COALESCE((SELECT SUM(pa.amount) FROM ap_payment_allocations pa WHERE pa.ap_invoice_id = i.id), 0))::text
			FROM ap_invoices i
			JOIN suppliers s ON s.id = i.supplier_id
			WHERE i.id = $1`, invoiceID.Int64).Scan(&invoiceSupplier, &invoiceCompany, &invoiceCurrency, &invoiceStatus, &invoiceBalance)
		if err != nil {
			return nil, fmt.Errorf("treasury: load AP invoice %d: %w", invoiceID.Int64, err)
		}
		if invoiceCompany != result.CompanyID || invoiceSupplier != supplierID || !strings.EqualFold(invoiceCurrency, batchCurrency) || invoiceStatus != "POSTED" {
			return nil, fmt.Errorf("treasury: AP invoice %d is not payable in this settlement scope", invoiceID.Int64)
		}
		balance, balanceErr := exactTreasuryMoney(invoiceBalance)
		if balanceErr != nil {
			return nil, balanceErr
		}
		if result.SettledAmount.Amount.Cmp(balance) > 0 {
			return nil, fmt.Errorf("treasury: settlement exceeds AP invoice %d balance", invoiceID.Int64)
		}
		var allocationID int64
		err = tx.QueryRow(ctx, `
			INSERT INTO ap_payment_allocations (
				ap_payment_id, ap_invoice_id, amount, currency,
				original_currency_amount, base_currency, base_amount, fx_rate,
				fx_rate_date, fx_rate_source, fx_rate_locked_at, created_at
			) VALUES ($1, $2, $3::numeric, $4, $3::numeric, $4, $3::numeric, 1,
			          $5::date, 'MIDTRANS_IRIS', $5, NOW())
			RETURNING id`, paymentID, invoiceID.Int64, amount, strings.ToUpper(batchCurrency), paidAt).Scan(&allocationID)
		if err != nil {
			return nil, fmt.Errorf("treasury: allocate AP payment: %w", err)
		}
		_, err = tx.Exec(ctx, `
			UPDATE ap_invoices SET status = 'PAID', updated_at = NOW()
			WHERE id = $1 AND status = 'POSTED' AND total <= COALESCE((SELECT SUM(amount) FROM ap_payment_allocations WHERE ap_invoice_id = $1), 0)`, invoiceID.Int64)
		if err != nil {
			return nil, fmt.Errorf("treasury: update AP invoice status: %w", err)
		}
		links = append(links, payments.SettlementEffectLink{
			LinkType:   "settlement.ap_allocation",
			EntityType: "ap_payment_allocation",
			EntityID:   strconv.FormatInt(allocationID, 10),
			Amount:     result.SettledAmount,
			Metadata: map[string]any{
				"invoice_id":      invoiceID.Int64,
				"provider_result": result.ResultID,
			},
		})
	}

	bankAmount := result.SettledAmount.Amount
	if result.ProviderFee.IsPositive() {
		bankAmount = bankAmount.Add(result.ProviderFee.Amount)
	}
	bankReference := "settlement:" + result.ResultID
	var bankTransactionID string
	err = tx.QueryRow(ctx, `
		INSERT INTO bank_transactions (
			bank_account_id, date, amount, description, reference, status,
			external_reference, fingerprint, created_at, updated_at
		) VALUES ($1, $2::date, -$3::numeric, $4, $5, 'CLEARED', $5, $5, NOW(), NOW())
		ON CONFLICT (bank_account_id, external_reference) WHERE external_reference IS NOT NULL
		DO UPDATE SET external_reference = EXCLUDED.external_reference
		RETURNING id::text`, sourceBankID, paidAt, bankAmount.String(), "Midtrans Iris settlement "+result.ResultID, bankReference).Scan(&bankTransactionID)
	if err != nil {
		return nil, fmt.Errorf("treasury: create source bank transaction: %w", err)
	}
	links = append(links, payments.SettlementEffectLink{
		LinkType:   "settlement.bank_transaction",
		EntityType: "bank_transaction",
		EntityID:   bankTransactionID,
		Metadata: map[string]any{
			"bank_account_id": sourceBankID,
			"amount":          bankAmount.String(),
			"provider_fee":    result.ProviderFee.Amount.String(),
			"provider_result": result.ResultID,
		},
	})
	journalID, err := postSettlementJournal(ctx, tx, result, paidAt, sourceGLAccountID, amount, bankAmount.String())
	if err != nil {
		return nil, err
	}
	links = append(links, payments.SettlementEffectLink{
		LinkType:   "settlement.gl_journal",
		EntityType: "journal_entry",
		EntityID:   strconv.FormatInt(journalID, 10),
		Amount:     result.SettledAmount,
		Metadata: map[string]any{
			"source_bank_account_id": sourceBankID,
			"provider_fee":           result.ProviderFee.Amount.String(),
			"provider_result":        result.ResultID,
		},
	})
	return links, nil
}

func postSettlementJournal(ctx context.Context, tx pgx.Tx, result payments.SettlementResult, paidAt time.Time, sourceGLAccountID int64, settledAmount, bankAmount string) (int64, error) {
	var periodID int64
	if err := tx.QueryRow(ctx, `
		SELECT id FROM periods
		WHERE status IN ('OPEN', 'CLOSED') AND $1::date BETWEEN start_date AND end_date
		ORDER BY start_date DESC LIMIT 1`, paidAt).Scan(&periodID); err != nil {
		return 0, fmt.Errorf("treasury: find open accounting period: %w", err)
	}
	apAccountID, err := settlementMapping(ctx, tx, result.CompanyID, "ap.payment.ap")
	if err != nil {
		return 0, err
	}
	feeAccountID := int64(0)
	if result.ProviderFee.IsPositive() {
		feeAccountID, err = settlementMapping(ctx, tx, result.CompanyID, "ap.payment.fee")
		if err != nil {
			// Existing installations commonly seed the realized-loss mapping
			// before adding a dedicated payout-fee account. It is an expense
			// account and is therefore a safe compatibility fallback.
			feeAccountID, err = settlementMapping(ctx, tx, result.CompanyID, "ap.payment.fx_loss")
			if err != nil {
				return 0, fmt.Errorf("treasury: provider fee account mapping is required: %w", err)
			}
		}
	}
	sourceID := uuid.NewSHA1(uuid.Nil, []byte("MIDTRANS_IRIS:"+result.ResultID))
	var journalID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO journal_entries (period_id, date, source_module, source_id, memo, posted_by, status)
		VALUES ($1, $2::date, 'FINANCE.MIDTRANS_IRIS', $3, $4, NULL, 'POSTED')
		RETURNING id`, periodID, paidAt, sourceID, "Midtrans Iris settlement "+result.ResultID).Scan(&journalID)
	if err != nil {
		return 0, fmt.Errorf("treasury: create settlement journal: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO journal_lines (je_id, account_id, debit, credit, dim_company_id)
		VALUES ($1, $2, $3::numeric, 0, $4)`, journalID, apAccountID, settledAmount, result.CompanyID); err != nil {
		return 0, fmt.Errorf("treasury: debit AP settlement journal: %w", err)
	}
	if feeAccountID > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO journal_lines (je_id, account_id, debit, credit, dim_company_id)
			VALUES ($1, $2, $3::numeric, 0, $4)`, journalID, feeAccountID, result.ProviderFee.Amount.String(), result.CompanyID); err != nil {
			return 0, fmt.Errorf("treasury: debit provider fee journal: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO journal_lines (je_id, account_id, debit, credit, dim_company_id)
		VALUES ($1, $2, 0, $3::numeric, $4)`, journalID, sourceGLAccountID, bankAmount, result.CompanyID); err != nil {
		return 0, fmt.Errorf("treasury: credit source bank journal: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO source_links (module, ref_id, je_id)
		VALUES ('FINANCE.MIDTRANS_IRIS', $1, $2)
		ON CONFLICT (module, ref_id) DO NOTHING`, sourceID, journalID); err != nil {
		return 0, fmt.Errorf("treasury: link settlement journal: %w", err)
	}
	return journalID, nil
}

func settlementMapping(ctx context.Context, tx pgx.Tx, companyID int64, key string) (int64, error) {
	var accountID int64
	if err := tx.QueryRow(ctx, `
		SELECT am.account_id
		FROM account_mappings am
		JOIN accounts a ON a.id = am.account_id AND a.is_active
		WHERE am.module = 'AP' AND am.key = $1
		  AND (am.company_id = $2 OR am.company_id IS NULL)
		  AND (a.company_id = $2 OR a.company_id IS NULL)
		ORDER BY (am.company_id = $2) DESC, (a.company_id = $2) DESC
		LIMIT 1`, key, companyID).Scan(&accountID); err != nil {
		return 0, fmt.Errorf("treasury: account mapping %q: %w", key, err)
	}
	return accountID, nil
}

func treasuryItemID(objectID string) (int64, error) {
	parts := strings.Split(strings.TrimSpace(objectID), "-")
	if len(parts) < 2 {
		return 0, fmt.Errorf("treasury: unsupported settlement instruction %q", objectID)
	}
	id, err := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("treasury: unsupported settlement instruction %q", objectID)
	}
	return id, nil
}

func nullableInt8(value pgtype.Int8) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}

func settlementPaymentNumber(companyID int64, resultID string) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", companyID, resultID)))
	return "PAY-IRIS-" + hex.EncodeToString(digest[:])[:24]
}
