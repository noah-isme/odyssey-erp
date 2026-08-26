package documentshttp

import (
	"context"
	"errors"
	"io"
	"log/slog"
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
	audit     *shared.AuditLogger
}

// NewHandler constructs a Handler value.
func NewHandler(logger *slog.Logger, service *documents.Service, templates *view.Engine, csrf *shared.CSRFManager, rbac rbac.Middleware, jobsClient *jobs.Client, pool *pgxpool.Pool) *Handler {
	var audit *shared.AuditLogger
	if pool != nil {
		audit = shared.NewAuditLogger(pool)
	}
	return &Handler{
		logger:    logger,
		service:   service,
		templates: templates,
		csrf:      csrf,
		rbac:      rbac,
		jobs:      jobsClient,
		pool:      pool,
		audit:     audit,
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
		r.With(h.rbac.RequireAny(shared.PermDocumentsShare, shared.PermDocumentsAdmin)).Get("/{id}/acl", h.listACLs)
		r.With(h.rbac.RequireAny(shared.PermDocumentsShare, shared.PermDocumentsAdmin)).Post("/{id}/acl", h.addACL)
		r.With(h.rbac.RequireAny(shared.PermDocumentsShare, shared.PermDocumentsAdmin)).Post("/{id}/acl/{aclID}/delete", h.removeACL)
		r.With(h.rbac.RequireAny(shared.PermDocumentsVersion)).Post("/{id}/versions", h.createVersion)
		r.With(h.rbac.RequireAny(shared.PermDocumentsView)).Get("/{id}/versions/{versionID}/download", h.downloadVersion)
		r.With(h.rbac.RequireAny(shared.PermDocumentsReview)).Post("/{id}/versions/{versionID}/submit-review", h.submitForReview)
		r.With(h.rbac.RequireAny(shared.PermDocumentsReview, shared.PermDocumentsApprove)).Post("/{id}/versions/{versionID}/review", h.recordReviewDecision)
		r.With(h.rbac.RequireAny(shared.PermDocumentsSign)).Post("/{id}/versions/{versionID}/challenge", h.createChallenge)
		r.With(h.rbac.RequireAny(shared.PermDocumentsSign)).Post("/{id}/versions/{versionID}/sign", h.signVersion)
		r.With(h.rbac.RequireAny(shared.PermDocumentsRetentionManage, shared.PermDocumentsAdmin)).Post("/{id}/versions/{versionID}/retention", h.applyRetention)

		// Advanced Documents
		r.With(h.rbac.RequireAny(shared.PermDocumentsVersion)).Post("/{id}/versions/{versionID}/ocr", h.processOCR)
		r.With(h.rbac.RequireAny(shared.PermDocumentsVersion)).Post("/{id}/sessions", h.createCollaborationSession)
		r.With(h.rbac.RequireAny(shared.PermDocumentsVersion)).Post("/{id}/sessions/{sessionID}/changes", h.recordCollaborationChange)
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
	if companyID <= 0 {
		h.writeAccessError(w, r, errors.New("documents: active company is required"))
		return
	}
	filter := documents.ListFilter{
		CompanyID: companyID,
		Search:    r.URL.Query().Get("q"),
		Limit:     50,
		Offset:    0,
	}
	if categoryID := parseInt64(r.URL.Query().Get("category_id")); categoryID > 0 {
		filter.CategoryID = &categoryID
	}
	if classificationID := parseInt64(r.URL.Query().Get("classification_id")); classificationID > 0 {
		filter.ClassificationID = &classificationID
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
	classifications = classificationsForCompany(classifications, companyID)

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
	if companyID <= 0 {
		h.writeAccessError(w, r, errors.New("documents: active company is required"))
		return
	}
	categories, _ := h.service.ListCategories(r.Context(), companyID)
	classifications, _ := h.service.ListClassifications(r.Context())
	classifications = classificationsForCompany(classifications, companyID)
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
	if actorID <= 0 || companyID <= 0 {
		h.redirectWithFlash(w, r, "/documents/library/new", "danger", "An authenticated company session is required")
		return
	}
	categoryID := parseInt64(r.PostFormValue("category_id"))
	classificationID := parseInt64(r.PostFormValue("classification_id"))
	if categoryID <= 0 || classificationID <= 0 {
		h.redirectWithFlash(w, r, "/documents/library/new", "danger", "Category and classification are required")
		return
	}
	category, err := h.service.GetCategory(r.Context(), categoryID)
	if err != nil || category.CompanyID != companyID {
		h.redirectWithFlash(w, r, "/documents/library/new", "danger", "Category is not available for this company")
		return
	}
	classification, err := h.service.GetClassification(r.Context(), classificationID)
	if err != nil || classification.CompanyID != companyID {
		h.redirectWithFlash(w, r, "/documents/library/new", "danger", "Classification is not available for this company")
		return
	}

	req := documents.CreateDocumentRequest{
		CompanyID:        companyID,
		Title:            strings.TrimSpace(r.PostFormValue("title")),
		Description:      strings.TrimSpace(r.PostFormValue("description")),
		CategoryID:       categoryID,
		ClassificationID: classificationID,
		OwnerID:          actorID,
		ActorID:          actorID,
	}

	doc, err := h.service.Create(r.Context(), req)
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("create document", slog.Any("error", err))
		}
		h.redirectWithFlash(w, r, "/documents/library/new", "danger", shared.UserSafeMessage(err))
		return
	}
	if err := h.recordAudit(r.Context(), actorID, "CREATE", "document", strconv.FormatInt(doc.ID, 10), map[string]any{
		"company_id": companyID, "number": doc.Number, "title": doc.Title,
	}); err != nil {
		h.logError("audit create document", err)
	}
	h.redirectWithFlash(w, r, "/documents/library/"+strconv.FormatInt(doc.ID, 10), "success", "Document "+doc.Number+" created")
}

