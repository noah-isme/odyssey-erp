package elimination

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// Repository persists elimination rules and runs.
type Repository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

// NewRepository constructs a Repository instance.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

// ListRules returns the latest elimination rules.
func (r *Repository) ListRules(ctx context.Context, limit int) ([]Rule, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.queries.ElimListRules(ctx, int32(limit))
	if err != nil {
		return nil, err
	}
	rules := make([]Rule, len(rows))
	for i, row := range rows {
		rules[i] = mapRule(row)
	}
	return rules, nil
}

// InsertRule inserts a new rule row.
func (r *Repository) InsertRule(ctx context.Context, input CreateRuleInput) (Rule, error) {
	criteria, err := json.Marshal(input.MatchCriteria)
	if err != nil {
		return Rule{}, err
	}
	
	row, err := r.queries.ElimInsertRule(ctx, sqlc.ElimInsertRuleParams{
		GroupID:         int8ToPointerInt8Original(input.GroupID),
		Name:            input.Name,
		SourceCompanyID: input.SourceCompanyID,
		TargetCompanyID: input.TargetCompanyID,
		AccountSrc:      input.AccountSource,
		AccountTgt:      input.AccountTarget,
		MatchCriteria:   criteria,
		CreatedBy:       input.ActorID,
	})
	if err != nil {
		return Rule{}, err
	}
	return mapRule(row), nil
}

// GetRule loads a rule by id.
func (r *Repository) GetRule(ctx context.Context, id int64) (Rule, error) {
	row, err := r.queries.ElimGetRule(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Rule{}, ErrRuleNotFound
		}
		return Rule{}, err
	}
	return mapRule(row), nil
}

// ListRuns returns recent elimination runs ordered by creation date.
func (r *Repository) ListRuns(ctx context.Context, filters ListFilters) ([]Run, int, error) {
	// Defaults
	if filters.Limit <= 0 {
		filters.Limit = 20
	}
	if filters.Page <= 0 {
		filters.Page = 1
	}
	offset := (filters.Page - 1) * filters.Limit

	// Count total
	total, err := r.queries.CountRuns(ctx)
	if err != nil {
		return nil, 0, err
	}

	// Fetch runs
	rows, err := r.queries.ListRuns(ctx, sqlc.ListRunsParams{
		Limit:   int32(filters.Limit),
		Offset:  int32(offset),
		SortBy:  filters.SortBy,
		SortDir: filters.SortDir,
	})
	if err != nil {
		return nil, 0, err
	}

	runs := make([]Run, len(rows))
	for i, row := range rows {
		runs[i] = mapRunFromList(row)
	}
	return runs, int(total), nil
}

// InsertRun creates a new run with draft status.
func (r *Repository) InsertRun(ctx context.Context, input CreateRunInput) (Run, error) {
	row, err := r.queries.InsertRun(ctx, sqlc.InsertRunParams{
		PeriodID:  input.PeriodID,
		RuleID:    input.RuleID,
		CreatedBy: input.ActorID,
	})
	if err != nil {
		return Run{}, err
	}
	
	return Run{
		ID:          row.ID,
		PeriodID:    row.PeriodID,
		RuleID:      row.RuleID,
		Status:      RunStatus(row.Status),
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt.Time,
		SimulatedAt: timeToPointer(row.SimulatedAt),
		PostedAt:    timeToPointer(row.PostedAt),
		JournalEntry: int8ToPointer(row.JournalEntryID),
		Summary:      nil, 
	}, nil
}

// GetRun loads an elimination run and optional summary.
func (r *Repository) GetRun(ctx context.Context, id int64) (Run, error) {
	row, err := r.queries.GetRun(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Run{}, ErrRunNotFound
		}
		return Run{}, err
	}
	return mapRunFromGet(row), nil
}

// SaveRunSimulation persists simulation summary and status.
func (r *Repository) SaveRunSimulation(ctx context.Context, id int64, summary SimulationSummary, status RunStatus, simulatedAt time.Time) error {
	payload, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	err = r.queries.SaveRunSimulation(ctx, sqlc.SaveRunSimulationParams{
		ID:          id,
		Status:      sqlc.EliminationRunStatus(status),
		SimulatedAt: pgtype.Timestamptz{Time: simulatedAt, Valid: true},
		Summary:     payload,
	})
	return err // Exec returns only error
}

// MarkRunPosted stores posting metadata for a run.
func (r *Repository) MarkRunPosted(ctx context.Context, id int64, journalID int64, postedAt time.Time) error {
	return r.queries.MarkRunPosted(ctx, sqlc.MarkRunPostedParams{
		ID:             id,
		PostedAt:       pgtype.Timestamptz{Time: postedAt, Valid: true},
		JournalEntryID: int8FromInt64(journalID),
	})
}

// SumAccountBalance aggregates net balance for company+account in a period.
func (r *Repository) SumAccountBalance(ctx context.Context, accountingPeriodID, companyID int64, accountCode string) (float64, error) {
	val, err := r.queries.SumAccountBalance(ctx, sqlc.SumAccountBalanceParams{
		ID:           accountingPeriodID,
		Code:         accountCode,
		DimCompanyID: int8FromInt64(companyID),
	})
	if err != nil {
		return 0, err
	}
	return val, nil
}

