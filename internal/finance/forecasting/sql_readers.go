package forecasting

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
)

// SQLSourceReader reads committed cash sources from the owning finance tables.
// It is intentionally a storage adapter: the forecast engine consumes only
// ExpectedCashFlow values and never sees sqlc rows or provider-specific types.
type SQLSourceReader struct {
	db   *pgxpool.Pool
	kind SourceType
	name string
}

// NewDatabaseReaders wires the production forecast inputs. MockReader remains
// available to unit tests but is not used by either application binary.
func NewDatabaseReaders(db *pgxpool.Pool) []SourceReader {
	return []SourceReader{
		&SQLSourceReader{db: db, kind: SourceTypeBankBalance, name: "bank-balances"},
		&SQLSourceReader{db: db, kind: SourceTypeOpenAR, name: "open-ar"},
		&SQLSourceReader{db: db, kind: SourceTypePostedAP, name: "posted-ap"},
		&SQLSourceReader{db: db, kind: SourceTypeApprovedPayroll, name: "posted-payroll"},
		&SQLSourceReader{db: db, kind: SourceTypeApprovedPayment, name: "approved-payments"},
		&SQLSourceReader{db: db, kind: SourceTypeApprovedPO, name: "approved-pos"},
		&SQLSourceReader{db: db, kind: SourceTypeTaxObligation, name: "tax-obligations"},
	}
}

func (r *SQLSourceReader) Name() string { return r.name }

func (r *SQLSourceReader) CompanyBaseCurrency(ctx context.Context, companyID int64) (string, error) {
	if r == nil || r.db == nil {
		return "", fmt.Errorf("forecast source reader database is not configured")
	}
	var currency string
	if err := r.db.QueryRow(ctx, `SELECT base_currency FROM companies WHERE id = $1`, companyID).Scan(&currency); err != nil {
		return "", err
	}
	return strings.ToUpper(strings.TrimSpace(currency)), nil
}

func (r *SQLSourceReader) ReadExpectedFlows(ctx context.Context, companyID int64, fromDate, toDate time.Time) ([]ExpectedCashFlow, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("forecast source reader database is not configured")
	}
	switch r.kind {
	case SourceTypeBankBalance:
		return r.readBankBalances(ctx, companyID, fromDate)
	case SourceTypeOpenAR:
		return r.readOpenAR(ctx, companyID, fromDate, toDate)
	case SourceTypePostedAP:
		return r.readPostedAP(ctx, companyID, fromDate, toDate)
	case SourceTypeApprovedPayroll:
		return r.readPayroll(ctx, companyID, fromDate, toDate)
	case SourceTypeApprovedPayment:
		return r.readApprovedPayments(ctx, companyID, fromDate, toDate)
	case SourceTypeApprovedPO:
		return r.readApprovedPOs(ctx, companyID, fromDate, toDate)
	case SourceTypeTaxObligation:
		return r.readTaxObligations(ctx, companyID, fromDate, toDate)
	default:
		return nil, fmt.Errorf("unsupported forecast source type %s", r.kind)
	}
}

