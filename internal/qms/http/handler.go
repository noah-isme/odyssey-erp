package qmshttp

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/outbox"
	"github.com/odyssey-erp/odyssey-erp/internal/qms"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// Handler wires HTTP endpoints for QMS.
type Handler struct {
	logger    *slog.Logger
	service   *qms.Service
	templates *view.Engine
	csrf      *shared.CSRFManager
	rbac      rbac.Middleware
	pool      *pgxpool.Pool
	outbox    *outbox.Repository
}

// NewHandler constructs a Handler value.
func NewHandler(logger *slog.Logger, service *qms.Service, templates *view.Engine, csrf *shared.CSRFManager, rbac rbac.Middleware, pool *pgxpool.Pool, ob *outbox.Repository) *Handler {
	return &Handler{
		logger:    logger,
		service:   service,
		templates: templates,
		csrf:      csrf,
		rbac:      rbac,
		pool:      pool,
		outbox:    ob,
	}
}

// MountRoutes registers HTTP routes for QMS.
func (h *Handler) MountRoutes(r chi.Router) {
	// Non-Conformance Reports
	r.Route("/ncrs", func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermQMSNCRView))
		r.Get("/", h.listNCRs)
		r.Get("/new", h.newNCRForm)
		r.With(h.rbac.RequireAny(shared.PermQMSNCRCreate)).Post("/", h.createNCR)
		r.Get("/{id}", h.getNCR)
		r.With(h.rbac.RequireAny(shared.PermQMSNCRManage)).Post("/{id}/status", h.updateNCRStatus)
		r.With(h.rbac.RequireAny(shared.PermQMSNCRManage)).Post("/{id}/disposition", h.recordDisposition)
	})

	// CAPAs
	r.Route("/capas", func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermQMSCAPAView))
		r.Get("/", h.listCAPAs)
		r.Get("/new", h.newCAPAForm)
		r.With(h.rbac.RequireAny(shared.PermQMSCAPACreate)).Post("/", h.createCAPA)
		r.Get("/{id}", h.getCAPA)
		r.With(h.rbac.RequireAny(shared.PermQMSCAPAManage)).Post("/{id}/status", h.updateCAPAStatus)
	})

	// Audits
	r.Route("/audits", func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermQMSAuditView))
		r.Get("/", h.listAudits)
		r.Get("/new", h.newAuditForm)
		r.With(h.rbac.RequireAny(shared.PermQMSAuditManage)).Post("/", h.createAudit)
		r.Get("/{id}", h.getAudit)
		r.With(h.rbac.RequireAny(shared.PermQMSAuditManage)).Post("/{id}/findings", h.addFinding)
		r.With(h.rbac.RequireAny(shared.PermQMSAuditManage)).Post("/{id}/findings/{findingID}/calibration", h.requestCalibration)
	})

	// Supplier Quality
	r.Route("/supplier-quality", func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermQMSSupplierQualityView))
		r.Get("/", h.listSupplierQuality)
		r.Get("/new", h.newSupplierQualityForm)
		r.With(h.rbac.RequireAny(shared.PermQMSSupplierQualityManage)).Post("/", h.createSupplierQuality)
		r.Get("/{id}", h.getSupplierQuality)
	})

	// Inspections
	r.Route("/inspections", func(r chi.Router) {
		r.Use(h.rbac.RequireAny("qms.inspection.view", shared.PermQMSAdmin))
		r.Get("/", h.listInspections)
		r.Get("/new", h.newInspection)
		r.With(h.rbac.RequireAny("qms.inspection.manage", shared.PermQMSAdmin)).Post("/", h.createInspection)
		r.Get("/{id}", h.getInspection)
	})

	// Complaints
	r.Route("/complaints", func(r chi.Router) {
		r.Use(h.rbac.RequireAny("qms.complaint.view", shared.PermQMSAdmin))
		r.Get("/", h.listComplaints)
		r.Get("/{id}", h.getComplaint)
	})
}

// ============================================================================
// NCRs
// ============================================================================

