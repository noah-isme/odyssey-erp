package crm

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/sales/quotations"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type Handler struct {
	logger    *slog.Logger
	service   *Service
	templates *view.Engine
	csrf      *shared.CSRFManager
	rbac      rbac.Middleware
}

func NewHandler(logger *slog.Logger, service *Service, templates *view.Engine, csrf *shared.CSRFManager, middleware rbac.Middleware) *Handler {
	return &Handler{logger: logger, service: service, templates: templates, csrf: csrf, rbac: middleware}
}
func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("crm.view", "crm.team.view", "crm.manage"))
		r.Get("/", h.pipeline)
		r.Get("/leads/{id}", h.lead)
		r.Get("/opportunities/{id}", h.opportunity)
		r.Get("/opportunities/{id}/convert", h.convertForm)
		r.Get("/dashboard", h.dashboard)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("crm.create", "crm.manage"))
		r.Post("/leads", h.createLead)
		r.Post("/leads/{id}/qualify", h.qualify)
		r.Post("/activities", h.addActivity)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("crm.edit", "crm.manage"))
		r.Post("/opportunities/{id}/stage", h.move)
		r.Post("/activities/{id}/complete", h.completeActivity)
		r.Post("/{entity}/{id}/reassign", h.reassign)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("crm.convert", "crm.manage"))
		r.Post("/opportunities/{id}/convert", h.convert)
	})
}
func actor(r *http.Request) int64 {
	s := shared.SessionFromContext(r.Context())
	if s == nil {
		return 0
	}
	id, _ := strconv.ParseInt(s.User(), 10, 64)
	return id
}
func company(r *http.Request) int64 {
	s := shared.SessionFromContext(r.Context())
	if s == nil {
		return 0
	}
	id, _ := strconv.ParseInt(s.Get("company_id"), 10, 64)
	return id
}
func (h *Handler) scope(r *http.Request) Scope {
	s := Scope{CompanyID: company(r), UserID: actor(r)}
	if h.rbac.Service != nil {
		permissions, _ := h.rbac.Service.EffectivePermissions(r.Context(), s.UserID)
		for _, permission := range permissions {
			if permission == "crm.team.view" || permission == "crm.manage" {
				s.ViewAll = true
			}
		}
	}
	return s
}
func (h *Handler) render(w http.ResponseWriter, r *http.Request, name, title string, data any) {
	var token string
	var flash *shared.FlashMessage
	s := shared.SessionFromContext(r.Context())
	if s != nil {
		flash = s.PopFlash()
		if h.csrf != nil {
			token, _ = h.csrf.EnsureToken(r.Context(), s)
		}
	}
	if err := h.templates.Render(w, name, view.TemplateData{Title: title, CurrentPath: r.URL.Path, CSRFToken: token, Flash: flash, Data: data}); err != nil {
		if h.logger != nil {
			h.logger.Error("render crm", slog.Any("error", err))
		}
		http.Error(w, http.StatusText(500), 500)
	}
}
func (h *Handler) redirect(w http.ResponseWriter, r *http.Request, target string, err error, message string) {
	kind := "success"
	if err != nil {
		kind = "error"
		message = shared.UserSafeMessage(err)
	}
	if s := shared.SessionFromContext(r.Context()); s != nil {
		s.AddFlash(shared.FlashMessage{Kind: kind, Message: message})
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}
func routeID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id
}
func parseDate(value string) *time.Time {
	if value == "" {
		return nil
	}
	v, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return nil
	}
	return &v
}
func parseTime(value string) *time.Time {
	if value == "" {
		return nil
	}
	v, err := time.Parse("2006-01-02T15:04", value)
	if err != nil {
		return nil
	}
	return &v
}

