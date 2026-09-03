package periods

import (
	"context"
	"testing"
	"time"
)

type serviceRepo struct {
	period Period
	date   time.Time
}

func (r *serviceRepo) FindOpenPeriodByDate(_ context.Context, date time.Time) (Period, error) {
	r.date = date
	return r.period, nil
}

func TestFindOpenPeriodByDateDelegatesDate(t *testing.T) {
	wantDate := time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC)
	repo := &serviceRepo{period: Period{ID: 7, Code: "2026-07", Status: PeriodStatusOpen}}
	got, err := NewService(repo).FindOpenPeriodByDate(context.Background(), wantDate)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 7 || !repo.date.Equal(wantDate) {
		t.Fatalf("period = %#v, date = %v", got, repo.date)
	}
}
