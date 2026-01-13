package customers

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
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

func NewHandler(
	logger *slog.Logger,
	service *Service,
	templates *view.Engine,
	csrf *shared.CSRFManager,
	rbac rbac.Middleware,
) *Handler {
	return &Handler{
		logger:    logger,
		service:   service,
		templates: templates,
		csrf:      csrf,
		rbac:      rbac,
	}
}

type formErrors map[string]string

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	companyID := h.getCurrentCompanyID(r)

	// Parse filters
	var isActive *bool
	if r.URL.Query().Get("is_active") != "" {
		val := r.URL.Query().Get("is_active") == "true"
		isActive = &val
	}

	search := r.URL.Query().Get("search")
	var searchPtr *string
	if search != "" {
		searchPtr = &search
	}

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	h.logger.Info("List customers request", "companyID", companyID)
	customers, total, err := h.service.List(r.Context(), ListCustomersRequest{
		CompanyID: companyID,
		IsActive:  isActive,
		Search:    searchPtr,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		h.logger.Error("list customers failed", "error", err)
		http.Error(w, "Failed to load customers", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/sales/customers_list.html", map[string]any{
		"Customers": customers,
		"Total":     total,
		"Limit":     limit,
		"Offset":    offset,
		"Filters": map[string]any{
			"IsActive": isActive,
			"Search":   searchPtr,
		},
	}, http.StatusOK)
}

func (h *Handler) Show(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid customer ID", http.StatusBadRequest)
		return
	}

	customer, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.logger.Error("get customer failed", "error", err, "id", id)
		http.Error(w, "Customer not found", http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/sales/customer_detail.html", map[string]any{
		"Customer": customer,
	}, http.StatusOK)
}

