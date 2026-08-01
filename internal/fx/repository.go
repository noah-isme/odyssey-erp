package fx

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type SQLRepository struct{ pool *pgxpool.Pool }

type RateStatus struct {
	BaseCurrency, QuoteCurrency, Source, Status, Rate string
	RateDate                                          time.Time
}

func NewRepository(pool *pgxpool.Pool) *SQLRepository { return &SQLRepository{pool: pool} }
func (r *SQLRepository) UpsertDailyRates(ctx context.Context, set FXQuoteSet) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("fx repository: pool is required")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for quote, rate := range set.Rates {
		if _, err := tx.Exec(ctx, `INSERT INTO fx_daily_rates (base_currency,quote_currency,rate_date,rate,source,source_reference,provider_updated_at,raw_payload_hash) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (base_currency,quote_currency,rate_date,source) DO UPDATE SET fetched_at=NOW(),source_reference=EXCLUDED.source_reference,provider_updated_at=EXCLUDED.provider_updated_at,raw_payload_hash=EXCLUDED.raw_payload_hash`, set.BaseCurrency, quote, set.RateDate, rate.String(), set.Source, set.SourceReference, set.ProviderUpdatedAt, set.RawPayloadHash); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (r *SQLRepository) RecordFetchRun(ctx context.Context, run FetchRun) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO fx_fetch_runs (rate_date,source,status,response_hash,error_message) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (rate_date,source) DO UPDATE SET requested_at=NOW(),status=EXCLUDED.status,response_hash=EXCLUDED.response_hash,error_message=EXCLUDED.error_message`, run.RateDate, run.Source, run.Status, run.ResponseHash, run.ErrorMessage)
	return err
}
func (r *SQLRepository) DailyRate(ctx context.Context, base, quote string, date time.Time, maxAge time.Duration) (FXQuote, error) {
	var q FXQuote
	var raw string
	var fetched time.Time
	query := `SELECT base_currency,quote_currency,rate::text,rate_date,source,COALESCE(source_reference,''),fetched_at FROM fx_daily_rates WHERE base_currency=$1 AND quote_currency=$2 AND rate_date <= $3 ORDER BY rate_date DESC,fetched_at DESC LIMIT 1`
	err := r.pool.QueryRow(ctx, query, base, quote, date).Scan(&q.BaseCurrency, &q.QuoteCurrency, &raw, &q.RateDate, &q.Source, &q.SourceReference, &fetched)
	inverse := false
	if err == pgx.ErrNoRows {
		err = r.pool.QueryRow(ctx, query, quote, base, date).Scan(&q.BaseCurrency, &q.QuoteCurrency, &raw, &q.RateDate, &q.Source, &q.SourceReference, &fetched)
		inverse = err == nil
	}
	if err != nil {
		return FXQuote{}, fmt.Errorf("%w: %v", ErrRateNotFound, err)
	}
	if maxAge > 0 && time.Since(fetched) > maxAge {
		return FXQuote{}, fmt.Errorf("%w: %s", ErrRateStale, q.Source)
	}
	q.Rate, err = ParseDecimal(raw)
	if err != nil {
		return FXQuote{}, err
	}
	if inverse {
		q.Rate = MustDecimal("1").Div(q.Rate).Round(10)
		q.BaseCurrency, q.QuoteCurrency = base, quote
	}
	return q, nil
}

func (r *SQLRepository) Status(ctx context.Context, date time.Time) ([]RateStatus, error) {
	rows, err := r.pool.Query(ctx, `SELECT base_currency,quote_currency,source,rate_date,rate::text,CASE WHEN fetched_at < NOW()-INTERVAL '48 hours' THEN 'STALE' ELSE 'CURRENT' END FROM fx_daily_rates WHERE rate_date=$1 ORDER BY base_currency,quote_currency,source`, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []RateStatus
	for rows.Next() {
		var item RateStatus
		if err := rows.Scan(&item.BaseCurrency, &item.QuoteCurrency, &item.Source, &item.RateDate, &item.Rate, &item.Status); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
