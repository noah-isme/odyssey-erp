package documentshttp

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/documents"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
	"github.com/odyssey-erp/odyssey-erp/jobs"
)

// Handler wires HTTP endpoints for Document Management.
type Handler struct {
	logger    *slog.Logger
	service   *documents.Service
	templates *view.Engine
	csrf      *shared.CSRFManager
	rbac      rbac.Middleware
	jobs      *jobs.Client
	pool      *pgxpool.Pool
}

// NewHandler constructs a Handler value.
func NewHandler(logger *slog.Logger, service *documents.Service, templates *view.Engine, csrf *shared.CSRFManager, rbac rbac.Middleware, jobsClient *jobs.Client, pool *pgxpool.Pool) *Handler {
	return &Handler{
		logger:    logger,
		service:   service,
		templates: templates,
		csrf:      csrf,
		rbac:      rbac,
		jobs:      jobsClient,
		pool:      pool,
	}
}

// MountRoutes registers HTTP routes for Document Management.
func (h *Handler) MountRoutes(r chi.Router) {
	// Document library
	r.Route("/library", func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermDocumentsView))
		r.Get("/", h.listDocuments)
		r.Get("/new", h.newDocumentForm)
		r.With(h.rbac.RequireAny(shared.PermDocumentsUpload)).Post("/", h.createDocument)
		r.Get("/{id}", h.getDocument)
		r.Get("/{id}/versions", h.listVersions)
		r.With(h.rbac.RequireAny(shared.PermDocumentsVersion)).Post("/{id}/versions", h.createVersion)
		r.Get("/{id}/versions/{versionID}/download", h.downloadVersion)
		r.Get("/{id}/acl", h.listACL)
		r.With(h.rbac.RequireAny(shared.PermDocumentsAdmin)).Post("/{id}/acl", h.createACL)
		r.With(h.rbac.RequireAny(shared.PermDocumentsSign)).Post("/{id}/versions/{versionID}/challenge", h.createChallenge)
		r.With(h.rbac.RequireAny(shared.PermDocumentsSign)).Post("/{id}/versions/{versionID}/sign", h.signVersion)
		r.With(h.rbac.RequireAny(shared.PermDocumentsAdmin)).Post("/{id}/versions/{versionID}/retention", h.applyRetention)

		// Advanced Documents
		r.Post("/{id}/versions/{versionID}/ocr", h.processOCR)
		r.Post("/{id}/sessions", h.createCollaborationSession)
		r.Post("/{id}/sessions/{sessionID}/changes", h.recordCollaborationChange)
	})

	r.Route("/search", func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermDocumentsView))
		r.Get("/", h.searchContent)
	})

	// Categories management
	r.Route("/categories", func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermDocumentsAdmin))
		r.Get("/", h.listCategories)
		r.Get("/new", h.newCategoryForm)
		r.Post("/", h.createCategory)
	})

	// Classifications
	r.Route("/classifications", func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermDocumentsAdmin))
		r.Get("/", h.listClassifications)
	})
}

// ============================================================================
// Documents
// ============================================================================

