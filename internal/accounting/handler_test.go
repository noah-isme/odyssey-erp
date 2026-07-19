package accounting

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestBudgetPeriod(t *testing.T) {
	now := time.Date(2026, time.July, 19, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		url       string
		wantYear  int
		wantMonth time.Month
		wantErr   bool
	}{
		{name: "defaults to current month", url: "/accounting/budget", wantYear: 2026, wantMonth: time.July},
		{name: "uses requested month", url: "/accounting/budget?period=2025-12", wantYear: 2025, wantMonth: time.December},
		{name: "rejects invalid period", url: "/accounting/budget?period=December", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			year, month, err := budgetPeriod(httptest.NewRequest("GET", tt.url, nil), now)
			if (err != nil) != tt.wantErr {
				t.Fatalf("budgetPeriod() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && (year != tt.wantYear || month != tt.wantMonth) {
				t.Fatalf("budgetPeriod() = %d-%02d, want %d-%02d", year, month, tt.wantYear, tt.wantMonth)
			}
		})
	}
}
