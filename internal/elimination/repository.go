package elimination

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists elimination rules and runs.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a Repository instance.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ListRules returns the latest elimination rules.
func (r *Repository) ListRules(ctx context.Context, limit int) ([]Rule, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("elimination: repository not initialised")
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
SELECT id, group_id, name, source_company_id, target_company_id, account_src, account_tgt, match_criteria,
       is_active, created_by, created_at, updated_at
FROM elimination_rules
ORDER BY created_at DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []Rule
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

// InsertRule inserts a new rule row.
func (r *Repository) InsertRule(ctx context.Context, input CreateRuleInput) (Rule, error) {
	if r == nil || r.pool == nil {
		return Rule{}, fmt.Errorf("elimination: repository not initialised")
	}
	var rule Rule
	var groupID sql.NullInt64
	if input.GroupID != nil {
		groupID = sql.NullInt64{Int64: *input.GroupID, Valid: true}
	}
	criteria, err := json.Marshal(input.MatchCriteria)
	if err != nil {
		return Rule{}, err
	}
	err = r.pool.QueryRow(ctx, `
INSERT INTO elimination_rules (group_id, name, source_company_id, target_company_id, account_src, account_tgt, match_criteria, created_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING id, group_id, name, source_company_id, target_company_id, account_src, account_tgt, match_criteria,
          is_active, created_by, created_at, updated_at`,
		groupID, input.Name, input.SourceCompanyID, input.TargetCompanyID, input.AccountSource, input.AccountTarget, criteria, input.ActorID,
	).Scan(
		&rule.ID,
		&groupID,
		&rule.Name,
		&rule.SourceCompanyID,
		&rule.TargetCompanyID,
		&rule.AccountSource,
		&rule.AccountTarget,
		&criteria,
		&rule.Active,
		&rule.CreatedBy,
		&rule.CreatedAt,
		&rule.UpdatedAt,
	)
	if err != nil {
		return Rule{}, err
	}
	if groupID.Valid {
		v := groupID.Int64
		rule.GroupID = &v
	}
	if err := json.Unmarshal(criteria, &rule.MatchCriteria); err != nil {
		rule.MatchCriteria = map[string]any{}
	}
	return rule, nil
}

// GetRule loads a rule by id.
func (r *Repository) GetRule(ctx context.Context, id int64) (Rule, error) {
	if r == nil || r.pool == nil {
		return Rule{}, fmt.Errorf("elimination: repository not initialised")
	}
	row := r.pool.QueryRow(ctx, `
SELECT id, group_id, name, source_company_id, target_company_id, account_src, account_tgt, match_criteria,
       is_active, created_by, created_at, updated_at
FROM elimination_rules WHERE id = $1`, id)
	rule, err := scanRule(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Rule{}, ErrRuleNotFound
		}
		return Rule{}, err
	}
	return rule, nil
}

