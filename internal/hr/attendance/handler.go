package attendance

import (
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

type Handler struct {
	logger    *slog.Logger
	service   *Service
	templates *view.Engine
	csrf      *shared.CSRFManager
	rbac      rbac.Middleware
}

func NewHandler(l *slog.Logger, s *Service, t *view.Engine, c *shared.CSRFManager, r rbac.Middleware) *Handler {
	return &Handler{l, s, t, c, r}
}

func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermHRAttendanceImport, shared.PermHREmployeeView, shared.PermHREmployeeAdmin))
		r.Get("/", h.page)
		r.Get("/template", h.downloadTemplate)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermHRAttendanceImport, shared.PermHREmployeeAdmin))
		r.Post("/import", h.importCSV)
		r.Post("/record", h.recordManual)
	})
}

func hrFlashError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if strings.HasPrefix(msg, "hr: ") {
		trimmed := strings.TrimSpace(strings.TrimPrefix(msg, "hr: "))
		if len(trimmed) > 0 {
			return strings.ToUpper(trimmed[:1]) + trimmed[1:]
		}
	}
	return shared.UserSafeMessage(err)
}

func identity(r *http.Request) (int64, int64) {
	s := shared.SessionFromContext(r.Context())
	if s == nil {
		return 0, 0
	}
	u, _ := strconv.ParseInt(s.User(), 10, 64)
	c, _ := strconv.ParseInt(s.Get("company_id"), 10, 64)
	return u, c
}

func opt(v string) *int64 {
	id, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if id <= 0 {
		return nil
	}
	return &id
}

func (h *Handler) page(w http.ResponseWriter, r *http.Request) {
	_, company := identity(r)

	dateStr := strings.TrimSpace(r.URL.Query().Get("date"))
	var targetDate time.Time
	var err error
	if dateStr != "" {
		targetDate, err = time.Parse("2006-01-02", dateStr)
	}
	if err != nil || targetDate.IsZero() {
		targetDate = time.Now()
		dateStr = targetDate.Format("2006-01-02")
	}

	deptID := opt(r.URL.Query().Get("department_id"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	activeTab := strings.TrimSpace(r.URL.Query().Get("tab"))
	if activeTab == "" {
		activeTab = "daily"
	}

	records, stats, listErr := h.service.ListRecords(r.Context(), company, AttendanceFilter{
		Date:         targetDate,
		DepartmentID: deptID,
		Status:       status,
		Search:       q,
	})

	imports, _ := h.service.Recent(r.Context(), company)
	employees, _ := h.service.ListEmployees(r.Context(), company)
	departments, _ := h.service.ListDepartments(r.Context(), company)

	s := shared.SessionFromContext(r.Context())
	token, _ := h.csrf.EnsureToken(r.Context(), s)

	var selectedDept int64
	if deptID != nil {
		selectedDept = *deptID
	}

	_ = h.templates.Render(w, "pages/hr/attendance.html", view.TemplateData{
		Title:       "Attendance Management",
		CurrentPath: r.URL.Path,
		CSRFToken:   token,
		Flash:       s.PopFlash(),
		Data: map[string]any{
			"Records":      records,
			"Stats":        stats,
			"Imports":      imports,
			"Employees":    employees,
			"Departments":  departments,
			"SelectedDate": dateStr,
			"Filter": map[string]any{
				"DepartmentID": selectedDept,
				"Status":       status,
				"Search":       q,
				"Tab":          activeTab,
			},
			"Error": listErr,
		},
	})
}

func (h *Handler) recordManual(w http.ResponseWriter, r *http.Request) {
	_, company := identity(r)

	empID, _ := strconv.ParseInt(r.FormValue("employee_id"), 10, 64)
	dateStr := strings.TrimSpace(r.FormValue("date"))
	date, _ := time.Parse("2006-01-02", dateStr)
	if date.IsZero() {
		date = time.Now()
		dateStr = date.Format("2006-01-02")
	}

	status := strings.TrimSpace(r.FormValue("status"))
	if status == "" {
		status = "PRESENT"
	}

	var checkIn, checkOut *time.Time
	if inTime := strings.TrimSpace(r.FormValue("check_in")); inTime != "" {
		if t, err := time.Parse("2006-01-02 15:04", dateStr+" "+inTime); err == nil {
			checkIn = &t
		}
	}
	if outTime := strings.TrimSpace(r.FormValue("check_out")); outTime != "" {
		if t, err := time.Parse("2006-01-02 15:04", dateStr+" "+outTime); err == nil {
			checkOut = &t
		}
	}

	err := h.service.RecordManual(r.Context(), company, ManualRecordInput{
		EmployeeID: empID,
		Date:       date,
		CheckIn:    checkIn,
		CheckOut:   checkOut,
		Status:     status,
	})

	s := shared.SessionFromContext(r.Context())
	if err != nil {
		s.AddFlash(shared.FlashMessage{Kind: "error", Message: hrFlashError(err)})
	} else {
		s.AddFlash(shared.FlashMessage{Kind: "success", Message: "Attendance recorded successfully"})
	}
	http.Redirect(w, r, "/hr/attendance?tab=daily&date="+dateStr, http.StatusSeeOther)
}

func (h *Handler) downloadTemplate(w http.ResponseWriter, r *http.Request) {
	templateBytes := h.service.CSVTemplate()
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="attendance_template.csv"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(templateBytes)
}

func (h *Handler) importCSV(w http.ResponseWriter, r *http.Request) {
	user, company := identity(r)
	err := r.ParseMultipartForm(5 << 20)
	var result ImportResult
	if err == nil {
		file, header, e := r.FormFile("file")
		err = e
		if e == nil {
			defer file.Close()
			result, err = h.service.Import(r.Context(), company, user, header.Filename, file)
		}
	}
	s := shared.SessionFromContext(r.Context())
	if err != nil {
		s.AddFlash(shared.FlashMessage{Kind: "error", Message: hrFlashError(err)})
	} else {
		s.AddFlash(shared.FlashMessage{Kind: "success", Message: "Imported " + strconv.Itoa(result.Accepted) + " attendance rows"})
	}
	http.Redirect(w, r, "/hr/attendance?tab=import", http.StatusSeeOther)
}
