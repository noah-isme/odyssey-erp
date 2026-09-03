package dashboard

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

type dashboardServiceStub struct {
	kpis          *KPIData
	kpisErr       error
	activities    []RecentActivity
	activitiesErr error
	companyID     int64
	activityLimit int
}

func (s *dashboardServiceStub) GetKPIs(_ context.Context, companyID int64) (*KPIData, error) {
	s.companyID = companyID
	return s.kpis, s.kpisErr
}

func (s *dashboardServiceStub) GetRecentActivity(_ context.Context, limit int) ([]RecentActivity, error) {
	s.activityLimit = limit
	return s.activities, s.activitiesErr
}

func TestDashboardRoutesReturnJSON(t *testing.T) {
	svc := &dashboardServiceStub{
		kpis:       &KPIData{OpenSalesOrders: 4, LowStockItems: 2},
		activities: []RecentActivity{{At: time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC), Actor: "ops@example.com", Action: "CREATE", Entity: "sales_order", EntityID: "SO-1"}},
	}
	router := chi.NewRouter()
	NewHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), svc, nil, nil).MountRoutes(router)

	for _, tc := range []struct {
		path string
		body string
	}{
		{"/api/dashboard/kpis", `"open_sales_orders":4`},
		{"/api/dashboard/activity", `"actor":"ops@example.com"`},
	} {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if rec.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("content type=%q", rec.Header().Get("Content-Type"))
			}
			if !containsJSON(rec.Body.String(), tc.body) {
				t.Fatalf("body=%s, missing %s", rec.Body.String(), tc.body)
			}
		})
	}

	if svc.companyID != 1 || svc.activityLimit != 10 {
		t.Fatalf("service calls company=%d activityLimit=%d", svc.companyID, svc.activityLimit)
	}
}

func TestDashboardRoutesReturnServerErrorWhenServiceFails(t *testing.T) {
	svc := &dashboardServiceStub{kpisErr: errors.New("kpis unavailable"), activitiesErr: errors.New("activity unavailable")}
	router := chi.NewRouter()
	NewHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), svc, nil, nil).MountRoutes(router)

	for _, path := range []string{"/api/dashboard/kpis", "/api/dashboard/activity"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func containsJSON(body, fragment string) bool {
	return len(body) >= len(fragment) && contains(body, fragment)
}

func contains(value, fragment string) bool {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
