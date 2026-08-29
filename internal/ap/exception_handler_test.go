package ap

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
	"github.com/stretchr/testify/require"
)

type exceptionWorkbenchFake struct {
	items             []APException
	byID              map[int64]APException
	listStatus        string
	listCompany       int64
	listOwnerID       int64
	listInvoice       int64
	listLimit         int
	listOffset        int
	getCompany        int64
	getErr            error
	resolveErr        error
	resolvedID        int64
	resolvedCompany   int64
	resolvedBy        int64
	resolution        string
	resolutionComment string
}

func (f *exceptionWorkbenchFake) GetException(_ context.Context, id int64) (APException, error) {
	if f.getErr != nil {
		return APException{}, f.getErr
	}
	if exception, ok := f.byID[id]; ok {
		return exception, nil
	}
	return APException{}, errors.New("exception not found")
}

func (f *exceptionWorkbenchFake) GetExceptionForCompany(ctx context.Context, companyID, id int64) (APException, error) {
	f.getCompany = companyID
	return f.GetException(ctx, id)
}

func (f *exceptionWorkbenchFake) ListExceptions(_ context.Context, status string, ownerID, invoiceID int64, limit, offset int) ([]APException, error) {
	f.listStatus, f.listOwnerID, f.listInvoice = status, ownerID, invoiceID
	f.listLimit, f.listOffset = limit, offset
	return f.items, nil
}

func (f *exceptionWorkbenchFake) ListExceptionsForCompany(ctx context.Context, companyID int64, status string, ownerID, invoiceID int64, limit, offset int) ([]APException, error) {
	f.listCompany = companyID
	return f.ListExceptions(ctx, status, ownerID, invoiceID, limit, offset)
}

func (f *exceptionWorkbenchFake) ResolveException(_ context.Context, id, resolvedBy int64, resolution string) error {
	f.resolvedID, f.resolvedBy, f.resolution = id, resolvedBy, resolution
	return f.resolveErr
}

func (f *exceptionWorkbenchFake) ResolveExceptionForCompany(ctx context.Context, companyID, id, resolvedBy int64, resolution string) error {
	f.resolvedCompany = companyID
	return f.ResolveException(ctx, id, resolvedBy, resolution)
}

func (f *exceptionWorkbenchFake) ResolveExceptionForCompanyWithComment(ctx context.Context, companyID, id, resolvedBy int64, resolution, comment string) error {
	f.resolutionComment = comment
	return f.ResolveExceptionForCompany(ctx, companyID, id, resolvedBy, resolution)
}

func exceptionRequest(method, target string, body string) *http.Request {
	session := &shared.Session{}
	session.SetUser("9")
	session.Set("company_id", "7")
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	return request.WithContext(shared.ContextWithSession(request.Context(), session))
}

func exceptionRouteRequest(request *http.Request, id string) *http.Request {
	route := chi.NewRouteContext()
	route.URLParams.Add("id", id)
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, route))
}

func TestExceptionRouteRequiresDedicatedPermission(t *testing.T) {
	router := chi.NewRouter()
	handler := NewHandler(nil, &Service{}, nil, nil, nil, rbac.Middleware{Service: apPermissionReader{}})
	router.Route("/finance/ap", handler.MountRoutes)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, exceptionRequest(http.MethodGet, "/finance/ap/exceptions", ""))

	require.Equal(t, http.StatusForbidden, response.Code)
}

func TestExceptionResolutionRouteRequiresDedicatedPermission(t *testing.T) {
	router := chi.NewRouter()
	handler := NewHandler(nil, &Service{}, nil, nil, nil, rbac.Middleware{Service: apPermissionReader{}})
	router.Route("/finance/ap", handler.MountRoutes)

	request := exceptionRouteRequest(exceptionRequest(http.MethodPost, "/finance/ap/exceptions/1/resolve", "resolution=RESOLVED"), "1")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusForbidden, response.Code)
}