// LookupAccountID resolves an account code to identifier.
func (r *Repository) LookupAccountID(ctx context.Context, code string) (int64, error) {
	id, err := r.queries.LookupAccountID(ctx, code)
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
	row, err := r.queries.ElimLoadAccountingPeriod(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PeriodView{}, ErrPeriodNotFound
		}
		return PeriodView{}, err
	}
	return PeriodView{
		ID:        row.ID,
		LedgerID:  row.PeriodID,
		Name:      row.Name,
		StartDate: row.StartDate.Time,
		EndDate:   row.EndDate.Time,
	}, nil
}

// ListRecentPeriods fetches the latest accounting periods for UI selection.
func (r *Repository) ListRecentPeriods(ctx context.Context, limit int) ([]PeriodView, error) {
	if limit <= 0 {
		limit = 12
	}
	rows, err := r.queries.ElimListRecentPeriods(ctx, int32(limit))
	if err != nil {
		return nil, err
	}
	periods := make([]PeriodView, len(rows))
	for i, row := range rows {
		periods[i] = PeriodView{
			ID:        row.ID,
			LedgerID:  row.PeriodID,
			Name:      row.Name,
			StartDate: row.StartDate.Time,
			EndDate:   row.EndDate.Time,
		}
	}
	return periods, nil
}

// Helpers

func int8ToPointerInt8Original(i *int64) pgtype.Int8 {
	if i == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *i, Valid: true}
}

func int8FromInt64(i int64) pgtype.Int8 {
    if i == 0 {
        return pgtype.Int8{}
    }
    return pgtype.Int8{Int64: i, Valid: true}
}

func int8ToPointer(i pgtype.Int8) *int64 {
	if !i.Valid {
		return nil
	}
	v := i.Int64
	return &v
}

func timeToPointer(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// Mappers

func mapRule(row sqlc.EliminationRule) Rule {
	r := Rule{
		ID:              row.ID,
		GroupID:         int8ToPointer(row.GroupID),
		Name:            row.Name,
		SourceCompanyID: row.SourceCompanyID,
		TargetCompanyID: row.TargetCompanyID,
		AccountSource:   row.AccountSrc,
		AccountTarget:   row.AccountTgt,
		Active:          row.IsActive,
		CreatedBy:       row.CreatedBy,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}
	if len(row.MatchCriteria) > 0 {
		_ = json.Unmarshal(row.MatchCriteria, &r.MatchCriteria)
	} else {
		r.MatchCriteria = map[string]any{}
	}
	return r
}

func mapRunFromList(row sqlc.ListRunsRow) Run {
	run := Run{
		ID:           row.ID,
		PeriodID:     row.PeriodID,
		RuleID:       row.RuleID,
		Status:       RunStatus(row.Status),
		CreatedBy:    row.CreatedBy,
		CreatedAt:    row.CreatedAt.Time,
		SimulatedAt:  timeToPointer(row.SimulatedAt),
		PostedAt:     timeToPointer(row.PostedAt),
		JournalEntry: int8ToPointer(row.JournalEntryID),
	}
	if len(row.Summary) > 0 {
		var s SimulationSummary
		if err := json.Unmarshal(row.Summary, &s); err == nil {
			run.Summary = &s
		}
	}
	// Map Rule
	rule := Rule{
		ID:              row.ID_2,
		GroupID:         int8ToPointer(row.GroupID),
		Name:            row.Name,
		SourceCompanyID: row.SourceCompanyID,
		TargetCompanyID: row.TargetCompanyID,
		AccountSource:   row.AccountSrc,
		AccountTarget:   row.AccountTgt,
		Active:          row.IsActive,
		CreatedBy:       row.CreatedBy_2,
		CreatedAt:       row.CreatedAt_2.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}
	if len(row.MatchCriteria) > 0 {
		_ = json.Unmarshal(row.MatchCriteria, &rule.MatchCriteria)
	} else {
		rule.MatchCriteria = map[string]any{}
	}
	run.Rule = &rule
	return run
}

func mapRunFromGet(row sqlc.GetRunRow) Run {
	run := Run{
		ID:           row.ID,
		PeriodID:     row.PeriodID,
		RuleID:       row.RuleID,
		Status:       RunStatus(row.Status),
		CreatedBy:    row.CreatedBy,
		CreatedAt:    row.CreatedAt.Time,
		SimulatedAt:  timeToPointer(row.SimulatedAt),
		PostedAt:     timeToPointer(row.PostedAt),
		JournalEntry: int8ToPointer(row.JournalEntryID),
	}
	if len(row.Summary) > 0 {
		var s SimulationSummary
		if err := json.Unmarshal(row.Summary, &s); err == nil {
			run.Summary = &s
		}
	}
	// Map Rule
	rule := Rule{
		ID:              row.ID_2,
		GroupID:         int8ToPointer(row.GroupID),
		Name:            row.Name,
		SourceCompanyID: row.SourceCompanyID,
		TargetCompanyID: row.TargetCompanyID,
		AccountSource:   row.AccountSrc,
		AccountTarget:   row.AccountTgt,
		Active:          row.IsActive,
		CreatedBy:       row.CreatedBy_2,
		CreatedAt:       row.CreatedAt_2.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}
	if len(row.MatchCriteria) > 0 {
		_ = json.Unmarshal(row.MatchCriteria, &rule.MatchCriteria)
	} else {
		rule.MatchCriteria = map[string]any{}
	}
	run.Rule = &rule
	return run
}
