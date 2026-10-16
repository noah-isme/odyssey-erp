package freight

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// recordingService embeds Service so only the methods under test are faked.
type recordingService struct {
	Service
	listCompanyID int64
	listCalls     int
	createInput   *CreateRateCardInput
}

func (s *recordingService) ListRateCards(_ context.Context, companyID int64, _ RateCardFilter) ([]*RateCard, error) {
	s.listCalls++
	s.listCompanyID = companyID
	return []*RateCard{}, nil
}

func (s *recordingService) CreateRateCard(_ context.Context, input CreateRateCardInput) (*RateCard, error) {
	s.createInput = &input
	return &RateCard{ID: 1, CompanyID: input.CompanyID}, nil
}

func sessionWith(user, company string) *shared.Session {
	sess := &shared.Session{}
	if user != "" {
		sess.SetUser(user)
	}
	if company != "" {
		sess.Set("company_id", company)
	}
	return sess
}

func TestHandlerSessionIdentity(t *testing.T) {
	const createBody = `{"base_rate":"10.00","currency":"IDR","effective_date":"2026-01-01"}`

	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		session     *shared.Session
		wantStatus  int
		wantCompany int64
		wantUser    int64
		wantReached bool
	}{
		{name: "list with session reaches service", method: http.MethodGet, path: "/api/freight/rate-cards",
			session: sessionWith("7", "3"), wantStatus: http.StatusOK, wantCompany: 3, wantReached: true},
		{name: "create with session passes company and user", method: http.MethodPost, path: "/api/freight/rate-cards",
			body: createBody, session: sessionWith("7", "3"), wantStatus: http.StatusCreated, wantCompany: 3, wantUser: 7, wantReached: true},
		{name: "list without session", method: http.MethodGet, path: "/api/freight/rate-cards",
			wantStatus: http.StatusUnauthorized},
		{name: "list without company", method: http.MethodGet, path: "/api/freight/rate-cards",
			session: sessionWith("7", ""), wantStatus: http.StatusUnauthorized},
		{name: "list without user", method: http.MethodGet, path: "/api/freight/rate-cards",
			session: sessionWith("", "3"), wantStatus: http.StatusUnauthorized},
		{name: "list with non-numeric company", method: http.MethodGet, path: "/api/freight/rate-cards",
			session: sessionWith("7", "abc"), wantStatus: http.StatusUnauthorized},
		{name: "create without session and invalid body", method: http.MethodPost, path: "/api/freight/rate-cards",
			body: "{", wantStatus: http.StatusUnauthorized},
		{name: "charges list without session", method: http.MethodGet, path: "/api/freight/charges",
			wantStatus: http.StatusUnauthorized},
		{name: "cost centers list without session", method: http.MethodGet, path: "/api/freight/cost-centers",
			wantStatus: http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &recordingService{}
			r := chi.NewRouter()
			NewHandler(svc).RegisterRoutes(r)

			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.session != nil {
				req = req.WithContext(shared.ContextWithSession(req.Context(), tt.session))
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			reached := svc.listCalls > 0 || svc.createInput != nil
			if reached != tt.wantReached {
				t.Fatalf("service reached = %v, want %v", reached, tt.wantReached)
			}
			if tt.wantCompany != 0 {
				got := svc.listCompanyID
				if svc.createInput != nil {
					got = svc.createInput.CompanyID
					if svc.createInput.CreatedBy != tt.wantUser {
						t.Fatalf("created_by = %d, want %d", svc.createInput.CreatedBy, tt.wantUser)
					}
				}
				if got != tt.wantCompany {
					t.Fatalf("company = %d, want %d", got, tt.wantCompany)
				}
			}
		})
	}
}
