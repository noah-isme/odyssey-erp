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
	// The return-by-id routes cannot load any return on this schema:
	// GetReturnByID's line query selects products.code, which does not exist
	// (products has sku), so show/confirm/cancel return fail for every
	// company before the company check is reached (pre-existing, reported
	// separately). For those three only the unchanged state is asserted
	// below; the service-level company check is covered by
	// TestReturnActionsHideOtherCompany.
	brokenReturnLoad := map[string]bool{"show return": true, "confirm return": true, "cancel return": true}
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
		if brokenReturnLoad[a.name] {
			assert.NotEqual(t, http.StatusOK, rec.Code, "%s %s (%s) from company A must not succeed", a.method, a.path, a.name)
			continue
		}
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
	movementsOwner, qtyOwner := stock()
	// confirm -2, cancel +2.
	require.Equal(t, movementsBefore+2, movementsOwner, "the owner's confirm and cancel post their stock movements")
	require.Equal(t, qtyBefore, qtyOwner)
}

// insertRC11DeliveredOrder inserts, for fixture f, a CONFIRMED sales order
// and a DELIVERED delivery order of 2 units with no returns, and returns the
// order and its line.
func insertRC11DeliveredOrder(t *testing.T, p *pgxpool.Pool, f rc11Fixture, tag string) (orderID, lineID int64) {
	t.Helper()
	ctx := context.Background()
	var soID, soLineID int64
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO sales_orders(doc_number,company_id,customer_id,order_date,status,currency,created_by)
		VALUES($1,$2,$3,CURRENT_DATE,'CONFIRMED','IDR',$4) RETURNING id`, "SO-"+tag+"-"+f.Suffix, f.CompanyID, f.CustomerID, f.AdminID).Scan(&soID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO sales_order_lines(sales_order_id,product_id,quantity,uom,unit_price,line_total,fulfillment_warehouse_id)
		VALUES($1,$2,2,'EA',100,200,$3) RETURNING id`, soID, f.ProductID, f.WarehouseID).Scan(&soLineID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO delivery_orders(doc_number,company_id,sales_order_id,warehouse_id,customer_id,delivery_date,status,created_by)
		VALUES($1,$2,$3,$4,$5,CURRENT_DATE,'DELIVERED',$6) RETURNING id`, "DO-"+tag+"-"+f.Suffix, f.CompanyID, soID, f.WarehouseID, f.CustomerID, f.AdminID).Scan(&orderID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO delivery_order_lines(delivery_order_id,sales_order_line_id,product_id,quantity_to_deliver,quantity_delivered,uom,unit_price)
		VALUES($1,$2,$3,2,2,'EA',100) RETURNING id`, orderID, soLineID, f.ProductID).Scan(&lineID))
	return orderID, lineID
}

