package audit

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// PGRepository adapts generated audit queries to the domain repository port.
type PGRepository struct {
	queries *sqlc.Queries
}

func NewRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{queries: sqlc.New(pool)}
}

func (r *PGRepository) AuditTimelineWindow(ctx context.Context, query TimelineQuery) ([]TimelineStorageRow, error) {
	rows, err := r.queries.AuditTimelineWindow(ctx, sqlc.AuditTimelineWindowParams{
		FromAt:     toPgTime(query.From),
		ToAt:       toPgTime(query.To),
		Actor:      optionalText(query.Actor),
		Entity:     optionalText(query.Entity),
		Action:     optionalText(query.Action),
		OffsetRows: int32(query.OffsetRows),
		LimitRows:  int32(query.LimitRows),
	})
	if err != nil {
		return nil, err
	}
	return mapTimelineRows(rows), nil
}

func (r *PGRepository) AuditTimelineAll(ctx context.Context, query TimelineQuery) ([]TimelineStorageRow, error) {
	rows, err := r.queries.AuditTimelineAll(ctx, sqlc.AuditTimelineAllParams{
		FromAt: toPgTime(query.From),
		ToAt:   toPgTime(query.To),
		Actor:  optionalText(query.Actor),
		Entity: optionalText(query.Entity),
		Action: optionalText(query.Action),
	})
	if err != nil {
		return nil, err
	}
	return mapAllTimelineRows(rows), nil
}

func mapTimelineRows(rows []sqlc.AuditTimelineWindowRow) []TimelineStorageRow {
	result := make([]TimelineStorageRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, TimelineStorageRow{
			At:         validTime(row.At),
			Actor:      row.Actor,
			Action:     row.Action,
			Entity:     row.Entity,
			EntityID:   row.EntityID,
			JournalNo:  validInt(row.JournalNo),
			PeriodCode: validText(row.PeriodCode),
		})
	}
	return result
}

func mapAllTimelineRows(rows []sqlc.AuditTimelineAllRow) []TimelineStorageRow {
	result := make([]TimelineStorageRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, TimelineStorageRow{
			At:         validTime(row.At),
			Actor:      row.Actor,
			Action:     row.Action,
			Entity:     row.Entity,
			EntityID:   row.EntityID,
			JournalNo:  validInt(row.JournalNo),
			PeriodCode: validText(row.PeriodCode),
		})
	}
	return result
}

func toPgTime(value time.Time) pgtype.Timestamptz {
	if value.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func optionalText(value string) pgtype.Text {
	value = strings.TrimSpace(value)
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func validTime(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func validInt(value pgtype.Int8) string {
	if !value.Valid {
		return ""
	}
	return strconv.FormatInt(value.Int64, 10)
}

func validText(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
