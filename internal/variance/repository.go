package variance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists variance configuration and snapshots.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a repo.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// InsertRule stores a new variance rule.
func (r *Repository) InsertRule(ctx context.Context, input CreateRuleInput) (Rule, error) {
	if r == nil || r.pool == nil {
		return Rule{}, fmt.Errorf("variance: repository not initialised")
	}
	var rule Rule
	var filters []byte
	var compare sql.NullInt64
	var thresholdAmt, thresholdPct sql.NullFloat64
	err := r.pool.QueryRow(ctx, `
INSERT INTO variance_rules (company_id, name, comparison_type, base_period_id, compare_period_id, created_by)
VALUES ($1,$2,$3,$4,$5,$6)
RETURNING id, company_id, name, comparison_type, base_period_id, compare_period_id, dimension_filters,
          threshold_amount, threshold_percent, is_active, created_by, created_at`,
		input.CompanyID, input.Name, input.ComparisonType, input.BasePeriodID, input.ComparePeriodID, input.ActorID,
	).Scan(
		&rule.ID, &rule.CompanyID, &rule.Name, &rule.ComparisonType, &rule.BasePeriodID, &compare,
		&filters, &thresholdAmt, &thresholdPct, &rule.Active, &rule.CreatedBy, &rule.CreatedAt,
	)
	if err != nil {
		return Rule{}, err
	}
	if compare.Valid {
		v := compare.Int64
		rule.ComparePeriodID = &v
	}
	if thresholdAmt.Valid {
		v := thresholdAmt.Float64
		rule.ThresholdAmount = &v
	}
	if thresholdPct.Valid {
		v := thresholdPct.Float64
		rule.ThresholdPercent = &v
	}
	if len(filters) > 0 {
		if err := json.Unmarshal(filters, &rule.DimensionFilter); err != nil {
			rule.DimensionFilter = map[string]any{}
		}
	} else {
		rule.DimensionFilter = map[string]any{}
	}
	rule.ComparisonType = RuleComparison(rule.ComparisonType)
	return rule, nil
}

// ListRules returns rules for a company.
func (r *Repository) ListRules(ctx context.Context, companyID int64) ([]Rule, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("variance: repository not initialised")
	}
	rows, err := r.pool.Query(ctx, `
SELECT id, company_id, name, comparison_type, base_period_id, compare_period_id, dimension_filters,
       threshold_amount, threshold_percent, is_active, created_by, created_at
FROM variance_rules
WHERE ($1 = 0 OR company_id = $1)
ORDER BY created_at DESC
LIMIT 100`, companyID)
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

// GetRule fetches by id.
func (r *Repository) GetRule(ctx context.Context, id int64) (Rule, error) {
	if r == nil || r.pool == nil {
		return Rule{}, fmt.Errorf("variance: repository not initialised")
	}
	row := r.pool.QueryRow(ctx, `
SELECT id, company_id, name, comparison_type, base_period_id, compare_period_id, dimension_filters,
       threshold_amount, threshold_percent, is_active, created_by, created_at
FROM variance_rules WHERE id = $1`, id)
	rule, err := scanRule(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Rule{}, ErrRuleNotFound
		}
		return Rule{}, err
	}
	return rule, nil
}

// InsertSnapshot enqueues a snapshot record.
func (r *Repository) InsertSnapshot(ctx context.Context, req SnapshotRequest) (Snapshot, error) {
	if r == nil || r.pool == nil {
		return Snapshot{}, fmt.Errorf("variance: repository not initialised")
	}
	var snapshot Snapshot
	err := r.pool.QueryRow(ctx, `
INSERT INTO variance_snapshots (rule_id, period_id, status, generated_by)
VALUES ($1,$2,'PENDING',$3)
RETURNING id, rule_id, period_id, status, generated_at, generated_by, error_message, payload, created_at, updated_at`,
		req.RuleID, req.PeriodID, req.ActorID,
	).Scan(
		&snapshot.ID, &snapshot.RuleID, &snapshot.PeriodID, &snapshot.Status,
		&snapshot.GeneratedAt, &snapshot.GeneratedBy, &snapshot.Error, &snapshot.Payload, &snapshot.CreatedAt, &snapshot.UpdatedAt,
	)
	return snapshot, err
}

