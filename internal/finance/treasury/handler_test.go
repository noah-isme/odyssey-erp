package treasury_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/treasury"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

func TestTreasuryHandlersUseTenantIdentityAndImplementFlows(t *testing.T) {
	repo := &mockRepo{accounts: []treasury.SupplierBankAccount{{
		ID:                 9,
		CompanyID:          1,
		SupplierID:         100,
		Currency:           "USD",
		EffectiveFrom:      time.Now().Add(-time.Hour),
		VerificationStatus: "VERIFIED",
	}}}
	service := treasury.NewService(repo, nil, nil)
	h := treasury.NewHandler(service)
	router := chi.NewRouter()
	h.MountRoutes(router)

	session := &shared.Session{}
	session.SetUser("42")
	session.Set("company_id", "1")
	ctx := shared.ContextWithSession(context.Background(), session)

	listReq := httptest.NewRequest(http.MethodGet, "/suppliers/100/bank-accounts", nil).WithContext(ctx)
	listReq = withTreasuryParams(listReq, map[string]string{"supplier_id": "100"})
	listResp := httptest.NewRecorder()
	h.ListBankAccounts(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("ListBankAccounts() status = %d, body = %s", listResp.Code, listResp.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, "/batches", strings.NewReader(`{"reference_code":"B-1","currency":"USD"}`)).WithContext(ctx)
	createResp := httptest.NewRecorder()
	h.CreateBatch(createResp, createReq)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("CreateBatch() status = %d, body = %s", createResp.Code, createResp.Body.String())
	}

	itemReq := httptest.NewRequest(http.MethodPost, "/batches/1/items", strings.NewReader(`{"supplier_id":100,"bank_account_id":9,"amount":25}`)).WithContext(ctx)
	itemReq = withTreasuryParams(itemReq, map[string]string{"id": "1"})
	itemResp := httptest.NewRecorder()
	h.AddBatchItem(itemResp, itemReq)
	if itemResp.Code != http.StatusCreated {
		t.Fatalf("AddBatchItem() status = %d, body = %s", itemResp.Code, itemResp.Body.String())
	}
	if repo.batches[0].TotalAmount != 25 {
		t.Fatalf("batch total = %v, want 25", repo.batches[0].TotalAmount)
	}
}

func TestTreasuryHandlersRejectMissingTenantIdentity(t *testing.T) {
	h := treasury.NewHandler(nil)
	request := httptest.NewRequest(http.MethodGet, "/suppliers/100/bank-accounts", nil)
	request = withTreasuryParams(request, map[string]string{"supplier_id": "100"})
	response := httptest.NewRecorder()
	h.ListBankAccounts(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("ListBankAccounts() status = %d, want 401", response.Code)
	}
}

func withTreasuryParams(request *http.Request, params map[string]string) *http.Request {
	routeContext := chi.NewRouteContext()
	for key, value := range params {
		routeContext.URLParams.Add(key, value)
	}
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
}
