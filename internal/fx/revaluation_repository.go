package fx

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SQLRevaluationRepository struct{ pool *pgxpool.Pool }

func NewSQLRevaluationRepository(pool *pgxpool.Pool) *SQLRevaluationRepository {
	return &SQLRevaluationRepository{pool: pool}
}

func (r *SQLRevaluationRepository) WithTx(ctx context.Context, fn func(context.Context, RevaluationTxRepository) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(ctx, &sqlRevaluationTx{tx: tx}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *SQLRevaluationRepository) PeriodLocked(ctx context.Context, periodID int64) (bool, error) {
	var locked bool
	err := r.pool.QueryRow(ctx, `SELECT status IN ('LOCKED') FROM periods WHERE id=$1`, periodID).Scan(&locked)
	return locked, err
}

func (r *SQLRevaluationRepository) ListOutstandingBalances(ctx context.Context, asOf time.Time) ([]OutstandingBalance, error) {
	rows, err := r.pool.Query(ctx, `
SELECT 'AR_INVOICE', i.id, i.currency, COALESCE(i.base_currency,co.base_currency,'IDR'),
       (i.total-COALESCE(SUM(pa.amount),0))::text,
       ((i.total-COALESCE(SUM(pa.amount),0))*COALESCE(i.fx_rate,1))::text
FROM ar_invoices i JOIN customers c ON c.id=i.customer_id JOIN companies co ON co.id=c.company_id LEFT JOIN ar_payment_allocations pa ON pa.ar_invoice_id=i.id AND pa.created_at <= $1
WHERE i.status='POSTED' AND i.posted_at <= $1 AND i.currency<>COALESCE(i.base_currency,co.base_currency,'IDR')
GROUP BY i.id HAVING (i.total-COALESCE(SUM(pa.amount),0)) > 0
UNION ALL
SELECT 'AP_INVOICE', i.id, i.currency, COALESCE(i.base_currency,co.base_currency,'IDR'),
       (i.total-COALESCE(SUM(pa.amount),0))::text,
       ((i.total-COALESCE(SUM(pa.amount),0))*COALESCE(i.fx_rate,1))::text
FROM ap_invoices i JOIN suppliers s ON s.id=i.supplier_id JOIN companies co ON co.id=s.company_id LEFT JOIN ap_payment_allocations pa ON pa.ap_invoice_id=i.id AND pa.created_at <= $1
WHERE i.status='POSTED' AND i.posted_at <= $1 AND i.currency<>COALESCE(i.base_currency,co.base_currency,'IDR')
GROUP BY i.id HAVING (i.total-COALESCE(SUM(pa.amount),0)) > 0`, asOf)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OutstandingBalance
	for rows.Next() {
		var typ string
		var b OutstandingBalance
		var original, previous string
		if err := rows.Scan(&typ, &b.DocumentID, &b.Currency, &b.BaseCurrency, &original, &previous); err != nil {
			return nil, err
		}
		b.DocumentType = DocumentType(typ)
		b.OriginalBalance, err = ParseDecimal(original)
		if err != nil {
			return nil, err
		}
		b.PreviousBaseAmount, err = ParseDecimal(previous)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *SQLRevaluationRepository) InsertRevaluation(ctx context.Context, v RevaluationRecord) error {
	return insertRevaluation(ctx, r.pool, v)
}
func (r *SQLRevaluationRepository) GetRevaluation(ctx context.Context, periodID int64, typ DocumentType, documentID int64) (RevaluationRecord, error) {
	return getRevaluation(ctx, r.pool, periodID, typ, documentID)
}
func (r *SQLRevaluationRepository) InsertReversal(ctx context.Context, v RevaluationReversal) error {
	return insertReversal(ctx, r.pool, v)
}
func (r *SQLRevaluationRepository) MarkReversed(ctx context.Context, id, journalID int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE fx_revaluations SET reversed_by_id=$2 WHERE id=$1`, id, journalID)
	return err
}

type sqlRevaluationTx struct{ tx pgx.Tx }

func (t *sqlRevaluationTx) PGXTx() pgx.Tx { return t.tx }
func (t *sqlRevaluationTx) ClaimRevaluation(ctx context.Context, periodID int64, typ DocumentType, documentID int64) (bool, error) {
	var claimed bool
	err := t.tx.QueryRow(ctx, `INSERT INTO fx_revaluation_idempotency(period_id,document_type,document_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING RETURNING TRUE`, periodID, typ, documentID).Scan(&claimed)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	return claimed, err
}
func (t *sqlRevaluationTx) InsertRevaluation(ctx context.Context, v RevaluationRecord) error {
	return insertRevaluation(ctx, t.tx, v)
}
func (t *sqlRevaluationTx) MarkRevaluationJournal(ctx context.Context, periodID int64, typ DocumentType, documentID, journalID int64) error {
	_, err := t.tx.Exec(ctx, `UPDATE fx_revaluations SET journal_entry_id=$4 WHERE period_id=$1 AND document_type=$2 AND document_id=$3`, periodID, typ, documentID, journalID)
	return err
}
func (t *sqlRevaluationTx) ClaimReversal(ctx context.Context, revaluationID int64) (bool, error) {
	if _, err := t.tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, revaluationID); err != nil {
		return false, err
	}
	var exists bool
	err := t.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fx_revaluation_reversals WHERE revaluation_id=$1)`, revaluationID).Scan(&exists)
	return !exists, err
}
func (t *sqlRevaluationTx) InsertReversal(ctx context.Context, v RevaluationReversal) error {
	return insertReversal(ctx, t.tx, v)
}
func (t *sqlRevaluationTx) MarkReversed(ctx context.Context, id, journalID int64) error {
	_, err := t.tx.Exec(ctx, `UPDATE fx_revaluations SET reversed_by_id=$2 WHERE id=$1`, id, journalID)
	return err
}

type sqlExec interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func insertRevaluation(ctx context.Context, db sqlExec, v RevaluationRecord) error {
	_, err := db.Exec(ctx, `INSERT INTO fx_revaluations(period_id,document_type,document_id,currency,original_balance,previous_base_amount,closing_base_amount,difference,closing_rate,rate_date,rate_source,journal_entry_id,actor_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, v.PeriodID, v.DocumentType, v.DocumentID, v.Currency, v.OriginalBalance.String(), v.PreviousBaseAmount.String(), v.ClosingBaseAmount.String(), v.Difference.String(), v.ClosingRate.String(), v.RateDate, v.RateSource, nil, v.ActorID)
	return err
}
func getRevaluation(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, periodID int64, typ DocumentType, documentID int64) (RevaluationRecord, error) {
	var v RevaluationRecord
	var original, previous, closing, difference, rate string
	err := db.QueryRow(ctx, `SELECT id,period_id,document_type,document_id,currency,original_balance::text,previous_base_amount::text,closing_base_amount::text,difference::text,closing_rate::text,rate_date,rate_source,COALESCE(journal_entry_id,0),COALESCE(actor_id,0) FROM fx_revaluations WHERE period_id=$1 AND document_type=$2 AND document_id=$3`, periodID, typ, documentID).Scan(&v.ID, &v.PeriodID, &v.DocumentType, &v.DocumentID, &v.Currency, &original, &previous, &closing, &difference, &rate, &v.RateDate, &v.RateSource, &v.JournalEntryID, &v.ActorID)
	if err != nil {
		return v, err
	}
	var e error
	v.OriginalBalance, e = ParseDecimal(original)
	if e != nil {
		return v, e
	}
	v.PreviousBaseAmount, e = ParseDecimal(previous)
	if e != nil {
		return v, e
	}
	v.ClosingBaseAmount, e = ParseDecimal(closing)
	if e != nil {
		return v, e
	}
	v.Difference, e = ParseDecimal(difference)
	if e != nil {
		return v, e
	}
	v.ClosingRate, e = ParseDecimal(rate)
	return v, e
}
func insertReversal(ctx context.Context, db sqlExec, v RevaluationReversal) error {
	_, err := db.Exec(ctx, `INSERT INTO fx_revaluation_reversals(revaluation_id,next_period_id,journal_entry_id,actor_id) VALUES($1,$2,$3,$4)`, v.RevaluationID, v.NextPeriodID, v.JournalEntryID, v.ActorID)
	return err
}