func (h *Handler) listNCRs(w http.ResponseWriter, r *http.Request) {
	companyID := currentCompany(r)
	filter := qms.ListNCRsFilter{
		CompanyID:  companyID,
		SourceType: r.URL.Query().Get("source_type"),
		Category:   r.URL.Query().Get("category"),
		Severity:   r.URL.Query().Get("severity"),
		Limit:      50,
		Offset:     0,
	}
	if s := r.URL.Query().Get("status"); s != "" {
		status := qms.NormaliseStatus(s)
		filter.Status = &status
	}

	ncrs, err := h.service.ListNCRs(r.Context(), filter)
	if err != nil {
		h.logger.Error("list ncrs", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/qms/ncrs.html", "Non-Conformance Reports", map[string]any{
		"NCRs":       ncrs,
		"Filter":     filter,
		"Severities": []string{"MINOR", "MAJOR", "CRITICAL"},
		"Categories": []string{"MATERIAL", "PROCESS", "PRODUCT", "DOCUMENTATION", "SERVICE"},
		"Statuses": []qms.Status{
			qms.NCRStatusOpen, qms.NCRStatusUnderReview,
			qms.NCRStatusDispositioned, qms.NCRStatusClosed, qms.NCRStatusCancelled,
		},
	})
}

func (h *Handler) newNCRForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "pages/qms/ncr_new.html", "New NCR", map[string]any{
		"Categories":  []string{"MATERIAL", "PROCESS", "PRODUCT", "DOCUMENTATION", "SERVICE"},
		"Severities":  []string{"MINOR", "MAJOR", "CRITICAL"},
		"SourceTypes": []string{"INTERNAL", "SUPPLIER", "CUSTOMER", "AUDIT", "PRODUCTION"},
	})
}

