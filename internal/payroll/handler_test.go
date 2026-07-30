package payroll

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/stretchr/testify/require"
)

func TestPayslipConcealsLookupFailureAndAllowsNilRBACService(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/payroll/payslips/99.pdf", nil)
	route := chi.NewRouteContext()
	route.URLParams.Add("id", "99")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))

	handler := NewHandler(nil, NewService(&storeFake{payslipErr: ErrUnauthorized}, nil, nil, nil), nil, nil, nil, rbac.Middleware{})
	response := httptest.NewRecorder()
	handler.payslip(response, req)
	require.Equal(t, http.StatusNotFound, response.Code)
}
