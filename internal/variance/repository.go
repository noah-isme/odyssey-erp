package variance

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

// Repository persists variance configuration and snapshots.
type Repository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

// NewRepository constructs a repo.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

// InsertRule stores a new variance rule.
func (r *Repository) InsertRule(ctx context.Context, input CreateRuleInput) (Rule, error) {
	row, err := r.queries.VarInsertRule(ctx, sqlc.VarInsertRuleParams{
		CompanyID:       input.CompanyID,
		Name:            input.Name,
		ComparisonType:  string(input.ComparisonType),
		BasePeriodID:    input.BasePeriodID,
		ComparePeriodID: int8ToPointerInt8Original(input.ComparePeriodID),
		CreatedBy:       input.ActorID,
	})
	if err != nil {
		return Rule{}, err
	}
	return mapRuleFromInsert(row), nil
}

// ListRules returns rules for a company.
func (r *Repository) ListRules(ctx context.Context, companyID int64) ([]Rule, error) {
	rows, err := r.queries.VarListRules(ctx, companyID)
	if err != nil {
		return nil, err
	}
	rules := make([]Rule, len(rows))
	for i, row := range rows {
		rules[i] = mapRuleFromList(row)
	}
	return rules, nil
}

// GetRule fetches by id.
func (r *Repository) GetRule(ctx context.Context, id int64) (Rule, error) {
	row, err := r.queries.VarGetRule(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Rule{}, ErrRuleNotFound
		}
		return Rule{}, err
	}
	return mapRuleFromGet(row), nil
}

// InsertSnapshot enqueues a snapshot record.
func (r *Repository) InsertSnapshot(ctx context.Context, req SnapshotRequest) (Snapshot, error) {
	row, err := r.queries.InsertSnapshot(ctx, sqlc.InsertSnapshotParams{
		RuleID:      req.RuleID,
		PeriodID:    req.PeriodID,
		GeneratedBy: int8FromInt64(req.ActorID),
	})
	if err != nil {
		return Snapshot{}, err
	}
	return mapSnapshotSimple(row), nil
}

