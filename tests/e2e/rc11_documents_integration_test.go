//go:build integration

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/documents"
	documentshttp "github.com/odyssey-erp/odyssey-erp/internal/documents/http"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/storage"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// The TestRC11Document* suites drive the documents HTTP routes in process
// with the request shapes of tests/staging/certification_test.go (J-DOC-001):
// form posts carrying a csrf_token scraped from a rendered page, multipart
// version upload, 303 redirects. They only use APIs that exist at tag
// v0.10.0-rc.10 so the same file shows the failure at the tag.

// rc11DocumentPerms grants every documents permission to one user id.
type rc11DocumentPerms struct{ userID int64 }

func (p rc11DocumentPerms) EffectivePermissions(_ context.Context, userID int64) ([]string, error) {
	if userID != p.userID {
		return nil, nil
	}
	return []string{
		shared.PermDocumentsView, shared.PermDocumentsUpload, shared.PermDocumentsVersion,
		shared.PermDocumentsReview, shared.PermDocumentsApprove, shared.PermDocumentsSign,
		shared.PermDocumentsShare, shared.PermDocumentsRetentionManage, shared.PermDocumentsAdmin,
	}, nil
}

// rc11LogWriter sends handler logs to the test log so a refused request
// shows the underlying error.
type rc11LogWriter struct{ t *testing.T }

func (w rc11LogWriter) Write(p []byte) (int, error) {
	w.t.Helper()
	w.t.Log(strings.TrimSpace(string(p)))
	return len(p), nil
}

type rc11DocsHarness struct {
	t      *testing.T
	pool   *pgxpool.Pool
	f      rc11Fixture
	router http.Handler
	sess   *shared.Session
}

type rc11Result struct {
	status   int
	location string
	body     []byte
	header   http.Header
	flash    *shared.FlashMessage
}

var rc11CSRFPattern = regexp.MustCompile(`name="csrf_token"\s+value="([^"]+)"`)

// newRC11DocsHarness mounts the documents handler under /documents behind the
// same RealIP, session and CSRF checks the app router applies, logged in as
// the fixture admin with the fixture company active.
func newRC11DocsHarness(t *testing.T, p *pgxpool.Pool, f rc11Fixture) *rc11DocsHarness {
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

	svc := documents.NewService(documents.NewRepository(p), storage.NewLocalStorage(t.TempDir()))
	h := documentshttp.NewHandler(logger, svc, templates, csrf, rbac.Middleware{Service: rc11DocumentPerms{userID: f.AdminID}, Logger: logger}, nil, nil)

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
	r.Route("/documents", h.MountRoutes)

	return &rc11DocsHarness{t: t, pool: p, f: f, router: r, sess: sess}
}

func (h *rc11DocsHarness) do(req *http.Request) rc11Result {
	h.t.Helper()
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	res := rc11Result{status: rec.Code, location: rec.Header().Get("Location"), body: rec.Body.Bytes(), header: rec.Header()}
	if rec.Code == http.StatusSeeOther {
		res.flash = h.sess.PopFlash()
	}
	return res
}

func (h *rc11DocsHarness) get(path string) rc11Result {
	return h.do(httptest.NewRequest(http.MethodGet, path, nil))
}

func (h *rc11DocsHarness) postForm(path string, values url.Values) rc11Result {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return h.do(req)
}

func (h *rc11DocsHarness) postMultipart(path, csrf, description string, content []byte) rc11Result {
	h.t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	require.NoError(h.t, w.WriteField("csrf_token", csrf))
	require.NoError(h.t, w.WriteField("description", description))
	part, err := w.CreateFormFile("file", "certification.txt")
	require.NoError(h.t, err)
	_, err = part.Write(content)
	require.NoError(h.t, err)
	require.NoError(h.t, w.Close())
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return h.do(req)
}

// fetchCSRF GETs a page, requires 200 and scrapes its csrf_token field.
func (h *rc11DocsHarness) fetchCSRF(path string) string {
	h.t.Helper()
	res := h.get(path)
	require.Equal(h.t, http.StatusOK, res.status, "GET %s", path)
	m := rc11CSRFPattern.FindSubmatch(res.body)
	require.NotNil(h.t, m, "csrf_token not found on %s", path)
	return string(m[1])
}

func (h *rc11DocsHarness) count(sql string, args ...any) int {
	h.t.Helper()
	var n int
	require.NoError(h.t, h.pool.QueryRow(context.Background(), sql, args...).Scan(&n), sql)
	return n
}

// createDocument seeds a company numbering rule (once per company) and
// creates a document through POST /documents/library, returning its id.
func (h *rc11DocsHarness) createDocument(title string) int64 {
	h.t.Helper()
	ctx := context.Background()
	_, err := h.pool.Exec(ctx, `INSERT INTO document_numbering_rules(company_id,code,name,prefix,pattern,scope,created_by)
		VALUES($1,$2,'RC11 numbering',$3,'{PREFIX}-{SEQ:05d}','COMPANY',$4) ON CONFLICT (company_id, code) DO NOTHING`,
		h.f.CompanyID, "NR-"+h.f.Suffix, "RC11", h.f.AdminID)
	require.NoError(h.t, err)

	res := h.postForm("/documents/library", url.Values{
		"csrf_token":        {h.fetchCSRF("/documents/library/new")},
		"title":             {title},
		"description":       {"rc11 integration document"},
		"category_id":       {strconv.FormatInt(h.f.CategoryID, 10)},
		"classification_id": {strconv.FormatInt(h.f.ClassificationID, 10)},
	})
	require.Equal(h.t, http.StatusSeeOther, res.status)
	id, err := strconv.ParseInt(strings.TrimPrefix(res.location, "/documents/library/"), 10, 64)
	require.NoError(h.t, err, "create document redirect %q flash %+v", res.location, res.flash)
	return id
}

