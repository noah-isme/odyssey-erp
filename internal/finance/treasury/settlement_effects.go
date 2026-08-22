package treasury

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/payments"
	"github.com/odyssey-erp/odyssey-erp/internal/fx"
)

const settlementFXMaxAge = 48 * time.Hour

var (
	// A settlement must stop before creating any financial row when its base
	// currency valuation cannot be proven from a recent daily rate.
	ErrSettlementFXRateRequired = errors.New("treasury: settlement-date FX rate is required")
	ErrSettlementFXRateStale    = errors.New("treasury: settlement-date FX rate is stale")
)

// SettlementFXResolver is an optional transaction-aware valuation port. The
// default adapter resolves from fx_daily_rates directly, while deployments
// can inject the same resolver used by AP/AR without changing the transaction
// boundary.
type SettlementFXResolver interface {
	ResolveTx(context.Context, pgx.Tx, string, string, time.Time) (fx.FXQuote, error)
}

// SettlementFXResolverFunc adapts a function to SettlementFXResolver.
type SettlementFXResolverFunc func(context.Context, pgx.Tx, string, string, time.Time) (fx.FXQuote, error)

func (f SettlementFXResolverFunc) ResolveTx(ctx context.Context, tx pgx.Tx, base, quote string, date time.Time) (fx.FXQuote, error) {
	if f == nil {
		return fx.FXQuote{}, ErrSettlementFXRateRequired
	}
	return f(ctx, tx, base, quote, date)
}

// TreasurySettlementEffects applies a confirmed Iris result to the bounded
// treasury accounting boundary. AP payment/allocation rows, the source-bank
// transaction, and the durable settlement-effect claim are committed by the
// same PostgreSQL transaction. The AP liability, provider-fee expense, and
// source-bank journal are posted in that transaction when mappings and an
// open accounting period are available.
type TreasurySettlementEffects struct {
	fxResolver SettlementFXResolver
	now        func() time.Time
}

var _ payments.SettlementEffectsApplier = TreasurySettlementEffects{}

func NewTreasurySettlementEffects() TreasurySettlementEffects {
	return TreasurySettlementEffects{now: time.Now}
}

// NewTreasurySettlementEffectsWithFXResolver keeps FX resolution under the
// same transaction as AP/GL/bank effects while preserving the no-argument
// constructor used by existing app wiring.
func NewTreasurySettlementEffectsWithFXResolver(resolver SettlementFXResolver) TreasurySettlementEffects {
	return TreasurySettlementEffects{fxResolver: resolver, now: time.Now}
}

