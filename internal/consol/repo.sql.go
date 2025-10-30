package consol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/consol/fx"
)

// Repository provides persistence helpers for consolidation workloads.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a consolidation repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ErrPeriodNotFound indicates the requested period code is missing.
var ErrPeriodNotFound = errors.New("consol: period not found")

// ErrGroupNotFound indicates the consolidation group is missing.
var ErrGroupNotFound = errors.New("consol: group not found")

// ErrFxRateNotFound indicates the FX rate for the requested pair/date is missing.
var ErrFxRateNotFound = errors.New("consol: fx rate not found")

// FxRateInput represents a single FX quote to be stored.
type FxRateInput struct {
	AsOf    time.Time
	Pair    string
	Average float64
	Closing float64
}

// FindPeriodID resolves a period code to its identifier.
func (r *Repository) FindPeriodID(ctx context.Context, code string) (int64, error) {
	if r == nil || r.pool == nil {
		return 0, fmt.Errorf("consol repo not initialised")
	}
	const query = `SELECT id FROM periods WHERE code = $1`
	var id int64
	if err := r.pool.QueryRow(ctx, query, code).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrPeriodNotFound
		}
		return 0, err
	}
	return id, nil
}

// GetGroup fetches minimal metadata for a consolidation group.
func (r *Repository) GetGroup(ctx context.Context, groupID int64) (string, string, error) {
	if r == nil || r.pool == nil {
		return "", "", fmt.Errorf("consol repo not initialised")
	}
	const query = `SELECT name, reporting_currency FROM consol_groups WHERE id = $1`
	var name, ccy string
	if err := r.pool.QueryRow(ctx, query, groupID).Scan(&name, &ccy); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", ErrGroupNotFound
		}
		return "", "", err
	}
	return name, ccy, nil
}

// GroupReportingCurrency fetches the configured reporting currency for the group.
func (r *Repository) GroupReportingCurrency(ctx context.Context, groupID int64) (string, error) {
	_, ccy, err := r.GetGroup(ctx, groupID)
	return ccy, err
}

// MemberRow describes a group member fetched from the database.
type MemberRow struct {
	CompanyID int64
	Name      string
	Enabled   bool
}