// uploadVersion posts a multipart version as the strict test does and
// returns the newest version id for the document (0 when none exists).
func (h *rc11DocsHarness) uploadVersion(documentID int64, content []byte) (rc11Result, int64) {
	h.t.Helper()
	docPath := fmt.Sprintf("/documents/library/%d", documentID)
	res := h.postMultipart(docPath+"/versions", h.fetchCSRF(docPath), "certification version 1", content)
	var versionID int64
	_ = h.pool.QueryRow(context.Background(), `SELECT id FROM document_versions WHERE document_id=$1 ORDER BY version_number DESC LIMIT 1`, documentID).Scan(&versionID)
	return res, versionID
}

func TestRC11DocumentVersionLifecycle(t *testing.T) {
	p, f := newRC11Fixture(t)
	h := newRC11DocsHarness(t, p, f)
	ctx := context.Background()

	documentID := h.createDocument("RC11 version lifecycle " + f.Suffix)
	docPath := fmt.Sprintf("/documents/library/%d", documentID)
	content := []byte("v0.10-core rc11 version lifecycle evidence\n")

	upload, versionID := h.uploadVersion(documentID, content)
	require.Equal(t, http.StatusSeeOther, upload.status)
	require.NotNil(t, upload.flash)
	require.Equal(t, "success", upload.flash.Kind, "version upload flash: %s", upload.flash.Message)
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM document_versions WHERE document_id=$1`, documentID))

	// The version points at the stored blob and inherits the document's
	// classification.
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM document_versions dv
		JOIN storage_blobs b ON b.id = dv.blob_id
		JOIN documents d ON d.id = dv.document_id
		WHERE dv.id=$1 AND b.company_id=$2 AND b.size_bytes=$3 AND dv.classification_id = d.classification_id`,
		versionID, f.CompanyID, len(content)))

	// Retention resolves the fixture's 7-year policy through the version
	// classification.
	versionPath := fmt.Sprintf("%s/versions/%d", docPath, versionID)
	retention := h.postForm(versionPath+"/retention", url.Values{"csrf_token": {h.fetchCSRF(docPath + "/versions")}})
	require.Equal(t, http.StatusSeeOther, retention.status)
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM document_retention WHERE document_version_id=$1 AND status='ACTIVE'`, versionID))

	// Download streams the content and records exactly one DOWNLOAD event
	// with the client address (ip:port form) parsed to the host.
	req := httptest.NewRequest(http.MethodGet, versionPath+"/download", nil)
	req.RemoteAddr = "192.0.2.10:52100"
	req.Header.Set("User-Agent", "rc11-integration")
	download := h.do(req)
	require.Equal(t, http.StatusOK, download.status)
	require.Equal(t, content, download.body)
	require.Contains(t, download.header.Get("Content-Disposition"), "attachment")
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM document_access_events WHERE document_version_id=$1 AND actor_id=$2 AND action='DOWNLOAD'
		AND ip_address = '192.0.2.10'::inet AND user_agent='rc11-integration'`, versionID, f.AdminID))

	// An address that is not an IP is stored as NULL, not rejected.
	req = httptest.NewRequest(http.MethodGet, versionPath+"/download", nil)
	req.RemoteAddr = "not-an-ip"
	download = h.do(req)
	require.Equal(t, http.StatusOK, download.status)
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM document_access_events WHERE document_version_id=$1 AND action='DOWNLOAD' AND ip_address IS NULL`, versionID))

	// Unknown version: 404.
	require.Equal(t, http.StatusNotFound, h.get(fmt.Sprintf("%s/versions/%d/download", docPath, versionID+1_000_000)).status)

	// A document of another company: the upload is refused and the blob it
	// stored is removed, so no orphan remains.
	var otherCompanyID, otherDocID int64
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO companies(code,name,base_currency) VALUES($1,'RC11 other','IDR') RETURNING id`, "RC11O-"+f.Suffix).Scan(&otherCompanyID))
	var otherClassID int64
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO document_classifications(company_id,code,name,created_by) VALUES($1,$2,'RC11 other',$3) RETURNING id`, otherCompanyID, "CLSO-"+f.Suffix, f.AdminID).Scan(&otherClassID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO documents(company_id,classification_id,document_number,title,owner_id,created_by,updated_by)
		VALUES($1,$2,$3,'RC11 foreign',$4,$4,$4) RETURNING id`, otherCompanyID, otherClassID, "FOREIGN-"+f.Suffix, f.AdminID).Scan(&otherDocID))
	blobsBefore := h.count(`SELECT COUNT(*) FROM storage_blobs WHERE company_id=$1`, f.CompanyID)
	foreign := h.postMultipart(fmt.Sprintf("/documents/library/%d/versions", otherDocID), h.fetchCSRF(docPath), "foreign", content)
	require.Equal(t, http.StatusSeeOther, foreign.status)
	require.NotNil(t, foreign.flash)
	require.Equal(t, "danger", foreign.flash.Kind)
	require.Zero(t, h.count(`SELECT COUNT(*) FROM document_versions WHERE document_id=$1`, otherDocID))
	require.Equal(t, blobsBefore, h.count(`SELECT COUNT(*) FROM storage_blobs WHERE company_id=$1`, f.CompanyID), "orphan blob left behind")
}
