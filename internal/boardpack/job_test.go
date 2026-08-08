package boardpack

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting/reports"
	"github.com/odyssey-erp/odyssey-erp/internal/notifications"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
	"github.com/odyssey-erp/odyssey-erp/jobs"
)

type boardPackPDFClientFake struct{}

func (boardPackPDFClientFake) RenderHTML(context.Context, string) ([]byte, error) {
	return []byte("pdf-bytes"), nil
}

type boardPackStorageFake struct {
	savedPath string
}

func (s *boardPackStorageFake) Save(context.Context, int64, []byte) (string, error) {
	if s.savedPath == "" {
		s.savedPath = "/tmp/board-pack.pdf"
	}
	return s.savedPath, nil
}

func (s *boardPackStorageFake) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("pdf-bytes")), nil
}

type boardPackNotificationStore struct {
	items []notifications.Notification
}

func (s *boardPackNotificationStore) Create(_ context.Context, n notifications.Notification) (notifications.Notification, error) {
	n.ID = int64(len(s.items) + 1)
	s.items = append(s.items, n)
	return n, nil
}

func (s *boardPackNotificationStore) ListRecent(context.Context, int64, int) ([]notifications.Notification, error) {
	return nil, nil
}

func (s *boardPackNotificationStore) ListUnread(context.Context, int64, int) ([]notifications.Notification, error) {
	return nil, nil
}

func (s *boardPackNotificationStore) UnreadCount(context.Context, int64) (int64, error) {
	return 0, nil
}

func (s *boardPackNotificationStore) MarkRead(context.Context, int64, int64, time.Time) (bool, error) {
	return false, nil
}

func (s *boardPackNotificationStore) MarkAllRead(context.Context, int64, time.Time) (int64, error) {
	return 0, nil
}

type boardPackPrefs struct{}

func (boardPackPrefs) Channels(context.Context, int64, string) (notifications.Channels, error) {
	return notifications.Channels{InApp: true, Email: false}, nil
}

func (boardPackPrefs) UserEmail(context.Context, int64) (string, error) { return "", nil }
func (boardPackPrefs) UserPhone(context.Context, int64) (string, error) { return "", nil }

func boardPackGetRow(status string, filePath string) []any {
	sections, _ := json.Marshal([]TemplateSection{{Type: SectionExecSummary, Title: "Executive"}})
	meta, _ := json.Marshal(map[string]any{"requested_by": 7})
	return []any{
		int64(1),
		int64(2),
		"PT Board",
		"BRD",
		int64(3),
		"2026-07",
		pgtype.Date{Time: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), Valid: true},
		pgtype.Date{Time: time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC), Valid: true},
		"OPEN",
		int64(4),
		"Board Pack",
		pgtype.Text{String: "Executive pack", Valid: true},
		sections,
		true,
		true,
		int64(9),
		pgtype.Timestamptz{Time: time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC), Valid: true},
		pgtype.Timestamptz{Time: time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC), Valid: true},
		pgtype.Int8{Valid: false},
		status,
		pgtype.Timestamptz{Time: time.Date(2026, 7, 31, 13, 0, 0, 0, time.UTC), Valid: status == "READY"},
		pgtype.Int8{Int64: 9, Valid: true},
		filePath,
		pgtype.Int8{Int64: 1200, Valid: status == "READY"},
		pgtype.Int4{Int32: 10, Valid: status == "READY"},
		"",
		meta,
		pgtype.Timestamptz{Time: time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC), Valid: true},
		pgtype.Timestamptz{Time: time.Date(2026, 7, 31, 12, 30, 0, 0, time.UTC), Valid: true},
	}
}

func TestJobDispatchesReportDeliveredNotification(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()

	db.ExpectQuery("SELECT").
		WithArgs(int64(1)).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "company_id", "company_name", "company_code", "period_id", "period_name", "period_start", "period_end", "period_status",
			"template_id", "template_name", "template_description", "template_sections", "template_is_default", "template_is_active",
			"template_created_by", "template_created_at", "template_updated_at", "variance_snapshot_id", "status", "generated_at", "generated_by",
			"file_path", "file_size", "page_count", "error_message", "metadata", "created_at", "updated_at",
		}).AddRow(boardPackGetRow("PENDING", "")...))
	db.ExpectExec("UPDATE board_packs SET status = 'IN_PROGRESS'").
		WithArgs(int64(1)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec("UPDATE board_packs SET status = 'READY'").
		WithArgs(int64(1), pgtype.Text{String: "/tmp/board-pack.pdf", Valid: true}, pgtype.Int8{Int64: 9, Valid: true}, pgtype.Int4{Valid: false}, pgxmock.AnyArg(), pgtype.Timestamptz{Time: time.Date(2026, 7, 31, 13, 0, 0, 0, time.UTC), Valid: true}).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectQuery("SELECT").
		WithArgs(int64(1)).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "company_id", "company_name", "company_code", "period_id", "period_name", "period_start", "period_end", "period_status",
			"template_id", "template_name", "template_description", "template_sections", "template_is_default", "template_is_active",
			"template_created_by", "template_created_at", "template_updated_at", "variance_snapshot_id", "status", "generated_at", "generated_by",
			"file_path", "file_size", "page_count", "error_message", "metadata", "created_at", "updated_at",
		}).AddRow(boardPackGetRow("READY", "/tmp/board-pack.pdf")...))

	repo := &Repository{queries: sqlc.New(db)}
	service := NewService(repo)
	service.WithNow(func() time.Time { return time.Date(2026, 7, 31, 13, 0, 0, 0, time.UTC) })
	builder := NewBuilder(&stubRepo{balances: []reports.AccountBalance{{Code: "4000", Name: "Revenue", Type: "REVENUE", Credit: 1200}}, template: Template{ID: 4, Name: "Board Pack", IsActive: true, Sections: []TemplateSection{{Type: SectionExecSummary, Title: "Executive"}}}}, nil, nil)
	renderer, err := NewRenderer(boardPackPDFClientFake{})
	require.NoError(t, err)
	store := &boardPackStorageFake{}
	notifStore := &boardPackNotificationStore{}
	dispatcher := notifications.NewDispatcher(notifications.NewService(notifStore), boardPackPrefs{}, nil, nil)
	job := NewJob(JobConfig{Service: service, Builder: builder, Renderer: renderer, Storage: store, Notifications: dispatcher})

	task, err := jobs.NewBoardPackTask(1)
	require.NoError(t, err)
	require.NoError(t, job.Handle(context.Background(), task))
	require.Len(t, notifStore.items, 1)
	require.Equal(t, notifications.TypeReportDelivered, notifStore.items[0].Type)
	require.Equal(t, int64(7), notifStore.items[0].RecipientID)
}