func (h *Handler) getDocument(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	doc, err := h.documentForCompany(r.Context(), id, currentCompany(r))
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
	acls, _ := h.service.ListACLs(r.Context(), doc.CompanyID, &doc.ID, nil)
	h.render(w, r, "pages/documents/document_detail.html", doc.Number+" – "+doc.Title, map[string]any{
		"Document": doc,
		"Versions": versions,
		"ACLs":     acls,
		"Statuses": []documents.Status{
			documents.StatusDraft, documents.StatusSubmitted, documents.StatusApproved,
			documents.StatusPublished, documents.StatusArchived,
		},
	})
}

func (h *Handler) listVersions(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	companyID := currentCompany(r)
	doc, err := h.documentForCompany(r.Context(), id, companyID)
	if err != nil {
		if errors.Is(err, documents.ErrDocumentNotFound) {
			http.NotFound(w, r)
			return
		}
		h.writeAccessError(w, r, err)
		return
	}
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
	acls, _ := h.service.ListACLs(r.Context(), companyID, &id, nil)
	h.render(w, r, "pages/documents/versions.html", "Document Versions", map[string]any{
		"Document": doc,
		"Versions": versions,
		"ACLs":     acls,
	})
}

func (h *Handler) createVersion(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	companyID := currentCompany(r)
	doc, err := h.documentForCompany(r.Context(), id, companyID)
	if err != nil {
		if errors.Is(err, documents.ErrDocumentNotFound) {
			http.NotFound(w, r)
			return
		}
		h.writeAccessError(w, r, err)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	actorID := currentUser(r)
	if actorID <= 0 {
		h.writeAccessError(w, r, errors.New("documents: authenticated user is required"))
		return
	}

	// Get existing versions to determine next version number
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
		CompanyID:        companyID,
		DocumentID:       id,
		VersionNumber:    nextVersion,
		Description:      strings.TrimSpace(r.PostFormValue("description")), // used as change summary
		ClassificationID: doc.ClassificationID,
		ActorID:          actorID,
	}

	// Extract the file
	file, header, err := r.FormFile("file")
	if err != nil {
		h.logger.Warn("extract file", slog.Any("error", err))
		http.Error(w, "File upload missing or invalid", http.StatusBadRequest)
		return
	}
	defer func() { _ = file.Close() }()

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
	if err := h.recordAudit(r.Context(), actorID, "CREATE", "document_version", strconv.FormatInt(version.ID, 10), map[string]any{
		"company_id": companyID, "document_id": id, "version_number": version.VersionNumber,
	}); err != nil {
		h.logError("audit create document version", err)
	}
	h.redirectWithFlash(w, r, "/documents/library/"+strconv.FormatInt(id, 10), "success",
		"Version "+strconv.Itoa(version.VersionNumber)+" created")
}