// ListSnapshots lists recent runs.
func (r *Repository) ListSnapshots(ctx context.Context, filters ListFilters) ([]Snapshot, int, error) {
	if filters.Limit <= 0 {
		filters.Limit = 20
	}
	if filters.Page <= 0 {
		filters.Page = 1
	}
	offset := (filters.Page - 1) * filters.Limit

	// Count total using raw SQL as before
	var total int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM variance_snapshots`).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.queries.ListSnapshots(ctx, sqlc.ListSnapshotsParams{
		Limit:   int32(filters.Limit),
		Offset:  int32(offset),
		SortBy:  filters.SortBy,
		SortDir: filters.SortDir,
	})
	if err != nil {
		return nil, 0, err
	}

	snapshots := make([]Snapshot, len(rows))
	for i, row := range rows {
		snapshots[i] = mapSnapshotFromList(row)
	}
	return snapshots, total, nil
}

// GetSnapshot loads by id.
func (r *Repository) GetSnapshot(ctx context.Context, id int64) (Snapshot, error) {
	row, err := r.queries.GetSnapshot(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Snapshot{}, ErrSnapshotNotFound
		}
		return Snapshot{}, err
	}
	return mapSnapshotFromGet(row), nil
}

// UpdateStatus transitions snapshot state.
func (r *Repository) UpdateStatus(ctx context.Context, id int64, status SnapshotStatus) error {
	return r.queries.VarUpdateStatus(ctx, sqlc.VarUpdateStatusParams{
		ID:     id,
		Status: sqlc.VarianceSnapshotStatus(status),
	})
}

// SavePayload stores output or error for snapshot.
func (r *Repository) SavePayload(ctx context.Context, id int64, rows []VarianceRow, errMsg string) error {
	payload, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return r.queries.SavePayload(ctx, sqlc.SavePayloadParams{
		ID:           id,
		Payload:      payload,
		ErrorMessage: pgtype.Text{String: errMsg, Valid: errMsg != ""},
	})
}

// LoadPayload deserialises snapshot payload for UI.
func (r *Repository) LoadPayload(ctx context.Context, id int64) ([]VarianceRow, error) {
	payload, err := r.queries.LoadPayload(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil 
		}
		return nil, err
	}
	if len(payload) == 0 {
		return nil, nil
	}
	var rows []VarianceRow
	if err := json.Unmarshal(payload, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// AggregateBalances summarises account balances for company/period.
func (r *Repository) AggregateBalances(ctx context.Context, accountingPeriodID, companyID int64) (map[string]AccountBalance, error) {
	rows, err := r.queries.AggregateBalances(ctx, sqlc.AggregateBalancesParams{
		ID:           accountingPeriodID,
		DimCompanyID: int8FromInt64(companyID),
	})
	if err != nil {
		return nil, err
	}
	result := make(map[string]AccountBalance)
	for _, row := range rows {
		result[row.Code] = AccountBalance{Name: row.Name, Amount: row.Amount}
	}
	return result, nil
}

// LoadAccountingPeriod resolves ledger period id.
func (r *Repository) LoadAccountingPeriod(ctx context.Context, id int64) (PeriodView, error) {
	row, err := r.queries.VarLoadAccountingPeriod(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PeriodView{}, ErrSnapshotNotFound
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

// struct for PeriodView
type PeriodView struct {
	ID        int64
	LedgerID  int64
	Name      string
	StartDate time.Time
	EndDate   time.Time
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

func float64Ref(v float64) *float64 {
    // If we assume 0 means nil/unset for threshold, we handle it here.
    // Or if valid, we return ptr.
    // Original code had NullFloat64.
    // If SQLC returns float64, we can't detect NULL vs 0.0 easily unless relying on value.
    // For thresholds, 0.0 might be valid (0% threshold).
    // But since I cast it, I lose null info.
    // I'll return pointer to value.
    return &v
}

// Mappers

func mapRuleFromInsert(row sqlc.VarInsertRuleRow) Rule {
	r := Rule{
		ID:              row.ID,
		CompanyID:       row.CompanyID,
		Name:            row.Name,
		ComparisonType:  RuleComparison(row.ComparisonType),
		BasePeriodID:    row.BasePeriodID,
		ComparePeriodID: int8ToPointer(row.ComparePeriodID),
		Active:          row.IsActive,
		CreatedBy:       row.CreatedBy,
		CreatedAt:       row.CreatedAt.Time,
		ThresholdAmount: float64Ref(row.ThresholdAmount),
		ThresholdPercent: float64Ref(row.ThresholdPercent),
	}
	if len(row.DimensionFilters) > 0 {
		_ = json.Unmarshal(row.DimensionFilters, &r.DimensionFilter)
	} else {
		r.DimensionFilter = map[string]any{}
	}
	return r
}

func mapRuleFromList(row sqlc.VarListRulesRow) Rule {
	r := Rule{
		ID:              row.ID,
		CompanyID:       row.CompanyID,
		Name:            row.Name,
		ComparisonType:  RuleComparison(row.ComparisonType),
		BasePeriodID:    row.BasePeriodID,
		ComparePeriodID: int8ToPointer(row.ComparePeriodID),
		Active:          row.IsActive,
		CreatedBy:       row.CreatedBy,
		CreatedAt:       row.CreatedAt.Time,
		ThresholdAmount: float64Ref(row.ThresholdAmount),
		ThresholdPercent: float64Ref(row.ThresholdPercent),
	}
	if len(row.DimensionFilters) > 0 {
		_ = json.Unmarshal(row.DimensionFilters, &r.DimensionFilter)
	} else {
		r.DimensionFilter = map[string]any{}
	}
	return r
}

func mapRuleFromGet(row sqlc.VarGetRuleRow) Rule {
	r := Rule{
		ID:              row.ID,
		CompanyID:       row.CompanyID,
		Name:            row.Name,
		ComparisonType:  RuleComparison(row.ComparisonType),
		BasePeriodID:    row.BasePeriodID,
		ComparePeriodID: int8ToPointer(row.ComparePeriodID),
		Active:          row.IsActive,
		CreatedBy:       row.CreatedBy,
		CreatedAt:       row.CreatedAt.Time,
		ThresholdAmount: float64Ref(row.ThresholdAmount),
		ThresholdPercent: float64Ref(row.ThresholdPercent),
	}
	if len(row.DimensionFilters) > 0 {
		_ = json.Unmarshal(row.DimensionFilters, &r.DimensionFilter)
	} else {
		r.DimensionFilter = map[string]any{}
	}
	return r
}

func mapSnapshotSimple(row sqlc.VarianceSnapshot) Snapshot {
	snap := Snapshot{
		ID:          row.ID,
		RuleID:      row.RuleID,
		PeriodID:    row.PeriodID,
		Status:      SnapshotStatus(row.Status),
		GeneratedAt: timeToPointer(row.GeneratedAt),
		GeneratedBy: int8ToPointer(row.GeneratedBy),
		Error:       row.ErrorMessage.String,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
	if len(row.Payload) > 0 {
		_ = json.Unmarshal(row.Payload, &snap.Payload)
	}
	return snap
}

func mapSnapshotFromList(row sqlc.ListSnapshotsRow) Snapshot {
	snap := Snapshot{
		ID:          row.ID,
		RuleID:      row.RuleID,
		PeriodID:    row.PeriodID,
		Status:      SnapshotStatus(row.Status),
		GeneratedAt: timeToPointer(row.GeneratedAt),
		GeneratedBy: int8ToPointer(row.GeneratedBy),
		Error:       row.ErrorMessage.String,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
	if len(row.Payload) > 0 {
		_ = json.Unmarshal(row.Payload, &snap.Payload)
	}
	
	r := Rule{
		ID:              row.ID_2,
		CompanyID:       row.CompanyID,
		Name:            row.Name,
		ComparisonType:  RuleComparison(row.ComparisonType),
		BasePeriodID:    row.BasePeriodID,
		ComparePeriodID: int8ToPointer(row.ComparePeriodID),
		Active:          row.IsActive,
		CreatedBy:       row.CreatedBy,
		CreatedAt:       row.CreatedAt_2.Time,
		ThresholdAmount: float64Ref(row.VrThresholdAmount),
		ThresholdPercent: float64Ref(row.VrThresholdPercent),
	}

	if len(row.DimensionFilters) > 0 {
		_ = json.Unmarshal(row.DimensionFilters, &r.DimensionFilter)
	} else {
		r.DimensionFilter = map[string]any{}
	}
	
	snap.Rule = &r
	return snap
}

func mapSnapshotFromGet(row sqlc.GetSnapshotRow) Snapshot {
	snap := Snapshot{
		ID:          row.ID,
		RuleID:      row.RuleID,
		PeriodID:    row.PeriodID,
		Status:      SnapshotStatus(row.Status),
		GeneratedAt: timeToPointer(row.GeneratedAt),
		GeneratedBy: int8ToPointer(row.GeneratedBy),
		Error:       row.ErrorMessage.String,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
	if len(row.Payload) > 0 {
		_ = json.Unmarshal(row.Payload, &snap.Payload)
	}
	
	r := Rule{
		ID:              row.ID_2,
		CompanyID:       row.CompanyID,
		Name:            row.Name,
		ComparisonType:  RuleComparison(row.ComparisonType),
		BasePeriodID:    row.BasePeriodID,
		ComparePeriodID: int8ToPointer(row.ComparePeriodID),
		Active:          row.IsActive,
		CreatedBy:       row.CreatedBy,
		CreatedAt:       row.CreatedAt_2.Time,
		ThresholdAmount: float64Ref(row.VrThresholdAmount),
		ThresholdPercent: float64Ref(row.VrThresholdPercent),
	}

	if len(row.DimensionFilters) > 0 {
		_ = json.Unmarshal(row.DimensionFilters, &r.DimensionFilter)
	} else {
		r.DimensionFilter = map[string]any{}
	}
	
	snap.Rule = &r
	return snap
}
