//go:build integration

package e2e

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/app"
	"github.com/odyssey-erp/odyssey-erp/internal/ar"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// TestRC11WorkspaceCompanyScope drives the real app router (release profile
// v0.10-core, Redis-backed sessions, CSRF, the AR routes behind the real RBAC
// service) with the request shapes of TestStagingTenantIsolation in
// tests/staging/certification_test.go. It only uses APIs that exist at tag
// v0.10.0-rc.10: RBAC_GLOBAL_ROLES is passed through the environment.

type rc11TenancyHarness struct {
	t        *testing.T
	router   http.Handler
	sessions *shared.SessionManager
}

type rc11TenancyClient struct {
	h      *rc11TenancyHarness
	cookie *http.Cookie
	ip     string
}

type rc11Workspace struct {
	ActiveCompanyID int64  `json:"activeCompanyID"`
	ReleaseProfile  string `json:"releaseProfile"`
	Companies       []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"companies"`
}

func (w rc11Workspace) companyIDs() []int64 {
	ids := make([]int64, 0, len(w.Companies))
	for _, c := range w.Companies {
		ids = append(ids, c.ID)
	}
	return ids
}

func newRC11TenancyHarness(t *testing.T, p *pgxpool.Pool, globalRole string) *rc11TenancyHarness {
	t.Helper()
	t.Setenv("APP_ENV", "development")
	t.Setenv("SESSION_SECRET", "rc11-session-secret")
	t.Setenv("CSRF_SECRET", "rc11-csrf-secret")
	t.Setenv("RELEASE_PROFILE", "v0.10-core")
	t.Setenv("RBAC_GLOBAL_ROLES", globalRole)
	cfg, err := app.LoadConfig()
	require.NoError(t, err)

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	templates, err := view.NewEngine()
	require.NoError(t, err)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sessions := shared.NewSessionManager(client, "odyssey_session", cfg.SessionSecret, time.Hour, false)
	csrf := shared.NewCSRFManager(cfg.CSRFSecret)
	rbacMW := rbac.Middleware{Service: rbac.NewService(rbac.NewRepository(p)), Logger: logger}
	arHandler := ar.NewHandler(logger, ar.NewService(ar.NewRepository(p)), templates, csrf, sessions, rbacMW, nil, nil)

	router := app.NewRouter(app.RouterParams{
		Logger:         logger,
		Config:         cfg,
		Templates:      templates,
		SessionManager: sessions,
		CSRFManager:    csrf,
		ARHandler:      arHandler,
		Pool:           p,
		RBACMiddleware: rbacMW,
	})
	return &rc11TenancyHarness{t: t, router: router, sessions: sessions}
}

// login stores a session for userID (optionally with a pre-set company, as
// an already-issued session would carry) and returns a client holding its
// cookie.
func (h *rc11TenancyHarness) login(userID int64, ip string, presetCompany int64) *rc11TenancyClient {
	h.t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	sess, err := h.sessions.Load(context.Background(), req)
	require.NoError(h.t, err)
	sess.SetUser(strconv.FormatInt(userID, 10))
	if presetCompany != 0 {
		sess.Set("company_id", strconv.FormatInt(presetCompany, 10))
	}
	rec := httptest.NewRecorder()
	require.NoError(h.t, h.sessions.Commit(context.Background(), rec, req, sess))
	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == h.sessions.CookieName() {
			cookie = c
		}
	}
	require.NotNil(h.t, cookie, "session cookie")
	return &rc11TenancyClient{h: h, cookie: cookie, ip: ip}
}

func (c *rc11TenancyClient) do(req *http.Request) *httptest.ResponseRecorder {
	c.h.t.Helper()
	req.AddCookie(c.cookie)
	req.RemoteAddr = c.ip + ":40000"
	rec := httptest.NewRecorder()
	c.h.router.ServeHTTP(rec, req)
	return rec
}

func (c *rc11TenancyClient) get(path string) *httptest.ResponseRecorder {
	return c.do(httptest.NewRequest(http.MethodGet, path, nil))
}

func (c *rc11TenancyClient) postForm(path string, values url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req)
}

func (c *rc11TenancyClient) csrf() string {
	c.h.t.Helper()
	res := c.get("/")
	require.Equal(c.h.t, http.StatusOK, res.Code, "GET / must render for CSRF")
	m := rc11CSRFPattern.FindSubmatch(res.Body.Bytes())
	require.NotNil(c.h.t, m, "csrf_token not found on /")
	return string(m[1])
}

func (c *rc11TenancyClient) me() rc11Workspace {
	c.h.t.Helper()
	res := c.get("/api/me")
	require.Equal(c.h.t, http.StatusOK, res.Code, "GET /api/me: %s", res.Body.String())
	var ws rc11Workspace
	require.NoError(c.h.t, json.Unmarshal(res.Body.Bytes(), &ws), res.Body.String())
	return ws
}

func TestRC11WorkspaceCompanyScope(t *testing.T) {
	p, f := newRC11Fixture(t)
	ctx := context.Background()
	scan := func(dst *int64, sql string, args ...any) {
		t.Helper()
		require.NoError(t, p.QueryRow(ctx, sql, args...).Scan(dst), sql)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		_, err := p.Exec(ctx, sql, args...)
		require.NoError(t, err, sql)
	}

	// Company B exists but the fixture admin holds no assignment in it.
	var companyB int64
	scan(&companyB, `INSERT INTO companies(code,name,base_currency) VALUES($1,'RC11 other company','IDR') RETURNING id`, "RC11B-"+f.Suffix)

	// A user whose only role is listed in RBAC_GLOBAL_ROLES and who holds no
	// assignment at all.
	globalRole := "rc11-global-" + f.Suffix
	var globalRoleID, globalUserID int64
	scan(&globalRoleID, `INSERT INTO roles(name,description) VALUES($1,'RC11 cross-company operator') RETURNING id`, globalRole)
	scan(&globalUserID, `INSERT INTO users(email,password_hash) VALUES($1,'integration') RETURNING id`, "rc11-global-"+f.Suffix+"@example.test")
	exec(`INSERT INTO user_roles(user_id,role_id) VALUES($1,$2)`, globalUserID, globalRoleID)

	// A branch user like the staging seed's: no user_roles, an active
	// branch-scoped assignment in A, and an expired company-wide one in B.
	var branchRoleID, branchUserID int64
	scan(&branchRoleID, `INSERT INTO roles(name,description) VALUES($1,'RC11 branch role') RETURNING id`, "rc11-branch-"+f.Suffix)
	scan(&branchUserID, `INSERT INTO users(email,password_hash) VALUES($1,'integration') RETURNING id`, "rc11-branch-"+f.Suffix+"@example.test")
	exec(`INSERT INTO rbac_user_role_assignments(company_id,user_id,role_id,branch_id,valid_from) VALUES($1,$2,$3,$4,NOW() - interval '1 day')`, f.CompanyID, branchUserID, branchRoleID, f.BranchID)
	exec(`INSERT INTO rbac_user_role_assignments(company_id,user_id,role_id,branch_id,valid_from,valid_to) VALUES($1,$2,$3,NULL,NOW() - interval '2 days',NOW() - interval '1 day')`, companyB, branchUserID, branchRoleID)

	h := newRC11TenancyHarness(t, p, globalRole)

	t.Run("assignment in A only lists A and rejects a forged select of B", func(t *testing.T) {
		admin := h.login(f.AdminID, "10.11.0.1", 0)
		ws := admin.me()
		assert.Equal(t, "v0.10-core", ws.ReleaseProfile, "releaseProfile")
		assert.Equal(t, []int64{f.CompanyID}, ws.companyIDs(), "companies")
		assert.Equal(t, f.CompanyID, ws.ActiveCompanyID, "activeCompanyID")

		res := admin.postForm("/company/select", url.Values{
			"csrf_token": {admin.csrf()},
			"company_id": {strconv.FormatInt(companyB, 10)},
		})
		assert.Equal(t, http.StatusSeeOther, res.Code, "forged /company/select")
		assert.Equal(t, f.CompanyID, admin.me().ActiveCompanyID, "active company after forged select")

		res = admin.postForm("/company/select", url.Values{
			"csrf_token": {admin.csrf()},
			"company_id": {strconv.FormatInt(f.CompanyID, 10)},
		})
		assert.Equal(t, http.StatusSeeOther, res.Code, "/company/select of a permitted company")
		assert.Equal(t, f.CompanyID, admin.me().ActiveCompanyID)
	})

	t.Run("an issued session pinned to a foreign company is moved back to A", func(t *testing.T) {
		admin := h.login(f.AdminID, "10.11.0.2", companyB)
		assert.Equal(t, f.CompanyID, admin.me().ActiveCompanyID)
	})

	t.Run("no assignment fails closed", func(t *testing.T) {
		noAccess := h.login(f.OtherUserID, "10.11.0.3", 0)
		ws := noAccess.me()
		assert.Equal(t, "v0.10-core", ws.ReleaseProfile)
		assert.Zero(t, ws.ActiveCompanyID, "activeCompanyID")
		assert.Empty(t, ws.Companies, "companies")

		home := noAccess.get("/")
		assert.Equal(t, http.StatusOK, home.Code)
		assert.Contains(t, home.Body.String(), "data-no-company-notice", "no-company notice on /")

		assert.Equal(t, http.StatusForbidden, noAccess.get("/finance/ar/invoices").Code, "GET /finance/ar/invoices")
		res := noAccess.postForm("/finance/ar/invoices", url.Values{"csrf_token": {noAccess.csrf()}})
		assert.Equal(t, http.StatusForbidden, res.Code, "POST /finance/ar/invoices")
		assert.Zero(t, noAccess.me().ActiveCompanyID, "no company may be assigned implicitly")
	})

	t.Run("a pre-set company on a no-assignment session is cleared", func(t *testing.T) {
		noAccess := h.login(f.OtherUserID, "10.11.0.4", f.CompanyID)
		assert.Zero(t, noAccess.me().ActiveCompanyID)
		assert.Equal(t, http.StatusForbidden, noAccess.get("/finance/ar/invoices").Code)
	})

	t.Run("a role listed in RBAC_GLOBAL_ROLES sees every company", func(t *testing.T) {
		global := h.login(globalUserID, "10.11.0.5", 0)
		ws := global.me()
		var total int
		require.NoError(t, p.QueryRow(ctx, `SELECT COUNT(*) FROM companies`).Scan(&total))
		assert.Len(t, ws.Companies, total)
		assert.Contains(t, ws.companyIDs(), f.CompanyID)
		assert.Contains(t, ws.companyIDs(), companyB)
		assert.NotZero(t, ws.ActiveCompanyID)
	})

	t.Run("a branch-scoped assignment keeps its company; expired ones do not count", func(t *testing.T) {
		branch := h.login(branchUserID, "10.11.0.6", 0)
		ws := branch.me()
		assert.Equal(t, "v0.10-core", ws.ReleaseProfile)
		assert.Equal(t, f.CompanyID, ws.ActiveCompanyID, "activeCompanyID (certification_test.go ISO-001)")
		assert.Equal(t, []int64{f.CompanyID}, ws.companyIDs())
	})
}
