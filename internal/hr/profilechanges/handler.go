package profilechanges

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/hr/compensation"
	"github.com/odyssey-erp/odyssey-erp/internal/hr/employees"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// HandlerService is the subset of *Service used by the HR admin page.
type HandlerService interface {
	ListAll(ctx context.Context, companyID int64, f Filter) ([]Request, error)
	RetryApply(ctx context.Context, companyID, requestID, actorID int64) error
}

// EmployeeReader supplies current employee values for the diff; satisfied by
// *employees.Service.
type EmployeeReader interface {
	GetByID(ctx context.Context, companyID, id int64) (employees.Employee, error)
	ListContacts(ctx context.Context, companyID, employeeID int64) ([]employees.EmergencyContact, error)
}

// BankReader supplies current bank details for the diff; satisfied by
// *compensation.Service.
type BankReader interface {
	AsOf(ctx context.Context, companyID, employeeID int64, date time.Time) (compensation.Assignment, error)
}

// Handler serves /hr/profile-changes, the HR admin view of profile change
// requests. Decisions happen in /approvals; this page shows each pending
// request's payload against the employee's current values and re-runs a
// failed apply.
type Handler struct {
	logger    *slog.Logger
	service   HandlerService
	employees EmployeeReader
	bank      BankReader
	templates *view.Engine
	csrf      *shared.CSRFManager
	rbac      rbac.Middleware
}

// NewHandler builds a Handler.
func NewHandler(l *slog.Logger, s HandlerService, e EmployeeReader, b BankReader, t *view.Engine, c *shared.CSRFManager, r rbac.Middleware) *Handler {
	if l == nil {
		l = slog.Default()
	}
	return &Handler{logger: l, service: s, employees: e, bank: b, templates: t, csrf: c, rbac: r}
}

// MountRoutes mounts /hr/profile-changes.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermHRProfileChangeAdmin))
		r.Get("/", h.list)
		r.Post("/{id}/retry", h.retry)
	})
}

// canSeeSensitive reports whether the user holds hr.employee.sensitive.
func (h *Handler) canSeeSensitive(r *http.Request, userID int64) bool {
	if h.rbac.Service == nil {
		return false
	}
	perms, err := h.rbac.Service.EffectivePermissions(r.Context(), userID)
	if err != nil {
		return false
	}
	for _, p := range perms {
		switch strings.ToLower(p) {
		case "admin", "*", shared.PermHREmployeeSensitive:
			return true
		}
	}
	return false
}

// DiffRow is one payload field compared with the employee's current value.
// Changed is set only when both values are visible and differ.
type DiffRow struct {
	Field    string
	Label    string
	Current  string
	Proposed string
	Changed  bool
}

// Row is a request plus its rendered diff. Diff is computed for PENDING
// requests only; decided requests show the submitted payload.
type Row struct {
	Request
	Diff     []DiffRow
	NewEntry bool
}

var fieldLabels = map[string]string{
	"address_line1":       "Address line 1",
	"address_line2":       "Address line 2",
	"city":                "City",
	"province":            "Province",
	"postal_code":         "Postal code",
	"country":             "Country",
	"phone":               "Phone",
	"bank_code":           "Bank",
	"bank_account_number": "Account number",
	"bank_account_name":   "Account name",
	"contact_id":          "Contact",
	"name":                "Name",
	"relationship":        "Relationship",
	"email":               "Email",
	"is_primary":          "Primary contact",
}

func fieldLabel(key string) string {
	if l, ok := fieldLabels[key]; ok {
		return l
	}
	return key
}

// currentValues resolves the employee's current values for one request.
type currentValues struct {
	employee    *employees.Employee
	contacts    []employees.EmergencyContact
	contactsErr error
	bank        *compensation.Assignment
	bankErr     error
}

// diffBuilder caches lookups per employee for one page render.
type diffBuilder struct {
	h         *Handler
	ctx       context.Context
	companyID int64
	sensitive bool
	cache     map[int64]*currentValues
}

func (d *diffBuilder) current(employeeID int64, changeType string) *currentValues {
	cv, ok := d.cache[employeeID]
	if !ok {
		cv = &currentValues{}
		d.cache[employeeID] = cv
		if d.h.employees != nil {
			if e, err := d.h.employees.GetByID(d.ctx, d.companyID, employeeID); err == nil {
				if !d.sensitive {
					employees.MaskSensitive(&e)
				}
				cv.employee = &e
			} else {
				d.h.logger.Error("profile change diff: load employee", slog.Int64("employee_id", employeeID), slog.Any("error", err))
			}
		}
	}
	switch changeType {
	case TypeEmergencyContact:
		if cv.contacts == nil && cv.contactsErr == nil && d.h.employees != nil {
			cv.contacts, cv.contactsErr = d.h.employees.ListContacts(d.ctx, d.companyID, employeeID)
			if cv.contacts == nil && cv.contactsErr == nil {
				cv.contacts = []employees.EmergencyContact{}
			}
		}
	case TypeBank:
		// Bank details are fetched only for viewers allowed to see them.
		if d.sensitive && cv.bank == nil && cv.bankErr == nil && d.h.bank != nil {
			a, err := d.h.bank.AsOf(d.ctx, d.companyID, employeeID, time.Now())
			if err == nil {
				cv.bank = &a
			} else {
				cv.bankErr = err
				if !errors.Is(err, compensation.ErrNoAssignment) {
					d.h.logger.Error("profile change diff: load bank", slog.Int64("employee_id", employeeID), slog.Any("error", err))
				}
			}
		}
	}
	return cv
}

