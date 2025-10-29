package consol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
