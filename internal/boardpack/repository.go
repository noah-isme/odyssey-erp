package boardpack

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

	"github.com/odyssey-erp/odyssey-erp/internal/accounting/reports"
)

// Repository persists board pack templates, requests, and supporting metadata.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a repository wrapper.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ListTemplates returns board pack templates filtered by active flag.
func (r *Repository) ListTemplates(ctx context.Context, includeInactive bool) ([]Template, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("boardpack: repository not initialised")
	}
	query := `SELECT id, name, COALESCE(description,''), sections, is_default, is_active, created_by, created_at, updated_at
FROM board_pack_templates
WHERE ($1 OR is_active)
ORDER BY is_default DESC, name`
	rows, err := r.pool.Query(ctx, query, includeInactive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var templates []Template
	for rows.Next() {
		tpl, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, tpl)
	}
	return templates, rows.Err()
}

// GetTemplate loads a template by id.
func (r *Repository) GetTemplate(ctx context.Context, id int64) (Template, error) {
	if r == nil || r.pool == nil {
		return Template{}, fmt.Errorf("boardpack: repository not initialised")
	}
	const query = `SELECT id, name, COALESCE(description,''), sections, is_default, is_active, created_by, created_at, updated_at
FROM board_pack_templates WHERE id = $1`
	tpl, err := scanTemplate(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Template{}, ErrTemplateNotFound
		}
		return Template{}, err
	}
	return tpl, nil
}

// InsertBoardPack stores a new board pack request.
func (r *Repository) InsertBoardPack(ctx context.Context, req CreateRequest) (BoardPack, error) {
	if r == nil || r.pool == nil {
		return BoardPack{}, fmt.Errorf("boardpack: repository not initialised")
	}
	meta := mergeMetadata(req.Metadata)
	meta["requested_by"] = req.ActorID
	payload, err := json.Marshal(meta)
	if err != nil {
		return BoardPack{}, err
	}
	var variance any
	if req.VarianceSnapshotID != nil {
		variance = *req.VarianceSnapshotID
	}
	var id int64
	const insert = `INSERT INTO board_packs (company_id, period_id, template_id, variance_snapshot_id, status, generated_by, metadata)
VALUES ($1,$2,$3,$4,'PENDING',$5,$6)
RETURNING id`
	if err := r.pool.QueryRow(ctx, insert, req.CompanyID, req.PeriodID, req.TemplateID, variance, req.ActorID, payload).Scan(&id); err != nil {
		return BoardPack{}, err
	}
	return r.GetBoardPack(ctx, id)
}

// GetBoardPack fetches a record with template, company, and period metadata.
func (r *Repository) GetBoardPack(ctx context.Context, id int64) (BoardPack, error) {
	if r == nil || r.pool == nil {
		return BoardPack{}, fmt.Errorf("boardpack: repository not initialised")
	}
	const query = `SELECT
    bp.id,
    bp.company_id,
    COALESCE(c.name,''),
    COALESCE(c.code,''),
    bp.period_id,
    ap.name,
    ap.start_date,
    ap.end_date,
    ap.status,
    bp.template_id,
    tpl.name,
    tpl.description,
    tpl.sections,
    tpl.is_default,
    tpl.is_active,
    tpl.created_by,
    tpl.created_at,
    tpl.updated_at,
    bp.variance_snapshot_id,
    bp.status,
    bp.generated_at,
    bp.generated_by,
    COALESCE(bp.file_path,''),
    bp.file_size,
    bp.page_count,
    COALESCE(bp.error_message,''),
    bp.metadata,
    bp.created_at,
    bp.updated_at
FROM board_packs bp
JOIN companies c ON c.id = bp.company_id
JOIN accounting_periods ap ON ap.id = bp.period_id
JOIN board_pack_templates tpl ON tpl.id = bp.template_id
WHERE bp.id = $1`
	pack, err := scanBoardPack(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BoardPack{}, ErrBoardPackNotFound
		}
		return BoardPack{}, err
	}
	return pack, nil
}

