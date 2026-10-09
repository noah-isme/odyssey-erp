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
