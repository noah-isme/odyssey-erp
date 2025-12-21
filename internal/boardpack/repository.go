package boardpack

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

	"github.com/odyssey-erp/odyssey-erp/internal/accounting/reports"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// Repository persists board pack templates, requests, and supporting metadata.
type Repository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

// NewRepository constructs a repository wrapper.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

// ListTemplates returns board pack templates filtered by active flag.
func (r *Repository) ListTemplates(ctx context.Context, includeInactive bool) ([]Template, error) {
	rows, err := r.queries.ListTemplates(ctx, includeInactive)
	if err != nil {
		return nil, err
	}
	templates := make([]Template, len(rows))
	for i, row := range rows {
		templates[i] = mapTemplateFromList(row)
	}
	return templates, nil
}

// GetTemplate loads a template by id.
func (r *Repository) GetTemplate(ctx context.Context, id int64) (Template, error) {
	row, err := r.queries.GetTemplate(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Template{}, ErrTemplateNotFound
		}
		return Template{}, err
	}
	return mapTemplateFromGet(row), nil
}

// InsertBoardPack stores a new board pack request.
func (r *Repository) InsertBoardPack(ctx context.Context, req CreateRequest) (BoardPack, error) {
	meta := mergeMetadata(req.Metadata)
	meta["requested_by"] = req.ActorID
	payload, err := json.Marshal(meta)
	if err != nil {
		return BoardPack{}, err
	}
	
	id, err := r.queries.InsertBoardPack(ctx, sqlc.InsertBoardPackParams{
		CompanyID:          req.CompanyID,
		PeriodID:           req.PeriodID,
		TemplateID:         req.TemplateID,
		VarianceSnapshotID: int8ToPointerInt8Original(req.VarianceSnapshotID),
		GeneratedBy:        int8FromInt64(req.ActorID),
		Metadata:           payload,
	})
	if err != nil {
		return BoardPack{}, err
	}
	return r.GetBoardPack(ctx, id)
}

// GetBoardPack fetches a record with template, company, and period metadata.
func (r *Repository) GetBoardPack(ctx context.Context, id int64) (BoardPack, error) {
	row, err := r.queries.GetBoardPack(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BoardPack{}, ErrBoardPackNotFound
		}
		return BoardPack{}, err
	}
	return mapBoardPackFromGet(row), nil
}

// ListBoardPacks returns paginated rows filtered by optional company, period, and status.
func (r *Repository) ListBoardPacks(ctx context.Context, filter ListFilter) ([]BoardPack, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	
	rows, err := r.queries.ListBoardPacks(ctx, sqlc.ListBoardPacksParams{
		Column1: filter.CompanyID, 
		Column2: filter.PeriodID,  
		Column3: string(filter.Status),
		Limit:   int32(limit),
		Offset:  int32(offset),
	})
	if err != nil {
		return nil, err
	}
	
	packs := make([]BoardPack, len(rows))
	for i, row := range rows {
		packs[i] = mapBoardPackFromList(row)
	}
	return packs, nil
}

// MarkInProgress transitions a pending pack to in-progress.
func (r *Repository) MarkInProgress(ctx context.Context, id int64) error {
	return r.queries.MarkInProgress(ctx, id)
}

// MarkReady stores the file artefact metadata and marks the pack as ready.
func (r *Repository) MarkReady(ctx context.Context, id int64, filePath string, fileSize int64, pageCount *int, generatedAt time.Time, metadata map[string]any) error {
	meta := mergeMetadata(metadata)
	payload, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	
	return r.queries.MarkReady(ctx, sqlc.MarkReadyParams{
		ID:          id,
		FilePath:    pgtype.Text{String: filePath, Valid: true},
		FileSize:    int8FromInt64(fileSize),
		PageCount:   intFromIntPointer(pageCount),
		Metadata:    payload,
		GeneratedAt: pgtype.Timestamptz{Time: generatedAt, Valid: true},
	})
}

// MarkFailed captures the error message and switches the status to failed.
func (r *Repository) MarkFailed(ctx context.Context, id int64, msg string) error {
	return r.queries.MarkFailed(ctx, sqlc.MarkFailedParams{
		ID:           id,
		ErrorMessage: pgtype.Text{String: truncateError(msg), Valid: true},
	})
}

// ListCompanies returns companies ordered by name for dropdowns.
func (r *Repository) ListCompanies(ctx context.Context) ([]Company, error) {
	rows, err := r.queries.ListCompanies(ctx)
	if err != nil {
		return nil, err
	}
	companies := make([]Company, len(rows))
	for i, row := range rows {
		companies[i] = Company{
			ID:   row.ID,
			Code: row.Code,
			Name: row.Name,
		}
	}
	return companies, nil
}