// listACLs returns the document-specific ACL entries. The company and
// document lookup is deliberately performed before loading ACLs so a caller
// cannot enumerate ACLs for another tenant by guessing a document ID.
func (h *Handler) listACLs(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	doc, err := h.documentForCompany(r.Context(), id, currentCompany(r))
	if err != nil {
		if errors.Is(err, documents.ErrDocumentNotFound) {
			http.NotFound(w, r)
			return
		}
		h.writeAccessError(w, r, err)
		return
	}
	acls, err := h.service.ListACLs(r.Context(), doc.CompanyID, &doc.ID, nil)
	if err != nil {
		h.logError("list document ACLs", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		shared.JSONResponse(w, http.StatusOK, acls)
		return
	}
	h.render(w, r, "pages/documents/acl.html", "Document Access Control", map[string]any{
		"Document": doc,
		"ACLs":     acls,
	})
}

// addACL creates a document-specific ACL entry. Principal and permission
// values are allow-listed here because the service/repository intentionally
// accepts the domain request as-is for internal callers.
func (h *Handler) addACL(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	id := parseInt64(chi.URLParam(r, "id"))
	companyID := currentCompany(r)
	doc, err := h.documentForCompany(r.Context(), id, companyID)
	if err != nil {
		if errors.Is(err, documents.ErrDocumentNotFound) {
			http.NotFound(w, r)
			return
		}
		h.writeAccessError(w, r, err)
		return
	}
	actorID := currentUser(r)
	if actorID <= 0 {
		h.writeAccessError(w, r, errors.New("documents: authenticated user is required"))
		return
	}

	principalType := strings.ToUpper(strings.TrimSpace(r.PostFormValue("principal_type")))
	if principalType != "USER" && principalType != "ROLE" {
		h.redirectWithFlash(w, r, documentACLPath(id), "danger", "Principal type must be USER or ROLE")
		return
	}
	principalID := parseInt64(r.PostFormValue("principal_id"))
	if principalID <= 0 {
		h.redirectWithFlash(w, r, documentACLPath(id), "danger", "A positive principal ID is required")
		return
	}
	permission := strings.ToUpper(strings.TrimSpace(r.PostFormValue("permission")))
	if !validACLPermission(permission) {
		h.redirectWithFlash(w, r, documentACLPath(id), "danger", "Unsupported document permission")
		return
	}
	effect := strings.ToUpper(strings.TrimSpace(r.PostFormValue("effect")))
	if effect == "" {
		effect = "ALLOW"
	}
	if effect != "ALLOW" && effect != "DENY" {
		h.redirectWithFlash(w, r, documentACLPath(id), "danger", "ACL effect must be ALLOW or DENY")
		return
	}
	var expiresAt *time.Time
	if rawExpiry := strings.TrimSpace(r.PostFormValue("expires_at")); rawExpiry != "" {
		parsed, parseErr := time.Parse(time.RFC3339, rawExpiry)
		if parseErr != nil {
			// datetime-local controls omit the zone; treat them as UTC rather than
			// silently creating a permanent rule.
			parsed, parseErr = time.ParseInLocation("2006-01-02T15:04", rawExpiry, time.UTC)
		}
		if parseErr != nil || !parsed.After(time.Now().UTC()) {
			h.redirectWithFlash(w, r, documentACLPath(id), "danger", "ACL expiry must be a future UTC timestamp")
			return
		}
		expiresAt = &parsed
	}

	acl, err := h.service.AddACL(r.Context(), documents.CreateACLRequest{
		CompanyID:     companyID,
		DocumentID:    &doc.ID,
		PrincipalType: principalType,
		PrincipalID:   &principalID,
		Permission:    permission,
		Effect:        effect,
		GrantedBy:     actorID,
		ExpiresAt:     expiresAt,
	})
	if err != nil {
		h.logError("add document ACL", err)
		h.redirectWithFlash(w, r, documentACLPath(id), "danger", shared.UserSafeMessage(err))
		return
	}
	if err := h.recordAudit(r.Context(), actorID, "ACL_ADD", "document_acl", strconv.FormatInt(acl.ID, 10), map[string]any{
		"company_id": companyID, "document_id": doc.ID, "principal_type": principalType,
		"principal_id": principalID, "permission": permission, "effect": effect,
	}); err != nil {
		h.logError("audit add document ACL", err)
	}
	h.redirectWithFlash(w, r, documentACLPath(id), "success", "Document access rule added")
}

func (h *Handler) removeACL(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	aclID := parseInt64(chi.URLParam(r, "aclID"))
	doc, err := h.documentForCompany(r.Context(), id, currentCompany(r))
	if err != nil {
		if errors.Is(err, documents.ErrDocumentNotFound) {
			http.NotFound(w, r)
			return
		}
		h.writeAccessError(w, r, err)
		return
	}
	if aclID <= 0 {
		h.redirectWithFlash(w, r, documentACLPath(id), "danger", "A valid ACL ID is required")
		return
	}
	acls, err := h.service.ListACLs(r.Context(), doc.CompanyID, &doc.ID, nil)
	if err != nil {
		h.logError("load document ACL", err)
		h.redirectWithFlash(w, r, documentACLPath(id), "danger", "Unable to verify access rule")
		return
	}
	owned := false
	for _, acl := range acls {
		if acl.ID == aclID {
			owned = true
			break
		}
	}
	if !owned {
		// Hide whether an ACL ID exists in another company/document.
		http.NotFound(w, r)
		return
	}
	if err := h.service.RemoveACL(r.Context(), aclID); err != nil {
		h.logError("remove document ACL", err)
		h.redirectWithFlash(w, r, documentACLPath(id), "danger", shared.UserSafeMessage(err))
		return
	}
	if err := h.recordAudit(r.Context(), currentUser(r), "ACL_REMOVE", "document_acl", strconv.FormatInt(aclID, 10), map[string]any{
		"company_id": doc.CompanyID, "document_id": doc.ID,
	}); err != nil {
		h.logError("audit remove document ACL", err)
	}
	h.redirectWithFlash(w, r, documentACLPath(id), "success", "Document access rule removed")
}

func (h *Handler) submitForReview(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	versionID := parseInt64(chi.URLParam(r, "versionID"))
	doc, version, err := h.documentVersionForCompany(r.Context(), id, versionID, currentCompany(r))
	if err != nil {
		h.handleDocumentActionError(w, r, id, err)
		return
	}
	if currentUser(r) <= 0 {
		h.handleDocumentActionError(w, r, id, errors.New("documents: authenticated user is required"))
		return
	}
	if _, err := h.service.SubmitForReview(r.Context(), version.ID, currentUser(r)); err != nil {
		h.logError("submit document version for review", err)
		h.redirectWithFlash(w, r, documentVersionsPath(doc.ID), "danger", shared.UserSafeMessage(err))
		return
	}
	if err := h.recordAudit(r.Context(), currentUser(r), "SUBMIT_REVIEW", "document_version", strconv.FormatInt(version.ID, 10), map[string]any{
		"company_id": version.CompanyID, "document_id": doc.ID,
	}); err != nil {
		h.logError("audit submit document version for review", err)
	}
	h.redirectWithFlash(w, r, documentVersionsPath(doc.ID), "success", "Version submitted for review")
}

func (h *Handler) recordReviewDecision(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	id := parseInt64(chi.URLParam(r, "id"))
	versionID := parseInt64(chi.URLParam(r, "versionID"))
	doc, version, err := h.documentVersionForCompany(r.Context(), id, versionID, currentCompany(r))
	if err != nil {
		h.handleDocumentActionError(w, r, id, err)
		return
	}
	actorID := currentUser(r)
	if actorID <= 0 {
		h.handleDocumentActionError(w, r, id, errors.New("documents: authenticated user is required"))
		return
	}
	stepID := parseInt64(r.PostFormValue("step_id"))
	decision := strings.ToUpper(strings.TrimSpace(r.PostFormValue("decision")))
	if stepID <= 0 || (decision != "APPROVED" && decision != "REJECTED") {
		h.redirectWithFlash(w, r, documentVersionsPath(doc.ID), "danger", "A valid review step and decision are required")
		return
	}
	_, err = h.service.RecordReviewDecision(r.Context(), documents.ReviewDecisionRequest{
		CompanyID:         version.CompanyID,
		DocumentVersionID: version.ID,
		StepID:            stepID,
		ReviewerID:        actorID,
		Decision:          decision,
		Comments:          strings.TrimSpace(r.PostFormValue("comments")),
	})
	if err != nil {
		h.logError("record document review decision", err)
		h.redirectWithFlash(w, r, documentVersionsPath(doc.ID), "danger", shared.UserSafeMessage(err))
		return
	}
	if err := h.recordAudit(r.Context(), actorID, "REVIEW_DECISION", "document_review_step", strconv.FormatInt(stepID, 10), map[string]any{
		"company_id": version.CompanyID, "document_version_id": version.ID, "decision": decision,
	}); err != nil {
		h.logError("audit document review decision", err)
	}
	h.redirectWithFlash(w, r, documentVersionsPath(doc.ID), "success", "Review decision recorded")
}

// downloadVersion streams a version only after the module permission and the
// record-level document ACL both authorize the active user.
func (h *Handler) downloadVersion(w http.ResponseWriter, r *http.Request) {
	docID := parseInt64(chi.URLParam(r, "id"))
	versionID := parseInt64(chi.URLParam(r, "versionID"))
	companyID := currentCompany(r)
	actorID := currentUser(r)
	doc, version, err := h.documentVersionForCompany(r.Context(), docID, versionID, companyID)
	if err != nil {
		h.handleDocumentActionError(w, r, docID, err)
		return
	}
	if actorID <= 0 {
		h.writeAccessError(w, r, errors.New("documents: authenticated user is required"))
		return
	}
	allowed, err := h.service.CheckAccess(r.Context(), companyID, actorID, doc.ID, "READ")
	if err != nil {
		h.logError("check document download access", err)
		h.writeAccessError(w, r, err)
		return
	}
	if !allowed {
		h.writeAccessError(w, r, shared.ErrForbidden)
		return
	}
	reader, _, contentType, err := h.service.DownloadVersion(r.Context(), version.ID, actorID)
	if err != nil {
		h.logError("download document version", err)
		h.handleDocumentActionError(w, r, docID, err)
		return
	}
	defer reader.Close()
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	filename := strings.NewReplacer(`"`, "", "\r", "", "\n", "").Replace(doc.Number + "-v" + strconv.Itoa(version.VersionNumber) + ".bin")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	_, _ = io.Copy(w, reader)
}

// ============================================================================
// Categories
// ============================================================================

func (h *Handler) listCategories(w http.ResponseWriter, r *http.Request) {
	companyID := currentCompany(r)
	if companyID <= 0 {
		h.writeAccessError(w, r, errors.New("documents: active company is required"))
		return
	}
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
	if companyID <= 0 {
		h.writeAccessError(w, r, errors.New("documents: active company is required"))
		return
	}
	categories, _ := h.service.ListCategories(r.Context(), companyID)
	h.render(w, r, "pages/documents/category_new.html", "New Category", map[string]any{
		"Categories": categories, // for parent selection
	})
}

// ============================================================================
// Signatures & Retention
// ============================================================================

func (h *Handler) createChallenge(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	docID := parseInt64(chi.URLParam(r, "id"))
	versionID := parseInt64(chi.URLParam(r, "versionID"))
	actorID := currentUser(r)
	companyID := currentCompany(r)
	doc, version, err := h.documentVersionForCompany(r.Context(), docID, versionID, companyID)
	if err != nil {
		h.handleDocumentActionError(w, r, docID, err)
		return
	}
	if actorID <= 0 {
		h.handleDocumentActionError(w, r, docID, errors.New("documents: authenticated user is required"))
		return
	}

	// Issue a 5-minute challenge
	challenge, err := h.service.CreateSignatureChallenge(r.Context(), companyID, version.ID, actorID, 5*60*1000*1000*1000, r.PostFormValue("meaning"))
	if err != nil {
		h.logError("create document signature challenge", err)
		h.redirectWithFlash(w, r, documentVersionsPath(doc.ID), "danger", shared.UserSafeMessage(err))
		return
	}
	if err := h.recordAudit(r.Context(), actorID, "CHALLENGE_CREATE", "document_version", strconv.FormatInt(version.ID, 10), map[string]any{
		"company_id": companyID, "meaning": challenge.Meaning, "expiry": challenge.Expiry.UTC().Format(time.RFC3339),
	}); err != nil {
		h.logError("audit create document signature challenge", err)
	}

	h.redirectWithFlash(w, r, documentVersionsPath(doc.ID), "success", "Signature challenge generated (check your email/SMS). POC ID: "+challenge.ChallengeID)
}

func (h *Handler) signVersion(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	docID := parseInt64(chi.URLParam(r, "id"))
	versionID := parseInt64(chi.URLParam(r, "versionID"))
	actorID := currentUser(r)
	companyID := currentCompany(r)
	doc, version, err := h.documentVersionForCompany(r.Context(), docID, versionID, companyID)
	if err != nil {
		h.handleDocumentActionError(w, r, docID, err)
		return
	}
	if actorID <= 0 {
		h.handleDocumentActionError(w, r, docID, errors.New("documents: authenticated user is required"))
		return
	}

	req := documents.SignDocumentRequest{
		CompanyID:         companyID,
		DocumentVersionID: version.ID,
		ChallengeID:       r.PostFormValue("challenge_id"),
		SignerID:          actorID,
		Meaning:           r.PostFormValue("meaning"),
		IPAddress:         r.RemoteAddr,
		UserAgent:         r.UserAgent(),
	}

	sig, err := h.service.SignDocument(r.Context(), req)
	if err != nil {
		h.logError("sign document version", err)
		h.redirectWithFlash(w, r, documentVersionsPath(doc.ID), "danger", shared.UserSafeMessage(err))
		return
	}
	if err := h.recordAudit(r.Context(), actorID, "SIGN", "document_signature", strconv.FormatInt(sig.ID, 10), map[string]any{
		"company_id": companyID, "document_version_id": version.ID, "challenge_id": sig.ChallengeID,
	}); err != nil {
		h.logError("audit sign document version", err)
	}

	h.redirectWithFlash(w, r, documentVersionsPath(doc.ID), "success", "Document signed successfully")
}

func (h *Handler) applyRetention(w http.ResponseWriter, r *http.Request) {
	docID := parseInt64(chi.URLParam(r, "id"))
	versionID := parseInt64(chi.URLParam(r, "versionID"))
	doc, version, err := h.documentVersionForCompany(r.Context(), docID, versionID, currentCompany(r))
	if err != nil {
		h.handleDocumentActionError(w, r, docID, err)
		return
	}
	actorID := currentUser(r)
	if actorID <= 0 {
		h.handleDocumentActionError(w, r, docID, errors.New("documents: authenticated user is required"))
		return
	}

	err = h.service.ApplyRetention(r.Context(), version.ID)
	if err != nil {
		h.logError("apply document retention", err)
		h.redirectWithFlash(w, r, documentVersionsPath(doc.ID), "danger", shared.UserSafeMessage(err))
		return
	}
	if err := h.recordAudit(r.Context(), actorID, "RETENTION_APPLY", "document_version", strconv.FormatInt(version.ID, 10), map[string]any{
		"company_id": version.CompanyID, "document_id": doc.ID,
	}); err != nil {
		h.logError("audit apply document retention", err)
	}

	h.redirectWithFlash(w, r, documentVersionsPath(doc.ID), "success", "Retention policy applied")
}

func (h *Handler) createCategory(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	actorID := currentUser(r)
	companyID := currentCompany(r)
	if actorID <= 0 || companyID <= 0 {
		h.redirectWithFlash(w, r, "/documents/categories/new", "danger", "An authenticated company session is required")
		return
	}

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

	category, err := h.service.CreateCategory(r.Context(), req)
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("create category", slog.Any("error", err))
		}
		h.redirectWithFlash(w, r, "/documents/categories/new", "danger", shared.UserSafeMessage(err))
		return
	}
	if err := h.recordAudit(r.Context(), actorID, "CREATE", "document_category", strconv.FormatInt(category.ID, 10), map[string]any{
		"company_id": companyID, "code": category.Code, "name": category.Name,
	}); err != nil {
		h.logError("audit create document category", err)
	}
	h.redirectWithFlash(w, r, "/documents/categories", "success", "Category created")
}