// ListSnapshots lists recent runs.
func (r *Repository) ListSnapshots(ctx context.Context, limit int) ([]Snapshot, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("variance: repository not initialised")
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
SELECT vs.id, vs.rule_id, vs.period_id, vs.status, vs.generated_at, vs.generated_by, vs.error_message, vs.payload,
       vs.created_at, vs.updated_at,
       vr.id, vr.company_id, vr.name, vr.comparison_type, vr.base_period_id, vr.compare_period_id, vr.dimension_filters,
       vr.threshold_amount, vr.threshold_percent, vr.is_active, vr.created_by, vr.created_at
FROM variance_snapshots vs
JOIN variance_rules vr ON vr.id = vs.rule_id
ORDER BY vs.created_at DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var snapshots []Snapshot
	for rows.Next() {
		snap, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snap)
	}
	return snapshots, rows.Err()
}

// GetSnapshot loads by id.
func (r *Repository) GetSnapshot(ctx context.Context, id int64) (Snapshot, error) {
	if r == nil || r.pool == nil {
		return Snapshot{}, fmt.Errorf("variance: repository not initialised")
	}
	row := r.pool.QueryRow(ctx, `
SELECT vs.id, vs.rule_id, vs.period_id, vs.status, vs.generated_at, vs.generated_by, vs.error_message, vs.payload,
       vs.created_at, vs.updated_at,
       vr.id, vr.company_id, vr.name, vr.comparison_type, vr.base_period_id, vr.compare_period_id, vr.dimension_filters,
       vr.threshold_amount, vr.threshold_percent, vr.is_active, vr.created_by, vr.created_at
FROM variance_snapshots vs
JOIN variance_rules vr ON vr.id = vs.rule_id
WHERE vs.id = $1`, id)
	snap, err := scanSnapshot(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Snapshot{}, ErrSnapshotNotFound
		}
		return Snapshot{}, err
	}
	return snap, nil
}

// UpdateStatus transitions snapshot state.
func (r *Repository) UpdateStatus(ctx context.Context, id int64, status SnapshotStatus) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("variance: repository not initialised")
	}
	tag, err := r.pool.Exec(ctx, `UPDATE variance_snapshots SET status = $2, updated_at = NOW() WHERE id = $1`, id, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSnapshotNotFound
	}
	return nil
}

// SavePayload stores output or error for snapshot.
func (r *Repository) SavePayload(ctx context.Context, id int64, rows []VarianceRow, errMsg string) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("variance: repository not initialised")
	}
	payload, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	tag, err := r.pool.Exec(ctx, `
UPDATE variance_snapshots
SET payload = $2,
    error_message = $3,
    updated_at = NOW(),
    generated_at = CASE WHEN $3 IS NULL OR $3 = '' THEN NOW() ELSE generated_at END
WHERE id = $1`, id, payload, nullableString(errMsg))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSnapshotNotFound
	}
	return nil
}

// LoadPayload deserialises snapshot payload for UI.
func (r *Repository) LoadPayload(ctx context.Context, id int64) ([]VarianceRow, error) {
	var data []byte
	err := r.pool.QueryRow(ctx, `SELECT payload FROM variance_snapshots WHERE id = $1`, id).Scan(&data)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var rows []VarianceRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// AggregateBalances summarises account balances for company/period.
func (r *Repository) AggregateBalances(ctx context.Context, accountingPeriodID, companyID int64) (map[string]AccountBalance, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("variance: repository not initialised")
	}
	rows, err := r.pool.Query(ctx, `
SELECT acc.code, acc.name, SUM(jl.debit - jl.credit) AS amount
FROM journal_lines jl
JOIN journal_entries je ON je.id = jl.je_id AND je.status = 'POSTED'
JOIN accounts acc ON acc.id = jl.account_id
JOIN accounting_periods ap ON ap.id = $1
WHERE je.period_id = ap.period_id AND COALESCE(jl.dim_company_id, 0) = $2
GROUP BY acc.code, acc.name`, accountingPeriodID, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]AccountBalance)
	for rows.Next() {
		var code, name string
		var amt float64
		if err := rows.Scan(&code, &name, &amt); err != nil {
			return nil, err
		}
		result[code] = AccountBalance{Name: name, Amount: amt}
	}
	return result, rows.Err()
}

