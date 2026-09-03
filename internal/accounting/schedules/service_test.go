package schedules

import (
	"context"
	"testing"
)

type serviceRepo struct {
	created CreateScheduleInput
	toggled [2]int64
	retried [2]int64
}

func (r *serviceRepo) List(context.Context, int64) ([]Schedule, error) {
	return []Schedule{{ID: 1}}, nil
}
func (r *serviceRepo) Create(_ context.Context, in CreateScheduleInput) error {
	r.created = in
	return nil
}
func (r *serviceRepo) Toggle(_ context.Context, id, companyID int64) error {
	r.toggled = [2]int64{id, companyID}
	return nil
}
func (r *serviceRepo) Retry(_ context.Context, id, companyID int64) error {
	r.retried = [2]int64{id, companyID}
	return nil
}
func (r *serviceRepo) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	return fn(ctx, r)
}

func TestServiceValidatesScheduleConfiguration(t *testing.T) {
	repo := &serviceRepo{}
	svc := NewService(repo)
	ctx := context.Background()
	if _, err := svc.List(ctx, 0); err == nil {
		t.Fatal("List() accepted a missing company")
	}
	if err := svc.Create(ctx, 1, "OTHER", "DAILY", 0, 0, 0, []string{"a@example.com"}); err == nil {
		t.Fatal("Create() accepted an unsupported report type")
	}
	if err := svc.Create(ctx, 1, "PNL", "YEARLY", 0, 0, 0, []string{"a@example.com"}); err == nil {
		t.Fatal("Create() accepted an unsupported frequency")
	}
	if err := svc.Create(ctx, 1, "PNL", "MONTHLY", 4, 5, -1, []string{"a@example.com"}); err != nil {
		t.Fatal(err)
	}
	if repo.created.Type != "PNL" || repo.created.Frequency != "MONTHLY" || repo.created.DepartmentID != 4 || repo.created.PeriodOffset != -1 {
		t.Fatalf("Create() input = %#v", repo.created)
	}
}

func TestServiceDelegatesToggleAndRetry(t *testing.T) {
	repo := &serviceRepo{}
	svc := NewService(repo)
	ctx := context.Background()
	if err := svc.Toggle(ctx, 8, 2); err != nil {
		t.Fatal(err)
	}
	if err := svc.Retry(ctx, 8, 2); err != nil {
		t.Fatal(err)
	}
	if repo.toggled != [2]int64{8, 2} || repo.retried != [2]int64{8, 2} {
		t.Fatalf("toggle = %v, retry = %v", repo.toggled, repo.retried)
	}
	if err := svc.Toggle(ctx, 0, 2); err == nil {
		t.Fatal("Toggle() accepted a missing ID")
	}
}
