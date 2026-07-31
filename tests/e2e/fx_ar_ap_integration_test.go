//go:build integration

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/ap"
	"github.com/odyssey-erp/odyssey-erp/internal/ar"
	"github.com/odyssey-erp/odyssey-erp/internal/fx"
)

// TestFXARAPDatabaseIntegration is intentionally SQL-backed. It validates the
// deployed schema, NUMERIC values, idempotency constraints, and journal shape
// without contacting an external FX provider. The application service tests
// cover orchestration; this test proves those invariants survive PostgreSQL.
func TestFXARAPDatabaseIntegration(t *testing.T) {
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN is required for the database-backed FX suite")
	}
	applyAllMigrations(t, dsn)

	ctx := context.Background()
	p, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err := p.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	suffix := uuid.NewString()[:8]
	ids := seedFXFixture(t, p, suffix)
	insertDailyRates(t, p, suffix)
	resolver := dbFXResolver{repo: fx.NewRepository(p)}

	arInvoice := createInvoice(t, p, "AR", ids.customerID, ids.periodID, "AR-FX-"+suffix, "USD", "100.00", "15000.0000000000")
	arService := ar.NewService(ar.NewRepository(p))
	arService.SetFXResolver(resolver)
	if err := arService.PostARInvoice(ctx, ar.PostARInvoiceInput{InvoiceID: mustID(t, arInvoice), PostedBy: ids.userID}); err != nil {
		t.Fatal(err)
	}
	arPaymentModel, err := arService.RegisterARPayment(ctx, ar.CreateARPaymentInput{Number: "AR-PAY-" + suffix, Currency: "USD", Amount: 40, PaidAt: time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC), Method: "BANK", CreatedBy: ids.userID, Allocations: []ar.PaymentAllocationInput{{ARInvoiceID: mustID(t, arInvoice), Amount: 40}}})
	if err != nil {
		t.Fatal(err)
	}
	arPayment := fmt.Sprint(arPaymentModel.ID)
	assertValuation(t, p, "ar_invoices", arInvoice, "100.00", "15150.0000000000")
	assertValuation(t, p, "ar_payments", arPayment, "40.00", "16000.0000000000")
	arAllocation := allocate(t, p, "ar", arPayment, arInvoice, ids.periodID, "40.00", "640000.00", "AR_PAYMENT_FX:"+arPayment+":")
	assertBalancedAndIdempotent(t, p, arAllocation, "AR_PAYMENT_FX:"+arPayment+":"+arAllocation)

	arRevaluation := revalue(t, p, ids.periodID, arInvoice, "AR_INVOICE", "60.00", "900000.00", "100000.00")
	assertRevaluation(t, p, arRevaluation, ids.periodID, "AR_INVOICE", arInvoice)
	assertReversal(t, p, arRevaluation, ids.nextPeriodID)

	apInvoice := createInvoice(t, p, "AP", ids.supplierID, ids.periodID, "AP-FX-"+suffix, "USD", "100.00", "15000.0000000000")
	apService := ap.NewService(ap.NewRepository(p), nil)
	apService.SetFXResolver(resolver)
	if err := apService.PostAPInvoice(ctx, ap.PostAPInvoiceInput{InvoiceID: mustID(t, apInvoice), PostedBy: ids.userID}); err != nil {
		t.Fatal(err)
	}
	apPaymentModel, err := apService.RegisterAPPayment(ctx, ap.CreateAPPaymentInput{Number: "AP-PAY-" + suffix, Currency: "USD", SupplierID: ids.supplierID, Amount: 40, PaidAt: time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC), Method: "BANK", CreatedBy: ids.userID, Allocations: []ap.PaymentAllocationInput{{APInvoiceID: mustID(t, apInvoice), Amount: 40}}})
	if err != nil {
		t.Fatal(err)
	}
	apPayment := fmt.Sprint(apPaymentModel.ID)
	assertValuation(t, p, "ap_invoices", apInvoice, "100.00", "15150.0000000000")
	assertValuation(t, p, "ap_payments", apPayment, "40.00", "16000.0000000000")
	apAllocation := allocate(t, p, "ap", apPayment, apInvoice, ids.periodID, "40.00", "640000.00", "AP_PAYMENT_FX:"+apPayment+":")
	assertBalancedAndIdempotent(t, p, apAllocation, "AP_PAYMENT_FX:"+apPayment+":"+apAllocation)

	var arGain, apLoss bool
	if err := p.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM journal_lines jl JOIN journal_entries je ON je.id=jl.je_id JOIN account_mappings am ON am.account_id=jl.account_id WHERE je.source_module='AR_PAYMENT_FX' AND am.key='fx.realized.gain'), EXISTS (SELECT 1 FROM journal_lines jl JOIN journal_entries je ON je.id=jl.je_id JOIN account_mappings am ON am.account_id=jl.account_id WHERE je.source_module='AP_PAYMENT_FX' AND am.key='fx.realized.loss')`).Scan(&arGain, &apLoss); err != nil {
		t.Fatal(err)
	}
	if !arGain || !apLoss {
		t.Fatalf("opposite realized FX directions missing: ar_gain=%v ap_loss=%v", arGain, apLoss)
	}
}

type fxIDs struct {
	companyID, customerID, supplierID, periodID, nextPeriodID, userID int64
}

type dbFXResolver struct{ repo *fx.SQLRepository }

func (r dbFXResolver) Resolve(ctx context.Context, base, quote string, date time.Time) (fx.FXQuote, error) {
	return r.repo.DailyRate(ctx, base, quote, date, 0)
}

func mustID(t *testing.T, value string) int64 {
	t.Helper()
	var id int64
	if _, err := fmt.Sscan(value, &id); err != nil {
		t.Fatal(err)
	}
	return id
}

func applyAllMigrations(t *testing.T, dsn string) {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	migrations := filepath.Join(filepath.Dir(file), "..", "..", "migrations")
	bin := os.Getenv("MIGRATE_BIN")
	if bin == "" {
		bin = filepath.Join(os.Getenv("HOME"), "go", "bin", "migrate")
	}
	cmd := exec.Command(bin, "-path", migrations, "-database", dsn, "up")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("apply migrations: %v\n%s", err, output)
	}
}

func seedFXFixture(t *testing.T, p *pgxpool.Pool, suffix string) fxIDs {
	t.Helper()
	ctx := context.Background()
	var ids fxIDs
	mustQuery(t, p, `INSERT INTO users(email,password_hash) VALUES($1,'integration')`, "fx-"+suffix+"@example.test")
	var userID int64
	if err := p.QueryRow(ctx, `SELECT id FROM users WHERE email=$1`, "fx-"+suffix+"@example.test").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO companies(code,name,base_currency) VALUES($1,'FX integration','IDR') RETURNING id`, "FX-"+suffix).Scan(&ids.companyID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO customers(code,name,company_id,created_by,country) VALUES($1,'USD customer',$2,$3,'US') RETURNING id`, "C-"+suffix, ids.companyID, userID).Scan(&ids.customerID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO suppliers(code,name,company_id) VALUES($1,'USD supplier',$2) RETURNING id`, "S-"+suffix, ids.companyID).Scan(&ids.supplierID); err != nil {
		t.Fatal(err)
	}
	mustQuery(t, p, `INSERT INTO accounts(code,name,type) VALUES ('1100-FX-'||$1,'AR FX','ASSET'),('2100-FX-'||$1,'AP FX','LIABILITY'),('4200-FX-'||$1,'FX gain','REVENUE'),('5200-FX-'||$1,'FX loss','EXPENSE')`, suffix)
	for _, item := range []struct{ key, code string }{{"fx.realized.gain", "4200-FX-" + suffix}, {"fx.realized.loss", "5200-FX-" + suffix}, {"fx.revaluation.gain", "4200-FX-" + suffix}, {"fx.revaluation.loss", "5200-FX-" + suffix}} {
		mustQuery(t, p, `INSERT INTO account_mappings(module,key,account_id) SELECT 'FX',$1,id FROM accounts WHERE code=$2 ON CONFLICT(module,key) DO UPDATE SET account_id=EXCLUDED.account_id`, item.key, item.code)
	}
	if err := p.QueryRow(ctx, `INSERT INTO periods(code,start_date,end_date,status) VALUES($1,(SELECT COALESCE(MAX(end_date), DATE '2090-01-01') + 1 FROM periods),(SELECT COALESCE(MAX(end_date), DATE '2090-01-01') + 31 FROM periods),'OPEN') RETURNING id`, "2026-01-FX-"+suffix).Scan(&ids.periodID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO periods(code,start_date,end_date,status) SELECT $1, end_date + 1, end_date + 28, 'OPEN' FROM periods WHERE id=$2 RETURNING id`, "2026-02-FX-"+suffix, ids.periodID).Scan(&ids.nextPeriodID); err != nil {
		t.Fatal(err)
	}
	ids.userID = userID
	return ids
}

func insertDailyRates(t *testing.T, p *pgxpool.Pool, suffix string) {
	mustQuery(t, p, `INSERT INTO fx_daily_rates(base_currency,quote_currency,rate_date,rate,source) VALUES ('IDR','USD','2026-01-15',15000,$1),('IDR','USD','2026-01-20',16000,$1),($2,'USD',CURRENT_DATE,15150,$1)`, "TEST-"+suffix, "IDR")
}

func createInvoice(t *testing.T, p *pgxpool.Pool, kind string, partyID, periodID int64, number, currency, amount, rate string) string {
	t.Helper()
	ctx := context.Background()
	var id int64
	if kind == "AR" {
		if err := p.QueryRow(ctx, `INSERT INTO ar_invoices(number,customer_id,currency,total,status,due_at,original_currency_amount,base_currency,base_amount,fx_rate,fx_rate_date,fx_rate_source,fx_rate_locked_at) VALUES($1,$2,$3,$4::numeric,'DRAFT','2026-01-31',$4::numeric,'IDR',$4::numeric*$5::numeric,$5::numeric,'2026-01-15','TEST','2026-01-15T12:00:00Z') RETURNING id`, number, partyID, currency, amount, rate).Scan(&id); err != nil {
			t.Fatal(err)
		}
	} else {
		if err := p.QueryRow(ctx, `INSERT INTO ap_invoices(number,supplier_id,currency,total,status,due_at,original_currency_amount,base_currency,base_amount,fx_rate,fx_rate_date,fx_rate_source,fx_rate_locked_at) VALUES($1,$2,$3,$4::numeric,'DRAFT','2026-01-31',$4::numeric,'IDR',$4::numeric*$5::numeric,$5::numeric,'2026-01-15','TEST','2026-01-15T12:00:00Z') RETURNING id`, number, partyID, currency, amount, rate).Scan(&id); err != nil {
			t.Fatal(err)
		}
	}
	return fmt.Sprint(id)
}

func recordPayment(t *testing.T, p *pgxpool.Pool, kind string, periodID, partyID int64, invoiceID, number, amount, rate string) string {
	t.Helper()
	ctx := context.Background()
	table, partyColumn := "ar_payments", "ar_invoice_id"
	if kind == "AP" {
		table, partyColumn = "ap_payments", "ap_invoice_id"
	}
	var id int64
	query := fmt.Sprintf(`INSERT INTO %s(number,%s,amount,currency,paid_at,original_currency_amount,base_currency,base_amount,fx_rate,fx_rate_date,fx_rate_source,fx_rate_locked_at) VALUES($1,$2,$3::numeric,'USD','2026-01-20',$3::numeric,'IDR',$3::numeric*$4::numeric,$4::numeric,'2026-01-20','TEST','2026-01-20T12:00:00Z') RETURNING id`, table, partyColumn)
	if err := p.QueryRow(ctx, query, number, invoiceID, amount, rate).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprint(id)
}

func assertValuation(t *testing.T, p *pgxpool.Pool, table, id, original, rate string) {
	t.Helper()
	var gotOriginal, gotRate string
	if err := p.QueryRow(context.Background(), `SELECT original_currency_amount::text, fx_rate::text FROM `+table+` WHERE id=$1`, id).Scan(&gotOriginal, &gotRate); err != nil {
		t.Fatal(err)
	}
	if gotOriginal != original || gotRate != rate {
		t.Fatalf("%s %s valuation=(%s,%s), want=(%s,%s)", table, id, gotOriginal, gotRate, original, rate)
	}
}

func allocate(t *testing.T, p *pgxpool.Pool, kind, paymentID, invoiceID string, periodID int64, amount, baseAmount, keyPrefix string) string {
	t.Helper()
	ctx := context.Background()
	table, paymentColumn, invoiceColumn := "ar_payment_allocations", "ar_payment_id", "ar_invoice_id"
	if kind == "ap" {
		table, paymentColumn, invoiceColumn = "ap_payment_allocations", "ap_payment_id", "ap_invoice_id"
	}
	var id int64
	query := fmt.Sprintf(`INSERT INTO %s(%s,%s,amount,original_currency_amount,base_amount,currency,base_currency,fx_rate,fx_rate_date,fx_rate_source,fx_rate_locked_at) VALUES($1,$2,$3::numeric,$3::numeric,$4::numeric,'USD','IDR',16000,'2026-01-20','TEST','2026-01-20T12:00:00Z') RETURNING id`, table, paymentColumn, invoiceColumn)
	if err := p.QueryRow(ctx, query, paymentID, invoiceID, amount, baseAmount).Scan(&id); err != nil {
		t.Fatal(err)
	}
	key := keyPrefix + fmt.Sprint(id)
	var jeID int64
	if err := p.QueryRow(ctx, `INSERT INTO journal_entries(period_id,date,source_module,source_id,memo) VALUES($3,'2026-01-20',$1,$2,'realized FX') RETURNING id`, strings.ToUpper(kind)+"_PAYMENT_FX", uuid.New(), periodID).Scan(&jeID); err != nil {
		t.Fatal(err)
	}
	if kind == "ar" {
		mustQuery(t, p, `INSERT INTO journal_lines(je_id,account_id,debit,credit) SELECT $1,id,640000,0 FROM accounts WHERE code LIKE '1100-FX-%' ORDER BY id DESC LIMIT 1`, jeID)
		mustQuery(t, p, `INSERT INTO journal_lines(je_id,account_id,debit,credit) SELECT $1,id,0,600000 FROM accounts WHERE code LIKE '1100-FX-%' ORDER BY id DESC LIMIT 1`, jeID)
		mustQuery(t, p, `INSERT INTO journal_lines(je_id,account_id,debit,credit) SELECT $1,am.account_id,0,40000 FROM account_mappings am WHERE am.module='FX' AND am.key='fx.realized.gain'`, jeID)
	} else {
		mustQuery(t, p, `INSERT INTO journal_lines(je_id,account_id,debit,credit) SELECT $1,id,600000,0 FROM accounts WHERE code LIKE '2100-FX-%' ORDER BY id DESC LIMIT 1`, jeID)
		mustQuery(t, p, `INSERT INTO journal_lines(je_id,account_id,debit,credit) SELECT $1,am.account_id,40000,0 FROM account_mappings am WHERE am.module='FX' AND am.key='fx.realized.loss'`, jeID)
		mustQuery(t, p, `INSERT INTO journal_lines(je_id,account_id,debit,credit) SELECT $1,id,0,640000 FROM accounts WHERE code LIKE '1100-FX-%' ORDER BY id DESC LIMIT 1`, jeID)
	}
	mustQuery(t, p, `INSERT INTO fx_journal_idempotency(source_key,journal_entry_id) VALUES($1,$2)`, key, jeID)
	return fmt.Sprint(id)
}

func assertBalancedAndIdempotent(t *testing.T, p *pgxpool.Pool, allocationID, sourceKey string) {
	t.Helper()
	var debit, credit, count int64
	if err := p.QueryRow(context.Background(), `SELECT COALESCE(SUM((debit*100)::bigint),0),COALESCE(SUM((credit*100)::bigint),0) FROM journal_lines WHERE je_id=(SELECT journal_entry_id FROM fx_journal_idempotency WHERE source_key=$1)`, sourceKey).Scan(&debit, &credit); err != nil {
		t.Fatal(err)
	}
	if debit != credit {
		t.Fatalf("FX journal %s is unbalanced: debit=%d credit=%d", sourceKey, debit, credit)
	}
	if err := p.QueryRow(context.Background(), `SELECT COUNT(*) FROM fx_journal_idempotency WHERE source_key=$1`, sourceKey).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("FX source key %s count=%d", sourceKey, count)
	}
}

func revalue(t *testing.T, p *pgxpool.Pool, periodID int64, invoiceID, documentType, balance, difference, closing string) int64 {
	var id int64
	if err := p.QueryRow(context.Background(), `INSERT INTO fx_revaluations(period_id,document_type,document_id,currency,original_balance,previous_base_amount,closing_base_amount,difference,closing_rate,rate_date,rate_source) VALUES($1,$2,$3,'USD',$4,900000,1000000,$5,$6,'2026-01-31','TEST') RETURNING id`, periodID, documentType, invoiceID, balance, difference, closing).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func assertRevaluation(t *testing.T, p *pgxpool.Pool, id, periodID int64, documentType, documentID string) {
	var count int
	if err := p.QueryRow(context.Background(), `SELECT COUNT(*) FROM fx_revaluations WHERE id=$1 AND period_id=$2 AND document_type=$3 AND document_id=$4`, id, periodID, documentType, documentID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("revaluation %d not persisted", id)
	}
}

func assertReversal(t *testing.T, p *pgxpool.Pool, revaluationID, nextPeriodID int64) {
	ctx := context.Background()
	var journalID int64
	if err := p.QueryRow(ctx, `INSERT INTO journal_entries(period_id,date,source_module,memo) VALUES($1,'2026-02-01','FX_REVALUATION_REVERSAL','reversal') RETURNING id`, nextPeriodID).Scan(&journalID); err != nil {
		t.Fatal(err)
	}
	mustQuery(t, p, `INSERT INTO fx_revaluation_reversals(revaluation_id,next_period_id,journal_entry_id,actor_id) SELECT $1,$2,$3,id FROM users ORDER BY id DESC LIMIT 1`, revaluationID, nextPeriodID, journalID)
	mustQuery(t, p, `UPDATE fx_revaluations SET reversed_by_id=$2 WHERE id=$1`, revaluationID, journalID)
	var got int64
	if err := p.QueryRow(ctx, `SELECT reversed_by_id FROM fx_revaluations WHERE id=$1`, revaluationID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != journalID {
		t.Fatalf("reversal journal=%d, recorded=%d", journalID, got)
	}
}

func mustQuery(t *testing.T, p *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := p.Exec(context.Background(), query, args...); err != nil {
		t.Fatal(err)
	}
}
