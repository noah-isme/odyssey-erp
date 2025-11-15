package close

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

// Repository persists period close state.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a Repository using the provided pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
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
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("close: repository not initialised")
	}
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	query := `
SELECT ap.id, ap.period_id, COALESCE(ap.company_id, 0), ap.name, ap.start_date, ap.end_date, ap.status,
       ap.soft_closed_by, ap.soft_closed_at, ap.closed_by, ap.closed_at, ap.metadata, ap.created_at, ap.updated_at,
       lr.id AS latest_run_id
FROM accounting_periods ap
LEFT JOIN LATERAL (
    SELECT id
    FROM period_close_runs r
    WHERE r.period_id = ap.id
    ORDER BY r.created_at DESC
    LIMIT 1
) lr ON TRUE
WHERE ($1 = 0 OR company_id = $1)
ORDER BY start_date DESC
LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, query, companyID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var periods []Period
	for rows.Next() {
		period, err := scanPeriod(rows)
		if err != nil {
			return nil, err
		}
		periods = append(periods, period)
	}
	return periods, rows.Err()
}

// LoadPeriod fetches a single period by its accounting_periods.id.
func (r *Repository) LoadPeriod(ctx context.Context, id int64) (Period, error) {
	if r == nil || r.pool == nil {
		return Period{}, fmt.Errorf("close: repository not initialised")
	}
	const query = `
SELECT ap.id, ap.period_id, COALESCE(ap.company_id, 0), ap.name, ap.start_date, ap.end_date, ap.status,
       ap.soft_closed_by, ap.soft_closed_at, ap.closed_by, ap.closed_at, ap.metadata, ap.created_at, ap.updated_at,
       lr.id AS latest_run_id
FROM accounting_periods ap
LEFT JOIN LATERAL (
    SELECT id
    FROM period_close_runs r
    WHERE r.period_id = ap.id
    ORDER BY r.created_at DESC
    LIMIT 1
) lr ON TRUE
WHERE ap.id = $1`
	row := r.pool.QueryRow(ctx, query, id)
	return scanPeriod(row)
}

// LoadPeriodByLedgerID fetches a period by the legacy periods.id reference.
func (r *Repository) LoadPeriodByLedgerID(ctx context.Context, periodID int64) (Period, error) {
	if r == nil || r.pool == nil {
		return Period{}, fmt.Errorf("close: repository not initialised")
	}
	const query = `
SELECT ap.id, ap.period_id, COALESCE(ap.company_id, 0), ap.name, ap.start_date, ap.end_date, ap.status,
       ap.soft_closed_by, ap.soft_closed_at, ap.closed_by, ap.closed_at, ap.metadata, ap.created_at, ap.updated_at,
       lr.id AS latest_run_id
FROM accounting_periods ap
LEFT JOIN LATERAL (
    SELECT id
    FROM period_close_runs r
    WHERE r.period_id = ap.id
    ORDER BY r.created_at DESC
    LIMIT 1
) lr ON TRUE
WHERE ap.period_id = $1`
	row := r.pool.QueryRow(ctx, query, periodID)
	return scanPeriod(row)
}

// LoadPeriodForUpdate locks a period row.
func (r *Repository) LoadPeriodForUpdate(ctx context.Context, tx pgx.Tx, id int64) (Period, error) {
	const query = `
SELECT ap.id, ap.period_id, COALESCE(ap.company_id, 0), ap.name, ap.start_date, ap.end_date, ap.status,
       ap.soft_closed_by, ap.soft_closed_at, ap.closed_by, ap.closed_at, ap.metadata, ap.created_at, ap.updated_at,
       lr.id AS latest_run_id