func addressValue(e *employees.Employee, key string) string {
	if e == nil {
		return ""
	}
	switch key {
	case "address_line1":
		return e.AddressLine1
	case "address_line2":
		return e.AddressLine2
	case "city":
		return e.City
	case "province":
		return e.Province
	case "postal_code":
		return e.PostalCode
	case "country":
		return e.Country
	case "phone":
		return e.Phone
	}
	return ""
}

func bankValue(a *compensation.Assignment, key string) string {
	if a == nil {
		return ""
	}
	switch key {
	case "bank_code":
		return a.BankCode
	case "bank_account_number":
		return a.BankAccountNumber
	case "bank_account_name":
		return a.BankAccountName
	}
	return ""
}

func contactValue(c *employees.EmergencyContact, key string) string {
	if c == nil {
		return ""
	}
	switch key {
	case "name":
		return c.Name
	case "relationship":
		return c.Relationship
	case "phone":
		return c.Phone
	case "email":
		return c.Email
	case "is_primary":
		return strconv.FormatBool(c.IsPrimary)
	}
	return ""
}

func (d *diffBuilder) build(req Request) Row {
	row := Row{Request: req}
	if req.Status != StatusPending {
		return row
	}
	cv := d.current(req.EmployeeID, req.ChangeType)
	var contact *employees.EmergencyContact
	if req.ChangeType == TypeEmergencyContact {
		id := contactID(req.Payload)
		if id <= 0 {
			row.NewEntry = true
		}
		for i := range cv.contacts {
			if cv.contacts[i].ID == id {
				contact = &cv.contacts[i]
				break
			}
		}
	}
	masked := sensitiveKeys[req.ChangeType]
	for _, key := range req.PayloadKeys() {
		if key == "contact_id" {
			continue
		}
		proposed := req.Payload[key]
		var current string
		switch req.ChangeType {
		case TypeAddress:
			current = addressValue(cv.employee, key)
		case TypeBank:
			current = bankValue(cv.bank, key)
		case TypeEmergencyContact:
			current = contactValue(contact, key)
		}
		hidden := !d.sensitive && masked[key]
		if hidden {
			current = MaskedValue
		}
		if req.ChangeType == TypeEmergencyContact && key == "is_primary" {
			if b, err := parseBool(proposed); err == nil {
				proposed = strconv.FormatBool(b)
			}
		}
		row.Diff = append(row.Diff, DiffRow{
			Field:    key,
			Label:    fieldLabel(key),
			Current:  current,
			Proposed: proposed,
			Changed:  !hidden && current != proposed,
		})
	}
	return row
}

var validChangeTypes = map[string]bool{TypeAddress: true, TypeBank: true, TypeEmergencyContact: true}
var validStatuses = map[string]bool{StatusPending: true, StatusApproved: true, StatusRejected: true, StatusCancelled: true}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	sensitive := h.canSeeSensitive(r, ident.UserID)
	q := r.URL.Query()
	statusFilter := strings.ToUpper(strings.TrimSpace(q.Get("status")))
	if !validStatuses[statusFilter] {
		statusFilter = ""
	}
	typeFilter := strings.ToUpper(strings.TrimSpace(q.Get("type")))
	if !validChangeTypes[typeFilter] {
		typeFilter = ""
	}
	search := strings.TrimSpace(q.Get("q"))

	requests, err := h.service.ListAll(r.Context(), ident.CompanyID, Filter{Status: statusFilter, ChangeType: typeFilter, Search: search, ShowSensitive: sensitive})
	if err != nil {
		h.logger.Error("list profile changes", slog.Any("error", err))
	}
	builder := &diffBuilder{h: h, ctx: r.Context(), companyID: ident.CompanyID, sensitive: sensitive, cache: map[int64]*currentValues{}}
	rows := make([]Row, 0, len(requests))
	for _, req := range requests {
		if !sensitive && !req.Masked {
			// Defence in depth: never render unmasked values to a viewer
			// without hr.employee.sensitive.
			MaskPayload(&req)
		}
		rows = append(rows, builder.build(req))
	}

	s := shared.SessionFromContext(r.Context())
	token, _ := h.csrf.EnsureToken(r.Context(), s)
	if rerr := h.templates.Render(w, "pages/hr/profile_changes.html", view.TemplateData{
		Title:       "Profile Changes",
		CurrentPath: r.URL.Path,
		CSRFToken:   token,
		Flash:       s.PopFlash(),
		Data: map[string]any{
			"Rows":          rows,
			"StatusFilter":  statusFilter,
			"TypeFilter":    typeFilter,
			"Search":        search,
			"Filtered":      statusFilter != "" || typeFilter != "" || search != "",
			"ShowSensitive": sensitive,
			"Error":         err,
		},
	}); rerr != nil {
		h.logger.Error("render profile changes page", slog.Any("error", rerr))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "")
	}
}

// flashError shows "hr: ..." domain errors (including finalizer apply
// errors returned by RetryApply) to the user and hides the rest.
func flashError(err error) string {
	msg := err.Error()
	if strings.HasPrefix(msg, "hr: ") {
		if trimmed := strings.TrimSpace(strings.TrimPrefix(msg, "hr: ")); trimmed != "" {
			return strings.ToUpper(trimmed[:1]) + trimmed[1:]
		}
	}
	return shared.UserSafeMessage(err)
}

func (h *Handler) retry(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	err := h.service.RetryApply(r.Context(), ident.CompanyID, id, ident.UserID)
	s := shared.SessionFromContext(r.Context())
	if err != nil {
		s.AddFlash(shared.FlashMessage{Kind: "error", Message: flashError(err)})
	} else {
		s.AddFlash(shared.FlashMessage{Kind: "success", Message: "Profile change decision applied; check the request status"})
	}
	http.Redirect(w, r, "/hr/profile-changes", http.StatusSeeOther)
}
