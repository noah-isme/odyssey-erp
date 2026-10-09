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
	svc    *documents.Service
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

	return &rc11DocsHarness{t: t, pool: p, f: f, router: r, sess: sess, svc: svc}
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

// foreignDocument inserts a document owned by a second company (created
// on first use) and returns its id.
func (h *rc11DocsHarness) foreignDocument() int64 {
	h.t.Helper()
	ctx := context.Background()
	var companyID, classID, docID int64
	err := h.pool.QueryRow(ctx, `SELECT id FROM companies WHERE code=$1`, "RC11O-"+h.f.Suffix).Scan(&companyID)
	if err != nil {
		require.NoError(h.t, h.pool.QueryRow(ctx, `INSERT INTO companies(code,name,base_currency) VALUES($1,'RC11 other','IDR') RETURNING id`, "RC11O-"+h.f.Suffix).Scan(&companyID))
		require.NoError(h.t, h.pool.QueryRow(ctx, `INSERT INTO document_classifications(company_id,code,name,created_by) VALUES($1,$2,'RC11 other',$3) RETURNING id`, companyID, "CLSO-"+h.f.Suffix, h.f.AdminID).Scan(&classID))
	} else {
		require.NoError(h.t, h.pool.QueryRow(ctx, `SELECT id FROM document_classifications WHERE company_id=$1 ORDER BY id LIMIT 1`, companyID).Scan(&classID))
	}
	require.NoError(h.t, h.pool.QueryRow(ctx, `INSERT INTO documents(company_id,classification_id,document_number,title,owner_id,created_by,updated_by)
		VALUES($1,$2,'FOREIGN-'||nextval('documents_id_seq'),'RC11 foreign',$3,$3,$3) RETURNING id`, companyID, classID, h.f.AdminID).Scan(&docID))
	return docID
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
	otherDocID := h.foreignDocument()
	blobsBefore := h.count(`SELECT COUNT(*) FROM storage_blobs WHERE company_id=$1`, f.CompanyID)
	foreign := h.postMultipart(fmt.Sprintf("/documents/library/%d/versions", otherDocID), h.fetchCSRF(docPath), "foreign", content)
	require.Equal(t, http.StatusSeeOther, foreign.status)
	require.NotNil(t, foreign.flash)
	require.Equal(t, "danger", foreign.flash.Kind)
	require.Zero(t, h.count(`SELECT COUNT(*) FROM document_versions WHERE document_id=$1`, otherDocID))
	require.Equal(t, blobsBefore, h.count(`SELECT COUNT(*) FROM storage_blobs WHERE company_id=$1`, f.CompanyID), "orphan blob left behind")
}