// ============================================================================
// Classifications
// ============================================================================

func (h *Handler) listClassifications(w http.ResponseWriter, r *http.Request) {
	companyID := currentCompany(r)
	if companyID <= 0 {
		h.writeAccessError(w, r, errors.New("documents: active company is required"))
		return
	}
	classifications, err := h.service.ListClassifications(r.Context())
	if err != nil {
		h.logger.Error("list classifications", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/documents/classifications.html", "Document Classifications", map[string]any{
		"Classifications": classificationsForCompany(classifications, companyID),
	})
}

// ============================================================================
// Helpers
// ============================================================================

func (h *Handler) documentForCompany(ctx context.Context, id, companyID int64) (documents.Document, error) {
	if h.service == nil {
		return documents.Document{}, errors.New("documents: service is not configured")
	}
	if id <= 0 || companyID <= 0 {
		return documents.Document{}, documents.ErrDocumentNotFound
	}
	doc, err := h.service.Get(ctx, id)
	if err != nil {
		return documents.Document{}, err
	}
	if doc.CompanyID != companyID {
		// Treat a foreign record as missing. This avoids exposing whether a
		// guessed identifier belongs to another tenant.
		return documents.Document{}, documents.ErrDocumentNotFound
	}
	return doc, nil
}

func (h *Handler) documentVersionForCompany(ctx context.Context, documentID, versionID, companyID int64) (documents.Document, documents.DocumentVersion, error) {
	if documentID <= 0 || companyID <= 0 {
		return documents.Document{}, documents.DocumentVersion{}, documents.ErrDocumentNotFound
	}
	if versionID <= 0 {
		return documents.Document{}, documents.DocumentVersion{}, documents.ErrDocumentVersionNotFound
	}
	doc, err := h.documentForCompany(ctx, documentID, companyID)
	if err != nil {
		return documents.Document{}, documents.DocumentVersion{}, err
	}
	if h.service == nil {
		return documents.Document{}, documents.DocumentVersion{}, documents.ErrDocumentVersionNotFound
	}
	version, err := h.service.GetVersion(ctx, versionID)
	if err != nil {
		return documents.Document{}, documents.DocumentVersion{}, err
	}
	if version.CompanyID != companyID || version.DocumentID != doc.ID {
		return documents.Document{}, documents.DocumentVersion{}, documents.ErrDocumentVersionNotFound
	}
	return doc, version, nil
}

