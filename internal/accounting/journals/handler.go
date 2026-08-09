package journals

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type Handler struct {
	service   *Service
	logger    *slog.Logger
	templates *view.Engine
	db        *pgxpool.Pool
	csrf      *shared.CSRFManager
}

func NewHandler(logger *slog.Logger, service *Service, templates *view.Engine, db *pgxpool.Pool, csrf *shared.CSRFManager) *Handler {
	return &Handler{logger: logger, service: service, templates: templates, db: db, csrf: csrf}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	entries, err := h.service.List(r.Context())
	if err != nil {
		h.logger.Error("list journals", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	data := map[string]any{"JournalEntries": entries}
	viewData := view.TemplateData{Title: "Journal Entries", CSRFToken: shared.CSRFTokenFromContext(r.Context()), Data: data}
	if err := h.templates.Render(w, "pages/accounting/journals_list.html", viewData); err != nil {
		h.logger.Error("render journals", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", 400)
		return
	}
	periodID, _ := strconv.ParseInt(r.PostFormValue("period_id"), 10, 64)
	date, err := time.Parse("2006-01-02", r.PostFormValue("date"))
	if err != nil {
		http.Error(w, "Invalid date", 400)
		return
	}
	accountIDs := r.PostForm["account_id"]
	debits := r.PostForm["debit"]
	credits := r.PostForm["credit"]
	departments := r.PostForm["department_id"]
	centers := r.PostForm["cost_center_id"]
	lines := make([]PostingLineInput, 0, len(accountIDs))
	for i, raw := range accountIDs {
		accountID, _ := strconv.ParseInt(raw, 10, 64)
		debit, _ := strconv.ParseFloat(valueAt(debits, i), 64)
		credit, _ := strconv.ParseFloat(valueAt(credits, i), 64)
		departmentID, _ := strconv.ParseInt(valueAt(departments, i), 10, 64)
		centerID, _ := strconv.ParseInt(valueAt(centers, i), 10, 64)
		var department, center *int64
		if departmentID > 0 {
			department = &departmentID
		}
		if centerID > 0 {
			center = &centerID
		}
		lines = append(lines, PostingLineInput{AccountID: accountID, Debit: debit, Credit: credit, DepartmentID: department, CostCenterID: center})
	}
	sess := shared.SessionFromContext(r.Context())
	actorID := int64(0)
	if sess != nil {
		actorID, _ = strconv.ParseInt(sess.User(), 10, 64)
	}
	_, err = h.service.PostJournal(r.Context(), PostingInput{PeriodID: periodID, Date: date, SourceModule: "MANUAL", SourceID: uuid.New(), Memo: r.PostFormValue("memo"), PostedBy: actorID, Lines: lines})
	if err != nil {
		shared.WriteErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	http.Redirect(w, r, "/accounting/journals", http.StatusSeeOther)
}

func (h *Handler) New(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		http.Error(w, "Database unavailable", 500)
		return
	}
	periods, _ := h.db.Query(r.Context(), `SELECT id,code,code FROM periods WHERE status='OPEN' ORDER BY start_date`)
	defer func() {
		if periods != nil {
			periods.Close()
		}
	}()
	accounts, _ := h.db.Query(r.Context(), `SELECT id,code,name FROM accounts WHERE is_active=true ORDER BY code`)
	defer func() {
		if accounts != nil {
			accounts.Close()
		}
	}()
	departments, _ := h.db.Query(r.Context(), `SELECT id,code,name FROM departments WHERE is_active=true ORDER BY code`)
	defer func() {
		if departments != nil {
			departments.Close()
		}
	}()
	centers, _ := h.db.Query(r.Context(), `SELECT id,code,name FROM cost_centers WHERE is_active=true ORDER BY code`)
	defer func() {
		if centers != nil {
			centers.Close()
		}
	}()
	data := map[string]any{"Periods": rowsToMaps(periods), "Accounts": rowsToMaps(accounts), "Departments": rowsToMaps(departments), "CostCenters": rowsToMaps(centers), "Today": time.Now().Format("2006-01-02")}
	token := ""
	if h.csrf != nil {
		token, _ = h.csrf.EnsureToken(r.Context(), shared.SessionFromContext(r.Context()))
	}
	_ = h.templates.Render(w, "pages/accounting/journal_form.html", view.TemplateData{Title: "New journal entry", CSRFToken: token, Data: data})
}
func valueAt(values []string, i int) string {
	if i < len(values) {
		return values[i]
	}
	return ""
}
func rowsToMaps(rows interface {
	Next() bool
	Scan(...any) error
}) []map[string]any {
	result := []map[string]any{}
	for rows != nil && rows.Next() {
		var id int64
		var code, name string
		if rows.Scan(&id, &code, &name) == nil {
			result = append(result, map[string]any{"ID": id, "Code": code, "Name": name})
		}
	}
	return result
}

func (h *Handler) Void(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented yet", http.StatusNotImplemented)
}

func (h *Handler) Reverse(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented yet", http.StatusNotImplemented)
}