func (h *Handler) ShowForm(w http.ResponseWriter, r *http.Request) {
	companyID := h.getCurrentCompanyID(r)

	// Generate customer code
	code, err := h.service.GenerateCode(r.Context(), companyID)
	if err != nil {
		h.logger.Error("generate customer code failed", "error", err)
		code = ""
	}

	h.render(w, r, "pages/sales/customer_form.html", map[string]any{
		"Errors":        formErrors{},
		"GeneratedCode": code,
		"Customer":      nil,
	}, http.StatusOK)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	companyID := h.getCurrentCompanyID(r)
	userID := h.getCurrentUserID(r)

	// Parse form data
	creditLimit, _ := strconv.ParseFloat(r.PostFormValue("credit_limit"), 64)
	paymentTerms, _ := strconv.Atoi(r.PostFormValue("payment_terms_days"))

	req := CreateCustomerRequest{
		Code:             r.PostFormValue("code"),
		Name:             r.PostFormValue("name"),
		CompanyID:        companyID,
		CreditLimit:      creditLimit,
		PaymentTermsDays: paymentTerms,
		Country:          r.PostFormValue("country"),
	}

	// Optional fields
	if email := r.PostFormValue("email"); email != "" {
		req.Email = &email
	}
	if phone := r.PostFormValue("phone"); phone != "" {
		req.Phone = &phone
	}
	if taxID := r.PostFormValue("tax_id"); taxID != "" {
		req.TaxID = &taxID
	}
	if addr1 := r.PostFormValue("address_line1"); addr1 != "" {
		req.AddressLine1 = &addr1
	}
	if addr2 := r.PostFormValue("address_line2"); addr2 != "" {
		req.AddressLine2 = &addr2
	}
	if city := r.PostFormValue("city"); city != "" {
		req.City = &city
	}
	if state := r.PostFormValue("state"); state != "" {
		req.State = &state
	}
	if postal := r.PostFormValue("postal_code"); postal != "" {
		req.PostalCode = &postal
	}
	if notes := r.PostFormValue("notes"); notes != "" {
		req.Notes = &notes
	}

	customer, err := h.service.Create(r.Context(), req, userID)
	if err != nil {
		h.logger.Error("create customer failed", "error", err)
		h.render(w, r, "pages/sales/customer_form.html", map[string]any{
			"Errors":        formErrors{"general": shared.UserSafeMessage(err)},
			"GeneratedCode": req.Code,
			"Customer":      nil,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/sales/customers/"+strconv.FormatInt(customer.ID, 10), "success", "Customer created successfully")
}

func (h *Handler) ShowEditForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid customer ID", http.StatusBadRequest)
		return
	}

	customer, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.logger.Error("get customer failed", "error", err, "id", id)
		http.Error(w, "Customer not found", http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/sales/customer_form.html", map[string]any{
		"Errors":   formErrors{},
		"Customer": customer,
	}, http.StatusOK)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid customer ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	req := UpdateCustomerRequest{}

	// Parse optional fields
	if name := r.PostFormValue("name"); name != "" {
		req.Name = &name
	}
	if email := r.PostFormValue("email"); email != "" {
		req.Email = &email
	}
	if phone := r.PostFormValue("phone"); phone != "" {
		req.Phone = &phone
	}
	if taxID := r.PostFormValue("tax_id"); taxID != "" {
		req.TaxID = &taxID
	}
	if creditLimit := r.PostFormValue("credit_limit"); creditLimit != "" {
		if val, err := strconv.ParseFloat(creditLimit, 64); err == nil {
			req.CreditLimit = &val
		}
	}
	if paymentTerms := r.PostFormValue("payment_terms_days"); paymentTerms != "" {
		if val, err := strconv.Atoi(paymentTerms); err == nil {
			req.PaymentTermsDays = &val
		}
	}
	if addr1 := r.PostFormValue("address_line1"); addr1 != "" {
		req.AddressLine1 = &addr1
	}
	if addr2 := r.PostFormValue("address_line2"); addr2 != "" {
		req.AddressLine2 = &addr2
	}
	if city := r.PostFormValue("city"); city != "" {
		req.City = &city
	}
	if state := r.PostFormValue("state"); state != "" {
		req.State = &state
	}
	if postal := r.PostFormValue("postal_code"); postal != "" {
		req.PostalCode = &postal
	}
	if country := r.PostFormValue("country"); country != "" {
		req.Country = &country
	}
	if notes := r.PostFormValue("notes"); notes != "" {
		req.Notes = &notes
	}
	if isActive := r.PostFormValue("is_active"); isActive != "" {
		val := isActive == "true"
		req.IsActive = &val
	}

	customer, err := h.service.Update(r.Context(), id, req)
	if err != nil {
		h.logger.Error("update customer failed", "error", err, "id", id)
		h.render(w, r, "pages/sales/customer_form.html", map[string]any{
			"Errors":   formErrors{"general": shared.UserSafeMessage(err)},
			"Customer": customer,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/sales/customers/"+strconv.FormatInt(customer.ID, 10), "success", "Customer updated successfully")
}

// Helpers
func (h *Handler) render(w http.ResponseWriter, r *http.Request, tmpl string, data map[string]any, status int) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)

	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}

	viewData := view.TemplateData{
		Title:       "Sales",
		CSRFToken:   csrfToken,
		Flash:       flash,
		CurrentPath: r.URL.Path,
		Data:        data,
	}

	w.WriteHeader(status)
	if err := h.templates.Render(w, tmpl, viewData); err != nil {
		h.logger.Error("template render failed", "error", err, "template", tmpl)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *Handler) redirectWithFlash(w http.ResponseWriter, r *http.Request, url, flashType, message string) {
	sess := shared.SessionFromContext(r.Context())
	if sess != nil {
		sess.AddFlash(shared.FlashMessage{Kind: flashType, Message: message})
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}

func (h *Handler) getCurrentUserID(r *http.Request) int64 {
	sess := shared.SessionFromContext(r.Context())
	if sess != nil {
		if userIDStr := sess.User(); userIDStr != "" {
			if userID, err := strconv.ParseInt(userIDStr, 10, 64); err == nil {
				return userID
			}
		}
	}
	return 1 // Default user for development
}

func (h *Handler) getCurrentCompanyID(r *http.Request) int64 {
	sess := shared.SessionFromContext(r.Context())
	if sess != nil && sess.Get("company_id") != "" {
		if id, err := strconv.ParseInt(sess.Get("company_id"), 10, 64); err == nil && id > 0 {
			return id
		}
	}
	return 1 // Default company for development
}