func TestRC11DocumentACLRoute(t *testing.T) {
	p, f := newRC11Fixture(t)
	h := newRC11DocsHarness(t, p, f)

	documentID := h.createDocument("RC11 ACL " + f.Suffix)
	aclPath := fmt.Sprintf("/documents/library/%d/acl", documentID)
	expires := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	grant := func(path string, overrides url.Values) rc11Result {
		values := url.Values{
			"csrf_token":     {h.fetchCSRF(aclPath)},
			"principal_type": {"USER"},
			"principal_id":   {strconv.FormatInt(f.AdminID, 10)},
			"permission":     {"READ"},
			"effect":         {"ALLOW"},
			"expires_at":     {expires},
		}
		for k, v := range overrides {
			values[k] = v
		}
		return h.postForm(path, values)
	}
	aclRows := func(docID int64) int {
		return h.count(`SELECT COUNT(*) FROM document_acls WHERE document_id=$1`, docID)
	}

	// The page renders (the strict test scrapes its CSRF token) and the
	// create posts back to it.
	created := grant(aclPath, nil)
	require.Equal(t, http.StatusSeeOther, created.status)
	require.Equal(t, aclPath, created.location)
	require.NotNil(t, created.flash)
	require.Equal(t, "success", created.flash.Kind, created.flash.Message)
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM document_acls WHERE company_id=$1 AND document_id=$2 AND principal_type='USER'
		AND principal_id=$3 AND permission='READ' AND effect='ALLOW' AND granted_by=$3 AND expires_at IS NOT NULL`, f.CompanyID, documentID, f.AdminID))
	page := h.get(aclPath)
	require.Equal(t, http.StatusOK, page.status)
	require.Contains(t, string(page.body), "READ")

	// A duplicate grant is refused with a flash, not a 500.
	dup := grant(aclPath, nil)
	require.Equal(t, http.StatusSeeOther, dup.status)
	require.NotNil(t, dup.flash)
	require.Equal(t, "danger", dup.flash.Kind)
	require.Equal(t, 1, aclRows(documentID))

	// Invalid input is refused.
	for name, bad := range map[string]url.Values{
		"principal type": {"principal_type": {"GROUP"}},
		"principal id":   {"principal_id": {"0"}},
		"permission":     {"permission": {"  "}},
		"effect":         {"effect": {"MAYBE"}},
		"expires_at":     {"expires_at": {"tomorrow"}},
	} {
		res := grant(aclPath, bad)
		require.Equal(t, http.StatusSeeOther, res.status, name)
		require.NotNil(t, res.flash, name)
		require.Equal(t, "danger", res.flash.Kind, name)
	}
	require.Equal(t, 1, aclRows(documentID))

	// Another company's document: no page, no row.
	foreignID := h.foreignDocument()
	foreignPath := fmt.Sprintf("/documents/library/%d/acl", foreignID)
	require.Equal(t, http.StatusNotFound, h.get(foreignPath).status)
	res := grant(foreignPath, nil)
	require.Equal(t, http.StatusSeeOther, res.status)
	require.NotNil(t, res.flash)
	require.Equal(t, "danger", res.flash.Kind)
	require.Zero(t, aclRows(foreignID))
}

func TestRC11DocumentReviewFlow(t *testing.T) {
	p, f := newRC11Fixture(t)
	h := newRC11DocsHarness(t, p, f)
	ctx := context.Background()

	status := func(versionID int64) string {
		var s string
		require.NoError(t, p.QueryRow(ctx, `SELECT status FROM document_versions WHERE id=$1`, versionID).Scan(&s))
		return s
	}
	newVersion := func(title string) (int64, int64) {
		documentID := h.createDocument(title)
		res, versionID := h.uploadVersion(documentID, []byte("review evidence "+title))
		require.Equal(t, http.StatusSeeOther, res.status)
		require.NotZero(t, versionID, "version upload flash %+v", res.flash)
		return documentID, versionID
	}

	documentID, versionID := newVersion("RC11 review " + f.Suffix)
	docPath := fmt.Sprintf("/documents/library/%d", documentID)
	versionsPath := docPath + "/versions"
	versionPath := fmt.Sprintf("%s/%d", versionsPath, versionID)
	sequence := []string{status(versionID)}

	// Submit: one default PENDING step for this version.
	submit := h.postForm(versionPath+"/submit-review", url.Values{"csrf_token": {h.fetchCSRF(versionsPath)}})
	require.Equal(t, http.StatusSeeOther, submit.status)
	require.Equal(t, versionsPath, submit.location)
	require.NotNil(t, submit.flash)
	require.Equal(t, "success", submit.flash.Kind, submit.flash.Message)
	sequence = append(sequence, status(versionID))
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM document_review_steps WHERE document_version_id=$1`, versionID))
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM document_review_steps WHERE document_version_id=$1 AND company_id=$2
		AND step_order=1 AND name='Review' AND required_approvals=1 AND status='PENDING'`, versionID, f.CompanyID))

	// Submitting again is refused: the version is no longer DRAFT.
	again := h.postForm(versionPath+"/submit-review", url.Values{"csrf_token": {h.fetchCSRF(versionsPath)}})
	require.Equal(t, http.StatusSeeOther, again.status)
	require.NotNil(t, again.flash)
	require.Equal(t, "danger", again.flash.Kind)
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM document_review_steps WHERE document_version_id=$1`, versionID))

	// The strict test reads the step id from the database.
	var stepID int64
	require.NoError(t, p.QueryRow(ctx, `SELECT id FROM document_review_steps WHERE document_version_id=$1 ORDER BY step_order LIMIT 1`, versionID).Scan(&stepID))

	// A step of another version cannot be decided through this version.
	_, otherVersionID := newVersion("RC11 review other " + f.Suffix)
	otherDocPath := fmt.Sprintf("/documents/library/%d", h.documentOf(otherVersionID))
	otherSubmit := h.postForm(fmt.Sprintf("%s/versions/%d/submit-review", otherDocPath, otherVersionID), url.Values{"csrf_token": {h.fetchCSRF(otherDocPath + "/versions")}})
	require.Equal(t, http.StatusSeeOther, otherSubmit.status)
	var otherStepID int64
	require.NoError(t, p.QueryRow(ctx, `SELECT id FROM document_review_steps WHERE document_version_id=$1`, otherVersionID).Scan(&otherStepID))
	crossed := h.postForm(versionPath+"/review", url.Values{
		"csrf_token": {h.fetchCSRF(versionsPath)}, "step_id": {strconv.FormatInt(otherStepID, 10)},
		"decision": {"APPROVED"}, "comments": {"wrong version"},
	})
	require.Equal(t, http.StatusSeeOther, crossed.status)
	require.NotNil(t, crossed.flash)
	require.NotNil(t, crossed.flash)
	require.Equal(t, "danger", crossed.flash.Kind)
	require.Equal(t, "PENDING", h.stepStatus(otherStepID))
	require.Zero(t, h.count(`SELECT COUNT(*) FROM document_review_decisions WHERE review_step_id=$1`, otherStepID))

	// An unknown decision is refused.
	bogus := h.postForm(versionPath+"/review", url.Values{
		"csrf_token": {h.fetchCSRF(versionsPath)}, "step_id": {strconv.FormatInt(stepID, 10)},
		"decision": {"MAYBE"}, "comments": {"?"},
	})
	require.Equal(t, http.StatusSeeOther, bogus.status)
	require.NotNil(t, bogus.flash)
	require.Equal(t, "danger", bogus.flash.Kind)

	// Approve exactly as the strict test does.
	review := h.postForm(versionPath+"/review", url.Values{
		"csrf_token": {h.fetchCSRF(versionsPath)},
		"step_id":    {strconv.FormatInt(stepID, 10)},
		"decision":   {"APPROVED"},
		"comments":   {"v0.10-core certification review"},
	})
	require.Equal(t, http.StatusSeeOther, review.status)
	require.Equal(t, versionsPath, review.location)
	require.NotNil(t, review.flash)
	require.Equal(t, "success", review.flash.Kind, review.flash.Message)
	sequence = append(sequence, status(versionID))
	require.Equal(t, []string{"DRAFT", "UNDER_REVIEW", "APPROVED"}, sequence)
	require.Equal(t, "APPROVED", h.stepStatus(stepID))
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM document_review_decisions WHERE review_step_id=$1 AND reviewer_id=$2 AND decision='APPROVED' AND company_id=$3`, stepID, f.AdminID, f.CompanyID))

	// A second decision by the same reviewer adds nothing.
	second := h.postForm(versionPath+"/review", url.Values{
		"csrf_token": {h.fetchCSRF(versionsPath)}, "step_id": {strconv.FormatInt(stepID, 10)},
		"decision": {"APPROVED"}, "comments": {"again"},
	})
	require.Equal(t, http.StatusSeeOther, second.status)
	require.NotNil(t, second.flash)
	require.Equal(t, "danger", second.flash.Kind)
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM document_review_decisions WHERE review_step_id=$1`, stepID))
	require.Equal(t, "APPROVED", status(versionID))

	// Rejecting the other version marks it REJECTED.
	reject := h.postForm(fmt.Sprintf("%s/versions/%d/review", otherDocPath, otherVersionID), url.Values{
		"csrf_token": {h.fetchCSRF(otherDocPath + "/versions")}, "step_id": {strconv.FormatInt(otherStepID, 10)},
		"decision": {"REJECTED"}, "comments": {"not ready"},
	})
	require.Equal(t, http.StatusSeeOther, reject.status)
	require.NotNil(t, reject.flash)
	require.Equal(t, "success", reject.flash.Kind, reject.flash.Message)
	require.Equal(t, "REJECTED", status(otherVersionID))
	require.Equal(t, "REJECTED", h.stepStatus(otherStepID))
}

