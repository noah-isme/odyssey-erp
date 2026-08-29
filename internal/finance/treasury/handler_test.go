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

	itemReq := httptest.NewRequest(http.MethodPost, "/batches/1/items", strings.NewReader(`{"supplier_id":100,"bank_account_id":9,"amount":"25"}`)).WithContext(ctx)
	itemReq = withTreasuryParams(itemReq, map[string]string{"id": "1"})
	itemResp := httptest.NewRecorder()
	h.AddBatchItem(itemResp, itemReq)
	if itemResp.Code != http.StatusCreated {
		t.Fatalf("AddBatchItem() status = %d, body = %s", itemResp.Code, itemResp.Body.String())
	}
	if repo.batches[0].TotalAmount.String() != "25" {
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

func TestMountScopedRoutesAssignsDutyPermissions(t *testing.T) {
	h := treasury.NewHandler(nil)
	router := chi.NewRouter()
	authorize := func(perms ...string) func(http.Handler) http.Handler {
		if len(perms) != 1 {
			t.Fatalf("route authorization received %d permissions, want one", len(perms))
		}
		permission := perms[0]
		return func(_ http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-Test-Permission", permission)
				w.WriteHeader(http.StatusNoContent)
			})
		}
	}
	h.MountScopedRoutes(router, authorize)

	tests := []struct {
		name       string
		method     string
		path       string
		permission string
	}{
		{name: "list beneficiaries", method: http.MethodGet, path: "/suppliers/1/bank-accounts", permission: shared.PermFinancePaymentView},
		{name: "add beneficiary", method: http.MethodPost, path: "/suppliers/1/bank-accounts", permission: shared.PermFinancePaymentPropose},
		{name: "approve beneficiary", method: http.MethodPost, path: "/bank-accounts/1/approve", permission: shared.PermFinancePaymentApprove},
		{name: "create batch", method: http.MethodPost, path: "/batches", permission: shared.PermFinancePaymentPropose},
		{name: "add item", method: http.MethodPost, path: "/batches/1/items", permission: shared.PermFinancePaymentPropose},
		{name: "remove item", method: http.MethodDelete, path: "/batches/1/items/2", permission: shared.PermFinancePaymentPropose},
		{name: "submit batch", method: http.MethodPost, path: "/batches/1/submit", permission: shared.PermFinancePaymentPropose},
		{name: "approve batch", method: http.MethodPost, path: "/batches/1/approve", permission: shared.PermFinancePaymentApprove},
		{name: "export batch", method: http.MethodPost, path: "/batches/1/export", permission: shared.PermFinancePaymentExport},
		{name: "execute batch", method: http.MethodPost, path: "/batches/1/execute", permission: shared.PermFinancePaymentExecute},
		{name: "settle batch", method: http.MethodPost, path: "/batches/1/settle", permission: shared.PermFinancePaymentExecute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(tt.method, tt.path, nil))
			if response.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
			}
			if got := response.Header().Get("X-Test-Permission"); got != tt.permission {
				t.Fatalf("permission = %q, want %q", got, tt.permission)
			}
		})
	}
}

func withTreasuryParams(request *http.Request, params map[string]string) *http.Request {
	routeContext := chi.NewRouteContext()
	for key, value := range params {
		routeContext.URLParams.Add(key, value)
	}
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
}
