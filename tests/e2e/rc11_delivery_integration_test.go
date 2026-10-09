//go:build integration

package e2e

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/delivery"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// rc11DeliveryPerms grants the delivery order permissions to one user id.
type rc11DeliveryPerms struct{ userID int64 }

func (p rc11DeliveryPerms) EffectivePermissions(_ context.Context, userID int64) ([]string, error) {
	if userID != p.userID {
		return nil, nil
	}
	return []string{
		shared.PermDeliveryOrderView, shared.PermDeliveryOrderCreate, shared.PermDeliveryOrderConfirm,
	}, nil
}

var rc11DeliveryCSRF = regexp.MustCompile(`name="csrf_token"[^>]*value="([^"]+)"`)

// TestRC11DeliveryOrderUsesSessionIdentity replays the delivery create and
// confirm requests of J-SALES-001 (tests/staging/certification_test.go)
// against the delivery routes in process, logged in as the fixture admin with
// the fixture company active. The delivery order must be created in the
// active company and record the session user as creator and confirmer. At tag
// v0.10.0-rc.10 the handler read the user and company from context keys that
// nothing sets, so created_by was 0 (foreign key violation, the form
// re-rendered with 200) and the company fell back to 1.
func TestRC11DeliveryOrderUsesSessionIdentity(t *testing.T) {
	p, f := newRC11Fixture(t)
	ctx := context.Background()

	var soID, soLineID int64
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO sales_orders(doc_number,company_id,customer_id,order_date,status,currency,created_by)
		VALUES($1,$2,$3,CURRENT_DATE,'CONFIRMED','IDR',$4) RETURNING id`, "SO-RC11-"+f.Suffix, f.CompanyID, f.CustomerID, f.AdminID).Scan(&soID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO sales_order_lines(sales_order_id,product_id,quantity,uom,unit_price,line_total,fulfillment_warehouse_id)
		VALUES($1,$2,1,'EA',100,100,$3) RETURNING id`, soID, f.ProductID, f.WarehouseID).Scan(&soLineID))

	templates, err := view.NewEngine()
	require.NoError(t, err)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	csrf := shared.NewCSRFManager("rc11-secret")
	sessions := shared.NewSessionManager(nil, "odyssey_session", "rc11-secret", time.Hour, false)
	sess, err := sessions.Load(ctx, httptest.NewRequest(http.MethodGet, "/", nil))
	require.NoError(t, err)
	sess.SetUser(strconv.FormatInt(f.AdminID, 10))
	sess.Set("company_id", strconv.FormatInt(f.CompanyID, 10))

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(shared.ContextWithSession(req.Context(), sess)))
		})
	})
	r.Route("/delivery", func(r chi.Router) {
		delivery.MountRoutes(r, p, logger, templates, csrf,
			rbac.Middleware{Service: rc11DeliveryPerms{userID: f.AdminID}, Logger: logger}, nil)
	})

	do := func(method, path string, values url.Values) *httptest.ResponseRecorder {
		t.Helper()
		var body io.Reader
		if values != nil {
			body = strings.NewReader(values.Encode())
		}
		req := httptest.NewRequest(method, path, body)
		if values != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	csrfFrom := func(path string) string {
		t.Helper()
		rec := do(http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, rec.Code, "GET %s", path)
		m := rc11DeliveryCSRF.FindStringSubmatch(rec.Body.String())
		require.NotNil(t, m, "csrf_token not found on %s", path)
		return m[1]
	}

	date := time.Now().UTC().Format("2006-01-02")
	create := do(http.MethodPost, "/delivery/orders", url.Values{
		"csrf_token":     {csrfFrom("/delivery/orders/new?sales_order_id=" + strconv.FormatInt(soID, 10))},
		"sales_order_id": {strconv.FormatInt(soID, 10)},
		"warehouse_id":   {strconv.FormatInt(f.WarehouseID, 10)},
		"delivery_date":  {date},
		"so_line_id[]":   {strconv.FormatInt(soLineID, 10)},
		"product_id[]":   {strconv.FormatInt(f.ProductID, 10)},
		"quantity[]":     {"1"},
		"line_notes[]":   {"rc11 delivery"},
	})
	require.Equal(t, http.StatusSeeOther, create.Code, "POST /delivery/orders must redirect; the form re-rendered instead")

	var deliveryID, companyID, createdBy int64
	require.NoError(t, p.QueryRow(ctx, `SELECT id, company_id, created_by FROM delivery_orders WHERE sales_order_id=$1`, soID).Scan(&deliveryID, &companyID, &createdBy))
	require.Equal(t, f.CompanyID, companyID, "delivery order must belong to the active company")
	require.Equal(t, f.AdminID, createdBy, "delivery order must record the session user as creator")

	detail := "/delivery/orders/" + strconv.FormatInt(deliveryID, 10)
	confirm := do(http.MethodPost, detail+"/confirm", url.Values{"csrf_token": {csrfFrom(detail)}})
	require.Equal(t, http.StatusSeeOther, confirm.Code)
	var status string
	var confirmedBy *int64
	require.NoError(t, p.QueryRow(ctx, `SELECT status::text, confirmed_by FROM delivery_orders WHERE id=$1`, deliveryID).Scan(&status, &confirmedBy))
	require.Equal(t, "CONFIRMED", status)
	require.NotNil(t, confirmedBy)
	require.Equal(t, f.AdminID, *confirmedBy, "delivery order must record the session user as confirmer")
}
