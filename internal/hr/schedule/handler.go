package schedule

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// HandlerService is the subset of *Service used by the HTTP handler.
type HandlerService interface {
	ListHolidays(ctx context.Context, companyID int64, year int) ([]Holiday, error)
	CreateHoliday(ctx context.Context, companyID int64, date time.Time, name string, actorID int64) (Holiday, error)
	DeleteHoliday(ctx context.Context, companyID, holidayID, actorID int64) error
	ListShifts(ctx context.Context, companyID int64) ([]Shift, error)
	CreateShift(ctx context.Context, companyID int64, in ShiftInput, actorID int64) (Shift, error)
	AssignShift(ctx context.Context, companyID, employeeID, shiftID int64, validFrom time.Time, actorID int64) (ShiftAssignment, error)
	ListEmployeeShifts(ctx context.Context, companyID int64, asOf time.Time) ([]EmployeeShift, error)
}

// Handler serves the holiday calendar and shift pages.
type Handler struct {
	logger    *slog.Logger
	service   HandlerService
	templates *view.Engine
	csrf      *shared.CSRFManager
	rbac      rbac.Middleware
}

// NewHandler builds a Handler.
func NewHandler(l *slog.Logger, s HandlerService, t *view.Engine, c *shared.CSRFManager, r rbac.Middleware) *Handler {
	if l == nil {
		l = slog.Default()
	}
	return &Handler{logger: l, service: s, templates: t, csrf: c, rbac: r}
}

// MountHolidayRoutes mounts /hr/holidays.
func (h *Handler) MountHolidayRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermHRScheduleAdmin))
		r.Get("/", h.listHolidays)
		r.Post("/", h.createHoliday)
		r.Post("/{id}/delete", h.deleteHoliday)
	})
}

// MountShiftRoutes mounts /hr/shifts.
func (h *Handler) MountShiftRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermHRScheduleAdmin))
		r.Get("/", h.listShifts)
		r.Post("/", h.createShift)
		r.Post("/assign", h.assignShift)
	})
}

// WeekdayOption is a work-day checkbox on the shift form.
type WeekdayOption struct {
	Value   int
	Label   string
	Default bool
}

// weekdayOptions lists Monday..Sunday with ISO values (1=Mon .. 7=Sun).
func weekdayOptions() []WeekdayOption {
	return []WeekdayOption{
		{1, "Mon", true}, {2, "Tue", true}, {3, "Wed", true}, {4, "Thu", true}, {5, "Fri", true},
		{6, "Sat", false}, {7, "Sun", false},
	}
}

// WorkDaysLabel renders the shift's work days, e.g. "Mon, Tue, Wed".
func (s Shift) WorkDaysLabel() string {
	labels := make([]string, 0, len(s.WorkDays))
	for _, d := range s.WorkDays {
		labels = append(labels, d.String()[:3])
	}
	return strings.Join(labels, ", ")
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, name, title string, data map[string]any) {
	s := shared.SessionFromContext(r.Context())
	token, _ := h.csrf.EnsureToken(r.Context(), s)
	if err := h.templates.Render(w, name, view.TemplateData{
		Title:       title,
		CurrentPath: r.URL.Path,
		CSRFToken:   token,
		Flash:       s.PopFlash(),
		Data:        data,
	}); err != nil {
		h.logger.Error("render schedule page", slog.String("template", name), slog.Any("error", err))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "")
	}
}