FROM accounting_periods ap
LEFT JOIN LATERAL (
    SELECT id
    FROM period_close_runs r
    WHERE r.period_id = ap.id
    ORDER BY r.created_at DESC
    LIMIT 1
) lr ON TRUE
WHERE ap.id = $1
FOR UPDATE`
	row := tx.QueryRow(ctx, query, id)
	return scanPeriod(row)
}

func scanPeriod(row pgx.Row) (Period, error) {
	var p Period
	var rawMeta []byte
	var latestRun sql.NullInt64
	if err := row.Scan(
		&p.ID,
		&p.PeriodID,
		&p.CompanyID,
		&p.Name,
		&p.StartDate,
		&p.EndDate,
		&p.Status,
		&p.SoftClosedBy,
		&p.SoftClosedAt,
		&p.ClosedBy,
		&p.ClosedAt,
		&rawMeta,
		&p.CreatedAt,
		&p.UpdatedAt,
		&latestRun,
	); err != nil {
		return Period{}, err
	}
	if len(rawMeta) > 0 {
		var meta map[string]any
		if err := json.Unmarshal(rawMeta, &meta); err == nil {
			p.Metadata = meta
		}
	}
	if latestRun.Valid {
		p.LatestRunID = latestRun.Int64
	}
	return p, nil
}

// InsertPeriod creates a new period row in both periods and accounting_periods tables.
func (r *Repository) InsertPeriod(ctx context.Context, tx pgx.Tx, in CreatePeriodInput, status PeriodStatus) (Period, error) {
	var legacyID int64
	var createdAt, updatedAt time.Time
	const insertLegacy = `
INSERT INTO periods (code, start_date, end_date, status)
VALUES ($1,$2,$3,$4)
RETURNING id, created_at, updated_at`
	legacyStatus := legacyStatusFromAccounting(status)
	if err := tx.QueryRow(ctx, insertLegacy, in.Name, in.StartDate, in.EndDate, legacyStatus).Scan(&legacyID, &createdAt, &updatedAt); err != nil {
		return Period{}, err
	}
	meta := in.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return Period{}, err
	}
	const insertAccounting = `
INSERT INTO accounting_periods (period_id, company_id, name, start_date, end_date, status, metadata, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
RETURNING id`
	var accountingID int64
	now := time.Now()
	if err := tx.QueryRow(
		ctx,
		insertAccounting,
		legacyID,
		in.CompanyID,
		in.Name,
		in.StartDate,
		in.EndDate,
		status,
		metaBytes,
		now,
		now,
	).Scan(&accountingID); err != nil {
		return Period{}, err
	}
	period := Period{
		ID:        accountingID,
		PeriodID:  legacyID,
		CompanyID: in.CompanyID,
		Name:      in.Name,
		StartDate: in.StartDate,
		EndDate:   in.EndDate,
		Status:    status,
		Metadata:  meta,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
	return period, nil
}

// UpdatePeriodStatus mutates the period status in both tables.
func (r *Repository) UpdatePeriodStatus(ctx context.Context, tx pgx.Tx, periodID int64, status PeriodStatus, actorID int64) error {
	const updateAccounting = `
UPDATE accounting_periods
SET status = $2,
    soft_closed_by = CASE
        WHEN $2 = 'SOFT_CLOSED' THEN $3
        WHEN $2 = 'OPEN' THEN NULL
        ELSE soft_closed_by
    END,
    soft_closed_at = CASE
        WHEN $2 = 'SOFT_CLOSED' THEN NOW()
        WHEN $2 = 'OPEN' THEN NULL
        ELSE soft_closed_at
    END,
    closed_by = CASE
        WHEN $2 = 'HARD_CLOSED' THEN $3
        WHEN $2 != 'HARD_CLOSED' THEN NULL
        ELSE closed_by
    END,
    closed_at = CASE
        WHEN $2 = 'HARD_CLOSED' THEN NOW()
        WHEN $2 != 'HARD_CLOSED' THEN NULL
        ELSE closed_at
    END,
    updated_at = NOW()
WHERE id = $1`
	if _, err := tx.Exec(ctx, updateAccounting, periodID, status, nullInt64(actorID)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE periods SET status = $2, updated_at = NOW() WHERE id = (SELECT period_id FROM accounting_periods WHERE id = $1)`, periodID, legacyStatusFromAccounting(status)); err != nil {
		return err
	}
	return nil
}