func (r *SQLSourceReader) readApprovedPayments(ctx context.Context, companyID int64, fromDate, toDate time.Time) ([]ExpectedCashFlow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT b.id,
		       b.currency,
		       GREATEST(COALESCE(b.approved_at, b.created_at)::date, $2::date),
		       b.total_amount::text
		FROM treasury_payment_batches b
		WHERE b.company_id = $1
		  AND b.status IN ('APPROVED', 'EXPORTED')
		  AND COALESCE(b.approved_at, b.created_at)::date < $3
		  AND b.total_amount > 0
		ORDER BY COALESCE(b.approved_at, b.created_at), b.id`, companyID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ExpectedCashFlow
	for rows.Next() {
		var id int64
		var currency, amountText string
		var expectedDate time.Time
		if err := rows.Scan(&id, &currency, &expectedDate, &amountText); err != nil {
			return nil, err
		}
		amount, err := exactAmount("-"+amountText, currency)
		if err != nil {
			return nil, fmt.Errorf("payment batch %d: %w", id, err)
		}
		result = append(result, ExpectedCashFlow{
			SourceType: SourceTypeApprovedPayment,
			SourceRef:  fmt.Sprintf("payment-batch:%d", id),
			Amount:     amount,
			Currency:   strings.ToUpper(strings.TrimSpace(currency)),
			Date:       dateOnlyUTC(expectedDate),
			Certainty:  CertaintyCommitted,
		})
	}
	return result, rows.Err()
}

func (r *SQLSourceReader) readApprovedPOs(ctx context.Context, companyID int64, fromDate, toDate time.Time) ([]ExpectedCashFlow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.id,
		       p.currency,
		       GREATEST(COALESCE(p.expected_date, p.approved_at::date, p.created_at::date), $2::date),
		       SUM(GREATEST(progress.ordered_amount - progress.invoiced_amount, 0))::text
		FROM pos p
		JOIN po_line_progress progress ON progress.po_id = p.id
		WHERE p.company_id = $1
		  AND p.status = 'APPROVED'
		  AND COALESCE(p.expected_date, p.approved_at::date, p.created_at::date) < $3
		GROUP BY p.id, p.currency, p.expected_date, p.approved_at, p.created_at
		HAVING SUM(GREATEST(progress.ordered_amount - progress.invoiced_amount, 0)) > 0
		ORDER BY COALESCE(p.expected_date, p.approved_at::date, p.created_at::date), p.id`, companyID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ExpectedCashFlow
	for rows.Next() {
		var id int64
		var currency, amountText string
		var expectedDate time.Time
		if err := rows.Scan(&id, &currency, &expectedDate, &amountText); err != nil {
			return nil, err
		}
		amount, err := exactAmount("-"+amountText, currency)
		if err != nil {
			return nil, fmt.Errorf("purchase order %d: %w", id, err)
		}
		result = append(result, ExpectedCashFlow{
			SourceType: SourceTypeApprovedPO,
			SourceRef:  fmt.Sprintf("purchase-order:%d", id),
			Amount:     amount,
			Currency:   strings.ToUpper(strings.TrimSpace(currency)),
			Date:       dateOnlyUTC(expectedDate),
			Certainty:  CertaintyProbable,
		})
	}
	return result, rows.Err()
}