func (h *Handler) redirect(w http.ResponseWriter, r *http.Request, to string, err error, success string) {
	s := shared.SessionFromContext(r.Context())
	if err != nil {
		s.AddFlash(shared.FlashMessage{Kind: "error", Message: flashError(err)})
	} else {
		s.AddFlash(shared.FlashMessage{Kind: "success", Message: success})
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// flashError shows "hr: ..." domain errors to the user and hides the rest.
func flashError(err error) string {
	msg := err.Error()
	if strings.HasPrefix(msg, "hr: ") {
		if trimmed := strings.TrimSpace(strings.TrimPrefix(msg, "hr: ")); trimmed != "" {
			return strings.ToUpper(trimmed[:1]) + trimmed[1:]
		}
	}
	return shared.UserSafeMessage(err)
}

func parseDate(v string) time.Time {
	t, _ := time.Parse("2006-01-02", strings.TrimSpace(v))
	return t
}

func (h *Handler) listHolidays(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	year := time.Now().Year()
	if y, err := strconv.Atoi(r.URL.Query().Get("year")); err == nil && y >= 1900 && y <= 9999 {
		year = y
	}
	holidays, err := h.service.ListHolidays(r.Context(), ident.CompanyID, year)
	if err != nil {
		h.logger.Error("list holidays", slog.Any("error", err))
	}
	h.render(w, r, "pages/hr/holidays.html", "Company Holidays", map[string]any{
		"Holidays": holidays,
		"Year":     year,
		"PrevYear": year - 1,
		"NextYear": year + 1,
		"Error":    err,
	})
}

func (h *Handler) createHoliday(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	date := parseDate(r.FormValue("holiday_date"))
	_, err := h.service.CreateHoliday(r.Context(), ident.CompanyID, date, r.FormValue("name"), ident.UserID)
	to := "/hr/holidays"
	if !date.IsZero() {
		to += "?year=" + strconv.Itoa(date.Year())
	}
	h.redirect(w, r, to, err, "Holiday added")
}

func (h *Handler) deleteHoliday(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	err := h.service.DeleteHoliday(r.Context(), ident.CompanyID, id, ident.UserID)
	to := "/hr/holidays"
	if y, convErr := strconv.Atoi(r.FormValue("year")); convErr == nil && y >= 1900 && y <= 9999 {
		to += "?year=" + strconv.Itoa(y)
	}
	h.redirect(w, r, to, err, "Holiday deleted")
}

func (h *Handler) listShifts(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	today := time.Now()
	shifts, err := h.service.ListShifts(r.Context(), ident.CompanyID)
	if err != nil {
		h.logger.Error("list shifts", slog.Any("error", err))
	}
	employees, empErr := h.service.ListEmployeeShifts(r.Context(), ident.CompanyID, today)
	if empErr != nil {
		h.logger.Error("list employee shifts", slog.Any("error", empErr))
		if err == nil {
			err = empErr
		}
	}
	var active []Shift
	for _, sh := range shifts {
		if sh.IsActive {
			active = append(active, sh)
		}
	}
	tab := strings.TrimSpace(r.URL.Query().Get("tab"))
	if tab != "assignments" {
		tab = "shifts"
	}
	h.render(w, r, "pages/hr/shifts.html", "Work Shifts", map[string]any{
		"Shifts":       shifts,
		"ActiveShifts": active,
		"Employees":    employees,
		"Weekdays":     weekdayOptions(),
		"Today":        today.Format("2006-01-02"),
		"Tab":          tab,
		"Error":        err,
	})
}

func (h *Handler) createShift(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	if err := r.ParseForm(); err != nil {
		h.redirect(w, r, "/hr/shifts", err, "")
		return
	}
	breakMinutes, _ := strconv.Atoi(strings.TrimSpace(r.PostFormValue("break_minutes")))
	var days []time.Weekday
	for _, v := range r.PostForm["work_days"] {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 7 {
			continue
		}
		days = append(days, time.Weekday(n%7))
	}
	_, err := h.service.CreateShift(r.Context(), ident.CompanyID, ShiftInput{
		Code:         r.PostFormValue("code"),
		Name:         r.PostFormValue("name"),
		StartTime:    r.PostFormValue("start_time"),
		EndTime:      r.PostFormValue("end_time"),
		BreakMinutes: breakMinutes,
		WorkDays:     days,
		IsActive:     r.PostFormValue("is_active") != "",
	}, ident.UserID)
	h.redirect(w, r, "/hr/shifts", err, "Shift created")
}

func (h *Handler) assignShift(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	employeeID, _ := strconv.ParseInt(r.FormValue("employee_id"), 10, 64)
	shiftID, _ := strconv.ParseInt(r.FormValue("shift_id"), 10, 64)
	validFrom := parseDate(r.FormValue("valid_from"))
	_, err := h.service.AssignShift(r.Context(), ident.CompanyID, employeeID, shiftID, validFrom, ident.UserID)
	h.redirect(w, r, "/hr/shifts?tab=assignments", err, "Shift assigned")
}
