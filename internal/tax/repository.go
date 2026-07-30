package tax

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

type sourceSnapshot struct {
	companyID, sourceID                 int64
	sourceType, number, kind, direction string
	postedAt                            time.Time
	counterpartyName, counterpartyTaxID string
	base, vat, gross                    Money
	sign                                int
}

func sourceDigest(s sourceSnapshot) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%d|%s|%s|%s|%s|%s|%s|%d|%d|%d|%d", s.companyID, s.sourceType, s.sourceID, s.number, s.kind, s.direction, s.counterpartyName, s.counterpartyTaxID, s.postedAt.UTC().Format(time.RFC3339Nano), s.base, s.vat, s.gross, s.sign)))
	return hex.EncodeToString(sum[:])
}

func prorateBase(allocation, subtotal, gross Money) Money {
	if gross <= 0 {
		return 0
	}
	return Money((int64(allocation)*int64(subtotal) + int64(gross)/2) / int64(gross))
}
func calculateWithholding(base Money, rateBPS int64) Money {
	return Money((int64(base)*rateBPS + 5000) / 10000)
}

func (r *Repository) CaptureARInvoice(ctx context.Context, id, actorID int64) (Document, error) {
	return r.capture(ctx, id, actorID, `SELECT c.company_id,i.id,'AR_INVOICE',i.number,'INVOICE','OUTPUT',COALESCE(i.posted_at,i.created_at),c.name,COALESCE(c.tax_id,''),ROUND(i.subtotal)::bigint,ROUND(i.tax_amount)::bigint,ROUND(i.total)::bigint,1 FROM ar_invoices i JOIN customers c ON c.id=i.customer_id WHERE i.id=$1 AND i.status IN ('POSTED','PAID')`)
}
func (r *Repository) CaptureARCreditNote(ctx context.Context, id, actorID int64) (Document, error) {
	return r.capture(ctx, id, actorID, `SELECT c.company_id,n.id,'AR_CREDIT_NOTE',n.number,'CREDIT_NOTE','OUTPUT',COALESCE(n.posted_at,n.created_at),c.name,COALESCE(c.tax_id,''),ROUND(n.subtotal)::bigint,ROUND(n.tax_amount)::bigint,ROUND(n.total)::bigint,-1 FROM ar_credit_notes n JOIN customers c ON c.id=n.customer_id WHERE n.id=$1 AND n.status='POSTED'`)
}
func (r *Repository) CaptureAPInvoice(ctx context.Context, id, actorID int64) (Document, error) {
	return r.capture(ctx, id, actorID, `SELECT COALESCE(i.company_id,s.company_id),i.id,'AP_INVOICE',i.number,'INVOICE','INPUT',COALESCE(i.posted_at,i.created_at),s.name,COALESCE(s.tax_id,''),ROUND(i.subtotal)::bigint,ROUND(i.tax_amount)::bigint,ROUND(i.total)::bigint,1 FROM ap_invoices i JOIN suppliers s ON s.id=i.supplier_id WHERE i.id=$1 AND i.status IN ('POSTED','PAID')`)
}
func (r *Repository) CaptureAPDebitNote(ctx context.Context, id, actorID int64) (Document, error) {
	return r.capture(ctx, id, actorID, `SELECT COALESCE(i.company_id,s.company_id),n.id,'AP_DEBIT_NOTE',n.number,'DEBIT_NOTE','INPUT',COALESCE(n.posted_at,n.created_at),s.name,COALESCE(s.tax_id,''),ROUND(n.subtotal)::bigint,ROUND(n.tax_amount)::bigint,ROUND(n.total)::bigint,-1 FROM ap_debit_notes n JOIN ap_invoices i ON i.id=n.ap_invoice_id JOIN suppliers s ON s.id=n.supplier_id WHERE n.id=$1 AND n.status='POSTED'`)
}