// PeriodRangeConflict reports whether a company already has a period overlapping the provided range.
func (r *Repository) PeriodRangeConflict(ctx context.Context, companyID int64, startDate, endDate time.Time) (bool, error) {
	if r == nil || r.pool == nil {
		return false, fmt.Errorf("close: repository not initialised")
	}
	const query = `
SELECT 1
FROM accounting_periods
WHERE company_id = $1
  AND daterange(start_date, end_date, '[]') && daterange($2, $3, '[]')
LIMIT 1`
	var exists int
	if err := r.pool.QueryRow(ctx, query, companyID, startDate, endDate).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// PeriodHasActiveRun returns true when a period already has an in-progress run.
func (r *Repository) PeriodHasActiveRun(ctx context.Context, tx pgx.Tx, periodID int64) (bool, error) {
	const query = `
SELECT 1 FROM period_close_runs
WHERE period_id = $1 AND status IN ('DRAFT','IN_PROGRESS')
LIMIT 1`
	var exists int
	if err := tx.QueryRow(ctx, query, periodID).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// InsertCloseRun creates a new close run row.
func (r *Repository) InsertCloseRun(ctx context.Context, tx pgx.Tx, in StartCloseRunInput) (CloseRun, error) {
	const insertRun = `
INSERT INTO period_close_runs (company_id, period_id, status, created_by, notes)
VALUES ($1,$2,'IN_PROGRESS',$3,$4)
RETURNING id, company_id, period_id, status, created_by, created_at, completed_at, notes`
	var run CloseRun
	if err := tx.QueryRow(ctx, insertRun, in.CompanyID, in.PeriodID, in.ActorID, in.Notes).Scan(
		&run.ID,
		&run.CompanyID,
		&run.PeriodID,
		&run.Status,
		&run.CreatedBy,
		&run.CreatedAt,
		&run.CompletedAt,
		&run.Notes,
	); err != nil {
		return CloseRun{}, err
	}
	return run, nil
}

// InsertChecklistItems seeds checklist entries for a run.
func (r *Repository) InsertChecklistItems(ctx context.Context, tx pgx.Tx, runID int64, defs []ChecklistDefinition) ([]ChecklistItem, error) {
	const insertItem = `
INSERT INTO period_close_checklist_items (period_close_run_id, code, label)
VALUES ($1,$2,$3)
RETURNING id, period_close_run_id, code, label, status, assigned_to, completed_at, comment, created_at, updated_at`
	items := make([]ChecklistItem, 0, len(defs))
	for _, def := range defs {
		var item ChecklistItem
		if err := tx.QueryRow(ctx, insertItem, runID, def.Code, def.Label).Scan(
			&item.ID,
			&item.RunID,
			&item.Code,
			&item.Label,
			&item.Status,
			&item.AssignedTo,
			&item.CompletedAt,
			&item.Comment,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// LoadCloseRun fetches a run without checklist details.
func (r *Repository) LoadCloseRun(ctx context.Context, id int64) (CloseRun, error) {
	if r == nil || r.pool == nil {
		return CloseRun{}, fmt.Errorf("close: repository not initialised")
	}
	const query = `
SELECT id, company_id, period_id, status, created_by, created_at, completed_at, notes
FROM period_close_runs WHERE id = $1`
	row := r.pool.QueryRow(ctx, query, id)
	return scanRun(row)
}

// LoadCloseRunForUpdate locks the run row for state transitions.
func (r *Repository) LoadCloseRunForUpdate(ctx context.Context, tx pgx.Tx, id int64) (CloseRun, error) {
	const query = `
SELECT id, company_id, period_id, status, created_by, created_at, completed_at, notes
FROM period_close_runs WHERE id = $1 FOR UPDATE`
	row := tx.QueryRow(ctx, query, id)
	return scanRun(row)
}

// LoadCloseRunWithChecklist returns a run with checklist items.
func (r *Repository) LoadCloseRunWithChecklist(ctx context.Context, id int64) (CloseRun, error) {
	run, err := r.LoadCloseRun(ctx, id)
	if err != nil {
		return CloseRun{}, err
	}
	const query = `
SELECT id, period_close_run_id, code, label, status, assigned_to, completed_at, comment, created_at, updated_at
FROM period_close_checklist_items
WHERE period_close_run_id = $1
ORDER BY id`
	rows, err := r.pool.Query(ctx, query, id)
	if err != nil {
		return CloseRun{}, err
	}
	defer rows.Close()
	var items []ChecklistItem
	for rows.Next() {
		item, err := scanChecklistItem(rows)
		if err != nil {
			return CloseRun{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return CloseRun{}, err
	}
	run.Checklist = items
	return run, nil
}

func scanRun(row pgx.Row) (CloseRun, error) {
	var run CloseRun
	if err := row.Scan(
		&run.ID,
		&run.CompanyID,
		&run.PeriodID,
		&run.Status,
		&run.CreatedBy,
		&run.CreatedAt,
		&run.CompletedAt,
		&run.Notes,
	); err != nil {
		return CloseRun{}, err
	}
	return run, nil
}

func scanChecklistItem(row pgx.Row) (ChecklistItem, error) {
	var item ChecklistItem
	if err := row.Scan(
		&item.ID,
		&item.RunID,
		&item.Code,
		&item.Label,
		&item.Status,
		&item.AssignedTo,
		&item.CompletedAt,
		&item.Comment,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return ChecklistItem{}, err
	}
	return item, nil
}

// UpdateChecklistStatus updates a checklist item state.
func (r *Repository) UpdateChecklistStatus(ctx context.Context, tx pgx.Tx, in ChecklistUpdateInput) (ChecklistItem, error) {
	const query = `
UPDATE period_close_checklist_items
SET status = $2,
    comment = COALESCE(NULLIF($3,''), comment),
    completed_at = CASE WHEN $2 IN ('DONE','SKIPPED') THEN NOW() ELSE NULL END,
    updated_at = NOW()
WHERE id = $1
RETURNING id, period_close_run_id, code, label, status, assigned_to, completed_at, comment, created_at, updated_at`
	row := tx.QueryRow(ctx, query, in.ItemID, in.Status, in.Comment)
	return scanChecklistItem(row)
}

// LockChecklistItemRun returns the owning run id while locking the checklist row.
func (r *Repository) LockChecklistItemRun(ctx context.Context, tx pgx.Tx, itemID int64) (int64, error) {
	const query = `SELECT period_close_run_id FROM period_close_checklist_items WHERE id = $1 FOR UPDATE`
	var runID int64
	if err := tx.QueryRow(ctx, query, itemID).Scan(&runID); err != nil {
		return 0, err
	}
	return runID, nil
}

// ChecklistCompletionState returns whether all checklist tasks are in a terminal state.
func (r *Repository) ChecklistCompletionState(ctx context.Context, tx pgx.Tx, runID int64) (bool, error) {
	const query = `
SELECT COUNT(*) FILTER (WHERE status NOT IN ('DONE','SKIPPED')) AS pending
FROM period_close_checklist_items
WHERE period_close_run_id = $1`
	var pending int
	if err := tx.QueryRow(ctx, query, runID).Scan(&pending); err != nil {
		return false, err
	}
	return pending == 0, nil
}

// UpdateRunStatus sets the run status and completion timestamp.
func (r *Repository) UpdateRunStatus(ctx context.Context, tx pgx.Tx, runID int64, status RunStatus) error {
	const query = `
UPDATE period_close_runs
SET status = $2,
    completed_at = CASE WHEN $2 = 'COMPLETED' THEN NOW() ELSE completed_at END,
    updated_at = NOW()
WHERE id = $1`
	_, err := tx.Exec(ctx, query, runID, status)
	return err
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

func nullInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}