func (h *Handler) handleDocumentActionError(w http.ResponseWriter, r *http.Request, documentID int64, err error) {
	if errors.Is(err, documents.ErrDocumentNotFound) || errors.Is(err, documents.ErrDocumentVersionNotFound) {
		http.NotFound(w, r)
		return
	}
	if strings.Contains(err.Error(), "authenticated") || strings.Contains(err.Error(), "active company") {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}
	h.logError("document action authorization", err)
	if documentID > 0 {
		h.redirectWithFlash(w, r, documentVersionsPath(documentID), "danger", shared.UserSafeMessage(err))
		return
	}
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func (h *Handler) writeAccessError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, documents.ErrDocumentNotFound) || errors.Is(err, documents.ErrDocumentVersionNotFound) {
		http.NotFound(w, r)
		return
	}
	if h.logger != nil {
		h.logger.Warn("document access denied", slog.Any("error", err))
	}
	http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
}

func (h *Handler) logError(message string, err error) {
	if h.logger != nil {
		h.logger.Error(message, slog.Any("error", err))
	}
}

func (h *Handler) recordAudit(ctx context.Context, actorID int64, action, entity, entityID string, meta map[string]any) error {
	if h.audit == nil {
		return nil
	}
	return h.audit.Record(ctx, shared.AuditLog{
		ActorID:  actorID,
		Action:   "documents:" + strings.ToLower(action),
		Entity:   entity,
		EntityID: entityID,
		Meta:     meta,
	})
}

