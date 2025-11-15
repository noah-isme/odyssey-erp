package boardpackhttp

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/boardpack"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
	"github.com/odyssey-erp/odyssey-erp/jobs"
)

// Handler wires HTTP endpoints for managing board packs.
type Handler struct {
	logger    *slog.Logger
	service   *boardpack.Service
	templates *view.Engine
	csrf      *shared.CSRFManager
	rbac      rbac.Middleware
	jobs      *jobs.Client
}

// NewHandler constructs a Handler value.
func NewHandler(logger *slog.Logger, service *boardpack.Service, templates *view.Engine, csrf *shared.CSRFManager, rbac rbac.Middleware, jobsClient *jobs.Client) *Handler {
	return &Handler{logger: logger, service: service, templates: templates, csrf: csrf, rbac: rbac, jobs: jobsClient}
}

// MountRoutes registers HTTP routes.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Route("/board-packs", func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermFinanceBoardPack))
		r.Get("/", h.list)
		r.Get("/new", h.newForm)
		r.Post("/", h.create)
		r.Get("/{id}", h.detail)
		r.Get("/{id}/download", h.download)
	})
}

type listPageData struct {
	Packs     []boardpack.BoardPack
	Companies []boardpack.Company
	Filter    boardpack.ListFilter
	Statuses  []boardpack.Status
	Templates []boardpack.Template
}

type newPageData struct {
	Companies       []boardpack.Company
	Periods         []boardpack.Period
	Templates       []boardpack.Template
	Snapshots       []boardpack.VarianceSnapshot
	SelectedCompany int64
}

// list renders board pack history.
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	filter := boardpack.ListFilter{
		CompanyID: parseInt64(r.URL.Query().Get("company_id")),
		PeriodID:  parseInt64(r.URL.Query().Get("period_id")),
		Limit:     50,
	}
	if status := strings.TrimSpace(r.URL.Query().Get("status")); status != "" {
		filter.Status = boardpack.NormaliseStatus(status)
	}
	packs, err := h.service.List(r.Context(), filter)
	if err != nil {
		h.logger.Error("list board packs", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	companies, _ := h.service.ListCompanies(r.Context())
	templates, _ := h.service.ListTemplates(r.Context())
	data := listPageData{
		Packs:     packs,
		Companies: companies,
		Filter:    filter,
		Statuses:  []boardpack.Status{boardpack.StatusPending, boardpack.StatusInProgress, boardpack.StatusReady, boardpack.StatusFailed},
		Templates: templates,
	}
	h.render(w, r, "pages/boardpacks/list.html", "Board Packs", data)
}

// newForm renders the creation form.
func (h *Handler) newForm(w http.ResponseWriter, r *http.Request) {
	companyID := parseInt64(r.URL.Query().Get("company_id"))
	companies, err := h.service.ListCompanies(r.Context())
	if err != nil {
		h.logger.Error("list companies", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	periods, err := h.service.ListPeriods(r.Context(), companyID, 36)
	if err != nil {
		h.logger.Error("list periods", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	templates, err := h.service.ListTemplates(r.Context())
	if err != nil {
		h.logger.Error("list templates", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	snapshots, _ := h.service.ListVarianceSnapshots(r.Context(), companyID, 20)
	data := newPageData{
		Companies:       companies,
		Periods:         periods,
		Templates:       templates,
		Snapshots:       snapshots,
		SelectedCompany: companyID,
	}
	h.render(w, r, "pages/boardpacks/new.html", "Generate Board Pack", data)
}

// create handles POST submission for a new board pack request.
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	req := boardpack.CreateRequest{
		CompanyID:          parseInt64(r.PostFormValue("company_id")),
		PeriodID:           parseInt64(r.PostFormValue("period_id")),
		TemplateID:         parseInt64(r.PostFormValue("template_id")),
		VarianceSnapshotID: parseOptionalInt(r.PostFormValue("variance_snapshot_id")),
		ActorID:            currentUser(r),
		Metadata:           map[string]any{"note": strings.TrimSpace(r.PostFormValue("note"))},
	}
	pack, err := h.service.Create(r.Context(), req)
	if err != nil {
		h.logger.Warn("create board pack", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/board-packs/new", "danger", err.Error())
		return
	}
	if h.jobs != nil {
		if _, err := h.jobs.EnqueueBoardPack(r.Context(), pack.ID); err != nil && h.logger != nil {
			h.logger.Warn("enqueue board pack", slog.Any("error", err))
		}
	}
	h.redirectWithFlash(w, r, "/board-packs/"+strconv.FormatInt(pack.ID, 10), "success", "Board pack dikirim ke antrian")
}

// detail renders a single board pack record.
func (h *Handler) detail(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	pack, err := h.service.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, boardpack.ErrBoardPackNotFound) {
			http.NotFound(w, r)
			return
		}
		h.logger.Error("get board pack", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/boardpacks/detail.html", "Board Pack Detail", map[string]any{"Pack": pack})
}

// download streams the generated PDF.
func (h *Handler) download(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	pack, err := h.service.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, boardpack.ErrBoardPackNotFound) {
			http.NotFound(w, r)
			return
		}
		h.logger.Error("download board pack", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if pack.Status != boardpack.StatusReady || pack.FilePath == "" {
		http.Error(w, "file belum siap", http.StatusBadRequest)
		return
	}
	file, err := os.Open(pack.FilePath)
	if err != nil {
		h.logger.Error("open board pack", slog.Any("error", err), slog.String("path", pack.FilePath))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=board-pack-"+strconv.FormatInt(pack.ID, 10)+".pdf")
	if _, err := io.Copy(w, file); err != nil && h.logger != nil {
		h.logger.Warn("stream board pack", slog.Any("error", err))
	}
}

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
		h.logger.Error("render boardpack template", slog.Any("error", err))
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

func parseInt64(value string) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func parseOptionalInt(value string) *int64 {
	v := parseInt64(value)
	if v == 0 {
		return nil
	}
	return &v
}
