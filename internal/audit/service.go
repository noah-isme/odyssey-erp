package audit

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	auditdb "github.com/odyssey-erp/odyssey-erp/internal/audit/db"
)

// Repository menyediakan akses ke query sqlc yang dibutuhkan.
type Repository interface {
	AuditTimelineWindow(ctx context.Context, arg auditdb.AuditTimelineWindowParams) ([]auditdb.AuditTimelineWindowRow, error)
	AuditTimelineAll(ctx context.Context, arg auditdb.AuditTimelineAllParams) ([]auditdb.AuditTimelineAllRow, error)
}

// Result membungkus hasil timeline dengan informasi paging.
type Result struct {
	Rows   []TimelineRow
	Paging PagingInfo
}

// Service mengoordinasikan pengambilan data audit.
type Service struct {
	repo Repository
}

// NewService membuat service audit timeline baru.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Timeline mengambil data audit dengan paging.
func (s *Service) Timeline(ctx context.Context, filters TimelineFilters) (Result, error) {
	if s.repo == nil {
		return Result{}, fmt.Errorf("audit: repository not configured")
	}
	pageSize := filters.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 50 {
		pageSize = 50
	}
	page := filters.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize
	params := auditdb.AuditTimelineWindowParams{
		FromAt:     toPgTime(filters.From),
		ToAt:       toPgTime(filters.To),
		Actor:      optionalText(filters.Actor),
		Entity:     optionalText(filters.Entity),
		Action:     optionalText(filters.Action),
		OffsetRows: int32(offset),
		LimitRows:  int32(pageSize + 1),
	}
	rows, err := s.repo.AuditTimelineWindow(ctx, params)
	if err != nil {
		return Result{}, err
	}
	hasNext := len(rows) > pageSize
	if hasNext {
		rows = rows[:pageSize]
	}
	resultRows := make([]TimelineRow, 0, len(rows))
	for _, row := range rows {
		resultRows = append(resultRows, mapTimelineRow(row.At, row.Actor, row.Action, row.Entity, row.EntityID, row.JournalNo, row.PeriodCode))
	}
	paging := PagingInfo{Page: page, PageSize: pageSize, HasNext: hasNext}
	if page > 1 {
		paging.PrevPage = page - 1
	}
	if hasNext {
		paging.NextPage = page + 1
	}
	return Result{Rows: resultRows, Paging: paging}, nil
}

// Export mengambil seluruh data timeline tanpa paging.
func (s *Service) Export(ctx context.Context, filters TimelineFilters) ([]TimelineRow, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("audit: repository not configured")
	}
	params := auditdb.AuditTimelineAllParams{
		FromAt: toPgTime(filters.From),
		ToAt:   toPgTime(filters.To),
		Actor:  optionalText(filters.Actor),
		Entity: optionalText(filters.Entity),
		Action: optionalText(filters.Action),
	}
	rows, err := s.repo.AuditTimelineAll(ctx, params)
	if err != nil {
		return nil, err
	}
	result := make([]TimelineRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapTimelineRow(row.At, row.Actor, row.Action, row.Entity, row.EntityID, row.JournalNo, row.PeriodCode))
	}
	return result, nil
}

func toPgTime(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func optionalText(value string) pgtype.Text {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: trimmed, Valid: true}
}

func mapTimelineRow(at pgtype.Timestamptz, actor, action, entity, entityID string, journal pgtype.Int8, period pgtype.Text) TimelineRow {
	var ts time.Time
	if at.Valid {
		ts = at.Time
	}
	var journalNo string
	if journal.Valid {
		journalNo = strconv.FormatInt(journal.Int64, 10)
	}
	var periodCode string
	if period.Valid {
		periodCode = period.String
	}
	return TimelineRow{
		At:        ts,
		Actor:     actor,
		Action:    action,
		Entity:    entity,
		EntityID:  entityID,
		Period:    periodCode,
		JournalNo: journalNo,
	}
}