func (h *Handler) listDocuments(w http.ResponseWriter, r *http.Request) {
	companyID := currentCompany(r)
	filter := documents.ListFilter{
		CompanyID: companyID,
		Search:    r.URL.Query().Get("q"),
		Limit:     50,
		Offset:    0,
	}
	if s := r.URL.Query().Get("status"); s != "" {
		status := documents.NormaliseStatus(s)
		filter.Status = &status
	}

	docs, err := h.service.List(r.Context(), filter)
	if err != nil {
		h.logger.Error("list documents", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	categories, _ := h.service.ListCategories(r.Context(), companyID)
	classifications, _ := h.service.ListClassifications(r.Context())

	h.render(w, r, "pages/documents/library.html", "Document Library", map[string]any{
		"Documents":       docs,
		"Filter":          filter,
		"Categories":      categories,
		"Classifications": classifications,
		"Statuses": []documents.Status{
			documents.StatusDraft, documents.StatusSubmitted, documents.StatusUnderReview,
			documents.StatusApproved, documents.StatusPublished, documents.StatusArchived,
		},
	})
}

func (h *Handler) newDocumentForm(w http.ResponseWriter, r *http.Request) {
	companyID := currentCompany(r)
	categories, _ := h.service.ListCategories(r.Context(), companyID)
	classifications, _ := h.service.ListClassifications(r.Context())
	h.render(w, r, "pages/documents/document_new.html", "New Document", map[string]any{
		"Categories":      categories,
		"Classifications": classifications,
	})
}

func (h *Handler) createDocument(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	actorID := currentUser(r)
	companyID := currentCompany(r)

	req := documents.CreateDocumentRequest{
		CompanyID:        companyID,
		Title:            strings.TrimSpace(r.PostFormValue("title")),
		Description:      strings.TrimSpace(r.PostFormValue("description")),
		CategoryID:       parseInt64(r.PostFormValue("category_id")),
		ClassificationID: parseInt64(r.PostFormValue("classification_id")),
		OwnerID:          actorID,
		ActorID:          actorID,
	}

	doc, err := h.service.Create(r.Context(), req)
	if err != nil {
		h.logger.Warn("create document", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/documents/library/new", "danger", shared.UserSafeMessage(err))
		return
	}
	h.redirectWithFlash(w, r, "/documents/library/"+strconv.FormatInt(doc.ID, 10), "success", "Document "+doc.Number+" created")
}

func (h *Handler) getDocument(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	doc, err := h.service.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, documents.ErrDocumentNotFound) {
			http.NotFound(w, r)
			return
		}
		h.logger.Error("get document", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	versions, _ := h.service.ListVersions(r.Context(), documents.ListVersionsFilter{
		CompanyID:  doc.CompanyID,
		DocumentID: doc.ID,
		Limit:      20,
	})
	h.render(w, r, "pages/documents/document_detail.html", doc.Number+" – "+doc.Title, map[string]any{
		"Document": doc,
		"Versions": versions,
		"Statuses": []documents.Status{
			documents.StatusDraft, documents.StatusSubmitted, documents.StatusApproved,
			documents.StatusPublished, documents.StatusArchived,
		},
	})
}

func (h *Handler) listVersions(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	companyID := currentCompany(r)
	versions, err := h.service.ListVersions(r.Context(), documents.ListVersionsFilter{
		CompanyID:  companyID,
		DocumentID: id,
		Limit:      50,
	})
	if err != nil {
		h.logger.Error("list versions", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	doc, _ := h.service.Get(r.Context(), id)
	h.render(w, r, "pages/documents/versions.html", "Document Versions", map[string]any{
		"Document": doc,
		"Versions": versions,
	})
}

func (h *Handler) createVersion(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	id := parseInt64(chi.URLParam(r, "id"))
	actorID := currentUser(r)

	// Get existing versions to determine next version number
	companyID := currentCompany(r)
	versions, _ := h.service.ListVersions(r.Context(), documents.ListVersionsFilter{
		CompanyID:  companyID,
		DocumentID: id,
		Limit:      1,
	})
	nextVersion := 1
	if len(versions) > 0 {
		nextVersion = versions[0].VersionNumber + 1
	}

	req := documents.CreateVersionRequest{
		CompanyID:     companyID,
		DocumentID:    id,
		VersionNumber: nextVersion,
		Description:   strings.TrimSpace(r.PostFormValue("description")), // used as change summary
		ActorID:       actorID,
	}

	// Extract the file
	file, header, err := r.FormFile("file")
	if err != nil {
		h.logger.Warn("extract file", slog.Any("error", err))
		http.Error(w, "File upload missing or invalid", http.StatusBadRequest)
		return
	}
	defer file.Close()

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	version, err := h.service.UploadAndCreateVersion(r.Context(), file, header.Size, mimeType, req)
	if err != nil {
		h.logger.Warn("create version", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/documents/library/"+strconv.FormatInt(id, 10), "danger", shared.UserSafeMessage(err))
		return
	}
	h.redirectWithFlash(w, r, "/documents/library/"+strconv.FormatInt(id, 10), "success",
		"Version "+strconv.Itoa(version.VersionNumber)+" created")
}

func (h *Handler) downloadVersion(w http.ResponseWriter, r *http.Request) {
	documentID := parseInt64(chi.URLParam(r, "id"))
	versionID := parseInt64(chi.URLParam(r, "versionID"))
	companyID := currentCompany(r)
	version, err := h.service.GetVersion(r.Context(), versionID)
	if err != nil {
		if errors.Is(err, documents.ErrDocumentVersionNotFound) {
			http.NotFound(w, r)
			return
		}
		h.logger.Error("get version for download", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	// Another company's version, or a version of another document, is
	// indistinguishable from a missing one.
	if companyID <= 0 || version.CompanyID != companyID || version.DocumentID != documentID {
		http.NotFound(w, r)
		return
	}

	reader, _, contentType, err := h.service.DownloadVersion(r.Context(), versionID, currentUser(r), clientIP(r), r.UserAgent())
	if err != nil {
		h.logger.Error("download version", slog.Int64("version_id", versionID), slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	if contentType == "" {
		contentType = "application/octet-stream"
	}
	filename := fmt.Sprintf("%s-v%d", version.DocumentNumber, version.VersionNumber)
	if version.DocumentNumber == "" {
		filename = fmt.Sprintf("document-%d-v%d", documentID, version.VersionNumber)
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, reader); err != nil {
		h.logger.Warn("stream version", slog.Int64("version_id", versionID), slog.Any("error", err))
	}
}

// ============================================================================
// Access control entries
// ============================================================================

func (h *Handler) listACL(w http.ResponseWriter, r *http.Request) {
	documentID := parseInt64(chi.URLParam(r, "id"))
	companyID := currentCompany(r)
	doc, err := h.service.Get(r.Context(), documentID)
	if err != nil {
		if errors.Is(err, documents.ErrDocumentNotFound) {
			http.NotFound(w, r)
			return
		}
		h.logger.Error("get document for acl", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if companyID <= 0 || doc.CompanyID != companyID {
		http.NotFound(w, r)
		return
	}
	acls, err := h.service.ListACLs(r.Context(), companyID, &doc.ID, nil)
	if err != nil {
		h.logger.Error("list acls", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/documents/acl.html", "Document Access", map[string]any{
		"Document":       doc,
		"ACLs":           acls,
		"PrincipalTypes": []string{"USER", "ROLE"},
		"Effects":        []string{"ALLOW", "DENY"},
	})
}

func (h *Handler) createACL(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	documentID := parseInt64(chi.URLParam(r, "id"))
	aclPath := "/documents/library/" + strconv.FormatInt(documentID, 10) + "/acl"

	expiresAt, err := parseACLExpiry(r.PostFormValue("expires_at"))
	if err != nil {
		h.redirectWithFlash(w, r, aclPath, "danger", "Expiry must be a date and time (RFC 3339 or YYYY-MM-DDTHH:MM).")
		return
	}
	principalID := parseInt64(r.PostFormValue("principal_id"))
	req := documents.CreateACLRequest{
		CompanyID:     currentCompany(r),
		DocumentID:    &documentID,
		PrincipalType: r.PostFormValue("principal_type"),
		PrincipalID:   &principalID,
		Permission:    r.PostFormValue("permission"),
		Effect:        r.PostFormValue("effect"),
		GrantedBy:     currentUser(r),
		ExpiresAt:     expiresAt,
	}
	if _, err := h.service.AddACL(r.Context(), req); err != nil {
		h.logger.Warn("create acl", slog.Int64("document_id", documentID), slog.Any("error", err))
		msg := shared.UserSafeMessage(err)
		switch {
		case errors.Is(err, documents.ErrACLExists):
			msg = "This principal already has that permission on the document."
		case errors.Is(err, documents.ErrInvalidACL):
			msg = "Principal type (USER or ROLE), principal id, permission and effect (ALLOW or DENY) are required."
		}
		h.redirectWithFlash(w, r, aclPath, "danger", msg)
		return
	}
	h.redirectWithFlash(w, r, aclPath, "success", "Access entry added")
}

// parseACLExpiry accepts an empty value (no expiry), RFC 3339, or the
// datetime-local form value YYYY-MM-DDTHH:MM (interpreted as UTC).
func parseACLExpiry(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return &t, nil
	}
	t, err := time.Parse("2006-01-02T15:04", raw)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// clientIP returns the request's client address as the router leaves it:
// chi's RealIP middleware may already have replaced RemoteAddr with a bare
// IP, otherwise it is host:port.
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// ============================================================================
// Categories
// ============================================================================

func (h *Handler) listCategories(w http.ResponseWriter, r *http.Request) {
	companyID := currentCompany(r)
	categories, err := h.service.ListCategories(r.Context(), companyID)
	if err != nil {
		h.logger.Error("list categories", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/documents/categories.html", "Document Categories", map[string]any{
		"Categories": categories,
	})
}

func (h *Handler) newCategoryForm(w http.ResponseWriter, r *http.Request) {
	companyID := currentCompany(r)
	categories, _ := h.service.ListCategories(r.Context(), companyID)
	h.render(w, r, "pages/documents/category_new.html", "New Category", map[string]any{
		"Categories": categories, // for parent selection
	})
}

// ============================================================================
// Signatures & Retention
// ============================================================================

func (h *Handler) createChallenge(w http.ResponseWriter, r *http.Request) {
	docID := chi.URLParam(r, "id")
	versionID := parseInt64(chi.URLParam(r, "versionID"))
	actorID := currentUser(r)
	companyID := currentCompany(r)

	// Issue a 5-minute challenge
	challenge, err := h.service.CreateSignatureChallenge(r.Context(), companyID, versionID, actorID, 5*60*1000*1000*1000)
	if err != nil {
		h.redirectWithFlash(w, r, "/documents/library/"+docID+"/versions", "danger", shared.UserSafeMessage(err))
		return
	}

	h.redirectWithFlash(w, r, "/documents/library/"+docID+"/versions", "success", "Signature challenge generated (check your email/SMS). POC ID: "+challenge.ChallengeID)
}

func (h *Handler) signVersion(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	docID := chi.URLParam(r, "id")
	versionID := parseInt64(chi.URLParam(r, "versionID"))
	actorID := currentUser(r)
	companyID := currentCompany(r)

	req := documents.SignDocumentRequest{
		CompanyID:         companyID,
		DocumentVersionID: versionID,
		ChallengeID:       r.PostFormValue("challenge_id"),
		SignerID:          actorID,
		Meaning:           r.PostFormValue("meaning"),
		IPAddress:         r.RemoteAddr,
		UserAgent:         r.UserAgent(),
	}

	_, err := h.service.SignDocument(r.Context(), req)
	if err != nil {
		h.redirectWithFlash(w, r, "/documents/library/"+docID+"/versions", "danger", shared.UserSafeMessage(err))
		return
	}

	h.redirectWithFlash(w, r, "/documents/library/"+docID+"/versions", "success", "Document signed successfully")
}

func (h *Handler) applyRetention(w http.ResponseWriter, r *http.Request) {
	docID := chi.URLParam(r, "id")
	versionID := parseInt64(chi.URLParam(r, "versionID"))

	err := h.service.ApplyRetention(r.Context(), versionID)
	if err != nil {
		h.redirectWithFlash(w, r, "/documents/library/"+docID+"/versions", "danger", shared.UserSafeMessage(err))
		return
	}

	h.redirectWithFlash(w, r, "/documents/library/"+docID+"/versions", "success", "Retention policy applied")
}

func (h *Handler) createCategory(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	actorID := currentUser(r)
	companyID := currentCompany(r)

	req := documents.CreateCategoryRequest{
		CompanyID:   companyID,
		Code:        strings.TrimSpace(r.PostFormValue("code")),
		Name:        strings.TrimSpace(r.PostFormValue("name")),
		Description: strings.TrimSpace(r.PostFormValue("description")),
		Active:      true,
		ActorID:     actorID,
	}
	if parentID := parseInt64(r.PostFormValue("parent_id")); parentID > 0 {
		req.ParentID = &parentID
	}

	_, err := h.service.CreateCategory(r.Context(), req)
	if err != nil {
		h.logger.Warn("create category", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/documents/categories/new", "danger", shared.UserSafeMessage(err))
		return
	}
	h.redirectWithFlash(w, r, "/documents/categories", "success", "Category created")
}

// ============================================================================
// Classifications
// ============================================================================

func (h *Handler) listClassifications(w http.ResponseWriter, r *http.Request) {
	classifications, err := h.service.ListClassifications(r.Context())
	if err != nil {
		h.logger.Error("list classifications", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/documents/classifications.html", "Document Classifications", map[string]any{
		"Classifications": classifications,
	})
}

// ============================================================================
// Helpers
// ============================================================================

func (h *Handler) render(w http.ResponseWriter, r *http.Request, tpl, title string, data any) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)
	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}
	viewData := view.TemplateData{
		Title:       title,
		CSRFToken:   csrfToken,
		Flash:       flash,
		CurrentPath: r.URL.Path,
		Data:        data,
	}
	if err := h.templates.Render(w, tpl, viewData); err != nil && h.logger != nil {
		h.logger.Error("render documents template", slog.Any("error", err))
	}
}

func (h *Handler) redirectWithFlash(w http.ResponseWriter, r *http.Request, location, kind, message string) {
	if sess := shared.SessionFromContext(r.Context()); sess != nil {
		sess.AddFlash(shared.FlashMessage{Kind: kind, Message: message})
	}
	http.Redirect(w, r, location, http.StatusSeeOther)
}

func currentUser(r *http.Request) int64 {
	sess := shared.SessionFromContext(r.Context())
	if sess == nil {
		return 0
	}
	id, _ := strconv.ParseInt(sess.User(), 10, 64)
	return id
}

func currentCompany(r *http.Request) int64 {
	sess := shared.SessionFromContext(r.Context())
	if sess == nil {
		return 0
	}
	id, _ := strconv.ParseInt(sess.Get("company_id"), 10, 64)
	return id
}

func parseInt64(value string) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// ============================================================================
// Advanced Documents (OCR, Collaboration, Search)
// ============================================================================

func (h *Handler) processOCR(w http.ResponseWriter, r *http.Request) {
	versionID := parseInt64(chi.URLParam(r, "versionID"))
	if versionID <= 0 {
		shared.JSONErrorFrom(w, http.StatusBadRequest, errors.New("documents: valid version id required"))
		return
	}
	if h.jobs == nil {
		shared.JSONErrorFrom(w, http.StatusServiceUnavailable, errors.New("documents: OCR worker is not configured"))
		return
	}
	version, err := h.service.GetVersion(r.Context(), versionID)
	if err != nil {
		shared.JSONErrorFrom(w, http.StatusNotFound, err)
		return
	}
	companyID := currentCompany(r)
	if companyID <= 0 || version.CompanyID != companyID || version.BlobID == nil {
		shared.JSONErrorFrom(w, http.StatusBadRequest, errors.New("documents: version is not available for this company"))
		return
	}
	jobID, err := h.service.InitiateOCRJob(r.Context(), companyID, versionID, *version.BlobID)
	if err != nil {
		shared.JSONErrorFrom(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := h.jobs.EnqueueDocumentOCR(r.Context(), jobID); err != nil {
		shared.JSONErrorFrom(w, http.StatusServiceUnavailable, err)
		return
	}
	shared.JSONResponse(w, http.StatusAccepted, map[string]any{"job_id": jobID, "status": "PENDING"})
}

func (h *Handler) createCollaborationSession(w http.ResponseWriter, r *http.Request) {
	var in documents.CollaborationSession
	if err := shared.DecodeJSON(r, &in); err != nil {
		shared.JSONErrorFrom(w, http.StatusBadRequest, err)
		return
	}
	in.CompanyID = currentCompany(r)
	in.HostUserID = currentUser(r)
	created, err := h.service.CreateCollaborationSession(r.Context(), in)
	if err != nil {
		shared.JSONErrorFrom(w, http.StatusInternalServerError, err)
		return
	}
	shared.JSONResponse(w, http.StatusCreated, created)
}

func (h *Handler) recordCollaborationChange(w http.ResponseWriter, r *http.Request) {
	var in documents.CollaborationChange
	if err := shared.DecodeJSON(r, &in); err != nil {
		shared.JSONErrorFrom(w, http.StatusBadRequest, err)
		return
	}
	in.CompanyID = currentCompany(r)
	in.ActorID = currentUser(r)
	in.SessionID = parseInt64(chi.URLParam(r, "sessionID"))
	created, err := h.service.RecordCollaborationChange(r.Context(), in)
	if err != nil {
		shared.JSONErrorFrom(w, http.StatusInternalServerError, err)
		return
	}
	shared.JSONResponse(w, http.StatusCreated, created)
}

func (h *Handler) searchContent(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	companyID := currentCompany(r)
	if companyID <= 0 || strings.TrimSpace(query) == "" {
		shared.JSONErrorFrom(w, http.StatusBadRequest, errors.New("documents: company and search query are required"))
		return
	}
	results, err := h.service.SearchContent(r.Context(), companyID, query)
	if err != nil {
		shared.JSONErrorFrom(w, http.StatusInternalServerError, err)
		return
	}
	shared.JSONResponse(w, http.StatusOK, results)
}