func (r *Repository) capture(ctx context.Context, sourceID, actorID int64, query string) (Document, error) {
	var result Document
	err := pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{IsoLevel: pgx.Serializable}, func(tx pgx.Tx) error {
		var s sourceSnapshot
		if err := tx.QueryRow(ctx, query, sourceID).Scan(&s.companyID, &s.sourceID, &s.sourceType, &s.number, &s.kind, &s.direction, &s.postedAt, &s.counterpartyName, &s.counterpartyTaxID, &s.base, &s.vat, &s.gross, &s.sign); err != nil {
			return err
		}
		if err := scanDocument(tx.QueryRow(ctx, `SELECT id,company_id,tax_period_id,source_id,rule_version_id,source_type,source_number,document_kind,direction,COALESCE(tax_number,''),counterparty_name,counterparty_tax_id,issue_date,ROUND(taxable_base)::bigint,ROUND(vat_amount)::bigint,ROUND(gross_amount)::bigint,sign,CASE WHEN EXISTS(SELECT 1 FROM tax_document_events e WHERE e.tax_document_id=d.id AND event_type='CANCELLED') THEN 'CANCELLED' ELSE 'ISSUED' END FROM tax_documents d WHERE source_type=$1 AND source_id=$2`, s.sourceType, sourceID), &result); err == nil {
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if s.companyID <= 0 || s.counterpartyTaxID == "" {
			return ErrConfiguration
		}
		var identity int64
		if err := tx.QueryRow(ctx, `SELECT id FROM company_tax_identities WHERE company_id=$1 AND effective_from<=$2::date AND (effective_to IS NULL OR effective_to >= $2::date)`, s.companyID, s.postedAt).Scan(&identity); err != nil {
			return ErrConfiguration
		}
		periodID, err := ensurePeriod(ctx, tx, s.companyID, s.postedAt)
		if err != nil {
			return err
		}
		category := "VAT_OUTPUT"
		if s.direction == "INPUT" {
			category = "VAT_INPUT"
		}
		var taxCodeID, ruleVersionID, accountID int64
		if err = tx.QueryRow(ctx, `SELECT tc.id,rv.id FROM tax_codes tc JOIN tax_rule_versions rv ON rv.id=tc.rule_version_id WHERE tc.tax_kind=$1 AND rv.reviewed_at IS NOT NULL AND rv.effective_from<=$2::date AND (rv.effective_to IS NULL OR rv.effective_to >= $2::date) ORDER BY rv.effective_from DESC,tc.id LIMIT 1`, category, s.postedAt).Scan(&taxCodeID, &ruleVersionID); err != nil {
			return ErrConfiguration
		}
		if err = tx.QueryRow(ctx, `SELECT account_id FROM tax_account_mappings WHERE company_id=$1 AND category=$2 AND effective_from<=$3::date AND (effective_to IS NULL OR effective_to >= $3::date)`, s.companyID, category, s.postedAt).Scan(&accountID); err != nil {
			return ErrConfiguration
		}
		taxNumber := s.number
		if s.direction == "OUTPUT" {
			err = tx.QueryRow(ctx, `UPDATE tax_invoice_number_ranges SET next_number=next_number+1 WHERE id=(SELECT id FROM tax_invoice_number_ranges WHERE company_id=$1 AND effective_from<=$2::date AND (effective_to IS NULL OR effective_to >= $2::date) AND next_number<=range_end ORDER BY effective_from DESC,id FOR UPDATE LIMIT 1) RETURNING prefix||LPAD((next_number-1)::text,LENGTH(range_end::text),'0')`, s.companyID, s.postedAt).Scan(&taxNumber)
			if err != nil {
				return ErrConfiguration
			}
		}
		err = tx.QueryRow(ctx, `INSERT INTO tax_documents(company_id,tax_period_id,source_type,source_id,source_number,source_posted_at,document_kind,direction,tax_number,issue_date,counterparty_name,counterparty_tax_id,taxable_base,vat_amount,gross_amount,sign,tax_code_id,rule_version_id,source_hash,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20) RETURNING id`, s.companyID, periodID, s.sourceType, s.sourceID, s.number, s.postedAt, s.kind, s.direction, taxNumber, s.postedAt, s.counterpartyName, s.counterpartyTaxID, s.base, s.vat, s.gross, s.sign, taxCodeID, ruleVersionID, sourceDigest(s), actorID).Scan(&result.ID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO tax_document_events(tax_document_id,event_type,actor_id) VALUES($1,'ISSUED',$2); INSERT INTO tax_ledger_entries(company_id,tax_period_id,category,tax_document_id,account_id,source_type,source_id,source_date,taxable_base,tax_amount,sign) VALUES($3,$4,$5,$1,$6,$7,$8,$9,$10,$11,$12); INSERT INTO tax_audit_events(company_id,entity_type,entity_id,action,actor_id) VALUES($3,'TAX_DOCUMENT',$1,'ISSUED',$2)`, result.ID, actorID, s.companyID, periodID, category, accountID, s.sourceType, s.sourceID, s.postedAt, s.base, s.vat, s.sign)
		if err != nil {
			return err
		}
		if s.sourceType == "AP_INVOICE" {
			if err = captureInvoiceWithholding(ctx, tx, s, periodID, actorID); err != nil {
				return err
			}
		}
		if s.sourceType == "AR_INVOICE" {
			_, err = tx.Exec(ctx, `UPDATE ar_invoices SET tax_document_id=$2,faktur_number=$3,faktur_issue_date=$4,buyer_tax_id=$5,faktur_taxable_base=$6,faktur_vat_amount=$7,faktur_status='ISSUED',tax_code_id=$8 WHERE id=$1`, s.sourceID, result.ID, taxNumber, s.postedAt, s.counterpartyTaxID, s.base, s.vat, taxCodeID)
			if err != nil {
				return err
			}
		}
		result = Document{ID: result.ID, CompanyID: s.companyID, PeriodID: periodID, SourceID: s.sourceID, RuleVersionID: ruleVersionID, SourceType: s.sourceType, SourceNumber: s.number, Kind: s.kind, Direction: s.direction, TaxNumber: taxNumber, CounterpartyName: s.counterpartyName, CounterpartyTaxID: s.counterpartyTaxID, IssueDate: s.postedAt, TaxableBase: s.base, VATAmount: s.vat, GrossAmount: s.gross, Sign: s.sign, Status: "ISSUED"}
		return nil
	})
	return result, err
}

func captureInvoiceWithholding(ctx context.Context, tx pgx.Tx, s sourceSnapshot, periodID, actorID int64) error {
	var typeID, rate, accountID, taxCodeID int64
	var article, taxID string
	err := tx.QueryRow(ctx, `SELECT w.id,w.rate_bps,w.article,s.tax_id FROM ap_invoices i JOIN suppliers s ON s.id=i.supplier_id JOIN tax_withholding_types w ON w.id=i.withholding_type_id JOIN tax_rule_versions rv ON rv.id=w.rule_version_id WHERE i.id=$1 AND w.recognition_event='INVOICE' AND rv.reviewed_at IS NOT NULL`, s.sourceID).Scan(&typeID, &rate, &article, &taxID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if taxID == "" {
		return ErrConfiguration
	}
	if err = tx.QueryRow(ctx, `SELECT account_id FROM tax_account_mappings WHERE company_id=$1 AND category=$2 AND effective_from<=$3::date AND (effective_to IS NULL OR effective_to >= $3::date)`, s.companyID, article, s.postedAt).Scan(&accountID); err != nil {
		return ErrConfiguration
	}
	if err = tx.QueryRow(ctx, `SELECT id FROM tax_codes WHERE withholding_type_id=$1 ORDER BY id LIMIT 1`, typeID).Scan(&taxCodeID); err != nil {
		return ErrConfiguration
	}
	amount := calculateWithholding(s.base, rate)
	digest := sha256.Sum256([]byte(fmt.Sprintf("AP_INVOICE|%d|%d|%d", s.sourceID, s.base, amount)))
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO tax_withholding_records(company_id,tax_period_id,ap_invoice_id,source_event,withholding_type_id,tax_code_id,recognition_date,taxable_base,withheld_amount,supplier_tax_id,source_hash,created_by) VALUES($1,$2,$3,'INVOICE',$4,NULLIF($5,0),$6,$7,$8,$9,$10,$11) ON CONFLICT(withholding_type_id,ap_invoice_id,source_event) WHERE ap_payment_id IS NULL DO UPDATE SET source_hash=tax_withholding_records.source_hash RETURNING id`, s.companyID, periodID, s.sourceID, typeID, taxCodeID, s.postedAt, s.base, amount, taxID, hex.EncodeToString(digest[:]), actorID).Scan(&id)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO tax_ledger_entries(company_id,tax_period_id,category,withholding_record_id,account_id,source_type,source_id,source_date,taxable_base,tax_amount,sign) VALUES($1,$2,$3,$4,$5,'AP_INVOICE_WITHHOLDING',$4,$6,$7,$8,1) ON CONFLICT(category,source_type,source_id) DO NOTHING`, s.companyID, periodID, article, id, accountID, s.postedAt, s.base, amount)
	return err
}

func ensurePeriod(ctx context.Context, tx pgx.Tx, companyID int64, date time.Time) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `INSERT INTO tax_periods(company_id,accounting_period_id) SELECT $1,id FROM accounting_periods WHERE (company_id=$1 OR company_id IS NULL) AND start_date<=$2::date AND end_date>=$2::date ORDER BY (company_id IS NOT NULL) DESC LIMIT 1 ON CONFLICT(company_id,accounting_period_id) DO UPDATE SET company_id=EXCLUDED.company_id RETURNING id`, companyID, date).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrConfiguration
	}
	return id, err
}

func scanDocument(row pgx.Row, d *Document) error {
	return row.Scan(&d.ID, &d.CompanyID, &d.PeriodID, &d.SourceID, &d.RuleVersionID, &d.SourceType, &d.SourceNumber, &d.Kind, &d.Direction, &d.TaxNumber, &d.CounterpartyName, &d.CounterpartyTaxID, &d.IssueDate, &d.TaxableBase, &d.VATAmount, &d.GrossAmount, &d.Sign, &d.Status)
}

func (r *Repository) CaptureAPPayment(ctx context.Context, paymentID, actorID int64) ([]Withholding, error) {
	var out []Withholding
	err := pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{IsoLevel: pgx.Serializable}, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT COALESCE(i.company_id,s.company_id),i.id,p.id,p.paid_at,ROUND(a.amount)::bigint,ROUND(i.subtotal)::bigint,ROUND(i.total)::bigint,s.tax_id,w.id,w.article,w.code,w.rate_bps FROM ap_payments p JOIN ap_payment_allocations a ON a.ap_payment_id=p.id JOIN ap_invoices i ON i.id=a.ap_invoice_id JOIN suppliers s ON s.id=i.supplier_id JOIN tax_withholding_types w ON w.id=i.withholding_type_id JOIN tax_rule_versions rv ON rv.id=w.rule_version_id WHERE p.id=$1 AND w.recognition_event='PAYMENT' AND rv.reviewed_at IS NOT NULL`, paymentID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var companyID, invoiceID, payID, typeID int64
			var paidAt time.Time
			var allocation, subtotal, gross Money
			var taxID, article, code string
			var rate int
			if err = rows.Scan(&companyID, &invoiceID, &payID, &paidAt, &allocation, &subtotal, &gross, &taxID, &typeID, &article, &code, &rate); err != nil {
				return err
			}
			if companyID <= 0 || taxID == "" {
				return ErrConfiguration
			}
			periodID, err := ensurePeriod(ctx, tx, companyID, paidAt)
			if err != nil {
				return err
			}
			base := prorateBase(allocation, subtotal, gross)
			amount := calculateWithholding(base, int64(rate))
			var accountID, taxCodeID int64
			if err = tx.QueryRow(ctx, `SELECT account_id FROM tax_account_mappings WHERE company_id=$1 AND category=$2 AND effective_from<=$3::date AND (effective_to IS NULL OR effective_to >= $3::date)`, companyID, article, paidAt).Scan(&accountID); err != nil {
				return ErrConfiguration
			}
			if err = tx.QueryRow(ctx, `SELECT id FROM tax_codes WHERE withholding_type_id=$1 ORDER BY id LIMIT 1`, typeID).Scan(&taxCodeID); err != nil {
				return ErrConfiguration
			}
			digest := sha256.Sum256([]byte(fmt.Sprintf("AP_PAYMENT|%d|%d|%d|%d", payID, invoiceID, base, amount)))
			var id int64
			err = tx.QueryRow(ctx, `INSERT INTO tax_withholding_records(company_id,tax_period_id,ap_invoice_id,ap_payment_id,source_event,withholding_type_id,tax_code_id,recognition_date,taxable_base,withheld_amount,supplier_tax_id,source_hash,created_by) VALUES($1,$2,$3,$4,'PAYMENT',$5,NULLIF($6,0),$7,$8,$9,$10,$11,$12) ON CONFLICT(withholding_type_id,ap_invoice_id,ap_payment_id,source_event) WHERE ap_payment_id IS NOT NULL DO UPDATE SET source_hash=tax_withholding_records.source_hash RETURNING id`, companyID, periodID, invoiceID, payID, typeID, taxCodeID, paidAt, base, amount, taxID, hex.EncodeToString(digest[:]), actorID).Scan(&id)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `INSERT INTO tax_ledger_entries(company_id,tax_period_id,category,withholding_record_id,account_id,source_type,source_id,source_date,taxable_base,tax_amount,sign) VALUES($1,$2,$3,$4,$5,'AP_PAYMENT_WITHHOLDING',$4,$6,$7,$8,1) ON CONFLICT(category,source_type,source_id) DO NOTHING`, companyID, periodID, article, id, accountID, paidAt, base, amount)
			if err != nil {
				return err
			}
			pay := payID
			out = append(out, Withholding{ID: id, CompanyID: companyID, PeriodID: periodID, APInvoiceID: invoiceID, APPaymentID: &pay, Article: article, Code: code, SupplierTaxID: taxID, RecognitionDate: paidAt, TaxableBase: base, Amount: amount})
		}
		return rows.Err()
	})
	return out, err
}