func (h *Handler) pipeline(w http.ResponseWriter, r *http.Request) {
	data, err := h.service.Pipeline(r.Context(), h.scope(r))
	if err != nil {
		http.Error(w, shared.UserSafeMessage(err), 500)
		return
	}
	h.render(w, r, "pages/crm/pipeline.html", "CRM Pipeline", data)
}
func (h *Handler) createLead(w http.ResponseWriter, r *http.Request) {
	owner, _ := strconv.ParseInt(r.FormValue("owner_id"), 10, 64)
	_, err := h.service.CreateLead(r.Context(), h.scope(r), CreateLeadInput{OwnerID: owner, Name: r.FormValue("name"), Organization: r.FormValue("organization"), Email: r.FormValue("email"), Phone: r.FormValue("phone"), Source: r.FormValue("source"), Notes: r.FormValue("notes")})
	h.redirect(w, r, "/crm", err, "Lead created")
}
func (h *Handler) lead(w http.ResponseWriter, r *http.Request) {
	lead, activities, events, err := h.service.Lead(r.Context(), h.scope(r), routeID(r))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.render(w, r, "pages/crm/lead_detail.html", "CRM Lead", map[string]any{"Lead": lead, "Activities": activities, "Events": events})
}
func (h *Handler) qualify(w http.ResponseWriter, r *http.Request) {
	value, _ := strconv.ParseInt(r.FormValue("expected_value"), 10, 64)
	opp, err := h.service.Qualify(r.Context(), h.scope(r), QualifyInput{LeadID: routeID(r), OpportunityName: r.FormValue("name"), ExpectedValue: value, CloseDate: parseDate(r.FormValue("close_date"))})
	target := "/crm/leads/" + strconv.FormatInt(routeID(r), 10)
	if err == nil {
		target = "/crm/opportunities/" + strconv.FormatInt(opp.ID, 10)
	}
	h.redirect(w, r, target, err, "Lead qualified")
}
func (h *Handler) opportunity(w http.ResponseWriter, r *http.Request) {
	scope := h.scope(r)
	opp, activities, events, err := h.service.Opportunity(r.Context(), scope, routeID(r))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	stages, _ := h.service.Stages(r.Context(), scope)
	h.render(w, r, "pages/crm/opportunity_detail.html", "CRM Opportunity", map[string]any{"Opportunity": opp, "Stages": stages, "Activities": activities, "Events": events})
}
func (h *Handler) move(w http.ResponseWriter, r *http.Request) {
	stage, _ := strconv.ParseInt(r.FormValue("stage_id"), 10, 64)
	_, err := h.service.Move(r.Context(), h.scope(r), routeID(r), stage, r.FormValue("reason"))
	h.redirect(w, r, "/crm/opportunities/"+strconv.FormatInt(routeID(r), 10), err, "Opportunity stage updated")
}
func (h *Handler) addActivity(w http.ResponseWriter, r *http.Request) {
	leadID, oppID, contactID, owner := nullableID(r.FormValue("lead_id")), nullableID(r.FormValue("opportunity_id")), nullableID(r.FormValue("contact_id")), int64(0)
	owner, _ = strconv.ParseInt(r.FormValue("owner_id"), 10, 64)
	_, err := h.service.AddActivity(r.Context(), h.scope(r), ActivityInput{OwnerID: owner, LeadID: leadID, OpportunityID: oppID, ContactID: contactID, Type: r.FormValue("type"), Subject: r.FormValue("subject"), Body: r.FormValue("body"), DueAt: parseTime(r.FormValue("due_at")), ReminderAt: parseTime(r.FormValue("reminder_at"))})
	target := "/crm"
	if oppID != nil {
		target = "/crm/opportunities/" + strconv.FormatInt(*oppID, 10)
	} else if leadID != nil {
		target = "/crm/leads/" + strconv.FormatInt(*leadID, 10)
	}
	h.redirect(w, r, target, err, "Activity scheduled")
}
func (h *Handler) completeActivity(w http.ResponseWriter, r *http.Request) {
	err := h.service.CompleteActivity(r.Context(), h.scope(r), routeID(r))
	h.redirect(w, r, "/crm", err, "Activity completed")
}
func nullableID(value string) *int64 {
	id, _ := strconv.ParseInt(value, 10, 64)
	if id <= 0 {
		return nil
	}
	return &id
}
func (h *Handler) reassign(w http.ResponseWriter, r *http.Request) {
	owner, _ := strconv.ParseInt(r.FormValue("owner_id"), 10, 64)
	entity := strings.ToUpper(chi.URLParam(r, "entity"))
	err := h.service.Reassign(r.Context(), h.scope(r), entity, routeID(r), owner)
	h.redirect(w, r, "/crm", err, "Owner reassigned")
}
func (h *Handler) convertForm(w http.ResponseWriter, r *http.Request) {
	opp, _, _, err := h.service.Opportunity(r.Context(), h.scope(r), routeID(r))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.render(w, r, "pages/crm/conversion.html", "Convert Opportunity", opp)
}
func (h *Handler) convert(w http.ResponseWriter, r *http.Request) {
	customer, _ := strconv.ParseInt(r.FormValue("customer_id"), 10, 64)
	product, _ := strconv.ParseInt(r.FormValue("product_id"), 10, 64)
	qty, _ := strconv.ParseFloat(r.FormValue("quantity"), 64)
	price, _ := strconv.ParseFloat(r.FormValue("unit_price"), 64)
	var lines []quotations.CreateQuotationLineReq
	if product > 0 && qty > 0 {
		lines = []quotations.CreateQuotationLineReq{{ProductID: product, Quantity: qty, UOM: r.FormValue("uom"), UnitPrice: price}}
	}
	result, err := h.service.Convert(r.Context(), h.scope(r), ConvertInput{OpportunityID: routeID(r), ExistingCustomerID: customer, CustomerName: r.FormValue("customer_name"), Country: r.FormValue("country"), Currency: r.FormValue("currency"), QuoteDate: valueDate(r.FormValue("quote_date")), ValidUntil: valueDate(r.FormValue("valid_until")), Lines: lines})
	target := "/crm/opportunities/" + strconv.FormatInt(routeID(r), 10)
	if err == nil && result.QuotationID > 0 {
		target = "/sales/quotations/" + strconv.FormatInt(result.QuotationID, 10)
	}
	h.redirect(w, r, target, err, "Opportunity converted")
}
func valueDate(value string) time.Time {
	v := parseDate(value)
	if v == nil {
		return time.Time{}
	}
	return *v
}
func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	data, err := h.service.WinLoss(r.Context(), h.scope(r))
	if err != nil {
		http.Error(w, shared.UserSafeMessage(err), 500)
		return
	}
	h.render(w, r, "pages/crm/dashboard.html", "CRM Win/Loss", data)
}