func (h *Handler) createNCR(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	actorID := currentUser(r)
	companyID := currentCompany(r)

	req := qms.CreateNCRRequest{
		CompanyID:       companyID,
		Title:           strings.TrimSpace(r.PostFormValue("title")),
		Description:     strings.TrimSpace(r.PostFormValue("description")),
		SourceType:      strings.ToUpper(strings.TrimSpace(r.PostFormValue("source_type"))),
		SourceReference: strings.TrimSpace(r.PostFormValue("source_reference")),
		Category:        strings.ToUpper(strings.TrimSpace(r.PostFormValue("category"))),
		Severity:        strings.ToUpper(strings.TrimSpace(r.PostFormValue("severity"))),
		DetectedBy:      actorID,
		DetectedLocation: strings.TrimSpace(r.PostFormValue("detected_location")),
		ActorID:         actorID,
	}
	if assigned := parseInt64(r.PostFormValue("assigned_to")); assigned > 0 {
		req.AssignedTo = &assigned
	}
	if tcd := strings.TrimSpace(r.PostFormValue("target_closure_date")); tcd != "" {
		if t, err := time.Parse("2006-01-02", tcd); err == nil {
			req.TargetClosureDate = &t
		}
	}

	ncr, err := h.service.CreateNCR(r.Context(), req)
	if err != nil {
		h.logger.Warn("create ncr", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/qms/ncrs/new", "danger", shared.UserSafeMessage(err))
		return
	}
	h.redirectWithFlash(w, r, "/qms/ncrs/"+strconv.FormatInt(ncr.ID, 10), "success", "NCR "+ncr.Number+" created")
}

func (h *Handler) getNCR(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	ncr, err := h.service.GetNCR(r.Context(), id)
	if err != nil {
		if errors.Is(err, qms.ErrNCRNotFound) {
			http.NotFound(w, r)
			return
		}
		h.logger.Error("get ncr", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/qms/ncr_detail.html", "NCR "+ncr.Number, map[string]any{
		"NCR": ncr,
		"DispositionTypes": []string{"REWORK", "REPAIR", "USE_AS_IS", "SCRAP", "RETURN_TO_SUPPLIER"},
		"Statuses": []qms.Status{
			qms.NCRStatusOpen, qms.NCRStatusUnderReview,
			qms.NCRStatusDispositioned, qms.NCRStatusClosed, qms.NCRStatusCancelled,
		},
	})
}

func (h *Handler) updateNCRStatus(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	id := parseInt64(chi.URLParam(r, "id"))
	status := qms.NormaliseStatus(r.PostFormValue("status"))
	actorID := currentUser(r)

	if _, err := h.service.UpdateNCRStatus(r.Context(), id, status, actorID); err != nil {
		h.logger.Warn("update ncr status", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/qms/ncrs/"+strconv.FormatInt(id, 10), "danger", shared.UserSafeMessage(err))
		return
	}
	h.redirectWithFlash(w, r, "/qms/ncrs/"+strconv.FormatInt(id, 10), "success", "NCR status updated")
}

func (h *Handler) recordDisposition(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	id := parseInt64(chi.URLParam(r, "id"))
	actorID := currentUser(r)

	req := qms.RecordDispositionRequest{
		NCRID:           id,
		DispositionType: strings.ToUpper(strings.TrimSpace(r.PostFormValue("disposition_type"))),
		Description:     strings.TrimSpace(r.PostFormValue("description")),
		ApprovedBy:      actorID,
		ActorID:         actorID,
	}
	if _, err := h.service.RecordDisposition(r.Context(), req); err != nil {
		h.logger.Warn("record ncr disposition", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/qms/ncrs/"+strconv.FormatInt(id, 10), "danger", shared.UserSafeMessage(err))
		return
	}
	h.redirectWithFlash(w, r, "/qms/ncrs/"+strconv.FormatInt(id, 10), "success", "Disposition recorded")
}

// ============================================================================
// CAPAs
// ============================================================================

func (h *Handler) listCAPAs(w http.ResponseWriter, r *http.Request) {
	companyID := currentCompany(r)
	filter := qms.ListCAPAsFilter{
		CompanyID:  companyID,
		SourceType: r.URL.Query().Get("source_type"),
		Priority:   r.URL.Query().Get("priority"),
		Limit:      50,
		Offset:     0,
	}
	if s := r.URL.Query().Get("status"); s != "" {
		status := qms.NormaliseStatus(s)
		filter.Status = &status
	}

	capas, err := h.service.ListCAPAs(r.Context(), filter)
	if err != nil {
		h.logger.Error("list capas", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/qms/capas.html", "Corrective Actions (CAPA)", map[string]any{
		"CAPAs":  capas,
		"Filter": filter,
		"Priorities": []string{"LOW", "MEDIUM", "HIGH", "CRITICAL"},
		"Statuses": []qms.Status{
			qms.CAPAStatusOpen, qms.CAPAStatusInProgress,
			qms.CAPAStatusVerifying, qms.CAPAStatusEffective, qms.CAPAStatusClosed,
		},
	})
}

func (h *Handler) newCAPAForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "pages/qms/capa_new.html", "New CAPA", map[string]any{
		"Priorities":       []string{"LOW", "MEDIUM", "HIGH", "CRITICAL"},
		"SourceTypes":      []string{"NCR", "AUDIT", "CUSTOMER_COMPLAINT", "REGULATORY", "INTERNAL"},
		"RootCauseMethods": []string{"FIVE_WHYS", "FISHBONE", "FAULT_TREE", "PARETO"},
	})
}

func (h *Handler) createCAPA(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	actorID := currentUser(r)
	companyID := currentCompany(r)

	req := qms.CreateCAPARequest{
		CompanyID:       companyID,
		Title:           strings.TrimSpace(r.PostFormValue("title")),
		Description:     strings.TrimSpace(r.PostFormValue("description")),
		SourceType:      strings.ToUpper(strings.TrimSpace(r.PostFormValue("source_type"))),
		SourceReference: strings.TrimSpace(r.PostFormValue("source_reference")),
		Priority:        strings.ToUpper(strings.TrimSpace(r.PostFormValue("priority"))),
		OwnerID:         actorID,
		RootCauseMethod: strings.ToUpper(strings.TrimSpace(r.PostFormValue("root_cause_method"))),
		ActorID:         actorID,
	}
	if td := strings.TrimSpace(r.PostFormValue("target_date")); td != "" {
		if t, err := time.Parse("2006-01-02", td); err == nil {
			req.TargetDate = &t
		}
	}

	capa, err := h.service.CreateCAPA(r.Context(), req)
	if err != nil {
		h.logger.Warn("create capa", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/qms/capas/new", "danger", shared.UserSafeMessage(err))
		return
	}
	h.redirectWithFlash(w, r, "/qms/capas/"+strconv.FormatInt(capa.ID, 10), "success", "CAPA "+capa.Number+" created")
}

func (h *Handler) getCAPA(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	capa, err := h.service.GetCAPA(r.Context(), id)
	if err != nil {
		if errors.Is(err, qms.ErrCAPANotFound) {
			http.NotFound(w, r)
			return
		}
		h.logger.Error("get capa", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/qms/capa_detail.html", "CAPA "+capa.Number, map[string]any{
		"CAPA": capa,
		"Statuses": []qms.Status{
			qms.CAPAStatusOpen, qms.CAPAStatusInProgress,
			qms.CAPAStatusVerifying, qms.CAPAStatusEffective, qms.CAPAStatusClosed,
		},
	})
}

func (h *Handler) updateCAPAStatus(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	id := parseInt64(chi.URLParam(r, "id"))
	status := qms.NormaliseStatus(r.PostFormValue("status"))
	actorID := currentUser(r)

	if _, err := h.service.UpdateCAPAStatus(r.Context(), id, status, actorID); err != nil {
		h.logger.Warn("update capa status", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/qms/capas/"+strconv.FormatInt(id, 10), "danger", shared.UserSafeMessage(err))
		return
	}
	h.redirectWithFlash(w, r, "/qms/capas/"+strconv.FormatInt(id, 10), "success", "CAPA status updated")
}

// ============================================================================
// Audits
// ============================================================================

func (h *Handler) listAudits(w http.ResponseWriter, r *http.Request) {
	companyID := currentCompany(r)
	var statusFilter *qms.Status
	if s := r.URL.Query().Get("status"); s != "" {
		st := qms.NormaliseStatus(s)
		statusFilter = &st
	}
	audits, err := h.service.ListAudits(r.Context(), companyID, statusFilter, r.URL.Query().Get("type"), 50, 0)
	if err != nil {
		h.logger.Error("list audits", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/qms/audits.html", "Quality Audits", map[string]any{
		"Audits": audits,
		"AuditTypes": []string{"INTERNAL", "SUPPLIER", "REGULATORY", "CERTIFICATION", "PROCESS", "PRODUCT"},
		"Statuses": []qms.Status{
			qms.AuditStatusPlanned, qms.AuditStatusInProgress,
			qms.AuditStatusCompleted, qms.AuditStatusReported, qms.AuditStatusClosed,
		},
	})
}

func (h *Handler) newAuditForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "pages/qms/audit_new.html", "New Audit", map[string]any{
		"AuditTypes": []string{"INTERNAL", "SUPPLIER", "REGULATORY", "CERTIFICATION", "PROCESS", "PRODUCT"},
		"Standards":  []string{"ISO9001", "ISO13485", "IATF16949", "AS9100", "CUSTOM"},
	})
}

func (h *Handler) createAudit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	actorID := currentUser(r)
	companyID := currentCompany(r)

	req := qms.CreateAuditRequest{
		CompanyID:     companyID,
		Title:         strings.TrimSpace(r.PostFormValue("title")),
		Description:   strings.TrimSpace(r.PostFormValue("description")),
		AuditType:     strings.ToUpper(strings.TrimSpace(r.PostFormValue("audit_type"))),
		Standard:      strings.TrimSpace(r.PostFormValue("standard")),
		Scope:         strings.TrimSpace(r.PostFormValue("scope")),
		LeadAuditorID: actorID,
		ActorID:       actorID,
	}
	if ps := strings.TrimSpace(r.PostFormValue("planned_start")); ps != "" {
		if t, err := time.Parse("2006-01-02", ps); err == nil {
			req.PlannedStart = &t
		}
	}
	if pe := strings.TrimSpace(r.PostFormValue("planned_end")); pe != "" {
		if t, err := time.Parse("2006-01-02", pe); err == nil {
			req.PlannedEnd = &t
		}
	}

	audit, err := h.service.CreateAudit(r.Context(), req)
	if err != nil {
		h.logger.Warn("create audit", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/qms/audits/new", "danger", shared.UserSafeMessage(err))
		return
	}
	h.redirectWithFlash(w, r, "/qms/audits/"+strconv.FormatInt(audit.ID, 10), "success", "Audit "+audit.Number+" created")
}

func (h *Handler) getAudit(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	audit, err := h.service.GetAudit(r.Context(), id)
	if err != nil {
		if errors.Is(err, qms.ErrAuditNotFound) {
			http.NotFound(w, r)
			return
		}
		h.logger.Error("get audit", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	findings, _ := h.service.GetFindings(r.Context(), audit.ID)
	h.render(w, r, "pages/qms/audit_detail.html", "Audit "+audit.Number, map[string]any{
		"Audit":    audit,
		"Findings": findings,
		"FindingCategories": []string{"MAJOR", "MINOR", "OBSERVATION", "OPPORTUNITY"},
		"RiskLevels":        []string{"HIGH", "MEDIUM", "LOW"},
	})
}

func (h *Handler) addFinding(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	id := parseInt64(chi.URLParam(r, "id"))
	actorID := currentUser(r)

	req := qms.CreateFindingRequest{
		AuditID:     id,
		Category:    strings.ToUpper(strings.TrimSpace(r.PostFormValue("category"))),
		Clause:      strings.TrimSpace(r.PostFormValue("clause")),
		Description: strings.TrimSpace(r.PostFormValue("description")),
		Evidence:    strings.TrimSpace(r.PostFormValue("evidence")),
		Requirement: strings.TrimSpace(r.PostFormValue("requirement")),
		RiskLevel:   strings.ToUpper(strings.TrimSpace(r.PostFormValue("risk_level"))),
		ActorID:     actorID,
	}

	if _, err := h.service.AddFinding(r.Context(), req); err != nil {
		h.logger.Warn("add finding", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/qms/audits/"+strconv.FormatInt(id, 10), "danger", shared.UserSafeMessage(err))
		return
	}
	h.redirectWithFlash(w, r, "/qms/audits/"+strconv.FormatInt(id, 10), "success", "Finding added")
}

// ============================================================================
// Supplier Quality
// ============================================================================

func (h *Handler) listSupplierQuality(w http.ResponseWriter, r *http.Request) {
	companyID := currentCompany(r)
	var statusFilter *qms.Status
	if s := r.URL.Query().Get("status"); s != "" {
		st := qms.NormaliseStatus(s)
		statusFilter = &st
	}
	records, err := h.service.ListSupplierQuality(r.Context(), companyID, statusFilter, 50, 0)
	if err != nil {
		h.logger.Error("list supplier quality", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/qms/supplier_quality.html", "Supplier Quality", map[string]any{
		"Records": records,
		"Statuses": []qms.Status{
			qms.SupplierStatusApproved, qms.SupplierStatusConditional,
			qms.SupplierStatusRejected, qms.SupplierStatusOnHold,
		},
	})
}

func (h *Handler) newSupplierQualityForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "pages/qms/supplier_quality_new.html", "New Supplier Quality Record", map[string]any{
		"Statuses":   []string{"APPROVED", "CONDITIONAL", "REJECTED", "ON_HOLD"},
		"RiskLevels": []string{"LOW", "MEDIUM", "HIGH", "CRITICAL"},
	})
}

func (h *Handler) createSupplierQuality(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	actorID := currentUser(r)
	companyID := currentCompany(r)

	status := qms.NormaliseStatus(r.PostFormValue("status"))
	req := qms.CreateSupplierQualityRequest{
		CompanyID:     companyID,
		SupplierID:    parseInt64(r.PostFormValue("supplier_id")),
		Status:        status,
		QualityRating: parseFloat64(r.PostFormValue("quality_rating")),
		RiskLevel:     strings.ToUpper(strings.TrimSpace(r.PostFormValue("risk_level"))),
		Notes:         strings.TrimSpace(r.PostFormValue("notes")),
		ActorID:       actorID,
	}

	record, err := h.service.CreateSupplierQuality(r.Context(), req)
	if err != nil {
		h.logger.Warn("create supplier quality", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/qms/supplier-quality/new", "danger", shared.UserSafeMessage(err))
		return
	}
	h.redirectWithFlash(w, r, "/qms/supplier-quality/"+strconv.FormatInt(record.ID, 10), "success", "Supplier quality record created")
}

func (h *Handler) getSupplierQuality(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	record, err := h.service.GetSupplierQuality(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.render(w, r, "pages/qms/supplier_quality_detail.html", "Supplier Quality Record", map[string]any{
		"Record": record,
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
		h.logger.Error("render qms template", slog.Any("error", err))
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

func parseFloat64(value string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0
	}
	return v
}

// ============================================================================
// Inspections
// ============================================================================

func (h *Handler) listInspections(w http.ResponseWriter, r *http.Request) {
	companyID := currentCompany(r)
	status := r.URL.Query().Get("status")
	refModule := r.URL.Query().Get("reference_module")

	inspections, err := h.service.ListInspections(r.Context(), companyID, status, refModule, 50, 0)
	if err != nil {
		h.logger.Error("list inspections", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/qms/inspections.html", "Inspections", map[string]any{
		"Inspections": inspections,
	})
}

func (h *Handler) getInspection(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	inspection, err := h.service.GetInspection(r.Context(), id)
	if err != nil {
		if errors.Is(err, qms.ErrInspectionNotFound) {
			http.NotFound(w, r)
			return
		}
		h.logger.Error("get inspection", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/qms/inspection_detail.html", "Inspection "+inspection.Name, map[string]any{
		"Inspection": inspection,
	})
}

func (h *Handler) newInspection(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "pages/qms/inspection_new.html", "Plan Inspection", nil)
}

func (h *Handler) createInspection(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.logger.Error("parse form", slog.Any("error", err))
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	companyID := currentCompany(r)
	userID := currentUser(r)

	req := qms.CreateInspectionRequest{
		CompanyID:       companyID,
		Name:            r.FormValue("name"),
		Description:     r.FormValue("description"),
		ReferenceModule: r.FormValue("reference_module"),
		ActorID:         userID,
	}

	if refID := r.FormValue("reference_id"); refID != "" {
		parsedRefID := parseInt64(refID)
		if parsedRefID > 0 {
			req.ReferenceID = &parsedRefID
		}
	}

	if inspectorID := r.FormValue("inspector_id"); inspectorID != "" {
		parsedInspectorID := parseInt64(inspectorID)
		if parsedInspectorID > 0 {
			req.InspectorID = &parsedInspectorID
		}
	}

	if scheduledAt := r.FormValue("scheduled_at"); scheduledAt != "" {
		if t, err := time.Parse("2006-01-02T15:04", scheduledAt); err == nil {
			req.ScheduledAt = &t
		}
	}

	_, err := h.service.CreateInspection(r.Context(), req)
	if err != nil {
		h.logger.Error("create inspection", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/qms/inspections/new", "danger", "Failed to plan inspection: "+err.Error())
		return
	}

	h.redirectWithFlash(w, r, "/qms/inspections", "success", "Inspection planned successfully.")
}

// ============================================================================
// Customer Complaints
// ============================================================================

func (h *Handler) listComplaints(w http.ResponseWriter, r *http.Request) {
	companyID := currentCompany(r)
	status := r.URL.Query().Get("status")
	severity := r.URL.Query().Get("severity")

	complaints, err := h.service.ListComplaints(r.Context(), companyID, status, severity, nil, 50, 0)
	if err != nil {
		h.logger.Error("list complaints", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/qms/complaints.html", "Customer Complaints", map[string]any{
		"Complaints": complaints,
	})
}

func (h *Handler) getComplaint(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	complaint, err := h.service.GetComplaint(r.Context(), id)
	if err != nil {
		if errors.Is(err, qms.ErrComplaintNotFound) {
			http.NotFound(w, r)
			return
		}
		h.logger.Error("get complaint", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/qms/complaint_detail.html", "Complaint "+complaint.ComplaintNumber, map[string]any{
		"Complaint": complaint,
	})
}

// CalibrationRequestedPayload defines the payload for qms.calibration.required
type CalibrationRequestedPayload struct {
	FindingID int64 `json:"finding_id"`
	AssetID   int64 `json:"asset_id"`
	ActorID   int64 `json:"actor_id"`
}

func (h *Handler) requestCalibration(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	findingID, err := strconv.ParseInt(chi.URLParam(r, "findingID"), 10, 64)
	if err != nil {
		http.Error(w, "invalid finding id", http.StatusBadRequest)
		return
	}
	auditID := chi.URLParam(r, "id")
    
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	
	assetID, err := strconv.ParseInt(r.FormValue("asset_id"), 10, 64)
	if err != nil {
		h.redirectWithFlash(w, r, "/qms/audits/"+auditID, "danger", "Asset ID is required for calibration.")
		return
	}

	session := shared.SessionFromContext(ctx)
	companyID := currentCompany(r)
	actorID := parseInt64(session.User())
	
	_, err = h.outbox.InsertEvent(ctx, h.outbox.Queries(), outbox.PublishRequest{
		CompanyID:     companyID,
		CorrelationID: uuid.New(),
		EventType:     "qms.calibration.required",
		AggregateType: "qms_audit_finding",
		AggregateID:   findingID,
		Payload:       CalibrationRequestedPayload{
			FindingID: findingID,
			AssetID:   assetID,
			ActorID:   actorID,
		},
	})
	
	if err != nil {
		h.logger.Error("failed to publish calibration event", slog.Any("err", err))
		h.redirectWithFlash(w, r, "/qms/audits/"+auditID, "danger", "Failed to request calibration.")
		return
	}
	
	h.redirectWithFlash(w, r, "/qms/audits/"+auditID, "success", "Calibration requested in CMMS.")
}
