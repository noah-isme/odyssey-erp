package ic

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository exposes persistence helpers for intercompany elimination workloads.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs an IC repository backed by a pgx pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// PairExposure represents the AR/AP exposure between two companies within a group and period.
type PairExposure struct {
	GroupID          int64
	PeriodID         int64
	PeriodCode       string
	CompanyAID       int64
	CompanyAName     string
	CompanyBID       int64
	CompanyBName     string
	ARGroupAccountID int64
	APGroupAccountID int64
	ARAmount         float64
	APAmount         float64
}

// UpsertLine describes a debit/credit line for an elimination journal.
type UpsertLine struct {
	GroupAccountID int64
	Debit          float64
	Credit         float64
	Memo           string
}

// UpsertParams configures the elimination upsert behaviour.
type UpsertParams struct {
	GroupID    int64
	PeriodID   int64
	SourceLink string
	CreatedBy  int64
	Lines      []UpsertLine
}

// UpsertResult summarises the database outcome for an elimination upsert.
type UpsertResult struct {
	HeaderID int64
	Created  bool
}

// ResolvePeriodID fetches the identifier for a given accounting period code.
func (r *Repository) ResolvePeriodID(ctx context.Context, code string) (int64, error) {
	if r == nil || r.pool == nil {
		return 0, fmt.Errorf("ic repo not initialised")
	}
	if code == "" {
		return 0, fmt.Errorf("period code is required")
	}
	const query = `SELECT id FROM periods WHERE code = $1`
	var id int64
	if err := r.pool.QueryRow(ctx, query, code).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("period %s not found", code)
		}
		return 0, err
	}
	return id, nil
}

// ListPairExposures retrieves the AR/AP pair balances for the provided scope.
func (r *Repository) ListPairExposures(ctx context.Context, groupID, periodID int64, periodCode string) ([]PairExposure, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("ic repo not initialised")
	}
	const query = `
SELECT
    p.group_id,
    p.period_id,
    p.company_a_id,
    ca.name AS company_a_name,
    p.company_b_id,
    cb.name AS company_b_name,
    p.ar_group_account_id,
    p.ap_group_account_id,
    p.ar_amount,
    p.ap_amount
FROM ic_arap_pairs p
JOIN companies ca ON ca.id = p.company_a_id
JOIN companies cb ON cb.id = p.company_b_id
WHERE p.group_id = $1
  AND p.period_id = $2
ORDER BY ca.name, cb.name`
	rows, err := r.pool.Query(ctx, query, groupID, periodID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	exposures := make([]PairExposure, 0)
	for rows.Next() {
		var row PairExposure
		if err := rows.Scan(
			&row.GroupID,
			&row.PeriodID,
			&row.CompanyAID,
			&row.CompanyAName,
			&row.CompanyBID,
			&row.CompanyBName,
			&row.ARGroupAccountID,
			&row.APGroupAccountID,
			&row.ARAmount,
			&row.APAmount,
		); err != nil {
			return nil, err
		}
		row.PeriodCode = periodCode
		exposures = append(exposures, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return exposures, nil
}

// UpsertElimination ensures the elimination journal header and lines match the provided payload.
func (r *Repository) UpsertElimination(ctx context.Context, params UpsertParams) (UpsertResult, error) {
	if r == nil || r.pool == nil {
		return UpsertResult{}, fmt.Errorf("ic repo not initialised")
	}
	if params.GroupID <= 0 || params.PeriodID <= 0 {
		return UpsertResult{}, fmt.Errorf("invalid group/period scope")
	}
	if params.SourceLink == "" {
		return UpsertResult{}, fmt.Errorf("source link required")
	}
	if len(params.Lines) == 0 {
		return UpsertResult{}, fmt.Errorf("no journal lines provided")
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return UpsertResult{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var headerID int64
	var created bool
	const findHeader = `SELECT id FROM elimination_journal_headers WHERE group_id = $1 AND period_id = $2 AND source_link = $3 FOR UPDATE`
	if err = tx.QueryRow(ctx, findHeader, params.GroupID, params.PeriodID, params.SourceLink).Scan(&headerID); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return UpsertResult{}, err
		}
		createdBy := params.CreatedBy
		if createdBy <= 0 {
			if err = tx.QueryRow(ctx, `SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&createdBy); err != nil {
				return UpsertResult{}, fmt.Errorf("resolve system actor: %w", err)
			}
		}
		const insertHeader = `
INSERT INTO elimination_journal_headers (group_id, period_id, status, source_link, created_by)
VALUES ($1, $2, 'DRAFT', $3, $4)
RETURNING id`
		if err = tx.QueryRow(ctx, insertHeader, params.GroupID, params.PeriodID, params.SourceLink, createdBy).Scan(&headerID); err != nil {
			return UpsertResult{}, err
		}
		created = true
	} else {
		err = nil
	}

	if _, err = tx.Exec(ctx, `DELETE FROM elimination_journal_lines WHERE header_id = $1`, headerID); err != nil {
		return UpsertResult{}, err
	}
	const insertLine = `
INSERT INTO elimination_journal_lines (header_id, group_account_id, debit, credit, memo)
VALUES ($1, $2, $3, $4, $5)`
	for _, line := range params.Lines {
		if _, err = tx.Exec(ctx, insertLine, headerID, line.GroupAccountID, line.Debit, line.Credit, line.Memo); err != nil {
			return UpsertResult{}, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE elimination_journal_headers SET updated_at = NOW() WHERE id = $1`, headerID); err != nil {
		return UpsertResult{}, err
	}
	err = tx.Commit(ctx)
	if err != nil {
		return UpsertResult{}, err
	}
	return UpsertResult{HeaderID: headerID, Created: created}, nil
}