// ListRuns returns recent elimination runs ordered by creation date.
func (r *Repository) ListRuns(ctx context.Context, filters ListFilters) ([]Run, int, error) {
	if r == nil || r.pool == nil {
		return nil, 0, fmt.Errorf("elimination: repository not initialised")
	}
	
	// Defaults
	if filters.Limit <= 0 {
		filters.Limit = 20
	}
	if filters.Page <= 0 {
		filters.Page = 1
	}
	offset := (filters.Page - 1) * filters.Limit

	// Count total
	var total int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM elimination_runs`).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Build Order By
	orderBy := "er.created_at"
	orderDir := "DESC"
	if filters.SortBy != "" {
		switch filters.SortBy {
		case "created_at":
			orderBy = "er.created_at"
		case "status":
			orderBy = "er.status"
		case "period_id":
			orderBy = "er.period_id"
		}
	}
	if filters.SortDir == "asc" {
		orderDir = "ASC"
	}
	
	query := fmt.Sprintf(`
		SELECT er.id, er.period_id, er.rule_id, er.status, er.created_by, er.created_at, er.simulated_at, er.posted_at, er.journal_entry_id, er.summary,
		       ru.id, ru.group_id, ru.name, ru.source_company_id, ru.target_company_id, ru.account_src, ru.account_tgt, ru.match_criteria,
		       ru.is_active, ru.created_by, ru.created_at, ru.updated_at
		FROM elimination_runs er
		JOIN elimination_rules ru ON ru.id = er.rule_id
		ORDER BY %s %s
		LIMIT $1 OFFSET $2`, orderBy, orderDir)

	rows, err := r.pool.Query(ctx, query, filters.Limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var runs []Run
	for rows.Next() {
		run, err := scanRunWithRule(rows)
		if err != nil {
			return nil, 0, err
		}
		runs = append(runs, run)
	}
	return runs, total, rows.Err()
}

// InsertRun creates a new run with draft status.
func (r *Repository) InsertRun(ctx context.Context, input CreateRunInput) (Run, error) {
	if r == nil || r.pool == nil {
		return Run{}, fmt.Errorf("elimination: repository not initialised")
	}
	var run Run
	var summary sql.NullString
	err := r.pool.QueryRow(ctx, `
INSERT INTO elimination_runs (period_id, rule_id, status, created_by)
VALUES ($1,$2,'DRAFT',$3)
RETURNING id, period_id, rule_id, status, created_by, created_at, simulated_at, posted_at, journal_entry_id, summary`,
		input.PeriodID, input.RuleID, input.ActorID,
	).Scan(
		&run.ID,
		&run.PeriodID,
		&run.RuleID,
		&run.Status,
		&run.CreatedBy,
		&run.CreatedAt,
		&run.SimulatedAt,
		&run.PostedAt,
		&run.JournalEntry,
		&summary,
	)
	if err != nil {
		return Run{}, err
	}
	if summary.Valid && summary.String != "" {
		var parsed SimulationSummary
		if err := json.Unmarshal([]byte(summary.String), &parsed); err == nil {
			run.Summary = &parsed
		}
	}
	return run, nil
}

// GetRun loads an elimination run and optional summary.
func (r *Repository) GetRun(ctx context.Context, id int64) (Run, error) {
	if r == nil || r.pool == nil {
		return Run{}, fmt.Errorf("elimination: repository not initialised")
	}
	row := r.pool.QueryRow(ctx, `
SELECT er.id, er.period_id, er.rule_id, er.status, er.created_by, er.created_at, er.simulated_at, er.posted_at, er.journal_entry_id, er.summary,
       ru.id, ru.group_id, ru.name, ru.source_company_id, ru.target_company_id, ru.account_src, ru.account_tgt, ru.match_criteria,
       ru.is_active, ru.created_by, ru.created_at, ru.updated_at
FROM elimination_runs er
JOIN elimination_rules ru ON ru.id = er.rule_id
WHERE er.id = $1`, id)
	run, err := scanRunWithRule(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Run{}, ErrRunNotFound
		}
		return Run{}, err
	}
	return run, nil
}

// SaveRunSimulation persists simulation summary and status.
func (r *Repository) SaveRunSimulation(ctx context.Context, id int64, summary SimulationSummary, status RunStatus, simulatedAt time.Time) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("elimination: repository not initialised")
	}
	payload, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	tag, err := r.pool.Exec(ctx, `
UPDATE elimination_runs
SET status = $2,
    simulated_at = $3,
    summary = $4
WHERE id = $1`, id, status, simulatedAt, payload)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrRunNotFound
	}
	return nil
}

// MarkRunPosted stores posting metadata for a run.
func (r *Repository) MarkRunPosted(ctx context.Context, id int64, journalID int64, postedAt time.Time) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("elimination: repository not initialised")
	}
	tag, err := r.pool.Exec(ctx, `
UPDATE elimination_runs
SET status = 'POSTED',
    posted_at = $2,
    journal_entry_id = $3
WHERE id = $1`, id, postedAt, journalID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrRunNotFound
	}
	return nil
}

// SumAccountBalance aggregates net balance for company+account in a period.
func (r *Repository) SumAccountBalance(ctx context.Context, accountingPeriodID, companyID int64, accountCode string) (float64, error) {
	if r == nil || r.pool == nil {
		return 0, fmt.Errorf("elimination: repository not initialised")
	}
	var balance sql.NullFloat64
	err := r.pool.QueryRow(ctx, `
SELECT COALESCE(SUM(jl.debit - jl.credit), 0)
FROM journal_lines jl
JOIN journal_entries je ON je.id = jl.je_id AND je.status = 'POSTED'
JOIN accounts acc ON acc.id = jl.account_id
JOIN accounting_periods ap ON ap.id = $1
WHERE je.period_id = ap.period_id
  AND acc.code = $2
  AND COALESCE(jl.dim_company_id, 0) = $3`, accountingPeriodID, accountCode, companyID).
		Scan(&balance)
	if err != nil {
		return 0, err
	}
	if !balance.Valid {
		return 0, nil
	}
	return balance.Float64, nil
}

// LookupAccountID resolves an account code to identifier.
func (r *Repository) LookupAccountID(ctx context.Context, code string) (int64, error) {
	if r == nil || r.pool == nil {
		return 0, fmt.Errorf("elimination: repository not initialised")
	}
	var id int64
	err := r.pool.QueryRow(ctx, `SELECT id FROM accounts WHERE code = $1`, code).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrAccountNotFound
		}
		return 0, err
	}
	return id, nil
}

// LoadAccountingPeriod returns ledger metadata for run posting.
func (r *Repository) LoadAccountingPeriod(ctx context.Context, id int64) (PeriodView, error) {
	if r == nil || r.pool == nil {
		return PeriodView{}, fmt.Errorf("elimination: repository not initialised")
	}
	var period PeriodView
	err := r.pool.QueryRow(ctx, `
SELECT ap.id, ap.period_id, ap.name, ap.start_date, ap.end_date
FROM accounting_periods ap
WHERE ap.id = $1`, id).Scan(&period.ID, &period.LedgerID, &period.Name, &period.StartDate, &period.EndDate)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PeriodView{}, ErrPeriodNotFound
		}
		return PeriodView{}, err
	}
	return period, nil
}

// ListRecentPeriods fetches the latest accounting periods for UI selection.
func (r *Repository) ListRecentPeriods(ctx context.Context, limit int) ([]PeriodView, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("elimination: repository not initialised")
	}
	if limit <= 0 {
		limit = 12
	}
	rows, err := r.pool.Query(ctx, `
SELECT ap.id, ap.period_id, ap.name, ap.start_date, ap.end_date
FROM accounting_periods ap
ORDER BY ap.start_date DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var periods []PeriodView
	for rows.Next() {
		var p PeriodView
		if err := rows.Scan(&p.ID, &p.LedgerID, &p.Name, &p.StartDate, &p.EndDate); err != nil {
			return nil, err
		}
		periods = append(periods, p)
	}
	return periods, rows.Err()
}

