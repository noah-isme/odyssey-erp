package close

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// Repository persists period close state.
type Repository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

// NewRepository constructs a Repository using the provided pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

// WithTx executes fn inside a repeatable-read transaction.
func (r *Repository) WithTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("close: repository not initialised")
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
	if err = fn(ctx, tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

// ListPeriods returns paginated periods for a company.
func (r *Repository) ListPeriods(ctx context.Context, companyID int64, limit, offset int) ([]Period, error) {
	rows, err := r.queries.ListPeriods(ctx, sqlc.ListPeriodsParams{
		Column1: companyID,
		Limit:   int32(limit),
		Offset:  int32(offset),
	})
	if err != nil {
		return nil, err
	}

	periods := make([]Period, 0, len(rows))
	for _, row := range rows {
		p := Period{
			ID:           row.ID,
			PeriodID:     row.PeriodID,
			CompanyID:    int64(row.CompanyID),
			Name:         row.Name,
			StartDate:    row.StartDate.Time,
			EndDate:      row.EndDate.Time,
			Status:       PeriodStatus(row.Status),
			SoftClosedBy: int8ToPointer(row.SoftClosedBy),
			SoftClosedAt: timeToPointer(row.SoftClosedAt),
			ClosedBy:     int8ToPointer(row.ClosedBy),
			ClosedAt:     timeToPointer(row.ClosedAt),
			LatestRunID:  row.LatestRunID,
			CreatedAt:    row.CreatedAt.Time,
			UpdatedAt:    row.UpdatedAt.Time,
		}
		if len(row.Metadata) > 0 {
			_ = json.Unmarshal(row.Metadata, &p.Metadata)
		}
		periods = append(periods, p)
	}
	return periods, nil
}

// LoadPeriod fetches a single period by its accounting_periods.id.
func (r *Repository) LoadPeriod(ctx context.Context, id int64) (Period, error) {
	row, err := r.queries.LoadPeriod(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Period{}, fmt.Errorf("period not found")
		}
		return Period{}, err
	}
	return mapPeriod(row), nil
}

// LoadPeriodByLedgerID fetches a period by the legacy periods.id reference.
func (r *Repository) LoadPeriodByLedgerID(ctx context.Context, periodID int64) (Period, error) {
	row, err := r.queries.LoadPeriodByLedgerID(ctx, periodID)
	if err != nil {
		return Period{}, err
	}
	return mapPeriodByLedger(row), nil
}

// LoadPeriodForUpdate locks a period row.
// Explicit tx usage requires we use q.WithTx(tx) or create new queries from tx.
func (r *Repository) LoadPeriodForUpdate(ctx context.Context, tx pgx.Tx, id int64) (Period, error) {
	q := sqlc.New(tx)
	row, err := q.LoadPeriodForUpdate(ctx, id)
	if err != nil {
		return Period{}, err
	}
	return mapPeriodForUpdate(row), nil
}