// TestRC11DeliveryReturnRefusesOtherCompanyWarehouse: a return of company
// A's own delivered order may only restock into a warehouse of company A.
// Naming company B's warehouse (restock_warehouse_id[] is a form field) must
// be refused without creating the return, so confirming it can never post
// stock into company B; company A's own warehouse still works. Before the
// fix the return was stored with B's warehouse, and confirming it restocks
// that warehouse through inventory PostAdjustment, which checks no company
// (return confirm is currently unreachable because of FIND-039).
func TestRC11DeliveryReturnRefusesOtherCompanyWarehouse(t *testing.T) {
	p, fA := newRC11Fixture(t)
	_, fB := newRC11Fixture(t)
	ctx := context.Background()
	orderID, lineID := insertRC11DeliveredOrder(t, p, fA, "RETWH")
	inv := inventory.NewService(inventory.NewRepository(p), shared.NewAuditLogger(p), shared.NewIdempotencyStore(p), inventory.ServiceConfig{}, nil)
	client := rc11DeliveryClient(t, p, fA.AdminID, fA.CompanyID, inv)
	restockLines := func(warehouseID int64) int {
		t.Helper()
		var n int
		require.NoError(t, p.QueryRow(ctx, `SELECT COUNT(*) FROM return_delivery_order_lines l JOIN return_delivery_orders r ON r.id = l.return_delivery_order_id
			WHERE r.original_delivery_order_id = $1 AND l.restock_warehouse_id = $2`, orderID, warehouseID).Scan(&n))
		return n
	}
	// generate_return_delivery_order_number numbers per company and month
	// while return_delivery_orders.number is globally unique (as for
	// delivery numbers, FIND-029), so each run uses its own far-future month.
	n := time.Now().UnixNano()
	returnDate := time.Date(2100+int(n%800), time.Month(1+(n/800)%12), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	post := func(warehouseID int64) {
		t.Helper()
		client(http.MethodPost, "/delivery/orders/"+strconv.FormatInt(orderID, 10)+"/returns", url.Values{
			"return_date": {returnDate}, "reason": {"damaged"},
			"delivery_order_line_id[]": {strconv.FormatInt(lineID, 10)}, "product_id[]": {strconv.FormatInt(fA.ProductID, 10)},
			"quantity[]": {"1"}, "unit_price[]": {"100"}, "restock_warehouse_id[]": {strconv.FormatInt(warehouseID, 10)},
		})
	}

	post(fB.WarehouseID)
	require.Zero(t, restockLines(fB.WarehouseID), "a return of company A must not be stored with company B's restock warehouse")

	post(fA.WarehouseID)
	require.Equal(t, 1, restockLines(fA.WarehouseID), "company A's own restock warehouse must be accepted")
}

// rc11FarFutureDate is a delivery or return date in its own far-future month:
// delivery and return numbers are per company and month but globally unique
// (FIND-029), so each run uses a fresh month.
func rc11FarFutureDate() string {
	n := time.Now().UnixNano()
	return time.Date(2100+int(n%800), time.Month(1+(n/800)%12), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
}

// TestRC11DeliveryRefusesOtherCompanyWarehouse: company A may not ship from
// or restock into company B's warehouse through the delivery routes.
// (a) A delivery order of A's sales order naming B's warehouse (the sales
// order line names it too, as legacy or unchecked sales data can) is refused:
// no delivery row is stored and B's stock does not move. (b) A return of A's
// delivered order whose own warehouse is B's (planted legacy data) with no
// restock warehouse falls back to that warehouse and is refused on create.
// Confirming such a return cannot run through the routes because of
// FIND-039; the confirm-side check is covered by
// TestReturnRefusesFallbackWarehouseOfAnotherCompany.
func TestRC11DeliveryRefusesOtherCompanyWarehouse(t *testing.T) {
	p, fA := newRC11Fixture(t)
	_, fB := newRC11Fixture(t)
	ctx := context.Background()
	_, err := p.Exec(ctx, `INSERT INTO inventory_balances(warehouse_id,product_id,qty,avg_cost) VALUES($1,$2,10,100)`, fB.WarehouseID, fA.ProductID)
	require.NoError(t, err)
	inv := inventory.NewService(inventory.NewRepository(p), shared.NewAuditLogger(p), shared.NewIdempotencyStore(p), inventory.ServiceConfig{}, nil)
	client := rc11DeliveryClient(t, p, fA.AdminID, fA.CompanyID, inv)
	count := func(sql string, args ...any) int {
		t.Helper()
		var n int
		require.NoError(t, p.QueryRow(ctx, sql, args...).Scan(&n), sql)
		return n
	}
	movementsInB := func() int {
		return count(`SELECT COUNT(*) FROM inventory_tx WHERE warehouse_id = $1`, fB.WarehouseID)
	}
	movementsBefore := movementsInB()

	// (a) delivery order from B's warehouse.
	var soID, soLineID int64
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO sales_orders(doc_number,company_id,customer_id,order_date,status,currency,created_by)
		VALUES($1,$2,$3,CURRENT_DATE,'CONFIRMED','IDR',$4) RETURNING id`, "SO-FWH-"+fA.Suffix, fA.CompanyID, fA.CustomerID, fA.AdminID).Scan(&soID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO sales_order_lines(sales_order_id,product_id,quantity,uom,unit_price,line_total,fulfillment_warehouse_id)
		VALUES($1,$2,1,'EA',100,100,$3) RETURNING id`, soID, fA.ProductID, fB.WarehouseID).Scan(&soLineID))
	client(http.MethodPost, "/delivery/orders", url.Values{
		"sales_order_id": {strconv.FormatInt(soID, 10)}, "warehouse_id": {strconv.FormatInt(fB.WarehouseID, 10)},
		"delivery_date": {rc11FarFutureDate()}, "so_line_id[]": {strconv.FormatInt(soLineID, 10)},
		"product_id[]": {strconv.FormatInt(fA.ProductID, 10)}, "quantity[]": {"1"},
	})
	if created := count(`SELECT COUNT(*) FROM delivery_orders WHERE sales_order_id = $1`, soID); created != 0 {
		// Show what the stored order allows: confirming it moves B's stock.
		var deliveryID int64
		require.NoError(t, p.QueryRow(ctx, `SELECT id FROM delivery_orders WHERE sales_order_id = $1`, soID).Scan(&deliveryID))
		client(http.MethodPost, "/delivery/orders/"+strconv.FormatInt(deliveryID, 10)+"/confirm", url.Values{})
		assert.Failf(t, "delivery order stored with another company's warehouse", "company A created delivery order %d from company B's warehouse; after its confirm B has %d new stock movement(s)", deliveryID, movementsInB()-movementsBefore)
	}

	// (b) return falling back to the delivery's warehouse, which is B's.
	var deliveryID, lineID int64
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO delivery_orders(doc_number,company_id,sales_order_id,warehouse_id,customer_id,delivery_date,status,created_by)
		VALUES($1,$2,$3,$4,$5,CURRENT_DATE,'DELIVERED',$6) RETURNING id`, "DO-FWH-"+fA.Suffix, fA.CompanyID, soID, fB.WarehouseID, fA.CustomerID, fA.AdminID).Scan(&deliveryID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO delivery_order_lines(delivery_order_id,sales_order_line_id,product_id,quantity_to_deliver,quantity_delivered,uom,unit_price)
		VALUES($1,$2,$3,1,1,'EA',100) RETURNING id`, deliveryID, soLineID, fA.ProductID).Scan(&lineID))
	client(http.MethodPost, "/delivery/orders/"+strconv.FormatInt(deliveryID, 10)+"/returns", url.Values{
		"return_date": {rc11FarFutureDate()}, "reason": {"damaged"},
		"delivery_order_line_id[]": {strconv.FormatInt(lineID, 10)}, "product_id[]": {strconv.FormatInt(fA.ProductID, 10)},
		"quantity[]": {"1"}, "unit_price[]": {"100"}, "restock_warehouse_id[]": {"0"},
	})
	assert.Zero(t, count(`SELECT COUNT(*) FROM return_delivery_orders WHERE original_delivery_order_id = $1`, deliveryID),
		"a return that falls back to company B's warehouse must not be stored")
	assert.Equal(t, movementsBefore, movementsInB(), "company A's requests must not move company B's stock")
}