// GetCompany returns company metadata by id.
func (r *Repository) GetCompany(ctx context.Context, id int64) (Company, error) {
	row, err := r.queries.GetCompany(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Company{}, ErrCompanyNotFound
		}
		return Company{}, err
	}
	return Company{
		ID:   row.ID,
		Code: row.Code,
		Name: row.Name,
	}, nil
}

// GetPeriod returns accounting period metadata by id.
func (r *Repository) GetPeriod(ctx context.Context, id int64) (Period, error) {
	row, err := r.queries.GetPeriod(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Period{}, ErrPeriodNotFound
		}
		return Period{}, err
	}
	return Period{
		ID:        row.ID,
		Name:      row.Name,
		StartDate: row.StartDate.Time,
		EndDate:   row.EndDate.Time,
		Status:    string(row.Status),
		CompanyID: row.CompanyID,
	}, nil
}

// ListRecentPeriods returns the latest periods for a company.
func (r *Repository) ListRecentPeriods(ctx context.Context, companyID int64, limit int) ([]Period, error) {
	if limit <= 0 || limit > 200 {
		limit = 36
	}
	rows, err := r.queries.ListRecentPeriods(ctx, sqlc.ListRecentPeriodsParams{
		Column1: companyID,
		Limit:   int32(limit),
	})
	if err != nil {
		return nil, err
	}
	periods := make([]Period, len(rows))
	for i, row := range rows {
		periods[i] = Period{
			ID:        row.ID,
			Name:      row.Name,
			StartDate: row.StartDate.Time,
			EndDate:   row.EndDate.Time,
			Status:    string(row.Status),
			CompanyID: row.CompanyID,
		}
	}
	return periods, nil
}

// ListVarianceSnapshots lists the latest ready variance snapshots for the specified company.
func (r *Repository) ListVarianceSnapshots(ctx context.Context, companyID int64, limit int) ([]VarianceSnapshot, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.queries.ListVarianceSnapshots(ctx, sqlc.ListVarianceSnapshotsParams{
		Column1: companyID,
		Limit:   int32(limit),
	})
	if err != nil {
		return nil, err
	}
	snaps := make([]VarianceSnapshot, len(rows))
	for i, row := range rows {
		snaps[i] = VarianceSnapshot{
			ID:        row.ID,
			RuleName:  row.RuleName,
			PeriodID:  row.PeriodID,
			CompanyID: row.CompanyID,
			Status:    string(row.Status),
		}
	}
	return snaps, nil
}

// GetVarianceSnapshot returns snapshot metadata for validation and UI.
func (r *Repository) GetVarianceSnapshot(ctx context.Context, id int64) (VarianceSnapshot, error) {
	row, err := r.queries.GetVarianceSnapshot(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return VarianceSnapshot{}, fmt.Errorf("boardpack: variance snapshot %d tidak ditemukan", id)
		}
		return VarianceSnapshot{}, err
	}
	return VarianceSnapshot{
		ID:        row.ID,
		RuleName:  row.RuleName,
		PeriodID:  row.PeriodID,
		CompanyID: row.CompanyID,
		Status:    string(row.Status),
	}, nil
}

// AggregateAccountBalances returns per-account balances scoped to company and period.
func (r *Repository) AggregateAccountBalances(ctx context.Context, companyID, periodID int64) ([]reports.AccountBalance, error) {
	rows, err := r.queries.AggregateAccountBalances(ctx, sqlc.AggregateAccountBalancesParams{
		DimCompanyID: int8FromInt64(companyID),
		ID:           periodID, // Target period ID
	})
	if err != nil {
		return nil, err
	}
	balances := make([]reports.AccountBalance, len(rows))
	for i, row := range rows {
		balances[i] = reports.AccountBalance{
			Code:    row.Code,
			Name:    row.Name,
			Type:    string(row.Type), // Assuming AccountType is string
			Opening: row.Opening,
			Debit:   row.Debit,
			Credit:  row.Credit,
		}
	}
	return balances, nil
}

// Helpers

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

func int8ToPointerInt8Original(i *int64) pgtype.Int8 {
	if i == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *i, Valid: true}
}

func int8FromInt64(i int64) pgtype.Int8 {
    return pgtype.Int8{Int64: i, Valid: true}
}

func intFromIntPointer(i *int) pgtype.Int4 {
	if i == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*i), Valid: true}
}