// ListBoardPacks returns paginated rows filtered by optional company, period, and status.
func (r *Repository) ListBoardPacks(ctx context.Context, filter ListFilter) ([]BoardPack, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("boardpack: repository not initialised")
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	query := `SELECT
    bp.id,
    bp.company_id,
    COALESCE(c.name,''),
    COALESCE(c.code,''),
    bp.period_id,
    ap.name,
    ap.start_date,
    ap.end_date,
    ap.status,
    bp.template_id,
    tpl.name,
    tpl.description,
    tpl.sections,
    tpl.is_default,
    tpl.is_active,
    tpl.created_by,
    tpl.created_at,
    tpl.updated_at,
    bp.variance_snapshot_id,
    bp.status,
    bp.generated_at,
    bp.generated_by,
    COALESCE(bp.file_path,''),
    bp.file_size,
    bp.page_count,
    COALESCE(bp.error_message,''),
    bp.metadata,
    bp.created_at,
    bp.updated_at
FROM board_packs bp
JOIN companies c ON c.id = bp.company_id
JOIN accounting_periods ap ON ap.id = bp.period_id
JOIN board_pack_templates tpl ON tpl.id = bp.template_id
WHERE ($1 = 0 OR bp.company_id = $1)
  AND ($2 = 0 OR bp.period_id = $2)
  AND ($3 = '' OR bp.status = $3)
ORDER BY bp.created_at DESC
LIMIT $4 OFFSET $5`
	rows, err := r.pool.Query(ctx, query, filter.CompanyID, filter.PeriodID, string(filter.Status), limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var packs []BoardPack
	for rows.Next() {
		pack, err := scanBoardPack(rows)
		if err != nil {
			return nil, err
		}
		packs = append(packs, pack)
	}
	return packs, rows.Err()
}

// MarkInProgress transitions a pending pack to in-progress.
func (r *Repository) MarkInProgress(ctx context.Context, id int64) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("boardpack: repository not initialised")
	}
	cmd, err := r.pool.Exec(ctx, `UPDATE board_packs
SET status = 'IN_PROGRESS', error_message = NULL, updated_at = NOW()
WHERE id = $1 AND status = 'PENDING'`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrInvalidStatus
	}
	return nil
}

// MarkReady stores the file artefact metadata and marks the pack as ready.
func (r *Repository) MarkReady(ctx context.Context, id int64, filePath string, fileSize int64, pageCount *int, generatedAt time.Time, metadata map[string]any) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("boardpack: repository not initialised")
	}
	meta := mergeMetadata(metadata)
	payload, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	var count any
	if pageCount != nil {
		count = *pageCount
	}
	cmd, err := r.pool.Exec(ctx, `UPDATE board_packs
SET status = 'READY', file_path = $2, file_size = $3, page_count = $4, metadata = $5,
    generated_at = $6, updated_at = NOW()
WHERE id = $1`, id, filePath, fileSize, count, payload, generatedAt)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrBoardPackNotFound
	}
	return nil
}

// MarkFailed captures the error message and switches the status to failed.
func (r *Repository) MarkFailed(ctx context.Context, id int64, msg string) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("boardpack: repository not initialised")
	}
	cmd, err := r.pool.Exec(ctx, `UPDATE board_packs SET status = 'FAILED', error_message = $2, updated_at = NOW() WHERE id = $1`, id, truncateError(msg))
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrBoardPackNotFound
	}
	return nil
}

