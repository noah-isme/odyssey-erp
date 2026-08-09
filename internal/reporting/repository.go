package reporting

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// PGRepository adapts generated reporting queries to reporting-owned values.
type PGRepository struct {
	queries *sqlc.Queries
}

// PGQueryExecutor runs a compiled report query and returns only the statistic
// the runner needs, keeping pgx rows out of the reporting service contract.
type PGQueryExecutor struct {
	pool *pgxpool.Pool
}

func NewQueryExecutor(pool *pgxpool.Pool) *PGQueryExecutor {
	return &PGQueryExecutor{pool: pool}
}

func (e *PGQueryExecutor) CountRows(ctx context.Context, query string, args ...any) (int, error) {
	rows, err := e.pool.Query(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	return count, rows.Err()
}

func NewRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{queries: sqlc.New(pool)}
}

func (r *PGRepository) GetDataset(ctx context.Context, companyID int64, key string, version int) (DatasetRecord, error) {
	row, err := r.queries.GetReportingDataset(ctx, sqlc.GetReportingDatasetParams{
		CompanyID: companyID,
		Key:       key,
		Version:   int32(version),
	})
	if err != nil {
		return DatasetRecord{}, err
	}
	return DatasetRecord{
		ID:      uuid.UUID(row.ID.Bytes),
		Key:     row.Key,
		Version: int(row.Version),
	}, nil
}

func (r *PGRepository) ListDatasetFields(ctx context.Context, datasetID uuid.UUID) ([]DatasetFieldRecord, error) {
	rows, err := r.queries.ListReportingDatasetFields(ctx, pgtype.UUID{Bytes: datasetID, Valid: true})
	if err != nil {
		return nil, err
	}
	fields := make([]DatasetFieldRecord, len(rows))
	for i, row := range rows {
		fields[i] = DatasetFieldRecord{
			Name:           row.FieldName,
			Type:           row.FieldType,
			Classification: row.Classification,
			IsDimension:    row.IsDimension,
			IsMeasure:      row.IsMeasure,
		}
	}
	return fields, nil
}

// ReportRunCreateInput is the persistence-neutral input for a report run.
type ReportRunCreateInput struct {
	CompanyID         int64
	DatasetID         uuid.UUID
	ActorID           int64
	Status            string
	QueryCostEstimate int
}

type ReportRunStatusUpdate struct {
	ID              uuid.UUID
	Status          string
	RowCount        int
	ErrorMessage    string
	ExecutedSQL     string
	ExecutionTimeMS int
	StartedAt       time.Time
	CompletedAt     time.Time
}

func (r *PGRepository) CreateReportRun(ctx context.Context, input ReportRunCreateInput) (uuid.UUID, error) {
	row, err := r.queries.CreateReportRun(ctx, sqlc.CreateReportRunParams{
		CompanyID:         input.CompanyID,
		DatasetID:         pgtype.UUID{Bytes: input.DatasetID, Valid: true},
		ActorID:           input.ActorID,
		Status:            input.Status,
		QueryCostEstimate: pgtype.Int4{Int32: int32(input.QueryCostEstimate), Valid: true},
	})
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.UUID(row.ID.Bytes), nil
}

func (r *PGRepository) UpdateReportRunStatus(ctx context.Context, update ReportRunStatusUpdate) error {
	return r.queries.UpdateReportRunStatus(ctx, sqlc.UpdateReportRunStatusParams{
		ID:              pgtype.UUID{Bytes: update.ID, Valid: true},
		Status:          update.Status,
		RowCount:        pgtype.Int4{Int32: int32(update.RowCount), Valid: true},
		ErrorMessage:    pgtype.Text{String: update.ErrorMessage, Valid: update.ErrorMessage != ""},
		ExecutedSql:     pgtype.Text{String: update.ExecutedSQL, Valid: update.ExecutedSQL != ""},
		ExecutionTimeMs: pgtype.Int4{Int32: int32(update.ExecutionTimeMS), Valid: true},
		StartedAt:       pgtype.Timestamptz{Time: update.StartedAt, Valid: true},
		CompletedAt:     pgtype.Timestamptz{Time: update.CompletedAt, Valid: true},
	})
}