// LoadAccountingPeriod resolves ledger period id.
func (r *Repository) LoadAccountingPeriod(ctx context.Context, id int64) (PeriodView, error) {
	var period PeriodView
	err := r.pool.QueryRow(ctx, `
SELECT ap.id, ap.period_id, ap.name, ap.start_date, ap.end_date
FROM accounting_periods ap WHERE ap.id = $1`, id).Scan(&period.ID, &period.LedgerID, &period.Name, &period.StartDate, &period.EndDate)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PeriodView{}, ErrSnapshotNotFound
		}
		return PeriodView{}, err
	}
	return period, nil
}

func scanRule(row interface{ Scan(dest ...any) error }) (Rule, error) {
	var rule Rule
	var filters []byte
	var compare sql.NullInt64
	var thresholdAmt, thresholdPct sql.NullFloat64
	if err := row.Scan(
		&rule.ID,
		&rule.CompanyID,
		&rule.Name,
		&rule.ComparisonType,
		&rule.BasePeriodID,
		&compare,
		&filters,
		&thresholdAmt,
		&thresholdPct,
		&rule.Active,
		&rule.CreatedBy,
		&rule.CreatedAt,
	); err != nil {
		return Rule{}, err
	}
	if compare.Valid {
		v := compare.Int64
		rule.ComparePeriodID = &v
	}
	if thresholdAmt.Valid {
		v := thresholdAmt.Float64
		rule.ThresholdAmount = &v
	}
	if thresholdPct.Valid {
		v := thresholdPct.Float64
		rule.ThresholdPercent = &v
	}
	if len(filters) > 0 {
		if err := json.Unmarshal(filters, &rule.DimensionFilter); err != nil {
			rule.DimensionFilter = map[string]any{}
		}
	} else {
		rule.DimensionFilter = map[string]any{}
	}
	rule.ComparisonType = RuleComparison(rule.ComparisonType)
	return rule, nil
}

func scanSnapshot(row interface{ Scan(dest ...any) error }) (Snapshot, error) {
	var snap Snapshot
	var payload []byte
	var generated sql.NullTime
	var genBy sql.NullInt64
	var errMsg sql.NullString
	rule, err := scanRuleWrapper(row, &snap, &payload, &generated, &genBy, &errMsg)
	if err != nil {
		return Snapshot{}, err
	}
	if generated.Valid {
		snap.GeneratedAt = &generated.Time
	}
	if genBy.Valid {
		v := genBy.Int64
		snap.GeneratedBy = &v
	}
	if errMsg.Valid {
		snap.Error = errMsg.String
	}
	if len(payload) > 0 {
		var rows []VarianceRow
		if err := json.Unmarshal(payload, &rows); err == nil {
			snap.Payload = rows
		}
	}
	snap.Rule = rule
	return snap, nil
}

// helper scanning snapshot + rule.
func scanRuleWrapper(row interface{ Scan(dest ...any) error }, snap *Snapshot, payload *[]byte, generated *sql.NullTime, genBy *sql.NullInt64, errMsg *sql.NullString) (*Rule, error) {
	var rule Rule
	var filters []byte
	var compare sql.NullInt64
	var thresholdAmt, thresholdPct sql.NullFloat64
	if err := row.Scan(
		&snap.ID,
		&snap.RuleID,
		&snap.PeriodID,
		&snap.Status,
		generated,
		genBy,
		errMsg,
		payload,
		&snap.CreatedAt,
		&snap.UpdatedAt,
		&rule.ID,
		&rule.CompanyID,
		&rule.Name,
		&rule.ComparisonType,
		&rule.BasePeriodID,
		&compare,
		&filters,
		&thresholdAmt,
		&thresholdPct,
		&rule.Active,
		&rule.CreatedBy,
		&rule.CreatedAt,
	); err != nil {
		return nil, err
	}
	if compare.Valid {
		v := compare.Int64
		rule.ComparePeriodID = &v
	}
	if thresholdAmt.Valid {
		v := thresholdAmt.Float64
		rule.ThresholdAmount = &v
	}
	if thresholdPct.Valid {
		v := thresholdPct.Float64
		rule.ThresholdPercent = &v
	}
	if len(filters) > 0 {
		if err := json.Unmarshal(filters, &rule.DimensionFilter); err != nil {
			rule.DimensionFilter = map[string]any{}
		}
	} else {
		rule.DimensionFilter = map[string]any{}
	}
	rule.ComparisonType = RuleComparison(rule.ComparisonType)
	return &rule, nil
}

type PeriodView struct {
	ID        int64
	LedgerID  int64
	Name      string
	StartDate time.Time
	EndDate   time.Time
}

func nullableString(value string) interface{} {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