func TestListExceptionsRendersOnlyActiveCompany(t *testing.T) {
	templates, err := view.NewEngine()
	require.NoError(t, err)
	repo := newMemoryAPRepo()
	activeCompanyID, otherCompanyID := int64(7), int64(8)
	repo.invoices[11] = APInvoice{ID: 11, Number: "INV-11", CompanyID: &activeCompanyID}
	repo.invoices[12] = APInvoice{ID: 12, Number: "INV-12", CompanyID: &otherCompanyID}
	now := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
	fake := &exceptionWorkbenchFake{items: []APException{
		{ID: 101, APInvoiceID: 11, ExceptionType: "MISMATCH", Severity: "HIGH", Status: "OPEN", Reason: "Price variance", CreatedAt: now},
		{ID: 102, APInvoiceID: 12, ExceptionType: "DUPLICATE", Severity: "CRITICAL", Status: "OPEN", Reason: "Other company", CreatedAt: now},
	}}
	handler := NewHandler(nil, NewService(repo, nil), templates, shared.NewCSRFManager("test-secret"), nil, rbac.Middleware{})
	handler.exceptions = fake

	response := httptest.NewRecorder()
	handler.listExceptions(response, exceptionRequest(http.MethodGet, "/finance/ap/exceptions?status=open", ""))

	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), "Price variance")
	require.Contains(t, response.Body.String(), "#11")
	require.Contains(t, response.Body.String(), `name="csrf_token"`)
	require.NotContains(t, response.Body.String(), "Other company")
	require.Equal(t, "OPEN", fake.listStatus)
	require.Equal(t, int64(7), fake.listCompany)
	require.Equal(t, int64(0), fake.listOwnerID)
	require.Equal(t, int64(0), fake.listInvoice)
	require.Equal(t, exceptionListLimit, fake.listLimit)
	require.Zero(t, fake.listOffset)
}

func TestResolveExceptionUsesTenantIdentityAndPRG(t *testing.T) {
	templates, err := view.NewEngine()
	require.NoError(t, err)
	repo := newMemoryAPRepo()
	companyID := int64(7)
	repo.invoices[11] = APInvoice{ID: 11, Number: "INV-11", CompanyID: &companyID}
	fake := &exceptionWorkbenchFake{
		byID: map[int64]APException{100: {ID: 100, APInvoiceID: 11, Status: "OPEN"}},
	}
	handler := NewHandler(nil, NewService(repo, nil), templates, shared.NewCSRFManager("test-secret"), nil, rbac.Middleware{})
	handler.exceptions = fake
	request := exceptionRouteRequest(exceptionRequest(http.MethodPost, "/finance/ap/exceptions/100/resolve", "resolution=resolved&comment=quantity+confirmed"), "100")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()

	handler.resolveException(response, request)

	require.Equal(t, http.StatusSeeOther, response.Code)
	require.Equal(t, "/finance/ap/exceptions", response.Header().Get("Location"))
	require.Equal(t, int64(100), fake.resolvedID)
	require.Equal(t, int64(7), fake.getCompany)
	require.Equal(t, int64(7), fake.resolvedCompany)
	require.Equal(t, int64(9), fake.resolvedBy)
	require.Equal(t, "RESOLVED", fake.resolution)
	require.Equal(t, "quantity confirmed", fake.resolutionComment)
}

func TestResolveExceptionRejectsInvalidResolution(t *testing.T) {
	companyID := int64(7)
	repo := newMemoryAPRepo()
	repo.invoices[11] = APInvoice{ID: 11, CompanyID: &companyID}
	fake := &exceptionWorkbenchFake{byID: map[int64]APException{100: {ID: 100, APInvoiceID: 11, Status: "OPEN"}}}
	handler := NewHandler(nil, NewService(repo, nil), nil, nil, nil, rbac.Middleware{})
	handler.exceptions = fake
	request := exceptionRouteRequest(exceptionRequest(http.MethodPost, "/finance/ap/exceptions/100/resolve", "resolution=IN_REVIEW"), "100")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()

	handler.resolveException(response, request)

	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Zero(t, fake.resolvedID)
}

func TestResolveExceptionHidesOtherCompany(t *testing.T) {
	otherCompanyID := int64(8)
	repo := newMemoryAPRepo()
	repo.invoices[12] = APInvoice{ID: 12, CompanyID: &otherCompanyID}
	fake := &exceptionWorkbenchFake{byID: map[int64]APException{100: {ID: 100, APInvoiceID: 12, Status: "OPEN"}}}
	handler := NewHandler(nil, NewService(repo, nil), nil, nil, nil, rbac.Middleware{})
	handler.exceptions = fake
	request := exceptionRouteRequest(exceptionRequest(http.MethodPost, "/finance/ap/exceptions/100/resolve", "resolution=RESOLVED"), "100")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()

	handler.resolveException(response, request)

	require.Equal(t, http.StatusNotFound, response.Code)
	require.Zero(t, fake.resolvedID)
}

func TestExceptionServiceRejectsNonTerminalResolution(t *testing.T) {
	svc := NewExceptionService(&mockExceptionRepo{})
	err := svc.ResolveException(context.Background(), 1, 2, "IN_REVIEW")
	require.ErrorIs(t, err, ErrInvalidExceptionResolution)
}