func (effects TreasurySettlementEffects) ApplySettlementEffectsTx(ctx context.Context, tx pgx.Tx, request payments.SettlementEffectRequest) ([]payments.SettlementEffectLink, error) {
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
	settledCurrency, err := fx.Currency(result.SettledAmount.Currency)
	if err != nil {
		return nil, fmt.Errorf("treasury: settlement currency: %w", err)
	}
	paidAt := result.SettledAt
	if paidAt.IsZero() {
		now := effects.now
		if now == nil {
			now = time.Now
		}
		paidAt = now().UTC()
	}

	var (
		companyID     int64
		batchID       int64
		supplierID    int64
		beneficiaryID int64
		sourceBankID  int64
		itemAmount    string
		batchCurrency string
		baseCurrency  string
		invoiceID     pgtype.Int8
	)
	err = tx.QueryRow(ctx, `
		SELECT b.company_id, b.id, i.supplier_id, i.bank_account_id,
		       b.source_bank_account_id, i.amount::text, b.currency, i.ap_invoice_id,
		       c.base_currency
		FROM treasury_payment_batch_items i
		JOIN treasury_payment_batches b ON b.id = i.batch_id
		JOIN companies c ON c.id = b.company_id
		WHERE i.id = $1 AND i.status = 'ACTIVE' AND b.company_id = $2
		FOR UPDATE OF i, b`, itemID, result.CompanyID).
		Scan(&companyID, &batchID, &supplierID, &beneficiaryID, &sourceBankID, &itemAmount, &batchCurrency, &invoiceID, &baseCurrency)
	if err != nil {
		return nil, fmt.Errorf("treasury: load settlement item %d: %w", itemID, err)
	}
	if companyID != result.CompanyID || sourceBankID <= 0 || beneficiaryID <= 0 {
		return nil, fmt.Errorf("treasury: settlement item is outside company scope")
	}
	if !invoiceID.Valid || invoiceID.Int64 <= 0 {
		return nil, fmt.Errorf("treasury: settlement item %d has no AP invoice", itemID)
	}
	batchCurrency, err = fx.Currency(batchCurrency)
	if err != nil {
		return nil, fmt.Errorf("treasury: batch currency: %w", err)
	}
	baseCurrency, err = fx.Currency(baseCurrency)
	if err != nil {
		return nil, fmt.Errorf("treasury: company base currency: %w", err)
	}
	if batchCurrency != settledCurrency {
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
	sourceCurrency, err = fx.Currency(sourceCurrency)
	if err != nil {
		return nil, fmt.Errorf("treasury: source bank currency: %w", err)
	}
	if sourceCurrency != settledCurrency {
		return nil, fmt.Errorf("treasury: source bank currency does not match settlement currency")
	}

	quote, err := resolveSettlementFX(ctx, tx, effects.fxResolver, baseCurrency, settledCurrency, paidAt)
	if err != nil {
		return nil, err
	}
	settledDecimal, err := fx.ParseDecimal(result.SettledAmount.Amount.String())
	if err != nil {
		return nil, fmt.Errorf("treasury: settlement amount: %w", err)
	}
	settlementBaseAmount, err := fx.CalculateBaseAmount(settledDecimal, quote.Rate)
	if err != nil {
		return nil, fmt.Errorf("treasury: settlement valuation: %w", err)
	}
	providerFeeDecimal := fx.MustDecimal("0")
	providerFeeBaseAmount := fx.MustDecimal("0")
	if result.ProviderFee.IsPositive() {
		providerFeeDecimal, err = fx.ParseDecimal(result.ProviderFee.Amount.String())
		if err != nil {
			return nil, fmt.Errorf("treasury: provider fee: %w", err)
		}
		providerFeeBaseAmount, err = fx.CalculateBaseAmount(providerFeeDecimal, quote.Rate)
		if err != nil {
			return nil, fmt.Errorf("treasury: provider fee valuation: %w", err)
		}
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
		) VALUES ($1, $2, $3, $4::numeric, $5, $4::numeric, $6, $7::numeric, $8::numeric,
		          $9::date, $10, $11::timestamptz, $12::date, 'MIDTRANS_IRIS', $13, NOW(), NOW())
		RETURNING id`, number, nullableInt8(invoiceID), supplierID, amount, settledCurrency,
		baseCurrency, settlementBaseAmount.String(), quote.Rate.String(), quote.RateDate, quote.Source,
		paidAt, paidAt, "settlement:"+result.ResultID).Scan(&paymentID)
	if err != nil {
		return nil, fmt.Errorf("treasury: create AP payment: %w", err)
	}

	links := []payments.SettlementEffectLink{{
		LinkType:   "settlement.ap_payment",
		EntityType: "ap_payment",
		EntityID:   strconv.FormatInt(paymentID, 10),
		Amount:     result.SettledAmount,
		Metadata: map[string]any{
			"batch_id":             batchID,
			"item_id":              itemID,
			"provider_result":      result.ResultID,
			"base_currency":        baseCurrency,
			"base_amount":          settlementBaseAmount.String(),
			"settlement_rate":      quote.Rate.String(),
			"settlement_rate_date": quote.RateDate.Format("2006-01-02"),
		},
	}}

	var (
		invoiceCurrency           string
		invoiceRateText           string
		invoiceBaseCurrency       string
		invoiceBaseAmountText     string
		invoiceRate               = fx.MustDecimal("1")
		invoiceCarryingBaseAmount = fx.MustDecimal("0")
	)
	if invoiceID.Valid {
		var invoiceStatus string
		var invoiceBalance string
		var invoiceSupplier, invoiceCompany int64
		err = tx.QueryRow(ctx, `
			SELECT i.supplier_id, s.company_id, i.currency, i.status,
			       (i.total - COALESCE((SELECT SUM(pa.amount) FROM ap_payment_allocations pa WHERE pa.ap_invoice_id = i.id), 0))::text,
			       COALESCE(i.base_currency,''), COALESCE(i.base_amount::text,''), COALESCE(i.fx_rate::text,'')
			FROM ap_invoices i
			JOIN suppliers s ON s.id = i.supplier_id
			WHERE i.id = $1
			FOR UPDATE`, invoiceID.Int64).Scan(&invoiceSupplier, &invoiceCompany, &invoiceCurrency, &invoiceStatus, &invoiceBalance, &invoiceBaseCurrency, &invoiceBaseAmountText, &invoiceRateText)
		if err != nil {
			return nil, fmt.Errorf("treasury: load AP invoice %d: %w", invoiceID.Int64, err)
		}
		invoiceCurrency, err = fx.Currency(invoiceCurrency)
		if err != nil {
			return nil, fmt.Errorf("treasury: AP invoice currency: %w", err)
		}
		if invoiceCompany != result.CompanyID || invoiceSupplier != supplierID || invoiceCurrency != batchCurrency || invoiceStatus != "POSTED" {
			return nil, fmt.Errorf("treasury: AP invoice %d is not payable in this settlement scope", invoiceID.Int64)
		}
		if invoiceCurrency != baseCurrency {
			if invoiceBaseCurrency != baseCurrency || strings.TrimSpace(invoiceRateText) == "" || strings.TrimSpace(invoiceBaseAmountText) == "" {
				return nil, fmt.Errorf("treasury: AP invoice %d has no locked FX valuation", invoiceID.Int64)
			}
			invoiceRate, err = fx.ParseDecimal(invoiceRateText)
			if err != nil || invoiceRate.Cmp(fx.MustDecimal("0")) <= 0 {
				return nil, fmt.Errorf("treasury: AP invoice %d has invalid FX rate", invoiceID.Int64)
			}
			if _, err := fx.ParseDecimal(invoiceBaseAmountText); err != nil {
				return nil, fmt.Errorf("treasury: AP invoice %d has invalid base valuation", invoiceID.Int64)
			}
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
			) VALUES ($1, $2, $3::numeric, $4, $3::numeric, $5, $6::numeric, $7::numeric,
			          $8::date, $9, $10::timestamptz, NOW())
			RETURNING id`, paymentID, invoiceID.Int64, amount, batchCurrency, baseCurrency,
			settlementBaseAmount.String(), quote.Rate.String(), quote.RateDate, quote.Source, paidAt).Scan(&allocationID)
		if err != nil {
			return nil, fmt.Errorf("treasury: allocate AP payment: %w", err)
		}
		if _, err = tx.Exec(ctx, `
			UPDATE ap_invoices SET status = 'PAID', updated_at = NOW()
			WHERE id = $1 AND status = 'POSTED' AND total <= COALESCE((SELECT SUM(amount) FROM ap_payment_allocations WHERE ap_invoice_id = $1), 0)`, invoiceID.Int64); err != nil {
			return nil, fmt.Errorf("treasury: update AP invoice status: %w", err)
		}
		invoiceCarryingBaseAmount, err = fx.CalculateBaseAmount(settledDecimal, invoiceRate)
		if err != nil {
			return nil, fmt.Errorf("treasury: AP invoice %d carrying valuation: %w", invoiceID.Int64, err)
		}
		links = append(links, payments.SettlementEffectLink{
			LinkType:   "settlement.ap_allocation",
			EntityType: "ap_payment_allocation",
			EntityID:   strconv.FormatInt(allocationID, 10),
			Amount:     result.SettledAmount,
			Metadata: map[string]any{
				"invoice_id":      invoiceID.Int64,
				"provider_result": result.ResultID,
				"base_currency":   baseCurrency,
				"base_amount":     settlementBaseAmount.String(),
			},
		})
	}

	bankAmount := settledDecimal
	if result.ProviderFee.IsPositive() {
		bankAmount = bankAmount.Add(providerFeeDecimal)
	}
	journalID, err := postSettlementJournal(ctx, tx, result, paidAt, sourceGLAccountID, settlementBaseAmount.String(), providerFeeBaseAmount.String(), settlementBaseAmount.Add(providerFeeBaseAmount).String())
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
			"provider_fee":           providerFeeDecimal.String(),
			"settlement_base_amount": settlementBaseAmount.String(),
			"base_currency":          baseCurrency,
			"provider_result":        result.ResultID,
		},
	})
	if invoiceID.Valid {
		fxJournalID, fxErr := postAPRealizedFXJournal(ctx, tx, result.CompanyID, paymentID, invoiceID.Int64, result, paidAt, invoiceRate, quote.Rate, invoiceCarryingBaseAmount, settlementBaseAmount)
		if fxErr != nil {
			return nil, fxErr
		}
		if fxJournalID > 0 {
			links = append(links, payments.SettlementEffectLink{
				LinkType:   "settlement.fx_journal",
				EntityType: "journal_entry",
				EntityID:   strconv.FormatInt(fxJournalID, 10),
				Metadata: map[string]any{
					"invoice_id":          invoiceID.Int64,
					"realized_difference": settlementBaseAmount.Sub(invoiceCarryingBaseAmount).String(),
					"base_currency":       baseCurrency,
					"provider_result":     result.ResultID,
				},
			})
		}
	}

	bankReference := "settlement:" + result.ResultID
	var bankTransactionID string
	err = tx.QueryRow(ctx, `
		INSERT INTO bank_transactions (
			bank_account_id, date, amount, description, reference, status, gl_journal_id,
			external_reference, fingerprint, created_at, updated_at
		) VALUES ($1, $2::date, -$3::numeric, $4, $5, 'PENDING', $6, $5, $5, NOW(), NOW())
		ON CONFLICT (bank_account_id, external_reference) WHERE external_reference IS NOT NULL
		DO UPDATE SET external_reference = EXCLUDED.external_reference
		RETURNING id::text`, sourceBankID, paidAt, bankAmount.String(), "Midtrans Iris settlement "+result.ResultID, bankReference, journalID).Scan(&bankTransactionID)
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
			"currency":        settledCurrency,
			"status":          "PENDING",
			"provider_fee":    providerFeeDecimal.String(),
			"provider_result": result.ResultID,
		},
	})

	var taxCaptureID int64
	if invoiceID.Valid {
		if _, err := tx.Exec(ctx, `
			INSERT INTO tax_capture_outbox (source_type, source_id, actor_id)
			SELECT 'AP_PAYMENT', p.id, p.created_by
			FROM ap_payments p
			JOIN ap_invoices i ON i.id = $2
			JOIN tax_withholding_types w ON w.id = i.withholding_type_id
			WHERE p.id = $1 AND w.recognition_event = 'PAYMENT'
			ON CONFLICT (source_type, source_id) DO NOTHING`, paymentID, invoiceID.Int64); err != nil {
			return nil, fmt.Errorf("treasury: enqueue AP payment tax capture: %w", err)
		}
		if err := tx.QueryRow(ctx, `SELECT id FROM tax_capture_outbox WHERE source_type='AP_PAYMENT' AND source_id=$1`, paymentID).Scan(&taxCaptureID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("treasury: load AP payment tax capture: %w", err)
		}
	}
	if taxCaptureID > 0 {
		links = append(links, payments.SettlementEffectLink{
			LinkType:   "settlement.tax_capture",
			EntityType: "tax_capture_outbox",
			EntityID:   strconv.FormatInt(taxCaptureID, 10),
			Metadata: map[string]any{
				"payment_id":      paymentID,
				"provider_result": result.ResultID,
			},
		})
	}

	meta := map[string]any{
		"company_id":           result.CompanyID,
		"payment_id":           paymentID,
		"journal_id":           journalID,
		"bank_transaction_id":  bankTransactionID,
		"base_currency":        baseCurrency,
		"settlement_rate":      quote.Rate.String(),
		"settlement_rate_date": quote.RateDate.Format("2006-01-02"),
		"provider_result":      result.ResultID,
	}
	metaPayload, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("treasury: encode settlement audit: %w", err)
	}
	var auditID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO audit_logs (actor_id, action, entity, entity_id, meta, occurred_at)
		VALUES (NULL, 'settlement.apply', 'payment_settlement', $1, $2::jsonb, $3)
		RETURNING id`, result.ResultID, metaPayload, paidAt).Scan(&auditID); err != nil {
		return nil, fmt.Errorf("treasury: record settlement audit: %w", err)
	}
	links = append(links, payments.SettlementEffectLink{
		LinkType:   "settlement.audit",
		EntityType: "audit_log",
		EntityID:   strconv.FormatInt(auditID, 10),
		Metadata:   meta,
	})
	return links, nil
}

func resolveSettlementFX(ctx context.Context, tx pgx.Tx, resolver SettlementFXResolver, base, quote string, date time.Time) (fx.FXQuote, error) {
	base, err := fx.Currency(base)
	if err != nil {
		return fx.FXQuote{}, fmt.Errorf("treasury: base currency: %w", err)
	}
	quote, err = fx.Currency(quote)
	if err != nil {
		return fx.FXQuote{}, fmt.Errorf("treasury: quote currency: %w", err)
	}
	if base == quote {
		return fx.FXQuote{BaseCurrency: base, QuoteCurrency: quote, Rate: fx.MustDecimal("1"), RateDate: date, Source: "INTERNAL"}, nil
	}
	if resolver != nil {
		resolved, err := resolver.ResolveTx(ctx, tx, base, quote, date)
		if err != nil {
			return fx.FXQuote{}, fmt.Errorf("treasury: resolve settlement FX rate: %w", err)
		}
		return validateSettlementQuote(resolved, base, quote, date)
	}

	var (
		raw       string
		rateDate  time.Time
		source    string
		fetchedAt time.Time
		inverse   bool
	)
	err = tx.QueryRow(ctx, `
		WITH candidates AS (
			SELECT rate::text AS rate, rate_date, source, fetched_at, FALSE AS inverse, 0 AS priority
			FROM fx_daily_rates
			WHERE base_currency = $1 AND quote_currency = $2 AND rate_date <= $3::date
			UNION ALL
			SELECT rate::text AS rate, rate_date, source, fetched_at, TRUE AS inverse, 1 AS priority
			FROM fx_daily_rates
			WHERE base_currency = $2 AND quote_currency = $1 AND rate_date <= $3::date
		)
		SELECT rate, rate_date, source, fetched_at, inverse
		FROM candidates
		ORDER BY priority, rate_date DESC, fetched_at DESC
		LIMIT 1`, base, quote, date).Scan(&raw, &rateDate, &source, &fetchedAt, &inverse)
	if errors.Is(err, pgx.ErrNoRows) {
		return fx.FXQuote{}, fmt.Errorf("%w: %s/%s on %s", ErrSettlementFXRateRequired, base, quote, date.Format("2006-01-02"))
	}
	if err != nil {
		return fx.FXQuote{}, fmt.Errorf("treasury: load settlement FX rate: %w", err)
	}
	now := time.Now().UTC()
	if fetchedAt.IsZero() || now.Sub(fetchedAt) > settlementFXMaxAge {
		return fx.FXQuote{}, fmt.Errorf("%w: %s/%s", ErrSettlementFXRateStale, base, quote)
	}
	rate, err := fx.ParseDecimal(raw)
	if err != nil || rate.Cmp(fx.MustDecimal("0")) <= 0 {
		return fx.FXQuote{}, fmt.Errorf("%w: invalid %s/%s rate", ErrSettlementFXRateRequired, base, quote)
	}
	if inverse {
		rate = fx.MustDecimal("1").Div(rate).Round(10)
	}
	return validateSettlementQuote(fx.FXQuote{BaseCurrency: base, QuoteCurrency: quote, Rate: rate, RateDate: rateDate, Source: source}, base, quote, date)
}

func validateSettlementQuote(quote fx.FXQuote, base, currency string, settlementDate time.Time) (fx.FXQuote, error) {
	if quote.BaseCurrency != base || quote.QuoteCurrency != currency || quote.Rate.Cmp(fx.MustDecimal("0")) <= 0 || strings.TrimSpace(quote.Source) == "" {
		return fx.FXQuote{}, ErrSettlementFXRateRequired
	}
	if quote.RateDate.IsZero() || quote.RateDate.After(settlementDate) {
		return fx.FXQuote{}, fmt.Errorf("%w: rate date is after settlement date", ErrSettlementFXRateRequired)
	}
	return quote, nil
}

func postSettlementJournal(ctx context.Context, tx pgx.Tx, result payments.SettlementResult, paidAt time.Time, sourceGLAccountID int64, settledBaseAmount, providerFeeBaseAmount, bankBaseAmount string) (int64, error) {
	var periodID int64
	if err := tx.QueryRow(ctx, `
		SELECT id FROM periods
		WHERE status = 'OPEN' AND $1::date BETWEEN start_date AND end_date
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
	sourceID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("MIDTRANS_IRIS:%d:%s", result.CompanyID, result.ResultID)))
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
		VALUES ($1, $2, $3::numeric, 0, $4)`, journalID, apAccountID, settledBaseAmount, result.CompanyID); err != nil {
		return 0, fmt.Errorf("treasury: debit AP settlement journal: %w", err)
	}
	if feeAccountID > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO journal_lines (je_id, account_id, debit, credit, dim_company_id)
			VALUES ($1, $2, $3::numeric, 0, $4)`, journalID, feeAccountID, providerFeeBaseAmount, result.CompanyID); err != nil {
			return 0, fmt.Errorf("treasury: debit provider fee journal: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO journal_lines (je_id, account_id, debit, credit, dim_company_id)
		VALUES ($1, $2, 0, $3::numeric, $4)`, journalID, sourceGLAccountID, bankBaseAmount, result.CompanyID); err != nil {
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

func postAPRealizedFXJournal(ctx context.Context, tx pgx.Tx, companyID, paymentID, invoiceID int64, result payments.SettlementResult, paidAt time.Time, invoiceRate, paymentRate, carryingBaseAmount, settlementBaseAmount fx.Decimal) (int64, error) {
	difference := settlementBaseAmount.Sub(carryingBaseAmount).Round(2)
	if difference.IsZero() {
		return 0, nil
	}
	sourceKey := fx.PaymentFXSourceKey("AP", paymentID, invoiceID)
	var existingID int64
	if err := tx.QueryRow(ctx, `SELECT journal_entry_id FROM fx_journal_idempotency WHERE source_key=$1`, sourceKey).Scan(&existingID); err == nil {
		return existingID, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("treasury: load realized FX journal claim: %w", err)
	}
	var periodID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM periods WHERE status='OPEN' AND $1::date BETWEEN start_date AND end_date ORDER BY start_date DESC LIMIT 1`, paidAt).Scan(&periodID); err != nil {
		return 0, fmt.Errorf("treasury: find realized FX period: %w", err)
	}
	apAccountID, err := settlementMappingForModule(ctx, tx, companyID, "AP", "ap.invoice.ap")
	if err != nil {
		return 0, err
	}
	gainAccountID, err := settlementMappingForModule(ctx, tx, companyID, "FX", "fx.realized.gain")
	if err != nil {
		return 0, err
	}
	lossAccountID, err := settlementMappingForModule(ctx, tx, companyID, "FX", "fx.realized.loss")
	if err != nil {
		return 0, err
	}
	sourceID := uuid.NewSHA1(uuid.Nil, []byte(sourceKey))
	var journalID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO journal_entries (period_id, date, source_module, source_id, memo, posted_by, status)
		VALUES ($1, $2::date, 'AP.PAYMENT.FX', $3, $4, NULL, 'POSTED') RETURNING id`, periodID, paidAt, sourceID, "AP realized FX settlement "+result.ResultID).Scan(&journalID); err != nil {
		return 0, fmt.Errorf("treasury: create realized FX journal: %w", err)
	}
	zero := fx.MustDecimal("0")
	if difference.Cmp(zero) > 0 {
		if _, err := tx.Exec(ctx, `INSERT INTO journal_lines (je_id, account_id, debit, credit, dim_company_id) VALUES ($1,$2,$3::numeric,0,$4),($1,$5,0,$3::numeric,$4)`, journalID, lossAccountID, difference.String(), companyID, apAccountID); err != nil {
			return 0, fmt.Errorf("treasury: post realized FX loss: %w", err)
		}
	} else {
		amount := zero.Sub(difference)
		if _, err := tx.Exec(ctx, `INSERT INTO journal_lines (je_id, account_id, debit, credit, dim_company_id) VALUES ($1,$2,$3::numeric,0,$4),($1,$5,0,$3::numeric,$4)`, journalID, apAccountID, amount.String(), companyID, gainAccountID); err != nil {
			return 0, fmt.Errorf("treasury: post realized FX gain: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO source_links (module, ref_id, je_id) VALUES ('AP.PAYMENT.FX',$1,$2) ON CONFLICT (module,ref_id) DO NOTHING`, sourceID, journalID); err != nil {
		return 0, fmt.Errorf("treasury: link realized FX journal: %w", err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO fx_journal_idempotency (source_key,journal_entry_id) VALUES ($1,$2) ON CONFLICT (source_key) DO NOTHING RETURNING journal_entry_id`, sourceKey, journalID).Scan(&existingID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if lookupErr := tx.QueryRow(ctx, `SELECT journal_entry_id FROM fx_journal_idempotency WHERE source_key=$1`, sourceKey).Scan(&existingID); lookupErr == nil {
				return existingID, nil
			}
		}
		return 0, fmt.Errorf("treasury: claim realized FX journal: %w", err)
	}
	return existingID, nil
}

func settlementMapping(ctx context.Context, tx pgx.Tx, companyID int64, key string) (int64, error) {
	return settlementMappingForModule(ctx, tx, companyID, "AP", key)
}

func settlementMappingForModule(ctx context.Context, tx pgx.Tx, companyID int64, module, key string) (int64, error) {
	var accountID int64
	if err := tx.QueryRow(ctx, `
		SELECT am.account_id
		FROM account_mappings am
		JOIN accounts a ON a.id = am.account_id AND a.is_active
		WHERE am.module = $1 AND am.key = $2
		  AND (am.company_id = $3 OR am.company_id IS NULL)
		  AND (a.company_id = $3 OR a.company_id IS NULL)
		ORDER BY (am.company_id = $3) DESC, (a.company_id = $3) DESC
		LIMIT 1`, module, key, companyID).Scan(&accountID); err != nil {
		return 0, fmt.Errorf("treasury: account mapping %s.%q: %w", module, key, err)
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