// Members returns enabled members for the group.
func (r *Repository) Members(ctx context.Context, groupID int64) ([]MemberRow, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("consol repo not initialised")
	}
	const query = `
SELECT cm.company_id, c.name, cm.enabled
FROM consol_members cm
JOIN companies c ON c.id = cm.company_id
WHERE cm.group_id = $1
ORDER BY c.name`
	rows, err := r.pool.Query(ctx, query, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []MemberRow
	for rows.Next() {
		var m MemberRow
		if err := rows.Scan(&m.CompanyID, &m.Name, &m.Enabled); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// MemberCurrencies returns the local currency configured for group members.
func (r *Repository) MemberCurrencies(ctx context.Context, groupID int64) (map[int64]string, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("consol repo not initialised")
	}
	const query = `
SELECT cm.company_id, c.currency
FROM consol_members cm
JOIN companies c ON c.id = cm.company_id
WHERE cm.group_id = $1`
	rows, err := r.pool.Query(ctx, query, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	currencies := make(map[int64]string)
	for rows.Next() {
		var companyID int64
		var currency string
		if err := rows.Scan(&companyID, &currency); err != nil {
			return nil, err
		}
		currencies[companyID] = strings.ToUpper(strings.TrimSpace(currency))
	}
	return currencies, rows.Err()
}

// RebuildConsolidation recomputes balances for the provided period and group.
func (r *Repository) RebuildConsolidation(ctx context.Context, groupID, periodID int64) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("consol repo not initialised")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()
	if _, err = tx.Exec(ctx, `DELETE FROM mv_consol_balances WHERE period_id = $1 AND group_id = $2`, periodID, groupID); err != nil {
		return err
	}
	const insert = `
INSERT INTO mv_consol_balances (period_id, group_id, group_account_id, local_ccy_amt, group_ccy_amt, members)
SELECT $1 AS period_id,
       $2 AS group_id,
       base.group_account_id,
       SUM(base.local_amt) AS local_ccy_amt,
       SUM(base.local_amt) AS group_ccy_amt,
       jsonb_agg(
           jsonb_build_object(
               'company_id', base.company_id,
               'company_name', base.company_name,
               'local_ccy_amt', base.local_amt
           ) ORDER BY base.company_id
       ) AS members
FROM (
    SELECT
        je.period_id,
        cm.group_id,
        am.group_account_id,
        cm.company_id,
        c.name AS company_name,
        SUM(jl.debit - jl.credit) AS local_amt
    FROM journal_lines jl
    JOIN journal_entries je ON je.id = jl.je_id AND je.status = 'POSTED' AND je.period_id = $1
    JOIN consol_members cm ON cm.company_id = jl.dim_company_id AND cm.group_id = $2 AND cm.enabled
    JOIN companies c ON c.id = cm.company_id
    JOIN account_map am ON am.group_id = cm.group_id AND am.company_id = cm.company_id AND am.local_account_id = jl.account_id
    GROUP BY je.period_id, cm.group_id, am.group_account_id, cm.company_id, c.name
) AS base
GROUP BY base.group_account_id`
	if _, err = tx.Exec(ctx, insert, periodID, groupID); err != nil {
		return err
	}
	err = tx.Commit(ctx)
	return err
}

// ListGroupIDs returns the identifiers for all consolidation groups.
func (r *Repository) ListGroupIDs(ctx context.Context) ([]int64, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("consol repo not initialised")
	}
	rows, err := r.pool.Query(ctx, `SELECT id FROM consol_groups ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

// ActiveConsolidationPeriod returns the period code flagged as OPEN_CONSOL.
func (r *Repository) ActiveConsolidationPeriod(ctx context.Context) (string, error) {
	if r == nil || r.pool == nil {
		return "", fmt.Errorf("consol repo not initialised")
	}
	const query = `SELECT code FROM periods WHERE status = 'OPEN_CONSOL' ORDER BY start_date DESC LIMIT 1`
	var code string
	if err := r.pool.QueryRow(ctx, query).Scan(&code); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return code, nil
}

// BalanceRow maps to consolidated balance output from the materialised view.
type BalanceRow struct {
	GroupAccountID   int64
	GroupAccountCode string
	GroupAccountName string
	LocalAmount      float64
	GroupAmount      float64
	MembersJSON      []byte
}

// Balances retrieves consolidated balances for the given scope.
func (r *Repository) Balances(ctx context.Context, groupID, periodID int64) ([]BalanceRow, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("consol repo not initialised")
	}
	const query = `
SELECT mv.group_account_id,
       ga.code,
       ga.name,
       mv.local_ccy_amt,
       mv.group_ccy_amt,
       mv.members
FROM mv_consol_balances mv
JOIN consol_group_accounts ga ON ga.id = mv.group_account_id
WHERE mv.group_id = $1 AND mv.period_id = $2
ORDER BY ga.code`
	rows, err := r.pool.Query(ctx, query, groupID, periodID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var balances []BalanceRow
	for rows.Next() {
		var row BalanceRow
		if err := rows.Scan(&row.GroupAccountID, &row.GroupAccountCode, &row.GroupAccountName, &row.LocalAmount, &row.GroupAmount, &row.MembersJSON); err != nil {
			return nil, err
		}
		balances = append(balances, row)
	}
	return balances, rows.Err()
}

// ParseMembers decodes the JSON members payload from the materialised view.
func ParseMembers(data []byte) ([]MemberShare, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var raw []struct {
		CompanyID   int64   `json:"company_id"`
		CompanyName string  `json:"company_name"`
		LocalAmount float64 `json:"local_ccy_amt"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	members := make([]MemberShare, 0, len(raw))
	for _, item := range raw {
		members = append(members, MemberShare{
			CompanyID:   item.CompanyID,
			CompanyName: item.CompanyName,
			LocalAmount: item.LocalAmount,
		})
	}
	return members, nil
}

// FxRateForPeriod fetches the FX quote for a given pair and as-of date.
func (r *Repository) FxRateForPeriod(ctx context.Context, asOf time.Time, pair string) (fx.Quote, error) {
	var zero fx.Quote
	if r == nil || r.pool == nil {
		return zero, fmt.Errorf("consol repo not initialised")
	}
	pair = strings.ToUpper(strings.TrimSpace(pair))
	if pair == "" {
		return zero, fmt.Errorf("fx pair required")
	}
	if asOf.IsZero() {
		return zero, fmt.Errorf("as of date required")
	}
	asOf = time.Date(asOf.Year(), asOf.Month(), 1, 0, 0, 0, 0, time.UTC)
	const query = `SELECT average_rate, closing_rate FROM fx_rates WHERE as_of_date = $1 AND pair = $2 LIMIT 1`
	var quote fx.Quote
	if err := r.pool.QueryRow(ctx, query, asOf, pair).Scan(&quote.Average, &quote.Closing); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return zero, ErrFxRateNotFound
		}
		return zero, err
	}
	return quote, nil
}

// UpsertFxRates persists FX quotes, replacing existing rows when necessary.
func (r *Repository) UpsertFxRates(ctx context.Context, rows []FxRateInput) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("consol repo not initialised")
	}
	if len(rows) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	const query = `
INSERT INTO fx_rates (as_of_date, pair, average_rate, closing_rate)
VALUES ($1, $2, $3, $4)
ON CONFLICT (as_of_date, pair)
DO UPDATE SET average_rate = EXCLUDED.average_rate, closing_rate = EXCLUDED.closing_rate`
	for _, row := range rows {
		pair := strings.ToUpper(strings.TrimSpace(row.Pair))
		if pair == "" {
			return fmt.Errorf("fx pair required")
		}
		if row.AsOf.IsZero() {
			return fmt.Errorf("as of date required for pair %s", pair)
		}
		if row.Average <= 0 || row.Closing <= 0 {
			return fmt.Errorf("fx rates must be positive for %s %s", pair, row.AsOf.Format("2006-01"))
		}
		asOf := time.Date(row.AsOf.Year(), row.AsOf.Month(), 1, 0, 0, 0, 0, time.UTC)
		batch.Queue(query, asOf, pair, row.Average, row.Closing)
	}
	results := r.pool.SendBatch(ctx, batch)
	for range rows {
		if _, err := results.Exec(); err != nil {
			return err
		}
	}
	return results.Close()
}
