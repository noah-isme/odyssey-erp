package journals

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/periods"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/shared"
)

// Repository encapsulates DB operations for journals.
// It also needs access to periods for transaction-safe checks.
type Repository interface {
	List(ctx context.Context) ([]JournalEntry, error)
	// Tx Operations are internal or exposed via specific service methods
	WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error
}

// TxRepository exposes methods available within a transaction
type TxRepository interface {
	InsertJournalEntry(ctx context.Context, in PostingInput) (JournalEntry, error)
	InsertJournalLines(ctx context.Context, entryID int64, lines []PostingLineInput) error
	LinkSource(ctx context.Context, module string, ref uuid.UUID, entryID int64) error
	GetJournalWithLines(ctx context.Context, entryID int64) (JournalEntry, []JournalLine, error)
	UpdateJournalStatus(ctx context.Context, entryID int64, status JournalStatus) error
	
	// Period operations needed within journal transactions
	GetPeriodForUpdate(ctx context.Context, periodID int64) (periods.Period, error)
	GetNextOpenPeriodAfter(ctx context.Context, date time.Time) (periods.Period, error)
}

type repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

func (r *repository) List(ctx context.Context) ([]JournalEntry, error) {
	rows, err := r.db.Query(ctx, `SELECT id, number, period_id, date, source_module, source_id, memo, posted_by, posted_at, status, created_at, updated_at FROM journal_entries ORDER BY number DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []JournalEntry
	for rows.Next() {
		var e JournalEntry
		err := rows.Scan(&e.ID, &e.Number, &e.PeriodID, &e.Date, &e.SourceModule, &e.SourceID, &e.Memo, &e.PostedBy, &e.PostedAt, &e.Status, &e.CreatedAt, &e.UpdatedAt)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (r *repository) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	wrapper := &txRepository{tx: tx}
	if err := fn(ctx, wrapper); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

type txRepository struct {
	tx pgx.Tx
}

func (r *txRepository) InsertJournalEntry(ctx context.Context, in PostingInput) (JournalEntry, error) {
	row := r.tx.QueryRow(ctx, `INSERT INTO journal_entries (period_id, date, source_module, source_id, memo, posted_by, status)
VALUES ($1,$2,$3,$4,$5,$6,'POSTED') RETURNING id, number, posted_at, created_at, updated_at`, in.PeriodID, in.Date, in.SourceModule, in.SourceID, in.Memo, nullInt(in.PostedBy))
	var entry JournalEntry
	entry.PeriodID = in.PeriodID
	entry.Date = in.Date
	entry.SourceModule = in.SourceModule
	entry.SourceID = in.SourceID
	entry.Memo = in.Memo
	entry.PostedBy = in.PostedBy
	entry.Status = JournalStatusPosted
	if err := row.Scan(&entry.ID, &entry.Number, &entry.PostedAt, &entry.CreatedAt, &entry.UpdatedAt); err != nil {
		return JournalEntry{}, err
	}
	return entry, nil
}

func (r *txRepository) InsertJournalLines(ctx context.Context, entryID int64, lines []PostingLineInput) error {
	for _, line := range lines {
		if _, err := r.tx.Exec(ctx, `INSERT INTO journal_lines (je_id, account_id, debit, credit, dim_company_id, dim_branch_id, dim_warehouse_id)
VALUES ($1,$2,$3,$4,$5,$6,$7)`, entryID, line.AccountID, toNumeric(line.Debit), toNumeric(line.Credit), nullIntPtr(line.CompanyID), nullIntPtr(line.BranchID), nullIntPtr(line.Warehouse)); err != nil {
			return err
		}
	}
	return nil
}

func (r *txRepository) LinkSource(ctx context.Context, module string, ref uuid.UUID, entryID int64) error {
	_, err := r.tx.Exec(ctx, `INSERT INTO source_links (module, ref_id, je_id) VALUES ($1,$2,$3)`, module, ref, entryID)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.ConstraintName == "uq_source_links" {
			return shared.ErrSourceConflict
		}
		return err
	}
	return nil
}

func (r *txRepository) GetJournalWithLines(ctx context.Context, entryID int64) (JournalEntry, []JournalLine, error) {
	var entry JournalEntry
	err := r.tx.QueryRow(ctx, `SELECT id, number, period_id, date, source_module, source_id, memo, posted_by, posted_at, status, created_at, updated_at
FROM journal_entries WHERE id=$1`, entryID).
		Scan(&entry.ID, &entry.Number, &entry.PeriodID, &entry.Date, &entry.SourceModule, &entry.SourceID, &entry.Memo, &entry.PostedBy, &entry.PostedAt, &entry.Status, &entry.CreatedAt, &entry.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return JournalEntry{}, nil, shared.ErrJournalNotFound
		}
		return JournalEntry{}, nil, err
	}
	rows, err := r.tx.Query(ctx, `SELECT id, je_id, account_id, debit, credit, dim_company_id, dim_branch_id, dim_warehouse_id, created_at, updated_at
FROM journal_lines WHERE je_id=$1 ORDER BY id ASC`, entryID)
	if err != nil {
		return JournalEntry{}, nil, err
	}
	defer rows.Close()
	var lines []JournalLine
	for rows.Next() {
		var line JournalLine
		if err := rows.Scan(&line.ID, &line.JournalID, &line.AccountID, &line.Debit, &line.Credit, &line.DimCompanyID, &line.DimBranchID, &line.DimWarehouseID, &line.CreatedAt, &line.UpdatedAt); err != nil {
			return JournalEntry{}, nil, err
		}
		lines = append(lines, line)
	}
	return entry, lines, rows.Err()
}

func (r *txRepository) UpdateJournalStatus(ctx context.Context, entryID int64, status JournalStatus) error {
	cmd, err := r.tx.Exec(ctx, `UPDATE journal_entries SET status=$2, updated_at=NOW() WHERE id=$1`, entryID, status)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return shared.ErrJournalNotFound
	}
	return nil
}

// GetPeriodForUpdate fetches period with a lock - duplicated logic from periods repo but needed here for transaction context
func (r *txRepository) GetPeriodForUpdate(ctx context.Context, periodID int64) (periods.Period, error) {
	var p periods.Period
	err := r.tx.QueryRow(ctx, `SELECT id, code, start_date, end_date, status, closed_at, locked_by, created_at, updated_at
FROM periods WHERE id=$1 FOR UPDATE`, periodID).
		Scan(&p.ID, &p.Code, &p.StartDate, &p.EndDate, &p.Status, &p.ClosedAt, &p.LockedBy, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return periods.Period{}, shared.ErrInvalidPeriod
		}
		return periods.Period{}, err
	}
	return p, nil
}

func (r *txRepository) GetNextOpenPeriodAfter(ctx context.Context, date time.Time) (periods.Period, error) {
	var p periods.Period
	err := r.tx.QueryRow(ctx, `SELECT id, code, start_date, end_date, status, closed_at, locked_by, created_at, updated_at
FROM periods WHERE status='OPEN' AND start_date >= $1 ORDER BY start_date ASC LIMIT 1`, date).
		Scan(&p.ID, &p.Code, &p.StartDate, &p.EndDate, &p.Status, &p.ClosedAt, &p.LockedBy, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return periods.Period{}, shared.ErrInvalidPeriod
		}
		return periods.Period{}, err
	}
	return p, nil
}

// Helpers
func nullInt(val int64) any {
	if val == 0 {
		return nil
	}
	return val
}

func nullIntPtr(val *int64) any {
	if val == nil {
		return nil
	}
	if *val == 0 {
		return nil
	}
	return *val
}

func toNumeric(v float64) any {
	return fmt.Sprintf("%.2f", v)
}