func (r *Repository) CancelDocument(ctx context.Context, documentID, actorID int64, reason string) error {
	return pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var companyID, periodID, sourceID, accountID int64
		var sourceType string
		var category string
		var base, amount Money
		var sign int
		if err := tx.QueryRow(ctx, `SELECT d.company_id,d.tax_period_id,d.source_type,d.source_id,l.category,l.account_id,ROUND(d.taxable_base)::bigint,ROUND(d.vat_amount)::bigint,d.sign FROM tax_documents d JOIN tax_ledger_entries l ON l.tax_document_id=d.id AND l.source_type=d.source_type WHERE d.id=$1`, documentID).Scan(&companyID, &periodID, &sourceType, &sourceID, &category, &accountID, &base, &amount, &sign); err != nil {
			return err
		}
		var locked bool
		if err := tx.QueryRow(ctx, `SELECT status='LOCKED' FROM tax_periods WHERE id=$1`, periodID).Scan(&locked); err != nil {
			return err
		}
		if locked {
			return ErrPeriodLocked
		}
		if _, err := tx.Exec(ctx, `INSERT INTO tax_document_events(tax_document_id,event_type,reason,actor_id) VALUES($1,'CANCELLED',$2,$3); INSERT INTO tax_audit_events(company_id,entity_type,entity_id,action,actor_id,details) VALUES($4,'TAX_DOCUMENT',$1,'CANCELLED',$3,jsonb_build_object('reason',$2))`, documentID, reason, actorID, companyID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO tax_ledger_entries(company_id,tax_period_id,category,tax_document_id,account_id,source_type,source_id,source_date,taxable_base,tax_amount,sign) VALUES($1,$2,$3,$4,$5,$6,$4,CURRENT_DATE,$7,$8,$9)`, companyID, periodID, category, documentID, accountID, sourceType+"_CANCEL", base, amount, -sign); err != nil {
			return err
		}
		if sourceType == "AR_INVOICE" {
			_, err := tx.Exec(ctx, `UPDATE ar_invoices SET faktur_status='CANCELLED' WHERE id=$1`, sourceID)
			return err
		}
		return nil
	})
}

func (r *Repository) CancelSource(ctx context.Context, sourceType string, sourceID, actorID int64, reason string) error {
	var id int64
	if err := r.pool.QueryRow(ctx, `SELECT id FROM tax_documents WHERE source_type=$1 AND source_id=$2`, sourceType, sourceID).Scan(&id); err != nil {
		return err
	}
	return r.CancelDocument(ctx, id, actorID, reason)
}

func (r *Repository) ReplaceDocument(ctx context.Context, originalID, replacementID, actorID int64, reason string) error {
	return pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var companyID, periodID, accountID, sourceID int64
		var category, sourceType string
		var base, amount Money
		var sign int
		err := tx.QueryRow(ctx, `SELECT d.company_id,d.tax_period_id,d.source_type,d.source_id,l.category,l.account_id,ROUND(d.taxable_base)::bigint,ROUND(d.vat_amount)::bigint,d.sign FROM tax_documents d JOIN tax_ledger_entries l ON l.tax_document_id=d.id AND l.source_type=d.source_type WHERE d.id=$1`, originalID).Scan(&companyID, &periodID, &sourceType, &sourceID, &category, &accountID, &base, &amount, &sign)
		if err != nil {
			return err
		}
		var replacementCompany int64
		if err = tx.QueryRow(ctx, `SELECT company_id FROM tax_documents WHERE id=$1`, replacementID).Scan(&replacementCompany); err != nil {
			return err
		}
		if replacementCompany != companyID {
			return ErrInvalidInput
		}
		var locked bool
		if err = tx.QueryRow(ctx, `SELECT status='LOCKED' FROM tax_periods WHERE id=$1`, periodID).Scan(&locked); err != nil {
			return err
		}
		if locked {
			return ErrPeriodLocked
		}
		_, err = tx.Exec(ctx, `INSERT INTO tax_document_events(tax_document_id,event_type,reason,replacement_document_id,actor_id) VALUES($1,'REPLACED',$3,$2,$4); INSERT INTO tax_ledger_entries(company_id,tax_period_id,category,tax_document_id,account_id,source_type,source_id,source_date,taxable_base,tax_amount,sign) VALUES($5,$6,$7,$1,$8,$9,$1,CURRENT_DATE,$10,$11,$12); INSERT INTO tax_audit_events(company_id,entity_type,entity_id,action,actor_id,details) VALUES($5,'TAX_DOCUMENT',$1,'REPLACED',$4,jsonb_build_object('replacement_document_id',$2,'reason',$3))`, originalID, replacementID, reason, actorID, companyID, periodID, category, accountID, sourceType+"_REPLACE", base, amount, -sign)
		if err != nil {
			return err
		}
		if sourceType == "AR_INVOICE" {
			_, err = tx.Exec(ctx, `UPDATE ar_invoices SET faktur_status='REPLACED' WHERE id=$1; UPDATE ar_invoices SET replacement_of_tax_document_id=$2 WHERE tax_document_id=$3`, sourceID, originalID, replacementID)
		}
		return err
	})
}

func (r *Repository) ListDocuments(ctx context.Context, companyID, periodID int64) ([]Document, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,company_id,tax_period_id,source_id,rule_version_id,source_type,source_number,document_kind,direction,COALESCE(tax_number,''),counterparty_name,counterparty_tax_id,issue_date,ROUND(taxable_base)::bigint,ROUND(vat_amount)::bigint,ROUND(gross_amount)::bigint,sign,status FROM v_tax_document_status WHERE company_id=$1 AND ($2=0 OR tax_period_id=$2) ORDER BY issue_date,id`, companyID, periodID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Document
	for rows.Next() {
		var d Document
		if err = scanDocument(rows, &d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *Repository) ListPeriods(ctx context.Context, companyID int64) ([]Period, error) {
	rows, err := r.pool.Query(ctx, `SELECT tp.id,tp.company_id,tp.accounting_period_id,ap.name,tp.status,ap.start_date,ap.end_date FROM tax_periods tp JOIN accounting_periods ap ON ap.id=tp.accounting_period_id WHERE tp.company_id=$1 ORDER BY ap.start_date DESC`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Period
	for rows.Next() {
		var p Period
		if err = rows.Scan(&p.ID, &p.CompanyID, &p.AccountingPeriodID, &p.Name, &p.Status, &p.StartDate, &p.EndDate); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) ListPostedSources(ctx context.Context, companyID, periodID int64) ([]PostedSource, error) {
	rows, err := r.pool.Query(ctx, `WITH bounds AS (SELECT ap.start_date,ap.end_date FROM tax_periods tp JOIN accounting_periods ap ON ap.id=tp.accounting_period_id WHERE tp.id=$2 AND tp.company_id=$1)
		SELECT source_type,source_id FROM (
		 SELECT 'AR_INVOICE' source_type,i.id source_id,COALESCE(i.posted_at,i.created_at)::date source_date FROM ar_invoices i JOIN customers c ON c.id=i.customer_id WHERE c.company_id=$1 AND i.status IN ('POSTED','PAID')
		 UNION ALL SELECT 'AR_CREDIT_NOTE',n.id,COALESCE(n.posted_at,n.created_at)::date FROM ar_credit_notes n JOIN customers c ON c.id=n.customer_id WHERE c.company_id=$1 AND n.status='POSTED'
		 UNION ALL SELECT 'AP_INVOICE',i.id,COALESCE(i.posted_at,i.created_at)::date FROM ap_invoices i JOIN suppliers s ON s.id=i.supplier_id WHERE COALESCE(i.company_id,s.company_id)=$1 AND i.status IN ('POSTED','PAID')
		 UNION ALL SELECT 'AP_DEBIT_NOTE',n.id,COALESCE(n.posted_at,n.created_at)::date FROM ap_debit_notes n JOIN ap_invoices i ON i.id=n.ap_invoice_id JOIN suppliers s ON s.id=n.supplier_id WHERE COALESCE(i.company_id,s.company_id)=$1 AND n.status='POSTED'
		 UNION SELECT 'AP_PAYMENT',p.id,p.paid_at::date FROM ap_payments p JOIN ap_payment_allocations a ON a.ap_payment_id=p.id JOIN ap_invoices i ON i.id=a.ap_invoice_id JOIN suppliers s ON s.id=i.supplier_id WHERE COALESCE(i.company_id,s.company_id)=$1
		) sources CROSS JOIN bounds WHERE source_date BETWEEN bounds.start_date AND bounds.end_date ORDER BY source_date,source_type,source_id`, companyID, periodID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PostedSource
	for rows.Next() {
		var source PostedSource
		if err = rows.Scan(&source.Type, &source.ID); err != nil {
			return nil, err
		}
		out = append(out, source)
	}
	return out, rows.Err()
}

func (r *Repository) Recap(ctx context.Context, companyID, periodID int64) ([]RecapLine, error) {
	// Liability categories reconcile on their natural credit balance. VAT input
	// is an asset and therefore reconciles on its natural debit balance.
	rows, err := r.pool.Query(ctx, `WITH tax AS (SELECT category,account_id,COUNT(*) count,ROUND(SUM(taxable_base*sign))::bigint base,ROUND(SUM(tax_amount*sign))::bigint amount FROM tax_ledger_entries WHERE company_id=$1 AND tax_period_id=$2 GROUP BY category,account_id), gl AS (SELECT t.category,t.account_id,ROUND(SUM(CASE WHEN t.category='VAT_INPUT' THEN jl.debit-jl.credit ELSE jl.credit-jl.debit END))::bigint amount FROM tax tax t JOIN tax_periods tp ON tp.id=$2 JOIN accounting_periods ap ON ap.id=tp.accounting_period_id JOIN journal_entries je ON je.date BETWEEN ap.start_date AND ap.end_date AND je.status='POSTED' JOIN journal_lines jl ON jl.je_id=je.id AND jl.account_id=t.account_id AND (jl.dim_company_id=$1 OR jl.dim_company_id IS NULL) GROUP BY t.category,t.account_id) SELECT t.category,a.code,a.name,t.count,t.base,t.amount,COALESCE(gl.amount,0),t.amount-COALESCE(gl.amount,0) FROM tax t JOIN accounts a ON a.id=t.account_id LEFT JOIN gl ON gl.category=t.category AND gl.account_id=t.account_id ORDER BY t.category,a.code`, companyID, periodID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecapLine
	for rows.Next() {
		var x RecapLine
		if err = rows.Scan(&x.Category, &x.AccountCode, &x.AccountName, &x.DocumentCount, &x.TaxableBase, &x.TaxAmount, &x.GLAmount, &x.Difference); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *Repository) LockPeriod(ctx context.Context, companyID, periodID, actorID int64) error {
	cmd, err := r.pool.Exec(ctx, `UPDATE tax_periods SET status='LOCKED',locked_by=$3,locked_at=NOW() WHERE id=$2 AND company_id=$1 AND status='OPEN'`, companyID, periodID, actorID)
	if err == nil && cmd.RowsAffected() != 1 {
		return ErrPeriodLocked
	}
	return err
}

func (r *Repository) LoadExport(ctx context.Context, companyID, periodID int64, kind string) (ExportSchema, []ExportRecord, error) {
	var s ExportSchema
	err := r.pool.QueryRow(ctx, `SELECT id,export_kind,version_code,media_type,schema_body,official_source_url,official_checksum,effective_from,xml_declaration,include_sign_element FROM tax_export_schemas WHERE export_kind=$3 AND reviewed_at IS NOT NULL AND effective_from<=(SELECT end_date FROM accounting_periods ap JOIN tax_periods tp ON tp.accounting_period_id=ap.id WHERE tp.id=$2 AND tp.company_id=$1) AND (effective_to IS NULL OR effective_to>=(SELECT end_date FROM accounting_periods ap JOIN tax_periods tp ON tp.accounting_period_id=ap.id WHERE tp.id=$2 AND tp.company_id=$1)) ORDER BY effective_from DESC LIMIT 1`, companyID, periodID, kind).Scan(&s.ID, &s.Kind, &s.Version, &s.MediaType, &s.Body, &s.OfficialSourceURL, &s.OfficialChecksum, &s.EffectiveFrom, &s.XMLDeclaration, &s.IncludeSignElement)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, nil, ErrConfiguration
	}
	if err != nil {
		return s, nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT COALESCE(tax_number,''),source_number,counterparty_name,counterparty_tax_id,issue_date,ROUND(taxable_base)::bigint,ROUND(vat_amount)::bigint,sign FROM v_tax_document_status WHERE company_id=$1 AND tax_period_id=$2 AND direction='OUTPUT' AND status='ISSUED' ORDER BY issue_date,id`, companyID, periodID)
	if err != nil {
		return s, nil, err
	}
	defer rows.Close()
	var records []ExportRecord
	for rows.Next() {
		var x ExportRecord
		if err = rows.Scan(&x.TaxNumber, &x.DocumentNumber, &x.CounterpartyName, &x.CounterpartyTaxID, &x.IssueDate, &x.TaxableBase, &x.TaxAmount, &x.Sign); err != nil {
			return s, nil, err
		}
		records = append(records, x)
	}
	return s, records, rows.Err()
}

func (r *Repository) RecordExport(ctx context.Context, companyID, periodID, schemaID int64, hash string, count int, base, amount Money, actorID int64) (int64, error) {
	_, err := r.pool.Exec(ctx, `INSERT INTO tax_exports(company_id,tax_period_id,schema_id,content_hash,record_count,taxable_base,tax_amount,generated_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(company_id,tax_period_id,schema_id,content_hash) DO NOTHING`, companyID, periodID, schemaID, hash, count, base, amount, actorID)
	if err != nil {
		return 0, err
	}
	var id int64
	err = r.pool.QueryRow(ctx, `SELECT id FROM tax_exports WHERE company_id=$1 AND tax_period_id=$2 AND schema_id=$3 AND content_hash=$4`, companyID, periodID, schemaID, hash).Scan(&id)
	return id, err
}

func (r *Repository) PendingCaptures(ctx context.Context, limit int) ([]PendingCapture, error) {
	rows, err := r.pool.Query(ctx, `WITH claimed AS (SELECT id FROM tax_capture_outbox WHERE completed_at IS NULL AND actor_id IS NOT NULL AND available_at<=NOW() ORDER BY available_at,id FOR UPDATE SKIP LOCKED LIMIT $1) UPDATE tax_capture_outbox o SET attempts=o.attempts+1,available_at=NOW()+INTERVAL '5 minutes',updated_at=NOW() FROM claimed WHERE o.id=claimed.id RETURNING o.id,o.source_type,o.source_id,o.actor_id`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingCapture
	for rows.Next() {
		var item PendingCapture
		if err = rows.Scan(&item.ID, &item.SourceType, &item.SourceID, &item.ActorID); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
func (r *Repository) CompleteCapture(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE tax_capture_outbox SET completed_at=NOW(),last_error=NULL,updated_at=NOW() WHERE id=$1`, id)
	return err
}
func (r *Repository) FailCapture(ctx context.Context, id int64, cause error) error {
	_, err := r.pool.Exec(ctx, `UPDATE tax_capture_outbox SET last_error=LEFT($2,2000),available_at=NOW()+(LEAST(attempts,60)||' minutes')::interval,updated_at=NOW() WHERE id=$1`, id, cause.Error())
	return err
}
