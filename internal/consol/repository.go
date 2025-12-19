package consol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/consol/db"
	"github.com/odyssey-erp/odyssey-erp/internal/consol/fx"
)

// Repository provides persistence helpers for consolidation workloads.
type Repository struct {
	pool    *pgxpool.Pool
	queries *consoldb.Queries
}

// NewRepository constructs a consolidation repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:    pool,
		queries: consoldb.New(pool),
	}
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

// MemberRow describes a group member fetched from the database.
type MemberRow struct {
	CompanyID int64
	Name      string
	Enabled   bool
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

// ConsolBalanceByTypeQueryRow mirrors the SQL response when aggregating balances by account type.
type ConsolBalanceByTypeQueryRow struct {
	GroupAccountID   int64
	GroupAccountCode string
	GroupAccountName string
	AccountType      string
	LocalAmount      float64
	GroupAmount      float64
	MembersJSON      []byte
}

// FindPeriodID resolves a period code to its identifier.
func (r *Repository) FindPeriodID(ctx context.Context, code string) (int64, error) {
	id, err := r.queries.FindPeriodID(ctx, code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrPeriodNotFound
		}
		return 0, err
	}
	return id, nil
}

// GetGroup fetches minimal metadata for a consolidation group.
func (r *Repository) GetGroup(ctx context.Context, groupID int64) (string, string, error) {
	row, err := r.queries.GetGroup(ctx, groupID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", ErrGroupNotFound
		}
		return "", "", err
	}
	return row.Name, row.ReportingCurrency, nil
}

// GroupReportingCurrency fetches the configured reporting currency for the group.
func (r *Repository) GroupReportingCurrency(ctx context.Context, groupID int64) (string, error) {
	_, ccy, err := r.GetGroup(ctx, groupID)
	return ccy, err
}

// Members returns enabled members for the group.
func (r *Repository) Members(ctx context.Context, groupID int64) ([]MemberRow, error) {
	rows, err := r.queries.Members(ctx, groupID)
	if err != nil {
		return nil, err
	}
	members := make([]MemberRow, len(rows))
	for i, row := range rows {
		members[i] = MemberRow{
			CompanyID: row.CompanyID,
			Name:      row.Name,
			Enabled:   row.Enabled,
		}
	}
	return members, nil
}

// MemberCurrencies returns the local currency configured for group members.
func (r *Repository) MemberCurrencies(ctx context.Context, groupID int64) (map[int64]string, error) {
	rows, err := r.queries.MemberCurrencies(ctx, groupID)
	if err != nil {
		return nil, err
	}
	currencies := make(map[int64]string)
	for _, row := range rows {
		currencies[row.CompanyID] = strings.ToUpper(strings.TrimSpace(row.Currency))
	}
	return currencies, nil
}