func (r *SQLSourceReader) readTaxObligations(ctx context.Context, companyID int64, fromDate, toDate time.Time) ([]ExpectedCashFlow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT tp.id,
		       c.base_currency,
		       ap.end_date,
		       SUM(tle.tax_amount * tle.sign)::text
		FROM tax_periods tp
		JOIN accounting_periods ap ON ap.id = tp.accounting_period_id
		JOIN companies c ON c.id = tp.company_id
		JOIN tax_ledger_entries tle ON tle.tax_period_id = tp.id
		WHERE tp.company_id = $1
		  AND tp.status = 'OPEN'
		  AND ap.end_date >= $2::date
		  AND ap.end_date < $3
		GROUP BY tp.id, c.base_currency, ap.end_date
		HAVING SUM(tle.tax_amount * tle.sign) > 0
		ORDER BY ap.end_date, tp.id`, companyID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ExpectedCashFlow
	for rows.Next() {
		var id int64
		var currency, amountText string
		var dueDate time.Time
		if err := rows.Scan(&id, &currency, &dueDate, &amountText); err != nil {
			return nil, err
		}
		amount, err := exactAmount("-"+amountText, currency)
		if err != nil {
			return nil, fmt.Errorf("tax period %d: %w", id, err)
		}
		result = append(result, ExpectedCashFlow{
			SourceType: SourceTypeTaxObligation,
			SourceRef:  fmt.Sprintf("tax-period:%d", id),
			Amount:     amount,
			Currency:   strings.ToUpper(strings.TrimSpace(currency)),
			Date:       dateOnlyUTC(dueDate),
			Certainty:  CertaintyCommitted,
		})
	}
	return result, rows.Err()
}

func (r *SQLSourceReader) readBankBalances(ctx context.Context, companyID int64, asOf time.Time) ([]ExpectedCashFlow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT ba.id,
		       ba.currency,
		       (ba.initial_balance + COALESCE(SUM(CASE WHEN bt.status IN ('CLEARED', 'RECONCILED') THEN bt.amount ELSE 0 END), 0))::text
		FROM bank_accounts ba
		LEFT JOIN bank_transactions bt ON bt.bank_account_id = ba.id AND bt.date <= $2::date
		WHERE ba.company_id = $1 AND ba.is_active = TRUE
		GROUP BY ba.id, ba.currency, ba.initial_balance
		ORDER BY ba.id`, companyID, asOf)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ExpectedCashFlow
	for rows.Next() {
		var id int64
		var currency, amountText string
		if err := rows.Scan(&id, &currency, &amountText); err != nil {
			return nil, err
		}
		amount, err := exactAmount(amountText, currency)
		if err != nil {
			return nil, fmt.Errorf("bank account %d: %w", id, err)
		}
		result = append(result, ExpectedCashFlow{
			SourceType: SourceTypeBankBalance,
			SourceRef:  fmt.Sprintf("bank-account:%d", id),
			Amount:     amount,
			Currency:   strings.ToUpper(strings.TrimSpace(currency)),
			Date:       asOf,
			Certainty:  CertaintyCommitted,
		})
	}
	return result, rows.Err()
}

func (r *SQLSourceReader) readOpenAR(ctx context.Context, companyID int64, fromDate, toDate time.Time) ([]ExpectedCashFlow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT i.id,
		       i.currency,
		       GREATEST(i.due_at::date, $2::date),
		       (i.total - COALESCE(SUM(pa.amount), 0))::text
		FROM ar_invoices i
		JOIN customers c ON c.id = i.customer_id
		LEFT JOIN ar_payment_allocations pa ON pa.ar_invoice_id = i.id
		WHERE c.company_id = $1
		  AND i.status = 'POSTED'
		  AND i.due_at < $3
		GROUP BY i.id, i.currency, i.due_at, i.total
		HAVING i.total - COALESCE(SUM(pa.amount), 0) > 0
		ORDER BY i.due_at, i.id`, companyID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ExpectedCashFlow
	for rows.Next() {
		var id int64
		var currency, amountText string
		var dueDate time.Time
		if err := rows.Scan(&id, &currency, &dueDate, &amountText); err != nil {
			return nil, err
		}
		amount, err := exactAmount(amountText, currency)
		if err != nil {
			return nil, fmt.Errorf("AR invoice %d: %w", id, err)
		}
		result = append(result, ExpectedCashFlow{
			SourceType: SourceTypeOpenAR,
			SourceRef:  fmt.Sprintf("ar-invoice:%d", id),
			Amount:     amount,
			Currency:   strings.ToUpper(strings.TrimSpace(currency)),
			Date:       dateOnlyUTC(dueDate),
			Certainty:  CertaintyCommitted,
		})
	}
	return result, rows.Err()
}

func (r *SQLSourceReader) readPostedAP(ctx context.Context, companyID int64, fromDate, toDate time.Time) ([]ExpectedCashFlow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT i.id,
		       i.currency,
		       GREATEST(COALESCE(i.due_at, i.issued_at)::date, $2::date),
		       (i.total - COALESCE(SUM(pa.amount), 0) -
		        COALESCE(approved.approved_amount, 0))::text
		FROM ap_invoices i
		JOIN suppliers s ON s.id = i.supplier_id
		LEFT JOIN ap_payment_allocations pa ON pa.ap_invoice_id = i.id
		LEFT JOIN (
			SELECT bi.ap_invoice_id, SUM(bi.amount) AS approved_amount
			FROM treasury_payment_batch_items bi
			JOIN treasury_payment_batches b ON b.id = bi.batch_id
			WHERE bi.status = 'ACTIVE'
			  AND b.status IN ('APPROVED', 'EXPORTED')
			  AND bi.ap_invoice_id IS NOT NULL
			GROUP BY bi.ap_invoice_id
		) approved ON approved.ap_invoice_id = i.id
		WHERE s.company_id = $1
		  AND i.status = 'POSTED'
		  AND COALESCE(i.due_at, i.issued_at) < $3
		GROUP BY i.id, i.currency, i.due_at, i.issued_at, i.total, approved.approved_amount
		HAVING i.total - COALESCE(SUM(pa.amount), 0) -
		       COALESCE(approved.approved_amount, 0) > 0
		ORDER BY COALESCE(i.due_at, i.issued_at), i.id`, companyID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ExpectedCashFlow
	for rows.Next() {
		var id int64
		var currency, amountText string
		var dueDate time.Time
		if err := rows.Scan(&id, &currency, &dueDate, &amountText); err != nil {
			return nil, err
		}
		amount, err := exactAmount("-"+amountText, currency)
		if err != nil {
			return nil, fmt.Errorf("AP invoice %d: %w", id, err)
		}
		result = append(result, ExpectedCashFlow{
			SourceType: SourceTypePostedAP,
			SourceRef:  fmt.Sprintf("ap-invoice:%d", id),
			Amount:     amount,
			Currency:   strings.ToUpper(strings.TrimSpace(currency)),
			Date:       dateOnlyUTC(dueDate),
			Certainty:  CertaintyCommitted,
		})
	}
	return result, rows.Err()
}