// ListCompanies returns companies ordered by name for dropdowns.
func (r *Repository) ListCompanies(ctx context.Context) ([]Company, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("boardpack: repository not initialised")
	}
	rows, err := r.pool.Query(ctx, `SELECT id, code, name FROM companies ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var companies []Company
	for rows.Next() {
		var c Company
		if err := rows.Scan(&c.ID, &c.Code, &c.Name); err != nil {
			return nil, err
		}
		companies = append(companies, c)
	}
	return companies, rows.Err()
}

// GetCompany returns company metadata by id.
func (r *Repository) GetCompany(ctx context.Context, id int64) (Company, error) {
	var c Company
	err := r.pool.QueryRow(ctx, `SELECT id, code, name FROM companies WHERE id = $1`, id).Scan(&c.ID, &c.Code, &c.Name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Company{}, ErrCompanyNotFound
		}
		return Company{}, err
	}
	return c, nil
}

// GetPeriod returns accounting period metadata by id.
func (r *Repository) GetPeriod(ctx context.Context, id int64) (Period, error) {
	var p Period
	err := r.pool.QueryRow(ctx, `SELECT id, name, start_date, end_date, status, COALESCE(company_id, 0)
FROM accounting_periods WHERE id = $1`, id).
		Scan(&p.ID, &p.Name, &p.StartDate, &p.EndDate, &p.Status, &p.CompanyID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Period{}, ErrPeriodNotFound
		}
		return Period{}, err
	}
	return p, nil
}

// ListRecentPeriods returns the latest periods for a company.
func (r *Repository) ListRecentPeriods(ctx context.Context, companyID int64, limit int) ([]Period, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("boardpack: repository not initialised")
	}
	if limit <= 0 || limit > 200 {
		limit = 36
	}
	rows, err := r.pool.Query(ctx, `SELECT id, name, start_date, end_date, status, COALESCE(company_id, 0)
FROM accounting_periods
WHERE ($1 = 0)
   OR company_id = $1
   OR company_id IS NULL
ORDER BY start_date DESC
LIMIT $2`, companyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var periods []Period
	for rows.Next() {
		var p Period
		if err := rows.Scan(&p.ID, &p.Name, &p.StartDate, &p.EndDate, &p.Status, &p.CompanyID); err != nil {
			return nil, err
		}
		periods = append(periods, p)
	}
	return periods, rows.Err()
}

// ListVarianceSnapshots lists the latest ready variance snapshots for the specified company.
func (r *Repository) ListVarianceSnapshots(ctx context.Context, companyID int64, limit int) ([]VarianceSnapshot, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("boardpack: repository not initialised")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.pool.Query(ctx, `SELECT vs.id, COALESCE(vr.name,''), vs.period_id, vr.company_id, vs.status
FROM variance_snapshots vs
JOIN variance_rules vr ON vr.id = vs.rule_id
WHERE vs.status = 'READY' AND ($1 = 0 OR vr.company_id = $1)
ORDER BY vs.updated_at DESC
LIMIT $2`, companyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var snaps []VarianceSnapshot
	for rows.Next() {
		var snap VarianceSnapshot
		if err := rows.Scan(&snap.ID, &snap.RuleName, &snap.PeriodID, &snap.CompanyID, &snap.Status); err != nil {
			return nil, err
		}
		snaps = append(snaps, snap)
	}
	return snaps, rows.Err()
}

// GetVarianceSnapshot returns snapshot metadata for validation and UI.
func (r *Repository) GetVarianceSnapshot(ctx context.Context, id int64) (VarianceSnapshot, error) {
	if r == nil || r.pool == nil {
		return VarianceSnapshot{}, fmt.Errorf("boardpack: repository not initialised")
	}
	var snap VarianceSnapshot
	err := r.pool.QueryRow(ctx, `SELECT vs.id, COALESCE(vr.name,''), vs.period_id, vr.company_id, vs.status
FROM variance_snapshots vs
JOIN variance_rules vr ON vr.id = vs.rule_id
WHERE vs.id = $1`, id).
		Scan(&snap.ID, &snap.RuleName, &snap.PeriodID, &snap.CompanyID, &snap.Status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return VarianceSnapshot{}, fmt.Errorf("boardpack: variance snapshot %d tidak ditemukan", id)
		}
		return VarianceSnapshot{}, err
	}
	return snap, nil
}

// AggregateAccountBalances returns per-account balances scoped to company and period.
func (r *Repository) AggregateAccountBalances(ctx context.Context, companyID, periodID int64) ([]reports.AccountBalance, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("boardpack: repository not initialised")
	}
	const query = `WITH target_period AS (
    SELECT id, start_date, end_date FROM accounting_periods WHERE id = $2
)
SELECT acc.code, acc.name, acc.type,
       COALESCE(SUM(CASE WHEN je.date < tp.start_date THEN (jl.debit - jl.credit) ELSE 0 END),0) AS opening,
       COALESCE(SUM(CASE WHEN je.date BETWEEN tp.start_date AND tp.end_date THEN jl.debit ELSE 0 END),0) AS debit,
       COALESCE(SUM(CASE WHEN je.date BETWEEN tp.start_date AND tp.end_date THEN jl.credit ELSE 0 END),0) AS credit
FROM accounts acc
JOIN journal_lines jl ON jl.account_id = acc.id
JOIN journal_entries je ON je.id = jl.je_id AND je.status = 'POSTED'
JOIN target_period tp ON TRUE
WHERE COALESCE(jl.dim_company_id, 0) = $1 AND je.date <= tp.end_date
GROUP BY acc.code, acc.name, acc.type
HAVING COALESCE(SUM(CASE WHEN je.date < tp.start_date THEN (jl.debit - jl.credit) ELSE 0 END),0) <> 0
    OR COALESCE(SUM(CASE WHEN je.date BETWEEN tp.start_date AND tp.end_date THEN (jl.debit - jl.credit) ELSE 0 END),0) <> 0
ORDER BY acc.code`
	rows, err := r.pool.Query(ctx, query, companyID, periodID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var balances []reports.AccountBalance
	for rows.Next() {
		var acc reports.AccountBalance
		if err := rows.Scan(&acc.Code, &acc.Name, &acc.Type, &acc.Opening, &acc.Debit, &acc.Credit); err != nil {
			return nil, err
		}
		balances = append(balances, acc)
	}
	return balances, rows.Err()
}

func scanTemplate(row interface{ Scan(dest ...any) error }) (Template, error) {
	var tpl Template
	var sections []byte
	if err := row.Scan(&tpl.ID, &tpl.Name, &tpl.Description, &sections, &tpl.IsDefault, &tpl.IsActive, &tpl.CreatedBy, &tpl.CreatedAt, &tpl.UpdatedAt); err != nil {
		return Template{}, err
	}
	if len(sections) > 0 {
		if err := json.Unmarshal(sections, &tpl.Sections); err != nil {
			tpl.Sections = nil
		}
	}
	return tpl, nil
}

func scanBoardPack(row interface{ Scan(dest ...any) error }) (BoardPack, error) {
	var bp BoardPack
	var sections []byte
	var variance sql.NullInt64
	var generatedAt sql.NullTime
	var generatedBy sql.NullInt64
	var fileSize sql.NullInt64
	var pageCount sql.NullInt32
	var errMsg sql.NullString
	var metadata []byte
	var tpl Template
	if err := row.Scan(
		&bp.ID,
		&bp.CompanyID,
		&bp.CompanyName,
		&bp.CompanyCode,
		&bp.PeriodID,
		&bp.PeriodName,
		&bp.PeriodStart,
		&bp.PeriodEnd,
		&bp.PeriodStatus,
		&bp.TemplateID,
		&tpl.Name,
		&tpl.Description,
		&sections,
		&tpl.IsDefault,
		&tpl.IsActive,
		&tpl.CreatedBy,
		&tpl.CreatedAt,
		&tpl.UpdatedAt,
		&variance,
		&bp.Status,
		&generatedAt,
		&generatedBy,
		&bp.FilePath,
		&fileSize,
		&pageCount,
		&errMsg,
		&metadata,
		&bp.CreatedAt,
		&bp.UpdatedAt,
	); err != nil {
		return BoardPack{}, err
	}
	if variance.Valid {
		v := variance.Int64
		bp.VarianceSnapshotID = &v
	}
	if generatedAt.Valid {
		t := generatedAt.Time
		bp.GeneratedAt = &t
	}
	if generatedBy.Valid {
		v := generatedBy.Int64
		bp.GeneratedBy = &v
	}
	if fileSize.Valid {
		v := fileSize.Int64
		bp.FileSize = &v
	}
	if pageCount.Valid {
		v := int(pageCount.Int32)
		bp.PageCount = &v
	}
	if errMsg.Valid {
		bp.ErrorMessage = errMsg.String
	}
	if len(sections) > 0 {
		if err := json.Unmarshal(sections, &tpl.Sections); err != nil {
			tpl.Sections = nil
		}
	}
	bp.TemplateName = tpl.Name
	tpl.ID = bp.TemplateID
	bp.Template = &tpl
	bp.Metadata = make(map[string]any)
	if len(metadata) > 0 {
		_ = json.Unmarshal(metadata, &bp.Metadata)
	}
	return bp, nil
}

func mergeMetadata(maps ...map[string]any) map[string]any {
	out := make(map[string]any)
	for _, meta := range maps {
		if meta == nil {
			continue
		}
		for k, v := range meta {
			out[k] = v
		}
	}
	return out
}

func truncateError(msg string) string {
	msg = strings.TrimSpace(msg)
	if len(msg) > 500 {
		return msg[:500]
	}
	return msg
}
