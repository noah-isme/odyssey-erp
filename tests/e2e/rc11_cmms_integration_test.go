//go:build integration

package e2e

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/cmms"
	cmmshttp "github.com/odyssey-erp/odyssey-erp/internal/cmms/http"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// The TestRC11CMMS* suites drive the CMMS HTTP routes in process with the
// request shapes of tests/staging/certification_test.go (J-CMMS-001): form
// posts carrying a csrf_token scraped from a rendered page, 303 redirects,
// database assertions. They compile against tag v0.10.0-rc.10 so the same
// file shows the failure there; the audit sink is attached by reflection
// because cmms.Service has no SetAuditLogger at the tag.

// rc11CMMSPerms grants every CMMS permission to one user id.
type rc11CMMSPerms struct{ userID int64 }

func (p rc11CMMSPerms) EffectivePermissions(_ context.Context, userID int64) ([]string, error) {
	if userID != p.userID {
		return nil, nil
	}
	return []string{
		shared.PermCMMSAssetView, shared.PermCMMSAssetManage, shared.PermCMMSRequestCreate,
		shared.PermCMMSRequestTriage, shared.PermCMMSPlanView, shared.PermCMMSPlanManage,
		shared.PermCMMSWorkOrderView, shared.PermCMMSWorkOrderRelease, shared.PermCMMSWorkOrderExecute,
		shared.PermCMMSWorkOrderClose, shared.PermCMMSCostView, shared.PermCMMSCostApprove, shared.PermCMMSAdmin,
	}, nil
}

type rc11CMMSHarness struct {
	t      *testing.T
	pool   *pgxpool.Pool
	f      rc11Fixture
	router http.Handler
	sess   *shared.Session
	svc    *cmms.Service
}

// newRC11CMMSHarness mounts the CMMS handler under /cmms behind the session
// and CSRF checks the app router applies, logged in as the fixture admin with
// the fixture company active, and attaches the shared audit logger.
func newRC11CMMSHarness(t *testing.T, p *pgxpool.Pool, f rc11Fixture) *rc11CMMSHarness {
	t.Helper()
	templates, err := view.NewEngine()
	require.NoError(t, err)
	logger := slog.New(slog.NewTextHandler(rc11LogWriter{t}, nil))
	csrf := shared.NewCSRFManager("rc11-secret")

	sessions := shared.NewSessionManager(nil, "odyssey_session", "rc11-secret", time.Hour, false)
	sess, err := sessions.Load(context.Background(), httptest.NewRequest(http.MethodGet, "/", nil))
	require.NoError(t, err)
	sess.SetUser(strconv.FormatInt(f.AdminID, 10))
	sess.Set("company_id", strconv.FormatInt(f.CompanyID, 10))

	svc := cmms.NewService(cmms.NewRepository(p))
	// cmd/odyssey and cmd/worker call SetAuditLogger(auditLogger).
	if setAudit := reflect.ValueOf(svc).MethodByName("SetAuditLogger"); setAudit.IsValid() {
		setAudit.Call([]reflect.Value{reflect.ValueOf(shared.NewAuditLogger(p))})
	} else {
		t.Errorf("cmms.Service has no SetAuditLogger: CMMS writes cannot be audited")
	}
	h := cmmshttp.NewHandler(logger, svc, templates, csrf, rbac.Middleware{Service: rc11CMMSPerms{userID: f.AdminID}, Logger: logger}, p)

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(shared.ContextWithSession(req.Context(), sess)))
		})
	})
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.Method == http.MethodGet || req.Method == http.MethodHead {
				next.ServeHTTP(w, req)
				return
			}
			if err := csrf.VerifyToken(req.Context(), shared.SessionFromContext(req.Context()), req.PostFormValue(shared.CSRFFormField)); err != nil {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, req)
		})
	})
	r.Route("/cmms", h.MountRoutes)

	return &rc11CMMSHarness{t: t, pool: p, f: f, router: r, sess: sess, svc: svc}
}

