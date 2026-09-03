package audit

import (
	"context"
	"fmt"
	"time"
)

// Repository exposes audit data in domain terms. PostgreSQL encodings are
// translated by the concrete repository adapter.
type Repository interface {
	AuditTimelineWindow(ctx context.Context, query TimelineQuery) ([]TimelineStorageRow, error)
	AuditTimelineAll(ctx context.Context, query TimelineQuery) ([]TimelineStorageRow, error)
}

type TimelineQuery struct {
	From       time.Time
	To         time.Time
	Actor      string
	Entity     string
	Action     string
	OffsetRows int
	LimitRows  int
}

type TimelineStorageRow struct {
	At         time.Time
	Actor      string
	Action     string
	Entity     string
	EntityID   string
	JournalNo  string
	PeriodCode string
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
	params := TimelineQuery{
		From:       filters.From,
		To:         filters.To,
		Actor:      filters.Actor,
		Entity:     filters.Entity,
		Action:     filters.Action,
		OffsetRows: offset,
		LimitRows:  pageSize + 1,
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
		resultRows = append(resultRows, mapTimelineRow(row))
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
	params := TimelineQuery{
		From:   filters.From,
		To:     filters.To,
		Actor:  filters.Actor,
		Entity: filters.Entity,
		Action: filters.Action,
	}
	rows, err := s.repo.AuditTimelineAll(ctx, params)
	if err != nil {
		return nil, err
	}
	result := make([]TimelineRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapTimelineRow(row))
	}
	return result, nil
}

func mapTimelineRow(row TimelineStorageRow) TimelineRow {
	return TimelineRow{
		At:        row.At,
		Actor:     row.Actor,
		Action:    row.Action,
		Entity:    row.Entity,
		EntityID:  row.EntityID,
		Period:    row.PeriodCode,
		JournalNo: row.JournalNo,
	}
}