// TestRC11DeliveryConfirmCancelRefuseOtherCompanyWarehouse: a delivery order
// of company A whose stored warehouse is company B's (legacy data, planted
// directly) must not be confirmed or cancelled by company A: both answer
// `warehouse not found` (as Create does), leave the status unchanged and post
// no stock movement in B. Company A's orders from its own warehouse still
// confirm and cancel.
func TestRC11DeliveryConfirmCancelRefuseOtherCompanyWarehouse(t *testing.T) {
	p, fA := newRC11Fixture(t)
	_, fB := newRC11Fixture(t)
	ctx := context.Background()
	for _, wh := range []int64{fA.WarehouseID, fB.WarehouseID} {
		_, err := p.Exec(ctx, `INSERT INTO inventory_balances(warehouse_id,product_id,qty,avg_cost) VALUES($1,$2,10,100)`, wh, fA.ProductID)
		require.NoError(t, err)
	}
	inv := inventory.NewService(inventory.NewRepository(p), shared.NewAuditLogger(p), shared.NewIdempotencyStore(p), inventory.ServiceConfig{}, nil)
	client := rc11DeliveryClient(t, p, fA.AdminID, fA.CompanyID, inv)
	count := func(sql string, args ...any) int {
		t.Helper()
		var n int
		require.NoError(t, p.QueryRow(ctx, sql, args...).Scan(&n), sql)
		return n
	}
	status := func(id int64) string {
		t.Helper()
		var s string
		require.NoError(t, p.QueryRow(ctx, `SELECT status::text FROM delivery_orders WHERE id = $1`, id).Scan(&s))
		return s
	}
	var soID, soLineID int64
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO sales_orders(doc_number,company_id,customer_id,order_date,status,currency,created_by)
		VALUES($1,$2,$3,CURRENT_DATE,'CONFIRMED','IDR',$4) RETURNING id`, "SO-CCWH-"+fA.Suffix, fA.CompanyID, fA.CustomerID, fA.AdminID).Scan(&soID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO sales_order_lines(sales_order_id,product_id,quantity,uom,unit_price,line_total,fulfillment_warehouse_id)
		VALUES($1,$2,10,'EA',100,1000,$3) RETURNING id`, soID, fA.ProductID, fA.WarehouseID).Scan(&soLineID))
	plant := func(tag, st string, warehouseID int64) int64 {
		t.Helper()
		var id int64
		require.NoError(t, p.QueryRow(ctx, `INSERT INTO delivery_orders(doc_number,company_id,sales_order_id,warehouse_id,customer_id,delivery_date,status,created_by)
			VALUES($1,$2,$3,$4,$5,CURRENT_DATE,$6::delivery_order_status,$7) RETURNING id`, "DO-CCWH-"+tag+"-"+fA.Suffix, fA.CompanyID, soID, warehouseID, fA.CustomerID, st, fA.AdminID).Scan(&id))
		_, err := p.Exec(ctx, `INSERT INTO delivery_order_lines(delivery_order_id,sales_order_line_id,product_id,quantity_to_deliver,quantity_delivered,uom,unit_price)
			VALUES($1,$2,$3,1,0,'EA',100)`, id, soLineID, fA.ProductID)
		require.NoError(t, err)
		return id
	}
	path := func(id int64, action string) string {
		return "/delivery/orders/" + strconv.FormatInt(id, 10) + "/" + action
	}
	movements := func(wh int64) int { return count(`SELECT COUNT(*) FROM inventory_tx WHERE warehouse_id = $1`, wh) }
	inB := movements(fB.WarehouseID)

	foreignDraft := plant("BDRAFT", "DRAFT", fB.WarehouseID)
	foreignConfirmed := plant("BCONF", "CONFIRMED", fB.WarehouseID)
	client(http.MethodPost, path(foreignDraft, "confirm"), url.Values{})
	client(http.MethodPost, path(foreignConfirmed, "cancel"), url.Values{"cancellation_reason": {"legacy"}})
	assert.Equal(t, "DRAFT", status(foreignDraft), "confirm of an order stored with company B's warehouse must be refused")
	assert.Equal(t, "CONFIRMED", status(foreignConfirmed), "cancel of an order stored with company B's warehouse must be refused")
	assert.Equal(t, inB, movements(fB.WarehouseID), "company A's confirm and cancel must not move company B's stock")

	// Company A's own warehouse still works.
	inA := movements(fA.WarehouseID)
	ownDraft := plant("ADRAFT", "DRAFT", fA.WarehouseID)
	client(http.MethodPost, path(ownDraft, "confirm"), url.Values{})
	assert.Equal(t, "CONFIRMED", status(ownDraft))
	client(http.MethodPost, path(ownDraft, "cancel"), url.Values{"cancellation_reason": {"own"}})
	assert.Equal(t, "CANCELLED", status(ownDraft))
	assert.Equal(t, inA+2, movements(fA.WarehouseID), "the owner's confirm and cancel post their stock movements")
}