func (h *rc11CMMSHarness) do(req *http.Request) rc11Result {
	h.t.Helper()
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	res := rc11Result{status: rec.Code, location: rec.Header().Get("Location"), body: rec.Body.Bytes(), header: rec.Header()}
	if rec.Code == http.StatusSeeOther {
		res.flash = h.sess.PopFlash()
	}
	return res
}

func (h *rc11CMMSHarness) get(path string) rc11Result {
	return h.do(httptest.NewRequest(http.MethodGet, path, nil))
}

func (h *rc11CMMSHarness) postForm(path string, values url.Values) rc11Result {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return h.do(req)
}

// fetchCSRF GETs a page, requires 200 and scrapes its csrf_token field.
func (h *rc11CMMSHarness) fetchCSRF(path string) string {
	h.t.Helper()
	res := h.get(path)
	require.Equal(h.t, http.StatusOK, res.status, "GET %s", path)
	m := rc11CSRFPattern.FindSubmatch(res.body)
	require.NotNil(h.t, m, "csrf_token not found on %s", path)
	return string(m[1])
}

// post sends a form with a CSRF token scraped from csrfPage.
func (h *rc11CMMSHarness) post(path, csrfPage string, values url.Values) rc11Result {
	h.t.Helper()
	if values == nil {
		values = url.Values{}
	}
	values.Set("csrf_token", h.fetchCSRF(csrfPage))
	return h.postForm(path, values)
}

func (h *rc11CMMSHarness) count(sql string, args ...any) int {
	h.t.Helper()
	var n int
	require.NoError(h.t, h.pool.QueryRow(context.Background(), sql, args...).Scan(&n), sql)
	return n
}

func (h *rc11CMMSHarness) str(sql string, args ...any) string {
	h.t.Helper()
	var s string
	require.NoError(h.t, h.pool.QueryRow(context.Background(), sql, args...).Scan(&s), sql)
	return s
}

func (h *rc11CMMSHarness) workOrderStatus(id int64) string {
	return h.str(`SELECT status FROM work_orders WHERE id=$1`, id)
}

// createWorkOrder posts the strict test's work-order form and returns the id.
func (h *rc11CMMSHarness) createWorkOrder(title string) int64 {
	h.t.Helper()
	res := h.post("/cmms/work-orders", "/cmms/work-orders/new", url.Values{
		"title": {title}, "description": {"rc11 lifecycle"},
		"asset_id": {strconv.FormatInt(h.f.AssetID, 10)}, "priority": {"MEDIUM"}, "category": {"CORRECTIVE"},
	})
	require.Equal(h.t, http.StatusSeeOther, res.status)
	id, err := strconv.ParseInt(strings.TrimPrefix(res.location, "/cmms/work-orders/"), 10, 64)
	require.NoError(h.t, err, "create work order redirect %q flash %+v", res.location, res.flash)
	return id
}

// foreignCompany returns a second company (created on first use) with one
// asset and one user-less work order inserted directly.
func (h *rc11CMMSHarness) foreignCompany() (companyID, assetID int64) {
	h.t.Helper()
	ctx := context.Background()
	code := "RC11CO-" + h.f.Suffix
	if err := h.pool.QueryRow(ctx, `SELECT id FROM companies WHERE code=$1`, code).Scan(&companyID); err != nil {
		require.NoError(h.t, h.pool.QueryRow(ctx, `INSERT INTO companies(code,name,base_currency) VALUES($1,'RC11 CMMS other','IDR') RETURNING id`, code).Scan(&companyID))
		require.NoError(h.t, h.pool.QueryRow(ctx, `INSERT INTO assets(company_id,code,name) VALUES($1,$2,'RC11 foreign pump') RETURNING id`, companyID, "AST-O-"+h.f.Suffix).Scan(&assetID))
		return companyID, assetID
	}
	require.NoError(h.t, h.pool.QueryRow(ctx, `SELECT id FROM assets WHERE company_id=$1 ORDER BY id LIMIT 1`, companyID).Scan(&assetID))
	return companyID, assetID
}

