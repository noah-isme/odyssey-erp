package periods

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/shared"
	pgxmock "github.com/pashagolub/pgxmock/v4"
)

func TestRepositoryFindsOpenPeriod(t *testing.T) {
	db, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := &repository{db: db}
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, -1)
	db.ExpectQuery("SELECT id, code, start_date, end_date, status, closed_at, locked_by").
		WithArgs(start.AddDate(0, 0, 15)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "code", "start_date", "end_date", "status", "closed_at", "locked_by", "created_at", "updated_at"}).
			AddRow(int64(7), "2026-07", start, end, "OPEN", nil, nil, start, start))
	period, err := repo.FindOpenPeriodByDate(context.Background(), start.AddDate(0, 0, 15))
	if err != nil || period.ID != 7 || period.Status != PeriodStatusOpen {
		t.Fatalf("FindOpenPeriodByDate() = %#v, %v", period, err)
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryMapsMissingPeriodToDomainError(t *testing.T) {
	db, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := &repository{db: db}
	db.ExpectQuery("SELECT id, code, start_date, end_date, status, closed_at, locked_by").
		WithArgs(time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)).
		WillReturnError(pgx.ErrNoRows)
	_, err = repo.FindOpenPeriodByDate(context.Background(), time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, shared.ErrInvalidPeriod) {
		t.Fatalf("FindOpenPeriodByDate() error = %v", err)
	}
}
