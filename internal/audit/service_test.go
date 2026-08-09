package audit

import (
	"context"
	"strconv"
	"testing"
	"time"
)

type stubTimelineRepo struct {
	windowRows     []TimelineStorageRow
	allRows        []TimelineStorageRow
	lastWindowCall TimelineQuery
	lastAllCall    TimelineQuery
}

func (s *stubTimelineRepo) AuditTimelineWindow(ctx context.Context, arg TimelineQuery) ([]TimelineStorageRow, error) {
	s.lastWindowCall = arg
	return s.windowRows, nil
}

func (s *stubTimelineRepo) AuditTimelineAll(ctx context.Context, arg TimelineQuery) ([]TimelineStorageRow, error) {
	s.lastAllCall = arg
	return s.allRows, nil
}

func TestServiceTimelinePaging(t *testing.T) {
	repo := &stubTimelineRepo{
		windowRows: []TimelineStorageRow{
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
		allRows: []TimelineStorageRow{
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
	if repo.lastAllCall.Actor != "" {
		t.Fatalf("expected actor filter empty")
	}
}

func mockWindowRow(ts, actor, action, entity, entityID string, journal int64, period string) TimelineStorageRow {
	tval, _ := time.Parse(time.RFC3339, ts)
	return TimelineStorageRow{
		At:         tval,
		Actor:      actor,
		Action:     action,
		Entity:     entity,
		EntityID:   entityID,
		JournalNo:  formatJournal(journal),
		PeriodCode: period,
	}
}

func mockAllRow(ts, actor, action, entity, entityID string, journal int64, period string) TimelineStorageRow {
	return mockWindowRow(ts, actor, action, entity, entityID, journal, period)
}

func formatJournal(value int64) string {
	if value == 0 {
		return ""
	}
	return strconv.FormatInt(value, 10)
}