func (h *rc11CMMSHarness) foreignWorkOrder(status string) int64 {
	h.t.Helper()
	companyID, assetID := h.foreignCompany()
	var id int64
	require.NoError(h.t, h.pool.QueryRow(context.Background(), `INSERT INTO work_orders(company_id,number,title,asset_id,status,requester_id,created_by)
		VALUES($1,'WO-FOREIGN-'||nextval('work_orders_id_seq'),'RC11 foreign',$2,$3,$4,$4) RETURNING id`, companyID, assetID, status, h.f.AdminID).Scan(&id))
	return id
}

func (h *rc11CMMSHarness) requireFlash(res rc11Result, kind string) {
	h.t.Helper()
	require.Equal(h.t, http.StatusSeeOther, res.status)
	require.NotNil(h.t, res.flash, "no flash on %s", res.location)
	require.Equal(h.t, kind, res.flash.Kind, "flash: %s", res.flash.Message)
}

func TestRC11CMMSWorkOrderLifecycle(t *testing.T) {
	p, f := newRC11Fixture(t)
	h := newRC11CMMSHarness(t, p, f)

	workID := h.createWorkOrder("RC11 lifecycle " + f.Suffix)
	workPath := fmt.Sprintf("/cmms/work-orders/%d", workID)
	require.Equal(t, "DRAFT", h.workOrderStatus(workID))

	started := h.post(workPath+"/status", workPath, url.Values{"status": {"IN_PROGRESS"}})
	h.requireFlash(started, "success")
	require.Equal(t, "IN_PROGRESS", h.workOrderStatus(workID))

	// The generic status action refuses the targets owned by complete/close.
	for _, target := range []string{"COMPLETED", "CLOSED"} {
		h.requireFlash(h.post(workPath+"/status", workPath, url.Values{"status": {target}}), "danger")
		require.Equal(t, "IN_PROGRESS", h.workOrderStatus(workID), "status %s", target)
	}
	// IN_PROGRESS cannot be closed.
	h.requireFlash(h.post(workPath+"/close", workPath, nil), "danger")
	require.Equal(t, "IN_PROGRESS", h.workOrderStatus(workID))

	complete := h.post(workPath+"/complete", workPath, url.Values{"actual_hours": {"1.5"}})
	h.requireFlash(complete, "success")
	require.Equal(t, workPath, complete.location)
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM work_orders WHERE id=$1 AND status='COMPLETED' AND actual_hours=1.5`, workID))

	// A repeated complete is a 303 success and changes nothing.
	retry := h.post(workPath+"/complete", workPath, url.Values{"actual_hours": {"9"}})
	h.requireFlash(retry, "success")
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM work_orders WHERE id=$1 AND status='COMPLETED' AND actual_hours=1.5`, workID))

	closed := h.post(workPath+"/close", workPath, nil)
	h.requireFlash(closed, "success")
	require.Equal(t, "CLOSED", h.workOrderStatus(workID))
	// CLOSED is terminal.
	h.requireFlash(h.post(workPath+"/status", workPath, url.Values{"status": {"IN_PROGRESS"}}), "danger")
	require.Equal(t, "CLOSED", h.workOrderStatus(workID))

	// One audit row per effective write; the no-op retry adds none.
	require.GreaterOrEqual(t, h.count(`SELECT COUNT(*) FROM audit_logs WHERE entity='cmms_work_order' AND entity_id=$1`, strconv.FormatInt(workID, 10)), 3)
	for action, want := range map[string]int{"create": 1, "status": 1, "complete": 1, "close": 1} {
		require.Equal(t, want, h.count(`SELECT COUNT(*) FROM audit_logs WHERE entity='cmms_work_order' AND entity_id=$1 AND action=$2 AND actor_id=$3`,
			strconv.FormatInt(workID, 10), action, f.AdminID), "audit action %s", action)
	}

	// DRAFT cannot be completed or closed directly.
	draftID := h.createWorkOrder("RC11 draft " + f.Suffix)
	draftPath := fmt.Sprintf("/cmms/work-orders/%d", draftID)
	h.requireFlash(h.post(draftPath+"/complete", draftPath, url.Values{"actual_hours": {"1"}}), "danger")
	h.requireFlash(h.post(draftPath+"/close", draftPath, nil), "danger")
	require.Equal(t, "DRAFT", h.workOrderStatus(draftID))

	// ON_HOLD can be completed.
	h.requireFlash(h.post(draftPath+"/status", draftPath, url.Values{"status": {"IN_PROGRESS"}}), "success")
	h.requireFlash(h.post(draftPath+"/status", draftPath, url.Values{"status": {"ON_HOLD"}}), "success")
	h.requireFlash(h.post(draftPath+"/complete", draftPath, nil), "success")
	require.Equal(t, "COMPLETED", h.workOrderStatus(draftID))

	// A work order of another company cannot be moved through any action.
	foreignID := h.foreignWorkOrder("IN_PROGRESS")
	foreignPath := fmt.Sprintf("/cmms/work-orders/%d", foreignID)
	h.requireFlash(h.post(foreignPath+"/status", workPath, url.Values{"status": {"ON_HOLD"}}), "danger")
	h.requireFlash(h.post(foreignPath+"/complete", workPath, url.Values{"actual_hours": {"2"}}), "danger")
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM work_orders WHERE id=$1 AND status='IN_PROGRESS' AND actual_hours=0`, foreignID))
	require.Zero(t, h.count(`SELECT COUNT(*) FROM audit_logs WHERE entity='cmms_work_order' AND entity_id=$1`, strconv.FormatInt(foreignID, 10)))
}

func TestRC11CMMSMeterReadings(t *testing.T) {
	p, f := newRC11Fixture(t)
	h := newRC11CMMSHarness(t, p, f)

	assetPath := fmt.Sprintf("/cmms/assets/%d", f.AssetID)
	readingsPath := assetPath + "/meter-readings"
	date := time.Now().UTC().Format("2006-01-02")

	// The strict test posts with the CSRF token of the asset page and later
	// GETs the readings page.
	res := h.post(readingsPath, assetPath, url.Values{
		"reading_type": {"HOURS"}, "value": {"10.5"}, "reading_date": {date},
		"notes": {"v0.10-core certification meter"},
	})
	h.requireFlash(res, "success")
	require.Equal(t, readingsPath, res.location)
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM meter_readings WHERE asset_id=$1 AND reading_type='HOURS' AND value=10.5
		AND reading_date=$2::date AND entered_by=$3 AND notes='v0.10-core certification meter'`, f.AssetID, date, f.AdminID))
	var readingID int64
	require.NoError(t, p.QueryRow(context.Background(), `SELECT id FROM meter_readings WHERE asset_id=$1 ORDER BY id DESC LIMIT 1`, f.AssetID).Scan(&readingID))
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM audit_logs WHERE entity='cmms_meter_reading' AND action='create' AND entity_id=$1 AND actor_id=$2`,
		strconv.FormatInt(readingID, 10), f.AdminID))

	// The readings page renders with a CSRF token and lists the reading.
	page := h.get(readingsPath)
	require.Equal(t, http.StatusOK, page.status)
	require.Regexp(t, rc11CSRFPattern, string(page.body))
	require.Contains(t, string(page.body), "v0.10-core certification meter")

	// RFC3339 dates are accepted; an unparseable value is refused.
	h.requireFlash(h.post(readingsPath, readingsPath, url.Values{"reading_type": {"CYCLES"}, "value": {"3"}, "reading_date": {time.Now().UTC().Format(time.RFC3339)}}), "success")
	h.requireFlash(h.post(readingsPath, readingsPath, url.Values{"reading_type": {"HOURS"}, "value": {"lots"}}), "danger")
	require.Equal(t, 2, h.count(`SELECT COUNT(*) FROM meter_readings WHERE asset_id=$1`, f.AssetID))

	// An asset of another company: no row, and its page is not found.
	_, foreignAssetID := h.foreignCompany()
	foreignPath := fmt.Sprintf("/cmms/assets/%d/meter-readings", foreignAssetID)
	h.requireFlash(h.post(foreignPath, readingsPath, url.Values{"reading_type": {"HOURS"}, "value": {"1"}, "reading_date": {date}}), "danger")
	require.Zero(t, h.count(`SELECT COUNT(*) FROM meter_readings WHERE asset_id=$1`, foreignAssetID))
	require.Equal(t, http.StatusNotFound, h.get(foreignPath).status)
}

// createSparePart posts the strict test's spare-part form and returns the id.
func (h *rc11CMMSHarness) createSparePart(code string) int64 {
	h.t.Helper()
	res := h.post("/cmms/spare-parts", "/cmms/spare-parts/new", url.Values{
		"code": {code}, "name": {"v0.10-core certification spare"},
		"category": {"MAINTENANCE"}, "unit_of_measure": {"EA"}, "unit_cost": {"12.50"},
		"min_quantity": {"0"}, "reorder_point": {"0"},
	})
	require.Equal(h.t, http.StatusSeeOther, res.status)
	id, err := strconv.ParseInt(strings.TrimPrefix(res.location, "/cmms/spare-parts/"), 10, 64)
	require.NoError(h.t, err, "create spare part redirect %q flash %+v", res.location, res.flash)
	return id
}

func TestRC11CMMSSparePartIssue(t *testing.T) {
	p, f := newRC11Fixture(t)
	h := newRC11CMMSHarness(t, p, f)
	ctx := context.Background()

	workID := h.createWorkOrder("RC11 spare parts " + f.Suffix)
	workPath := fmt.Sprintf("/cmms/work-orders/%d", workID)
	h.requireFlash(h.post(workPath+"/status", workPath, url.Values{"status": {"IN_PROGRESS"}}), "success")
	spareID := h.createSparePart("CERT-SPARE-" + f.Suffix)

	add := h.post(workPath+"/spare-parts", workPath, url.Values{
		"spare_part_id": {strconv.FormatInt(spareID, 10)}, "quantity": {"1"}, "unit_cost": {"12.50"},
	})
	h.requireFlash(add, "success")
	require.Equal(t, workPath, add.location)
	var usageID int64
	require.NoError(t, p.QueryRow(ctx, `SELECT id FROM work_order_spare_parts WHERE work_order_id=$1 AND spare_part_id=$2 ORDER BY id DESC LIMIT 1`, workID, spareID).Scan(&usageID))
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM work_order_spare_parts WHERE id=$1 AND quantity=1 AND unit_cost=12.5 AND total_cost=12.5 AND issued_at IS NULL`, usageID))
	usagePath := fmt.Sprintf("/cmms/work-order-spare-parts/%d/issue", usageID)

	issue := h.post(usagePath, workPath, nil)
	h.requireFlash(issue, "success")
	require.Equal(t, workPath, issue.location)
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM work_order_spare_parts WHERE id=$1 AND issued_at IS NOT NULL AND issued_by=$2`, usageID, f.AdminID))
	issuedAt := h.str(`SELECT issued_at::text FROM work_order_spare_parts WHERE id=$1`, usageID)

	// A second issue is an idempotent 303 that changes nothing.
	again := h.post(usagePath, workPath, nil)
	h.requireFlash(again, "info")
	require.Equal(t, workPath, again.location)
	require.Equal(t, issuedAt, h.str(`SELECT issued_at::text FROM work_order_spare_parts WHERE id=$1`, usageID))

	usageKey := strconv.FormatInt(usageID, 10)
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM audit_logs WHERE entity='cmms_work_order_spare_part' AND entity_id=$1 AND action='add' AND actor_id=$2`, usageKey, f.AdminID))
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM audit_logs WHERE entity='cmms_work_order_spare_part' AND entity_id=$1 AND action='issue' AND actor_id=$2`, usageKey, f.AdminID))
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM audit_logs WHERE entity='cmms_spare_part' AND entity_id=$1 AND action='create'`, strconv.FormatInt(spareID, 10)))

	// Invalid quantity is refused.
	h.requireFlash(h.post(workPath+"/spare-parts", workPath, url.Values{"spare_part_id": {strconv.FormatInt(spareID, 10)}, "quantity": {"0"}}), "danger")

	// Cross-company: a foreign spare part on our work order, our spare part
	// on a foreign work order, and issuing a foreign usage are all refused.
	foreignCompanyID, _ := h.foreignCompany()
	var foreignSpareID int64
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO spare_parts(company_id,code,name) VALUES($1,$2,'RC11 foreign spare') RETURNING id`, foreignCompanyID, "SP-O-"+f.Suffix).Scan(&foreignSpareID))
	h.requireFlash(h.post(workPath+"/spare-parts", workPath, url.Values{"spare_part_id": {strconv.FormatInt(foreignSpareID, 10)}, "quantity": {"1"}}), "danger")
	require.Zero(t, h.count(`SELECT COUNT(*) FROM work_order_spare_parts WHERE spare_part_id=$1`, foreignSpareID))

	foreignWorkID := h.foreignWorkOrder("IN_PROGRESS")
	h.requireFlash(h.post(fmt.Sprintf("/cmms/work-orders/%d/spare-parts", foreignWorkID), workPath, url.Values{"spare_part_id": {strconv.FormatInt(spareID, 10)}, "quantity": {"1"}}), "danger")
	require.Zero(t, h.count(`SELECT COUNT(*) FROM work_order_spare_parts WHERE work_order_id=$1`, foreignWorkID))

	var foreignUsageID int64
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO work_order_spare_parts(work_order_id,spare_part_id,quantity) VALUES($1,$2,1) RETURNING id`, foreignWorkID, foreignSpareID).Scan(&foreignUsageID))
	h.requireFlash(h.post(fmt.Sprintf("/cmms/work-order-spare-parts/%d/issue", foreignUsageID), workPath, nil), "danger")
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM work_order_spare_parts WHERE id=$1 AND issued_at IS NULL AND issued_by IS NULL`, foreignUsageID))

	// A closed work order takes no more spare parts.
	h.requireFlash(h.post(workPath+"/complete", workPath, url.Values{"actual_hours": {"1.5"}}), "success")
	h.requireFlash(h.post(workPath+"/close", workPath, nil), "success")
	h.requireFlash(h.post(workPath+"/spare-parts", workPath, url.Values{"spare_part_id": {strconv.FormatInt(spareID, 10)}, "quantity": {"1"}}), "danger")
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM work_order_spare_parts WHERE work_order_id=$1`, workID))

	// The strict test's audit query (certification_test.go L1097-1100) over
	// the asset, work order and spare usage.
	require.GreaterOrEqual(t, h.count(`SELECT COUNT(*) FROM audit_logs WHERE entity IN ('cmms_asset','cmms_pm_schedule','cmms_work_order','cmms_meter_reading','cmms_spare_part','cmms_work_order_spare_part') AND (entity_id=$1 OR entity_id=$2 OR entity_id=$3)`,
		strconv.FormatInt(f.AssetID, 10), strconv.FormatInt(workID, 10), usageKey), 4)
}