// InsertPeriod creates a new period row in both periods and accounting_periods tables.
func (r *Repository) InsertPeriod(ctx context.Context, tx pgx.Tx, in CreatePeriodInput, status PeriodStatus) (Period, error) {
	q := sqlc.New(tx)

	// Insert Legacy
	legacyStatus := legacyStatusFromAccounting(status)
	legacyRow, err := q.InsertPeriodLegacy(ctx, sqlc.InsertPeriodLegacyParams{
		Code:      in.Name,
		StartDate: pgtype.Date{Time: in.StartDate, Valid: true},
		EndDate:   pgtype.Date{Time: in.EndDate, Valid: true},
		Status:    sqlc.PeriodStatus(legacyStatus),
	})
	if err != nil {
		return Period{}, err
	}

	metaBytes, _ := json.Marshal(in.Metadata)

	// Insert Accounting
	accountingID, err := q.InsertAccountingPeriod(ctx, sqlc.InsertAccountingPeriodParams{
		PeriodID:  legacyRow.ID,
		CompanyID: int8FromInt64(in.CompanyID),
		Name:      in.Name,
		StartDate: pgtype.Date{Time: in.StartDate, Valid: true},
		EndDate:   pgtype.Date{Time: in.EndDate, Valid: true},
		Status:    sqlc.AccountingPeriodStatus(status),
		Metadata:  metaBytes,
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		return Period{}, err
	}

	return Period{
		ID:        accountingID,
		PeriodID:  legacyRow.ID,
		CompanyID: in.CompanyID,
		Name:      in.Name,
		StartDate: in.StartDate,
		EndDate:   in.EndDate,
		Status:    status,
		Metadata:  in.Metadata,
		CreatedAt: legacyRow.CreatedAt.Time,
		UpdatedAt: legacyRow.UpdatedAt.Time,
	}, nil
}

// UpdatePeriodStatus mutates the period status in both tables.
func (r *Repository) UpdatePeriodStatus(ctx context.Context, tx pgx.Tx, periodID int64, status PeriodStatus, actorID int64) error {
	q := sqlc.New(tx)

	err := q.UpdateAccountingPeriodStatus(ctx, sqlc.UpdateAccountingPeriodStatusParams{
		ID:           periodID,
		Status:       sqlc.AccountingPeriodStatus(status),
		SoftClosedBy: int8FromInt64(actorID),
	})
	if err != nil {
		return err
	}

	return q.UpdateLegacyPeriodStatus(ctx, sqlc.UpdateLegacyPeriodStatusParams{
		ID:     periodID,
		Status: sqlc.PeriodStatus(legacyStatusFromAccounting(status)),
	})
}

// PeriodRangeConflict reports whether a company already has a period overlapping the provided range.
func (r *Repository) PeriodRangeConflict(ctx context.Context, companyID int64, startDate, endDate time.Time) (bool, error) {
	_, err := r.queries.PeriodRangeConflict(ctx, sqlc.PeriodRangeConflictParams{
		CompanyID: int8FromInt64(companyID),
		Daterange: pgtype.Date{Time: startDate, Valid: true},
		Daterange_2: pgtype.Date{Time: endDate, Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// PeriodHasActiveRun returns true when a period already has an in-progress run.
func (r *Repository) PeriodHasActiveRun(ctx context.Context, tx pgx.Tx, periodID int64) (bool, error) {
	q := sqlc.New(tx)
	_, err := q.PeriodHasActiveRun(ctx, periodID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// InsertCloseRun creates a new close run row.
func (r *Repository) InsertCloseRun(ctx context.Context, tx pgx.Tx, in StartCloseRunInput) (CloseRun, error) {
	q := sqlc.New(tx)
	row, err := q.InsertCloseRun(ctx, sqlc.InsertCloseRunParams{
		CompanyID: in.CompanyID,
		PeriodID:  in.PeriodID,
		CreatedBy: in.ActorID,
		Notes:     pgtype.Text{String: in.Notes, Valid: true}, // Fix: Notes is pgtype.Text in generated code
	})
	if err != nil {
		return CloseRun{}, err
	}

	return CloseRun{
		ID:          row.ID,
		CompanyID:   row.CompanyID,
		PeriodID:    row.PeriodID,
		Status:      RunStatus(row.Status),
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt.Time,
		CompletedAt: timeToPointer(row.CompletedAt),
		Notes:       row.Notes.String,
	}, nil
}

// InsertChecklistItems seeds checklist entries for a run.
func (r *Repository) InsertChecklistItems(ctx context.Context, tx pgx.Tx, runID int64, defs []ChecklistDefinition) ([]ChecklistItem, error) {
	q := sqlc.New(tx)
	var items []ChecklistItem
	for _, def := range defs {
		row, err := q.InsertChecklistItem(ctx, sqlc.InsertChecklistItemParams{
			PeriodCloseRunID: runID,
			Code:             def.Code,
			Label:            def.Label,
		})
		if err != nil {
			return nil, err
		}
		items = append(items, mapChecklistItem(row))
	}
	return items, nil
}

// LoadCloseRun fetches a run without checklist details.
func (r *Repository) LoadCloseRun(ctx context.Context, id int64) (CloseRun, error) {
	row, err := r.queries.LoadCloseRun(ctx, id)
	if err != nil {
		return CloseRun{}, err
	}
	return mapRun(row), nil
}

// LoadCloseRunForUpdate locks the run row for state transitions.
func (r *Repository) LoadCloseRunForUpdate(ctx context.Context, tx pgx.Tx, id int64) (CloseRun, error) {
	q := sqlc.New(tx)
	row, err := q.LoadCloseRunForUpdate(ctx, id)
	if err != nil {
		return CloseRun{}, err
	}
	return mapRunForUpdate(row), nil
}

// LoadCloseRunWithChecklist returns a run with checklist items.
func (r *Repository) LoadCloseRunWithChecklist(ctx context.Context, id int64) (CloseRun, error) {
	run, err := r.LoadCloseRun(ctx, id)
	if err != nil {
		return CloseRun{}, err
	}

	itemsRows, err := r.queries.ListChecklistItems(ctx, id)
	if err != nil {
		return CloseRun{}, err
	}

	items := make([]ChecklistItem, len(itemsRows))
	for i, row := range itemsRows {
		items[i] = mapChecklistItem(row)
	}
	run.Checklist = items
	return run, nil
}

// UpdateChecklistStatus updates a checklist item state.
func (r *Repository) UpdateChecklistStatus(ctx context.Context, tx pgx.Tx, in ChecklistUpdateInput) (ChecklistItem, error) {
	q := sqlc.New(tx)
	row, err := q.UpdateChecklistStatus(ctx, sqlc.UpdateChecklistStatusParams{
		ID:      in.ItemID,
		Status:  sqlc.PeriodCloseChecklistStatus(in.Status),
		Column3: in.Comment,
	})
	if err != nil {
		return ChecklistItem{}, err
	}
	return mapChecklistItem(row), nil
}

// LockChecklistItemRun returns the owning run id while locking the checklist row.
func (r *Repository) LockChecklistItemRun(ctx context.Context, tx pgx.Tx, itemID int64) (int64, error) {
	q := sqlc.New(tx)
	return q.LockChecklistItemRun(ctx, itemID)
}

// ChecklistCompletionState returns whether all checklist tasks are in a terminal state.
func (r *Repository) ChecklistCompletionState(ctx context.Context, tx pgx.Tx, runID int64) (bool, error) {
	q := sqlc.New(tx)
	pending, err := q.CountPendingChecklistItems(ctx, runID)
	if err != nil {
		return false, err
	}
	return pending == 0, nil
}

// UpdateRunStatus sets the run status and completion timestamp.
func (r *Repository) UpdateRunStatus(ctx context.Context, tx pgx.Tx, runID int64, status RunStatus) error {
	q := sqlc.New(tx)
	return q.UpdateRunStatus(ctx, sqlc.UpdateRunStatusParams{
		ID:     runID,
		Status: sqlc.PeriodCloseRunStatus(status),
	})
}

func legacyStatusFromAccounting(status PeriodStatus) string {
	switch status {
	case PeriodStatusSoftClosed:
		return "CLOSED"
	case PeriodStatusHardClosed:
		return "LOCKED"
	default:
		return "OPEN"
	}
}

// Mappers

func mapPeriod(row sqlc.LoadPeriodRow) Period {
	p := Period{
		ID:           row.ID,
		PeriodID:     row.PeriodID,
		CompanyID:    int64(row.CompanyID),
		Name:         row.Name,
		StartDate:    row.StartDate.Time,
		EndDate:      row.EndDate.Time,
		Status:       PeriodStatus(row.Status),
		SoftClosedBy: int8ToPointer(row.SoftClosedBy),
		SoftClosedAt: timeToPointer(row.SoftClosedAt),
		ClosedBy:     int8ToPointer(row.ClosedBy),
		ClosedAt:     timeToPointer(row.ClosedAt),
		LatestRunID:  row.LatestRunID,
		CreatedAt:    row.CreatedAt.Time,
		UpdatedAt:    row.UpdatedAt.Time,
	}
	if len(row.Metadata) > 0 {
		_ = json.Unmarshal(row.Metadata, &p.Metadata)
	}
	return p
}

func mapPeriodByLedger(row sqlc.LoadPeriodByLedgerIDRow) Period {
	p := Period{
		ID:           row.ID,
		PeriodID:     row.PeriodID,
		CompanyID:    int64(row.CompanyID),
		Name:         row.Name,
		StartDate:    row.StartDate.Time,
		EndDate:      row.EndDate.Time,
		Status:       PeriodStatus(row.Status),
		SoftClosedBy: int8ToPointer(row.SoftClosedBy),
		SoftClosedAt: timeToPointer(row.SoftClosedAt),
		ClosedBy:     int8ToPointer(row.ClosedBy),
		ClosedAt:     timeToPointer(row.ClosedAt),
		LatestRunID:  row.LatestRunID,
		CreatedAt:    row.CreatedAt.Time,
		UpdatedAt:    row.UpdatedAt.Time,
	}
	if len(row.Metadata) > 0 {
		_ = json.Unmarshal(row.Metadata, &p.Metadata)
	}
	return p
}

func mapPeriodForUpdate(row sqlc.LoadPeriodForUpdateRow) Period {
	p := Period{
		ID:           row.ID,
		PeriodID:     row.PeriodID,
		CompanyID:    int64(row.CompanyID),
		Name:         row.Name,
		StartDate:    row.StartDate.Time,
		EndDate:      row.EndDate.Time,
		Status:       PeriodStatus(row.Status),
		SoftClosedBy: int8ToPointer(row.SoftClosedBy),
		SoftClosedAt: timeToPointer(row.SoftClosedAt),
		ClosedBy:     int8ToPointer(row.ClosedBy),
		ClosedAt:     timeToPointer(row.ClosedAt),
		LatestRunID:  row.LatestRunID,
		CreatedAt:    row.CreatedAt.Time,
		UpdatedAt:    row.UpdatedAt.Time,
	}
	if len(row.Metadata) > 0 {
		_ = json.Unmarshal(row.Metadata, &p.Metadata)
	}
	return p
}

func mapRun(row sqlc.LoadCloseRunRow) CloseRun {
	return CloseRun{
		ID:          row.ID,
		CompanyID:   row.CompanyID,
		PeriodID:    row.PeriodID,
		Status:      RunStatus(row.Status),
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt.Time,
		CompletedAt: timeToPointer(row.CompletedAt),
		Notes:       row.Notes.String,
	}
}

func mapRunForUpdate(row sqlc.LoadCloseRunForUpdateRow) CloseRun {
	return CloseRun{
		ID:          row.ID,
		CompanyID:   row.CompanyID,
		PeriodID:    row.PeriodID,
		Status:      RunStatus(row.Status),
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt.Time,
		CompletedAt: timeToPointer(row.CompletedAt),
		Notes:       row.Notes.String,
	}
}

func mapChecklistItem(row sqlc.PeriodCloseChecklistItem) ChecklistItem {
	return ChecklistItem{
		ID:          row.ID,
		RunID:       row.PeriodCloseRunID,
		Code:        row.Code,
		Label:       row.Label,
		Status:      ChecklistStatus(row.Status),
		AssignedTo:  int8ToPointer(row.AssignedTo),
		CompletedAt: timeToPointer(row.CompletedAt),
		Comment:     row.Comment.String,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
}

// Helpers

func int8ToPointer(i pgtype.Int8) *int64 {
	if !i.Valid {
		return nil
	}
	v := i.Int64
	return &v
}

func int8FromInt64(i int64) pgtype.Int8 {
	if i == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: i, Valid: true}
}

func timeToPointer(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}
