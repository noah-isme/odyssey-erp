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
	default:
		return nil, fmt.Errorf("unsupported forecast source type %s", r.kind)
	}
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
		       (i.total - COALESCE(SUM(pa.amount), 0))::text
		FROM ap_invoices i
		JOIN suppliers s ON s.id = i.supplier_id
		LEFT JOIN ap_payment_allocations pa ON pa.ap_invoice_id = i.id
		WHERE s.company_id = $1
		  AND i.status = 'POSTED'
		  AND COALESCE(i.due_at, i.issued_at) < $3
		GROUP BY i.id, i.currency, i.due_at, i.issued_at, i.total
		HAVING i.total - COALESCE(SUM(pa.amount), 0) > 0
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
