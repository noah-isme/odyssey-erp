package accounts

import (
	"context"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting/reports"
)

type serviceRepo struct {
	accounts []Account
	balances []reports.AccountBalance
	filter   reports.DimensionFilter
	year     int
	month    time.Month
}

func (r *serviceRepo) List(context.Context) ([]Account, error) { return r.accounts, nil }
func (r *serviceRepo) ListBalances(context.Context) ([]reports.AccountBalance, error) {
	return r.balances, nil
}
func (r *serviceRepo) ListBalancesForPeriod(_ context.Context, year int, month time.Month) ([]reports.AccountBalance, error) {
	r.year, r.month = year, month
	return r.balances, nil
}
func (r *serviceRepo) ListBalancesForPeriodAndDimensions(_ context.Context, year int, month time.Month, filter reports.DimensionFilter) ([]reports.AccountBalance, error) {
	r.year, r.month, r.filter = year, month, filter
	return r.balances, nil
}

func TestServiceDelegatesAccountAndBalanceQueries(t *testing.T) {
	repo := &serviceRepo{
		accounts: []Account{{ID: 1, Code: "1000", Name: "Cash", Type: AccountTypeAsset}},
		balances: []reports.AccountBalance{{ID: 1, Code: "1000", Debit: 10}},
	}
	svc := NewService(repo)
	ctx := context.Background()

	accounts, err := svc.List(ctx)
	if err != nil || len(accounts) != 1 {
		t.Fatalf("List() = %#v, %v", accounts, err)
	}
	balances, err := svc.ListBalances(ctx)
	if err != nil || len(balances) != 1 {
		t.Fatalf("ListBalances() = %#v, %v", balances, err)
	}
	_, err = svc.ListBalancesForPeriod(ctx, 2026, time.July)
	if err != nil || repo.year != 2026 || repo.month != time.July {
		t.Fatalf("ListBalancesForPeriod() did not forward period: %v", err)
	}
	filter := reports.DimensionFilter{DepartmentID: 7, CostCenterID: 8}
	_, err = svc.ListBalancesForPeriodAndDimensions(ctx, 2026, time.August, filter)
	if err != nil || repo.filter != filter {
		t.Fatalf("ListBalancesForPeriodAndDimensions() did not forward filter: %v", err)
	}
}