func int8ToPointer(i pgtype.Int8) *int64 {
	if !i.Valid {
		return nil
	}
	v := i.Int64
	return &v
}
func int4ToPointer(i pgtype.Int4) *int {
	if !i.Valid {
		return nil
	}
	v := int(i.Int32)
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

func mapTemplateFromList(row sqlc.ListTemplatesRow) Template {
	t := Template{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		IsDefault:   row.IsDefault,
		IsActive:    row.IsActive,
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
	if len(row.Sections) > 0 {
		_ = json.Unmarshal(row.Sections, &t.Sections)
	}
	return t
}

func mapTemplateFromGet(row sqlc.GetTemplateRow) Template {
	t := Template{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		IsDefault:   row.IsDefault,
		IsActive:    row.IsActive,
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
	if len(row.Sections) > 0 {
		_ = json.Unmarshal(row.Sections, &t.Sections)
	}
	return t
}

func mapBoardPackFromGet(row sqlc.GetBoardPackRow) BoardPack {
	bp := BoardPack{
		ID:                 row.ID,
		CompanyID:          row.CompanyID,
		CompanyName:        row.CompanyName,
		CompanyCode:        row.CompanyCode,
		PeriodID:           row.PeriodID,
		PeriodName:         row.PeriodName,
		PeriodStart:        row.PeriodStart.Time,
		PeriodEnd:          row.PeriodEnd.Time,
		PeriodStatus:       string(row.PeriodStatus),
		TemplateID:         row.TemplateID,
		VarianceSnapshotID: int8ToPointer(row.VarianceSnapshotID),
		Status:             Status(row.Status),
		GeneratedAt:        timeToPointer(row.GeneratedAt),
		GeneratedBy:        int8ToPointer(row.GeneratedBy),
		FilePath:           row.FilePath,
		FileSize:           int8ToPointer(row.FileSize),
		PageCount:          int4ToPointer(row.PageCount),
		ErrorMessage:       row.ErrorMessage,
		CreatedAt:          row.CreatedAt.Time,
		UpdatedAt:          row.UpdatedAt.Time,
	}

	// Map Template
	t := Template{
		ID:          row.TemplateID,
		Name:        row.TemplateName,
		Description: row.TemplateDescription.String,
		IsDefault:   row.TemplateIsDefault,
		IsActive:    row.TemplateIsActive,
		CreatedBy:   row.TemplateCreatedBy,
		CreatedAt:   row.TemplateCreatedAt.Time,
		UpdatedAt:   row.TemplateUpdatedAt.Time,
	}
	if len(row.TemplateSections) > 0 {
		_ = json.Unmarshal(row.TemplateSections, &t.Sections)
	}
	bp.TemplateName = t.Name
	bp.Template = &t

	// Metadata
	bp.Metadata = make(map[string]any)
	if len(row.Metadata) > 0 {
		_ = json.Unmarshal(row.Metadata, &bp.Metadata)
	}

	return bp
}

func mapBoardPackFromList(row sqlc.ListBoardPacksRow) BoardPack {
	bp := BoardPack{
		ID:                 row.ID,
		CompanyID:          row.CompanyID,
		CompanyName:        row.CompanyName,
		CompanyCode:        row.CompanyCode,
		PeriodID:           row.PeriodID,
		PeriodName:         row.PeriodName,
		PeriodStart:        row.PeriodStart.Time,
		PeriodEnd:          row.PeriodEnd.Time,
		PeriodStatus:       string(row.PeriodStatus),
		TemplateID:         row.TemplateID,
		VarianceSnapshotID: int8ToPointer(row.VarianceSnapshotID),
		Status:             Status(row.Status),
		GeneratedAt:        timeToPointer(row.GeneratedAt),
		GeneratedBy:        int8ToPointer(row.GeneratedBy),
		FilePath:           row.FilePath,
		FileSize:           int8ToPointer(row.FileSize),
		PageCount:          int4ToPointer(row.PageCount),
		ErrorMessage:       row.ErrorMessage,
		CreatedAt:          row.CreatedAt.Time,
		UpdatedAt:          row.UpdatedAt.Time,
	}

	// Map Template
	t := Template{
		ID:          row.TemplateID,
		Name:        row.TemplateName,
		Description: row.TemplateDescription.String,
		IsDefault:   row.TemplateIsDefault,
		IsActive:    row.TemplateIsActive,
		CreatedBy:   row.TemplateCreatedBy,
		CreatedAt:   row.TemplateCreatedAt.Time,
		UpdatedAt:   row.TemplateUpdatedAt.Time,
	}
	if len(row.TemplateSections) > 0 {
		_ = json.Unmarshal(row.TemplateSections, &t.Sections)
	}
	bp.TemplateName = t.Name
	bp.Template = &t

	// Metadata
	bp.Metadata = make(map[string]any)
	if len(row.Metadata) > 0 {
		_ = json.Unmarshal(row.Metadata, &bp.Metadata)
	}

	return bp
}