func TestRC11CMMSRunDue(t *testing.T) {
	p, f := newRC11Fixture(t)
	h := newRC11CMMSHarness(t, p, f)
	ctx := context.Background()
	today := time.Now().UTC().Format("2006-01-02")
	tomorrow := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	runDue := func() rc11Result {
		return h.post("/cmms/pm-schedules/run-due", "/cmms/pm-schedules", nil)
	}

	// A schedule of another company that is already due must stay untouched.
	foreignCompanyID, foreignAssetID := h.foreignCompany()
	var foreignSchedID int64
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO pm_schedules(company_id,asset_id,name,frequency_type,frequency_value,next_due_date,active)
		VALUES($1,$2,'RC11 foreign PM','DAILY',1,$3::date - 1,true) RETURNING id`, foreignCompanyID, foreignAssetID, today).Scan(&foreignSchedID))

	// The strict test's schedule: DAILY/1, active, no next_due_date.
	create := h.post("/cmms/pm-schedules", "/cmms/pm-schedules/new", url.Values{
		"asset_id": {strconv.FormatInt(f.AssetID, 10)}, "name": {"v0.10-core daily preventive maintenance"},
		"description": {"staging PM generation"}, "frequency_type": {"DAILY"}, "frequency_value": {"1"}, "active": {"true"},
	})
	require.Equal(t, http.StatusSeeOther, create.status)
	schedID, err := strconv.ParseInt(strings.TrimPrefix(create.location, "/cmms/pm-schedules/"), 10, 64)
	require.NoError(t, err, "create PM schedule redirect %q flash %+v", create.location, create.flash)
	require.Equal(t, today, h.str(`SELECT next_due_date::text FROM pm_schedules WHERE id=$1 AND company_id=$2`, schedID, f.CompanyID),
		"a calendar schedule without a first due date is due today (UTC)")

	first := runDue()
	h.requireFlash(first, "success")
	require.Equal(t, "/cmms/pm-schedules", first.location)
	require.Contains(t, first.flash.Message, "Generated 1 ")
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM work_orders WHERE company_id=$1 AND pm_schedule_id=$2`, f.CompanyID, schedID))
	var workID int64
	require.NoError(t, p.QueryRow(ctx, `SELECT id FROM work_orders WHERE company_id=$1 AND pm_schedule_id=$2`, f.CompanyID, schedID).Scan(&workID))
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM work_orders WHERE id=$1 AND category='PREVENTIVE' AND asset_id=$2 AND status='REQUESTED'
		AND requester_id=$3 AND created_by=$3`, workID, f.AssetID, f.AdminID))
	require.Equal(t, tomorrow, h.str(`SELECT next_due_date::text FROM pm_schedules WHERE id=$1`, schedID))

	// The retry finds nothing due: still one work order.
	retry := runDue()
	h.requireFlash(retry, "success")
	require.Contains(t, retry.flash.Message, "Generated 0 ")
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM work_orders WHERE company_id=$1 AND pm_schedule_id=$2`, f.CompanyID, schedID))
	require.Equal(t, tomorrow, h.str(`SELECT next_due_date::text FROM pm_schedules WHERE id=$1`, schedID))

	// The other company's due schedule was not touched.
	require.Zero(t, h.count(`SELECT COUNT(*) FROM work_orders WHERE pm_schedule_id=$1`, foreignSchedID))
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM pm_schedules WHERE id=$1 AND next_due_date = $2::date - 1`, foreignSchedID, today))

	// Audit: schedule create, one run_due, one generated work-order create.
	sched := strconv.FormatInt(schedID, 10)
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM audit_logs WHERE entity='cmms_pm_schedule' AND entity_id=$1 AND action='create' AND actor_id=$2`, sched, f.AdminID))
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM audit_logs WHERE entity='cmms_pm_schedule' AND entity_id=$1 AND action='run_due' AND actor_id=$2`, sched, f.AdminID))
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM audit_logs WHERE entity='cmms_work_order' AND entity_id=$1 AND action='create' AND actor_id=$2`, strconv.FormatInt(workID, 10), f.AdminID))

	// A due schedule that cannot be advanced (unknown frequency) is reported,
	// generates nothing and stays due.
	var badID int64
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO pm_schedules(company_id,asset_id,name,frequency_type,frequency_value,next_due_date,active)
		VALUES($1,$2,'RC11 bad PM','YEARLY',1,$3::date,true) RETURNING id`, f.CompanyID, f.AssetID, today).Scan(&badID))
	h.requireFlash(runDue(), "danger")
	require.Zero(t, h.count(`SELECT COUNT(*) FROM work_orders WHERE pm_schedule_id=$1`, badID))
	require.Equal(t, today, h.str(`SELECT next_due_date::text FROM pm_schedules WHERE id=$1`, badID))
}
