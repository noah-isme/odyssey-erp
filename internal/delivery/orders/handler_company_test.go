package orders

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

func companyTestRequest(t *testing.T, method, target string, body io.Reader, user, company string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, target, body)
	sessions := shared.NewSessionManager(nil, "s", "secret", time.Hour, false)
	sess, err := sessions.Load(context.Background(), req)
	require.NoError(t, err)
	if user != "" {
		sess.SetUser(user)
	}
	if company != "" {
		sess.Set("company_id", company)
	}
	return req.WithContext(shared.ContextWithSession(req.Context(), sess))
}

func TestGetCompanyIDFailsClosed(t *testing.T) {
	require.Zero(t, getCompanyID(httptest.NewRequest(http.MethodGet, "/", nil)), "no session")
	for company, want := range map[string]int64{"": 0, "0": 0, "-3": 0, "abc": 0, "5": 5} {
		req := companyTestRequest(t, http.MethodGet, "/", nil, "7", company)
		require.Equal(t, want, getCompanyID(req), "company_id %q", company)
	}
}

// createCountingRepo serves a CONFIRMED sales order of company 1 (the old
// fallback company) and counts delivery orders created.
type createCountingRepo struct {
	fulfillmentWarehouseRepoFake
	created int
}

func (r *createCountingRepo) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	return fn(ctx, &createCountingTx{returnTxFake: returnTxFake{repo: &r.returnRepoFake}, repo: r})
}

type createCountingTx struct {
	returnTxFake
	repo *createCountingRepo
}

func (t *createCountingTx) CreateDeliveryOrder(context.Context, DeliveryOrder) (int64, error) {
	t.repo.created++
	return 99, nil
}

// TestCreateWithoutCompanyCreatesNothing posts a valid delivery form from a
// session with no active company: no delivery order is created, not even
// for a sales order of company 1.
func TestCreateWithoutCompanyCreatesNothing(t *testing.T) {
	warehouseID := int64(12)
	repo := &createCountingRepo{fulfillmentWarehouseRepoFake: fulfillmentWarehouseRepoFake{
		returnRepoFake: returnRepoFake{delivery: &DeliveryOrder{ID: 99, CompanyID: 1}},
		salesOrder:     &SalesOrderInfo{ID: 1, CompanyID: 1, CustomerID: 7, Status: "CONFIRMED"},
		lines:          []DeliverableSOLine{{SalesOrderLineID: 1, ProductID: 3, Quantity: 2, RemainingQuantity: 2, FulfillmentWarehouseID: &warehouseID}},
	}}
	templates, err := view.NewEngine()
	require.NoError(t, err)
	h := NewHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), NewService(repo), templates, shared.NewCSRFManager("secret"), rbac.Middleware{})

	form := url.Values{
		"sales_order_id": {"1"}, "warehouse_id": {"12"}, "delivery_date": {"2100-01-01"},
		"so_line_id[]": {"1"}, "product_id[]": {"3"}, "quantity[]": {"1"},
	}
	req := companyTestRequest(t, http.MethodPost, "/delivery/orders", strings.NewReader(form.Encode()), "7", "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.create(rec, req)

	require.NotEqual(t, http.StatusSeeOther, rec.Code, "no redirect to a created order")
	require.Zero(t, repo.created, "a request without an active company must not create a delivery order")
}
