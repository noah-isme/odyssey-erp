//go:build integration

package e2e

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/delivery"
	"github.com/odyssey-erp/odyssey-erp/internal/inventory"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// rc11AllDeliveryPerms grants every delivery order and return permission to
// one user id.
type rc11AllDeliveryPerms struct{ userID int64 }

func (p rc11AllDeliveryPerms) EffectivePermissions(_ context.Context, userID int64) ([]string, error) {
	if userID != p.userID {
		return nil, nil
	}
	return []string{
		shared.PermDeliveryOrderView, shared.PermDeliveryOrderCreate, shared.PermDeliveryOrderEdit,
		shared.PermDeliveryOrderConfirm, shared.PermDeliveryOrderShip, shared.PermDeliveryOrderComplete,
		shared.PermDeliveryOrderCancel, shared.PermDeliveryReturnView, shared.PermDeliveryReturnCreate,
		shared.PermDeliveryReturnPost, shared.PermDeliveryReturnVoid,
	}, nil
}

// rc11DeliveryClient mounts the delivery routes (with inv as the stock
// service) behind a session for userID with companyID active and returns a
// request function.
func rc11DeliveryClient(t *testing.T, p *pgxpool.Pool, userID, companyID int64, inv *inventory.Service) func(method, path string, values url.Values) *httptest.ResponseRecorder {
	t.Helper()
	templates, err := view.NewEngine()
	require.NoError(t, err)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sessions := shared.NewSessionManager(nil, "odyssey_session", "rc11-secret", time.Hour, false)
	sess, err := sessions.Load(context.Background(), httptest.NewRequest(http.MethodGet, "/", nil))
	require.NoError(t, err)
	sess.SetUser(strconv.FormatInt(userID, 10))
	sess.Set("company_id", strconv.FormatInt(companyID, 10))

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := shared.ContextWithSession(req.Context(), sess)
			// At tag v0.10.0-rc.10 the delivery handler read the actor and
			// company from these untyped context keys instead of the session;
			// setting both lets the same requests run against the tag. The
			// current handler ignores them.
			ctx = context.WithValue(ctx, "user_id", userID)       //nolint:staticcheck // the tag's key type
			ctx = context.WithValue(ctx, "company_id", companyID) //nolint:staticcheck // the tag's key type
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	r.Route("/delivery", func(r chi.Router) {
		delivery.MountRoutes(r, p, logger, templates, shared.NewCSRFManager("rc11-secret"),
			rbac.Middleware{Service: rc11AllDeliveryPerms{userID: userID}, Logger: logger}, inv)
	})

	return func(method, path string, values url.Values) *httptest.ResponseRecorder {
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
}

// rc11ScopedDeliveries holds company B's delivery orders in each state the
// by-id routes act on.
type rc11ScopedDeliveries struct {
	draft, confirmed, inTransit, delivered int64
	returnDraft, returnConfirmed           int64
}

// seedRC11ScopedDeliveries inserts, for fixture f, a CONFIRMED sales order
// and one delivery order per status plus a DRAFT and a CONFIRMED return of
// the delivered order, with 10 units on hand in the fixture warehouse.
func seedRC11ScopedDeliveries(t *testing.T, p *pgxpool.Pool, f rc11Fixture) rc11ScopedDeliveries {
	t.Helper()
	ctx := context.Background()
	scan := func(sql string, args ...any) int64 {
		t.Helper()
		var id int64
		require.NoError(t, p.QueryRow(ctx, sql, args...).Scan(&id), sql)
		return id
	}
	_, err := p.Exec(ctx, `INSERT INTO inventory_balances(warehouse_id,product_id,qty,avg_cost) VALUES($1,$2,10,100)`, f.WarehouseID, f.ProductID)
	require.NoError(t, err)

	soID := scan(`INSERT INTO sales_orders(doc_number,company_id,customer_id,order_date,status,currency,created_by)
		VALUES($1,$2,$3,CURRENT_DATE,'CONFIRMED','IDR',$4) RETURNING id`, "SO-SCOPE-"+f.Suffix, f.CompanyID, f.CustomerID, f.AdminID)
	soLineID := scan(`INSERT INTO sales_order_lines(sales_order_id,product_id,quantity,uom,unit_price,line_total,fulfillment_warehouse_id)
		VALUES($1,$2,10,'EA',100,1000,$3) RETURNING id`, soID, f.ProductID, f.WarehouseID)

	order := func(status string, delivered float64) (int64, int64) {
		t.Helper()
		id := scan(`INSERT INTO delivery_orders(doc_number,company_id,sales_order_id,warehouse_id,customer_id,delivery_date,status,driver_name,created_by)
			VALUES($1,$2,$3,$4,$5,CURRENT_DATE,$6::delivery_order_status,'owner driver',$7) RETURNING id`,
			"DO-SCOPE-"+status+"-"+f.Suffix, f.CompanyID, soID, f.WarehouseID, f.CustomerID, status, f.AdminID)
		lineID := scan(`INSERT INTO delivery_order_lines(delivery_order_id,sales_order_line_id,product_id,quantity_to_deliver,quantity_delivered,uom,unit_price)
			VALUES($1,$2,$3,2,$4,'EA',100) RETURNING id`, id, soLineID, f.ProductID, delivered)
		return id, lineID
	}
	var d rc11ScopedDeliveries
	d.draft, _ = order("DRAFT", 0)
	d.confirmed, _ = order("CONFIRMED", 0)
	d.inTransit, _ = order("IN_TRANSIT", 0)
	var deliveredLine int64
	d.delivered, deliveredLine = order("DELIVERED", 2)

	ret := func(status string) int64 {
		t.Helper()
		id := scan(`INSERT INTO return_delivery_orders(number,company_id,customer_id,original_delivery_order_id,warehouse_id,return_date,status,reason,created_by)
			VALUES($1,$2,$3,$4,$5,CURRENT_DATE,$6::return_delivery_order_status,'damaged',$7) RETURNING id`,
			"RDO-SCOPE-"+status+"-"+f.Suffix, f.CompanyID, f.CustomerID, d.delivered, f.WarehouseID, status, f.AdminID)
		_, err := p.Exec(ctx, `INSERT INTO return_delivery_order_lines(return_delivery_order_id,delivery_order_line_id,product_id,quantity_returned,unit_price,restock_warehouse_id)
			VALUES($1,$2,$3,1,100,$4)`, id, deliveredLine, f.ProductID, f.WarehouseID)
		require.NoError(t, err)
		return id
	}
	d.returnDraft = ret("DRAFT")
	d.returnConfirmed = ret("CONFIRMED")
	return d
}

// TestRC11DeliveryRefusesOtherCompanyOrders (FIND-036): a user holding every
// delivery permission in company A requests company B's delivery orders and
// returns by id. Every show, edit form, update, confirm, ship, complete,
// cancel and return route must answer 404 and leave B's orders, returns and
// stock untouched; B's own user can then perform the same actions. At tag
// v0.10.0-rc.10 the orders were loaded by id alone, so A could view B's
// orders and confirm, ship, complete or cancel them, moving B's stock.
func TestRC11DeliveryRefusesOtherCompanyOrders(t *testing.T) {
	p, fA := newRC11Fixture(t)
	_, fB := newRC11Fixture(t)
	ctx := context.Background()
	b := seedRC11ScopedDeliveries(t, p, fB)
	inv := inventory.NewService(inventory.NewRepository(p), shared.NewAuditLogger(p), shared.NewIdempotencyStore(p), inventory.ServiceConfig{}, nil)

	status := func(table string, id int64) string {
		t.Helper()
		var s string
		require.NoError(t, p.QueryRow(ctx, `SELECT status::text FROM `+table+` WHERE id=$1`, id).Scan(&s))
		return s
	}
	stock := func() (movements int, qty float64) {
		t.Helper()
		require.NoError(t, p.QueryRow(ctx, `SELECT COUNT(*) FROM inventory_tx WHERE warehouse_id=$1`, fB.WarehouseID).Scan(&movements))
		require.NoError(t, p.QueryRow(ctx, `SELECT qty::float8 FROM inventory_balances WHERE warehouse_id=$1 AND product_id=$2`, fB.WarehouseID, fB.ProductID).Scan(&qty))
		return movements, qty
	}
	returnsOf := func(deliveryID int64) int {
		t.Helper()
		var n int
		require.NoError(t, p.QueryRow(ctx, `SELECT COUNT(*) FROM return_delivery_orders WHERE original_delivery_order_id=$1`, deliveryID).Scan(&n))
		return n
	}
	order := func(id int64) string { return "/delivery/orders/" + strconv.FormatInt(id, 10) }
	ret := func(id int64) string { return "/delivery/orders/returns/" + strconv.FormatInt(id, 10) }

	movementsBefore, qtyBefore := stock()
	returnsBefore := returnsOf(b.delivered)

	attacker := rc11DeliveryClient(t, p, fA.AdminID, fA.CompanyID, inv)
	attempts := []struct {
		name, method, path string
		form               url.Values
	}{
		{"show", http.MethodGet, order(b.draft), nil},
		{"edit form", http.MethodGet, order(b.draft) + "/edit", nil},
		{"update", http.MethodPost, order(b.draft) + "/edit", url.Values{"driver_name": {"attacker"}, "notes": {"attacker"}}},
		{"confirm", http.MethodPost, order(b.draft) + "/confirm", url.Values{}},
		{"ship", http.MethodPost, order(b.confirmed) + "/ship", url.Values{"tracking_number": {"attacker"}}},
		{"complete", http.MethodPost, order(b.inTransit) + "/complete", url.Values{}},
		{"cancel", http.MethodPost, order(b.draft) + "/cancel", url.Values{"cancellation_reason": {"attacker"}}},
		{"return form", http.MethodGet, order(b.delivered) + "/returns/new", nil},
		{"create return", http.MethodPost, order(b.delivered) + "/returns", url.Values{
			"return_date": {time.Now().Format("2006-01-02")}, "reason": {"attacker"},
			"delivery_order_line_id[]": {"0"}, "product_id[]": {strconv.FormatInt(fB.ProductID, 10)}, "quantity[]": {"1"},
		}},
		{"show return", http.MethodGet, ret(b.returnDraft), nil},
		{"confirm return", http.MethodPost, ret(b.returnDraft) + "/confirm", url.Values{}},
		{"cancel return", http.MethodPost, ret(b.returnConfirmed) + "/cancel", url.Values{"reason": {"attacker"}}},
	}
	for _, a := range attempts {
		rec := attacker(a.method, a.path, a.form)
		assert.Equal(t, http.StatusNotFound, rec.Code, "%s %s (%s) from company A must answer 404", a.method, a.path, a.name)
	}

	assert.Equal(t, "DRAFT", status("delivery_orders", b.draft))
	assert.Equal(t, "CONFIRMED", status("delivery_orders", b.confirmed))
	assert.Equal(t, "IN_TRANSIT", status("delivery_orders", b.inTransit))
	assert.Equal(t, "DELIVERED", status("delivery_orders", b.delivered))
	assert.Equal(t, "DRAFT", status("return_delivery_orders", b.returnDraft))
	assert.Equal(t, "CONFIRMED", status("return_delivery_orders", b.returnConfirmed))
	var driver, notes *string
	var tracking *string
	require.NoError(t, p.QueryRow(ctx, `SELECT driver_name, notes FROM delivery_orders WHERE id=$1`, b.draft).Scan(&driver, &notes))
	assert.Equal(t, "owner driver", *driver, "update from company A must not change the order")
	assert.Nil(t, notes, "update or cancel from company A must not change the notes")
	require.NoError(t, p.QueryRow(ctx, `SELECT tracking_number FROM delivery_orders WHERE id=$1`, b.confirmed).Scan(&tracking))
	assert.Nil(t, tracking)
	assert.Equal(t, returnsBefore, returnsOf(b.delivered), "company A must not create a return of company B's delivery")
	movementsAfter, qtyAfter := stock()
	assert.Equal(t, movementsBefore, movementsAfter, "company A's requests must not post stock movements in company B")
	assert.Equal(t, qtyBefore, qtyAfter, "company A's requests must not change company B's stock")

	if t.Failed() {
		return // company A changed company B's data; the owner checks would not be meaningful
	}

	// The owning company still works.
	owner := rc11DeliveryClient(t, p, fB.AdminID, fB.CompanyID, inv)
	require.Equal(t, http.StatusOK, owner(http.MethodGet, order(b.draft), nil).Code)
	require.Equal(t, http.StatusOK, owner(http.MethodGet, order(b.draft)+"/edit", nil).Code)
	require.Equal(t, http.StatusSeeOther, owner(http.MethodPost, order(b.draft)+"/edit", url.Values{"driver_name": {"owner edit"}}).Code)
	require.NoError(t, p.QueryRow(ctx, `SELECT driver_name FROM delivery_orders WHERE id=$1`, b.draft).Scan(&driver))
	require.Equal(t, "owner edit", *driver)
	require.Equal(t, http.StatusSeeOther, owner(http.MethodPost, order(b.draft)+"/confirm", url.Values{}).Code)
	require.Equal(t, "CONFIRMED", status("delivery_orders", b.draft))
	require.Equal(t, http.StatusSeeOther, owner(http.MethodPost, order(b.confirmed)+"/ship", url.Values{}).Code)
	require.Equal(t, "IN_TRANSIT", status("delivery_orders", b.confirmed))
	require.Equal(t, http.StatusSeeOther, owner(http.MethodPost, order(b.inTransit)+"/complete", url.Values{}).Code)
	require.Equal(t, "DELIVERED", status("delivery_orders", b.inTransit))
	require.Equal(t, http.StatusSeeOther, owner(http.MethodPost, order(b.draft)+"/cancel", url.Values{"cancellation_reason": {"owner cancel"}}).Code)
	require.Equal(t, "CANCELLED", status("delivery_orders", b.draft))
	require.Equal(t, http.StatusOK, owner(http.MethodGet, order(b.delivered)+"/returns/new", nil).Code)
	require.Equal(t, http.StatusOK, owner(http.MethodGet, ret(b.returnDraft), nil).Code)
	require.Equal(t, http.StatusSeeOther, owner(http.MethodPost, ret(b.returnDraft)+"/confirm", url.Values{}).Code)
	require.Equal(t, "CONFIRMED", status("return_delivery_orders", b.returnDraft))
	require.Equal(t, http.StatusSeeOther, owner(http.MethodPost, ret(b.returnConfirmed)+"/cancel", url.Values{"reason": {"owner void"}}).Code)
	require.Equal(t, "CANCELLED", status("return_delivery_orders", b.returnConfirmed))
	movementsOwner, qtyOwner := stock()
	// confirm -2, cancel +2, return confirm +1, return cancel -1.
	require.Equal(t, movementsBefore+4, movementsOwner, "the owner's confirm, cancel and return actions post their stock movements")
	require.Equal(t, qtyBefore, qtyOwner)
}