func (r *SQLSourceReader) readPayroll(ctx context.Context, companyID int64, fromDate, toDate time.Time) ([]ExpectedCashFlow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT r.id,
		       p.pay_date,
		       policy.currency,
		       SUM(l.net_pay)::text
		FROM payroll_runs r
		JOIN payroll_periods p ON p.id = r.period_id
		JOIN payroll_company_policies policy ON policy.id = r.company_policy_id
		JOIN payroll_run_lines l ON l.run_id = r.id
		WHERE r.company_id = $1
		  AND r.status = 'POSTED'
		  AND p.pay_date >= $2
		  AND p.pay_date < $3
		GROUP BY r.id, p.pay_date, policy.currency
		ORDER BY p.pay_date, r.id`, companyID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ExpectedCashFlow
	for rows.Next() {
		var id int64
		var payDate time.Time
		var currency, amountText string
		if err := rows.Scan(&id, &payDate, &currency, &amountText); err != nil {
			return nil, err
		}
		amount, err := exactAmount("-"+amountText, currency)
		if err != nil {
			return nil, fmt.Errorf("payroll run %d: %w", id, err)
		}
		result = append(result, ExpectedCashFlow{
			SourceType: SourceTypeApprovedPayroll,
			SourceRef:  fmt.Sprintf("payroll-run:%d", id),
			Amount:     amount,
			Currency:   strings.ToUpper(strings.TrimSpace(currency)),
			Date:       dateOnlyUTC(payDate),
			Certainty:  CertaintyCommitted,
		})
	}
	return result, rows.Err()
}

func exactAmount(value, currency string) (automation.ExactAmount, error) {
	currency, err := normalizeCurrency(currency)
	if err != nil {
		return automation.ExactAmount{}, err
	}
	money, err := accountingmoney.Parse(value, 4)
	if err != nil {
		return automation.ExactAmount{}, err
	}
	return automation.ExactAmount{Amount: money, Currency: currency}, nil
}

func normalizeCurrency(currency string) (string, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if len(currency) != 3 {
		return "", fmt.Errorf("invalid currency %q", currency)
	}
	return currency, nil
}