func (h *rc11DocsHarness) documentOf(versionID int64) int64 {
	h.t.Helper()
	var id int64
	require.NoError(h.t, h.pool.QueryRow(context.Background(), `SELECT document_id FROM document_versions WHERE id=$1`, versionID).Scan(&id))
	return id
}

func (h *rc11DocsHarness) stepStatus(stepID int64) string {
	h.t.Helper()
	var s string
	require.NoError(h.t, h.pool.QueryRow(context.Background(), `SELECT status FROM document_review_steps WHERE id=$1`, stepID).Scan(&s))
	return s
}

var rc11UUIDPattern = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

func TestRC11DocumentSignatureReplay(t *testing.T) {
	p, f := newRC11Fixture(t)
	h := newRC11DocsHarness(t, p, f)
	ctx := context.Background()

	documentID := h.createDocument("RC11 signature " + f.Suffix)
	res, versionID := h.uploadVersion(documentID, []byte("signature evidence"))
	require.Equal(t, http.StatusSeeOther, res.status)
	require.NotZero(t, versionID, "version upload flash %+v", res.flash)
	docPath := fmt.Sprintf("/documents/library/%d", documentID)
	versionsPath := docPath + "/versions"
	versionPath := fmt.Sprintf("%s/%d", versionsPath, versionID)
	const meaning = "Approved as effective version"

	// challenge issues one through the route and returns the id shown in
	// the flash, checking it against the row's challenge_id::text.
	challenge := func(path string, extra url.Values) string {
		t.Helper()
		values := url.Values{"csrf_token": {h.fetchCSRF(versionsPath)}}
		for k, v := range extra {
			values[k] = v
		}
		res := h.postForm(path+"/challenge", values)
		require.Equal(t, http.StatusSeeOther, res.status)
		require.NotNil(t, res.flash)
		require.Equal(t, "success", res.flash.Kind, res.flash.Message)
		shown := rc11UUIDPattern.FindString(res.flash.Message)
		require.NotEmpty(t, shown, "flash %q carries no challenge id", res.flash.Message)
		require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM document_signature_challenges WHERE challenge_id::text=$1 AND used=false`, shown))
		return shown
	}
	sign := func(path, challengeID, meaning string) rc11Result {
		t.Helper()
		return h.postForm(path+"/sign", url.Values{
			"csrf_token":   {h.fetchCSRF(versionsPath)},
			"challenge_id": {challengeID},
			"meaning":      {meaning},
		})
	}
	used := func(challengeID string) bool {
		t.Helper()
		var u bool
		require.NoError(t, p.QueryRow(ctx, `SELECT used FROM document_signature_challenges WHERE challenge_id=$1::uuid`, challengeID).Scan(&u))
		return u
	}
	signatures := func() int {
		return h.count(`SELECT COUNT(*) FROM document_signatures WHERE document_version_id=$1`, versionID)
	}

	// Without a meaning the challenge carries the default one and policy 1.
	first := challenge(versionPath, nil)
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM document_signature_challenges WHERE challenge_id=$1::uuid
		AND company_id=$2 AND document_version_id=$3 AND signer_id=$4 AND meaning=$5 AND policy_version=1 AND expiry > NOW()`,
		first, f.CompanyID, versionID, f.AdminID, meaning))
	custom := challenge(versionPath, url.Values{"meaning": {"Reviewed for accuracy"}})
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM document_signature_challenges WHERE challenge_id=$1::uuid AND meaning='Reviewed for accuracy'`, custom))

	// Sign as the strict test does: the challenge is consumed, one signature.
	signed := sign(versionPath, first, meaning)
	require.Equal(t, http.StatusSeeOther, signed.status)
	require.NotNil(t, signed.flash)
	require.Equal(t, "success", signed.flash.Kind, signed.flash.Message)
	require.True(t, used(first))
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM document_signatures WHERE document_version_id=$1 AND meaning=$2 AND challenge_id=$3::uuid AND signer_id=$4 AND policy_version=1`, versionID, meaning, first, f.AdminID))
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM document_access_events WHERE document_version_id=$1 AND action='SIGNED' AND actor_id=$2`, versionID, f.AdminID))

	// Replay of a used challenge is refused.
	replay := sign(versionPath, first, meaning)
	require.Equal(t, http.StatusSeeOther, replay.status)
	require.NotNil(t, replay.flash)
	require.Equal(t, "danger", replay.flash.Kind)
	require.Equal(t, 1, signatures())

	// A meaning is required on the sign request; the challenge stays usable.
	blank := sign(versionPath, custom, "  ")
	require.Equal(t, http.StatusSeeOther, blank.status)
	require.NotNil(t, blank.flash)
	require.Equal(t, "danger", blank.flash.Kind)
	require.False(t, used(custom))

	// A challenge for this version cannot sign another version, and the
	// refused attempt does not consume it.
	res2, version2ID := h.uploadVersion(documentID, []byte("signature evidence v2"))
	require.Equal(t, http.StatusSeeOther, res2.status)
	require.NotEqual(t, versionID, version2ID)
	crossed := sign(fmt.Sprintf("%s/%d", versionsPath, version2ID), custom, meaning)
	require.Equal(t, http.StatusSeeOther, crossed.status)
	require.NotNil(t, crossed.flash)
	require.Equal(t, "danger", crossed.flash.Kind)
	require.False(t, used(custom))
	require.Zero(t, h.count(`SELECT COUNT(*) FROM document_signatures WHERE document_version_id=$1`, version2ID))

	// An expired challenge is refused and not consumed.
	expired := challenge(versionPath, nil)
	_, err := p.Exec(ctx, `UPDATE document_signature_challenges SET expiry = NOW() - interval '1 minute' WHERE challenge_id=$1::uuid`, expired)
	require.NoError(t, err)
	late := sign(versionPath, expired, meaning)
	require.Equal(t, http.StatusSeeOther, late.status)
	require.NotNil(t, late.flash)
	require.Equal(t, "danger", late.flash.Kind)
	require.False(t, used(expired))
	require.Equal(t, 1, signatures())

	// Concurrent use of one challenge yields exactly one signature.
	racing := challenge(versionPath, nil)
	const attempts = 8
	errs := make(chan error, attempts)
	start := make(chan struct{})
	for i := 0; i < attempts; i++ {
		go func() {
			<-start
			_, err := h.svc.SignDocument(ctx, documents.SignDocumentRequest{
				CompanyID: f.CompanyID, DocumentVersionID: versionID, ChallengeID: racing,
				SignerID: f.AdminID, Meaning: meaning, IPAddress: "192.0.2.20:4000", UserAgent: "rc11-race",
			})
			errs <- err
		}()
	}
	close(start)
	succeeded := 0
	for i := 0; i < attempts; i++ {
		if <-errs == nil {
			succeeded++
		}
	}
	require.Equal(t, 1, succeeded)
	require.True(t, used(racing))
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM document_signatures WHERE challenge_id=$1::uuid`, racing))
	require.Equal(t, 2, signatures())
}