func scanRule(row interface{ Scan(dest ...any) error }) (Rule, error) {
	var rule Rule
	var groupID sql.NullInt64
	var raw json.RawMessage
	if err := row.Scan(
		&rule.ID,
		&groupID,
		&rule.Name,
		&rule.SourceCompanyID,
		&rule.TargetCompanyID,
		&rule.AccountSource,
		&rule.AccountTarget,
		&raw,
		&rule.Active,
		&rule.CreatedBy,
		&rule.CreatedAt,
		&rule.UpdatedAt,
	); err != nil {
		return Rule{}, err
	}
	if groupID.Valid {
		v := groupID.Int64
		rule.GroupID = &v
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &rule.MatchCriteria); err != nil {
			rule.MatchCriteria = map[string]any{}
		}
	} else {
		rule.MatchCriteria = map[string]any{}
	}
	return rule, nil
}

func scanRunWithRule(row interface{ Scan(dest ...any) error }) (Run, error) {
	var run Run
	var summary sql.NullString
	var simulated, posted sql.NullTime
	var journal sql.NullInt64
	var raw json.RawMessage
	var groupID sql.NullInt64
	var rule Rule
	if err := row.Scan(
		&run.ID,
		&run.PeriodID,
		&run.RuleID,
		&run.Status,
		&run.CreatedBy,
		&run.CreatedAt,
		&simulated,
		&posted,
		&journal,
		&summary,
		&rule.ID,
		&groupID,
		&rule.Name,
		&rule.SourceCompanyID,
		&rule.TargetCompanyID,
		&rule.AccountSource,
		&rule.AccountTarget,
		&raw,
		&rule.Active,
		&rule.CreatedBy,
		&rule.CreatedAt,
		&rule.UpdatedAt,
	); err != nil {
		return Run{}, err
	}
	if simulated.Valid {
		run.SimulatedAt = &simulated.Time
	}
	if posted.Valid {
		run.PostedAt = &posted.Time
	}
	if journal.Valid {
		v := journal.Int64
		run.JournalEntry = &v
	}
	if summary.Valid && summary.String != "" {
		var parsed SimulationSummary
		if err := json.Unmarshal([]byte(summary.String), &parsed); err == nil {
			run.Summary = &parsed
		}
	}
	if groupID.Valid {
		v := groupID.Int64
		rule.GroupID = &v
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &rule.MatchCriteria); err != nil {
			rule.MatchCriteria = map[string]any{}
		}
	} else {
		rule.MatchCriteria = map[string]any{}
	}
	run.Rule = &rule
	return run, nil
}