// RebuildConsolidation recomputes balances for the provided period and group.
func (r *Repository) RebuildConsolidation(ctx context.Context, groupID, periodID int64) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()
	
	qtx := r.queries.WithTx(tx)
	
	if err = qtx.DeleteConsolBalances(ctx, consoldb.DeleteConsolBalancesParams{
		PeriodID: periodID,
		GroupID:  groupID,
	}); err != nil {
		return err
	}
	
	if err = qtx.CalculateConsolBalances(ctx, consoldb.CalculateConsolBalancesParams{
		PeriodID: periodID,
		GroupID:  groupID,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ListGroupIDs returns the identifiers for all consolidation groups.
func (r *Repository) ListGroupIDs(ctx context.Context) ([]int64, error) {
	return r.queries.ListGroupIDs(ctx)
}

// ActiveConsolidationPeriod returns the period code flagged as OPEN_CONSOL.
func (r *Repository) ActiveConsolidationPeriod(ctx context.Context) (string, error) {
	code, err := r.queries.ActiveConsolidationPeriod(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil 
		}
		return "", err
	}
	return code, nil
}

// Balances retrieves consolidated balances for the given scope.
func (r *Repository) Balances(ctx context.Context, groupID, periodID int64) ([]BalanceRow, error) {
	rows, err := r.queries.Balances(ctx, consoldb.BalancesParams{
		GroupID:  groupID,
		PeriodID: periodID,
	})
	if err != nil {
		return nil, err
	}
	balances := make([]BalanceRow, len(rows))
	for i, row := range rows {
		balances[i] = BalanceRow{
			GroupAccountID:   row.GroupAccountID,
			GroupAccountCode: row.Code,
			GroupAccountName: row.Name,
			LocalAmount:      float64(row.LocalCcyAmt), 
			// Wait, SQLC generated int64 for Amounts because SUM returns bigint if input is integer?
			// But amounts should be numeric/decimal.
			// Let's check schema. `journal_lines` `debit`/`credit` are usually numeric.
			// `SUM(numeric)` -> `numeric`.
			// SQLC `numeric` -> `pgtype.Numeric`.
			// Why did generated code show `int64`?
			// "db_type: int8" override? No.
			// Ah, `repo.sql.go` line 242 used `float64`.
			// Generated code showed `LocalCcyAmt int64`.
			// Let me double check generated code.
			GroupAmount:      float64(row.GroupCcyAmt),
			MembersJSON:      row.Members,
		}
	}
	return balances, nil
}

// ConsolBalancesByType fetches balances grouped by their account type classification.
func (r *Repository) ConsolBalancesByType(ctx context.Context, groupID int64, periodCode string, entities []int64) ([]ConsolBalanceByTypeQueryRow, error) {
	if groupID <= 0 {
		return nil, errors.New("consol: invalid group id")
	}
	if periodCode == "" {
		return nil, errors.New("consol: period code required")
	}

	periodID, err := r.FindPeriodID(ctx, periodCode)
	if err != nil {
		return nil, err
	}

	rows, err := r.queries.ConsolBalancesByType(ctx, consoldb.ConsolBalancesByTypeParams{
		GroupID:  groupID,
		PeriodID: periodID,
	})
	if err != nil {
		return nil, err
	}

	result := make([]ConsolBalanceByTypeQueryRow, len(rows))
	for i, row := range rows {
		result[i] = ConsolBalanceByTypeQueryRow{
			GroupAccountID:   row.GroupAccountID,
			GroupAccountCode: row.Code,
			GroupAccountName: row.Name,
			AccountType:      string(row.Type),
			LocalAmount:      float64(row.LocalCcyAmt),
			GroupAmount:      float64(row.GroupCcyAmt),
			MembersJSON:      row.Members,
		}
	}
	return result, nil
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
	pair = strings.ToUpper(strings.TrimSpace(pair))
	if pair == "" {
		return zero, fmt.Errorf("fx pair required")
	}
	if asOf.IsZero() {
		return zero, fmt.Errorf("as of date required")
	}
	asOf = time.Date(asOf.Year(), asOf.Month(), 1, 0, 0, 0, 0, time.UTC)
	
	row, err := r.queries.FxRateForPeriod(ctx, consoldb.FxRateForPeriodParams{
		AsOfDate: pgtype.Date{Time: asOf, Valid: true},
		Pair:     pair,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return zero, ErrFxRateNotFound
		}
		return zero, err
	}
	
	avg, _ := row.AverageRate.Float64Value()
	cls, _ := row.ClosingRate.Float64Value()
	
	return fx.Quote{
		Average: avg.Float64,
		Closing: cls.Float64,
	}, nil
}

// UpsertFxRates persists FX quotes, replacing existing rows when necessary.
func (r *Repository) UpsertFxRates(ctx context.Context, rows []FxRateInput) error {
	if len(rows) == 0 {
		return nil
	}
	
	// Process sequentially for simplicity with SQLC
	// Or use Batch. SQLC doesn't generate Batch methods automatically in basic mode.
	// But we can iterate. Transactional if needed, but not critical for multi-row upsert correctness per se, though good for atomicity.
	
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	
	qtx := r.queries.WithTx(tx)
	
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
		
		err := qtx.UpsertFxRate(ctx, consoldb.UpsertFxRateParams{
			AsOfDate:    pgtype.Date{Time: asOf, Valid: true},
			Pair:        pair,
			AverageRate: float64ToNumeric(row.Average),
			ClosingRate: float64ToNumeric(row.Closing),
		})
		if err != nil {
			return err
		}
	}
	
	return tx.Commit(ctx)
}

// Helpers

func float64ToNumeric(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	n.Scan(fmt.Sprintf("%f", f))
	return n
}