// TestRC11DocumentCertificationJourney replays J-DOC-001 of
// tests/staging/certification_test.go (L919-994) in order, with its request
// shapes and database assertions.
func TestRC11DocumentCertificationJourney(t *testing.T) {
	p, f := newRC11Fixture(t)
	h := newRC11DocsHarness(t, p, f)
	ctx := context.Background()
	queryInt := func(sql string, args ...any) int64 {
		t.Helper()
		var v int64
		require.NoError(t, p.QueryRow(ctx, sql, args...).Scan(&v), sql)
		return v
	}

	documentID := h.createDocument("v0.10-core certification document")
	documentPath := fmt.Sprintf("/documents/library/%d", documentID)
	require.Equal(t, http.StatusOK, h.get(documentPath).status)
	version, versionID := h.uploadVersion(documentID, []byte("v0.10-core staging certification evidence\n"))
	require.Equal(t, http.StatusSeeOther, version.status)
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM document_versions WHERE document_id=$1`, documentID))

	aclPath := documentPath + "/acl"
	acl := h.postForm(aclPath, url.Values{
		"csrf_token": {h.fetchCSRF(aclPath)}, "principal_type": {"USER"},
		"principal_id": {strconv.FormatInt(f.AdminID, 10)}, "permission": {"READ"}, "effect": {"ALLOW"},
		"expires_at": {time.Now().UTC().Add(time.Hour).Format(time.RFC3339)},
	})
	require.Equal(t, http.StatusSeeOther, acl.status)
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM document_acls WHERE company_id=$1 AND document_id=$2 AND principal_id=$3 AND permission='READ' AND effect='ALLOW' AND expires_at IS NOT NULL`, f.CompanyID, documentID, f.AdminID))

	versionPath := fmt.Sprintf("%s/versions/%d", documentPath, versionID)
	submit := h.postForm(versionPath+"/submit-review", url.Values{"csrf_token": {h.fetchCSRF(documentPath + "/versions")}})
	require.Equal(t, http.StatusSeeOther, submit.status)
	stepID := queryInt(`SELECT id FROM document_review_steps WHERE document_version_id=$1 ORDER BY step_order LIMIT 1`, versionID)
	review := h.postForm(versionPath+"/review", url.Values{
		"csrf_token": {h.fetchCSRF(documentPath + "/versions")}, "step_id": {strconv.FormatInt(stepID, 10)},
		"decision": {"APPROVED"}, "comments": {"v0.10-core certification review"},
	})
	require.Equal(t, http.StatusSeeOther, review.status)
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM document_versions WHERE id=$1 AND status='APPROVED'`, versionID))

	challenge := h.postForm(versionPath+"/challenge", url.Values{"csrf_token": {h.fetchCSRF(documentPath + "/versions")}})
	require.Equal(t, http.StatusSeeOther, challenge.status)
	var challengeID string
	require.NoError(t, p.QueryRow(ctx, `SELECT challenge_id::text FROM document_signature_challenges WHERE document_version_id=$1 ORDER BY created_at DESC LIMIT 1`, versionID).Scan(&challengeID))
	sign := h.postForm(versionPath+"/sign", url.Values{
		"csrf_token": {h.fetchCSRF(documentPath + "/versions")}, "challenge_id": {challengeID},
		"meaning": {"Approved as effective version"},
	})
	require.Equal(t, http.StatusSeeOther, sign.status)
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM document_signature_challenges WHERE challenge_id=$1::uuid AND used=true`, challengeID))
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM document_signatures WHERE document_version_id=$1 AND meaning=$2`, versionID, "Approved as effective version"))

	retention := h.postForm(versionPath+"/retention", url.Values{"csrf_token": {h.fetchCSRF(documentPath + "/versions")}})
	require.Equal(t, http.StatusSeeOther, retention.status)
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM document_retention WHERE document_version_id=$1 AND status='ACTIVE'`, versionID))

	download := h.get(versionPath + "/download")
	require.Equal(t, http.StatusOK, download.status)
	require.NotEmpty(t, download.body)
	require.Equal(t, 1, h.count(`SELECT COUNT(*) FROM document_access_events WHERE document_version_id=$1 AND actor_id=$2 AND action='DOWNLOAD'`, versionID, f.AdminID))
}