func classificationsForCompany(items []documents.DocumentClassification, companyID int64) []documents.DocumentClassification {
	filtered := make([]documents.DocumentClassification, 0, len(items))
	for _, item := range items {
		if item.CompanyID == companyID {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func validACLPermission(permission string) bool {
	switch permission {
	case "READ", "WRITE", "ADMIN", "APPROVE", "SIGN":
		return true
	default:
		return false
	}
}

func documentACLPath(documentID int64) string {
	return "/documents/library/" + strconv.FormatInt(documentID, 10) + "/acl"
}

func documentVersionsPath(documentID int64) string {
	return "/documents/library/" + strconv.FormatInt(documentID, 10) + "/versions"
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, tpl, title string, data any) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken := shared.CSRFTokenFromContext(r.Context())
	if h.csrf != nil {
		csrfToken, _ = h.csrf.EnsureToken(r.Context(), sess)
	}
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
	if h.templates == nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
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
	documentID := parseInt64(chi.URLParam(r, "id"))
	versionID := parseInt64(chi.URLParam(r, "versionID"))
	if versionID <= 0 {
		shared.JSONErrorFrom(w, http.StatusBadRequest, errors.New("documents: valid version id required"))
		return
	}
	if h.jobs == nil {
		shared.JSONErrorFrom(w, http.StatusServiceUnavailable, errors.New("documents: OCR worker is not configured"))
		return
	}
	_, version, err := h.documentVersionForCompany(r.Context(), documentID, versionID, currentCompany(r))
	if err != nil {
		if errors.Is(err, documents.ErrDocumentNotFound) || errors.Is(err, documents.ErrDocumentVersionNotFound) {
			shared.JSONErrorFrom(w, http.StatusNotFound, err)
			return
		}
		shared.JSONErrorFrom(w, http.StatusForbidden, err)
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
	documentID := parseInt64(chi.URLParam(r, "id"))
	companyID := currentCompany(r)
	_, version, err := h.documentVersionForCompany(r.Context(), documentID, in.VersionID, companyID)
	if err != nil {
		if errors.Is(err, documents.ErrDocumentNotFound) || errors.Is(err, documents.ErrDocumentVersionNotFound) {
			shared.JSONErrorFrom(w, http.StatusNotFound, err)
			return
		}
		shared.JSONErrorFrom(w, http.StatusForbidden, err)
		return
	}
	in.CompanyID = companyID
	in.VersionID = version.ID
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
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	companyID := currentCompany(r)
	if companyID <= 0 {
		shared.JSONErrorFrom(w, http.StatusBadRequest, errors.New("documents: company and search query are required"))
		return
	}
	// The route is linked from browser navigation as well as consumed by the
	// search client. A bare browser request should land on a usable workspace;
	// JSON callers receive an empty result set until a query is supplied.
	if query == "" {
		if strings.Contains(r.Header.Get("Accept"), "application/json") {
			shared.JSONResponse(w, http.StatusOK, []documents.Document{})
			return
		}
		h.listDocuments(w, r)
		return
	}
	results, err := h.service.SearchContent(r.Context(), companyID, query)
	if err != nil {
		shared.JSONErrorFrom(w, http.StatusInternalServerError, err)
		return
	}
	shared.JSONResponse(w, http.StatusOK, results)
}
