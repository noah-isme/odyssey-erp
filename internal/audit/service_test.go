package audit

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

type stubTimelineRepo struct {
	windowRows     []sqlc.AuditTimelineWindowRow
	allRows        []sqlc.AuditTimelineAllRow
	lastWindowCall sqlc.AuditTimelineWindowParams
	lastAllCall    sqlc.AuditTimelineAllParams
}

func (s *stubTimelineRepo) AuditTimelineWindow(ctx context.Context, arg sqlc.AuditTimelineWindowParams) ([]sqlc.AuditTimelineWindowRow, error) {
	s.lastWindowCall = arg
	return s.windowRows, nil
}

func (s *stubTimelineRepo) AuditTimelineAll(ctx context.Context, arg sqlc.AuditTimelineAllParams) ([]sqlc.AuditTimelineAllRow, error) {
	s.lastAllCall = arg
	return s.allRows, nil
}

func TestServiceTimelinePaging(t *testing.T) {
	repo := &stubTimelineRepo{
		windowRows: []sqlc.AuditTimelineWindowRow{
			mockWindowRow("2024-03-10T10:00:00Z", "user@example.com", "UPDATE", "journal_entries", "1", 1001, "2024-03"),
			mockWindowRow("2024-03-09T09:00:00Z", "user@example.com", "UPDATE", "periods", "2", 0, "2024-02"),
			mockWindowRow("2024-03-08T08:00:00Z", "user@example.com", "CREATE", "periods", "3", 0, "2024-01"),
		},
	}
	svc := NewService(repo)
	result, err := svc.Timeline(context.Background(), TimelineFilters{
		From:     time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
		To:       time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC),
		Page:     1,
		PageSize: 2,
	})
	if err != nil {
		t.Fatalf("timeline: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result.Rows))
	}
	if !result.Paging.HasNext {
		t.Fatalf("expected hasNext true")
	}
	if repo.lastWindowCall.LimitRows != 3 {
		t.Fatalf("expected limitRows 3, got %d", repo.lastWindowCall.LimitRows)
	}
	if repo.lastWindowCall.OffsetRows != 0 {
		t.Fatalf("expected offset 0, got %d", repo.lastWindowCall.OffsetRows)
	}
}

func TestServiceExportReturnsAllRows(t *testing.T) {
	repo := &stubTimelineRepo{
		allRows: []sqlc.AuditTimelineAllRow{
			mockAllRow("2024-03-10T10:00:00Z", "actor", "UPDATE", "journal_entries", "1", 2001, "2024-03"),
			mockAllRow("2024-03-09T09:00:00Z", "actor", "CREATE", "periods", "2", 0, "2024-02"),
		},
	}
	svc := NewService(repo)
	rows, err := svc.Export(context.Background(), TimelineFilters{From: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if repo.lastAllCall.Actor != (pgtype.Text{}) {
		t.Fatalf("expected actor filter empty")
	}
}

func mockWindowRow(ts, actor, action, entity, entityID string, journal int64, period string) sqlc.AuditTimelineWindowRow {
	tval, _ := time.Parse(time.RFC3339, ts)
	row := sqlc.AuditTimelineWindowRow{
		At:       pgtype.Timestamptz{Time: tval, Valid: true},
		Actor:    actor,
		Action:   action,
		Entity:   entity,
		EntityID: entityID,
	}
	if journal != 0 {
		row.JournalNo = pgtype.Int8{Int64: journal, Valid: true}
	}
	if period != "" {
		row.PeriodCode = pgtype.Text{String: period, Valid: true}
	}
	return row
}

func mockAllRow(ts, actor, action, entity, entityID string, journal int64, period string) sqlc.AuditTimelineAllRow {
	tval, _ := time.Parse(time.RFC3339, ts)
	row := sqlc.AuditTimelineAllRow{
		At:       pgtype.Timestamptz{Time: tval, Valid: true},
		Actor:    actor,
		Action:   action,
		Entity:   entity,
		EntityID: entityID,
	}
	if journal != 0 {
		row.JournalNo = pgtype.Int8{Int64: journal, Valid: true}
	}
	if period != "" {
		row.PeriodCode = pgtype.Text{String: period, Valid: true}
	}
	return row
}
