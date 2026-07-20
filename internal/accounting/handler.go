package accounting

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting/accounts"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/banks"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/reports"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// Handler wires finance ledger endpoints.
type Handler struct {
	logger         *slog.Logger
	templates      *view.Engine
	accountHandler *accounts.Handler
	accountService *accounts.Service
	journalHandler *journals.Handler
	banksHandler   *banks.Handler
	budgets        *sqlc.Queries
	db             *pgxpool.Pool
	audit          journals.AuditPort
	csrf           *shared.CSRFManager
}

// NewHandler builds a Handler instance.
func NewHandler(logger *slog.Logger, db *pgxpool.Pool, templates *view.Engine, csrf *shared.CSRFManager, audit journals.AuditPort, guard journals.PeriodGuard) *Handler {
	// Repositories
	accountRepo := accounts.NewRepository(db)
	journalRepo := journals.NewRepository(db)

	// Services
	accountService := accounts.NewService(accountRepo)
	journalService := journals.NewService(journalRepo, audit, guard)
	bankService := banks.NewService(db)

	// Handlers
	accountHandler := accounts.NewHandler(logger, accountService, templates)
	journalHandler := journals.NewHandler(logger, journalService, templates)
	banksHandler := banks.NewHandler(logger, bankService, templates)

	return &Handler{
		logger:         logger,
		templates:      templates,
		accountHandler: accountHandler,
		accountService: accountService,
		journalHandler: journalHandler,
		banksHandler:   banksHandler,
		budgets:        sqlc.New(db),
		db:             db,
		audit:          audit,
		csrf:           csrf,
	}
}

// MountRoutes registers HTTP routes for the ledger module.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Route("/coa", func(r chi.Router) {
		h.accountHandler.MountRoutes(r)
	})
	r.Route("/journals", func(r chi.Router) {
		h.journalHandler.MountRoutes(r)
	})
	r.Route("/banks", func(r chi.Router) {
		h.banksHandler.MountRoutes(r)
	})
	r.Get("/gl", h.handleGeneralLedger)
	r.Get("/trial-balance", h.handleTrialBalance)
	r.Get("/pnl", h.handleProfitLoss)
	r.Get("/balance-sheet", h.handleBalanceSheet)
	r.Get("/cash-flow", h.handleCashFlow)
	r.Get("/budget", h.handleBudget)
	r.Get("/dimensions", h.handleDimensions)
	r.Post("/dimensions/departments", h.createDepartment)
	r.Post("/dimensions/cost-centers", h.createCostCenter)
	r.Get("/report-schedules", h.handleReportSchedules)
	r.Post("/report-schedules", h.createReportSchedule)
	r.Post("/report-schedules/{id}/toggle", h.toggleReportSchedule)
	r.Get("/pnl/export.xlsx", h.handleProfitLossExcel)
	r.Get("/budget/export.xlsx", h.handleBudgetExcel)

	r.Get("/finance/reports/trial-balance/pdf", h.handleNotImplemented)
	r.Get("/finance/reports/pl/pdf", h.handleNotImplemented)
	r.Get("/finance/reports/bs/pdf", h.handleNotImplemented)
}

func (h *Handler) companyID(r *http.Request) int64 {
	sess := shared.SessionFromContext(r.Context())
	if sess == nil {
		return 0
	}
	id, _ := strconv.ParseInt(sess.Get("company_id"), 10, 64)
	return id
}
func (h *Handler) csrfToken(r *http.Request) string {
	if h.csrf == nil {
		return ""
	}
	token, _ := h.csrf.EnsureToken(r.Context(), shared.SessionFromContext(r.Context()))
	return token
}
func (h *Handler) renderAdmin(w http.ResponseWriter, r *http.Request, name, title string, data map[string]any) {
	if err := h.templates.Render(w, name, view.TemplateData{Title: title, CSRFToken: h.csrfToken(r), Data: data}); err != nil {
		h.reportError(w, err)
	}
}

func (h *Handler) handleDimensions(w http.ResponseWriter, r *http.Request) {
	companyID := h.companyID(r)
	if companyID == 0 {
		shared.WriteHTTPError(w, http.StatusBadRequest, "Pilih perusahaan aktif terlebih dahulu")
		return
	}
	departments, err := h.db.Query(r.Context(), `SELECT id, code, name, is_active FROM departments WHERE company_id=$1 ORDER BY code`, companyID)
	if err != nil {
		h.reportError(w, err)
		return
	}
	defer departments.Close()
	var deps []map[string]any
	for departments.Next() {
		var id int64
		var code, name string
		var active bool
		if departments.Scan(&id, &code, &name, &active) == nil {
			deps = append(deps, map[string]any{"ID": id, "Code": code, "Name": name, "Active": active})
		}
	}
	centers, err := h.db.Query(r.Context(), `SELECT c.id, c.code, c.name, COALESCE(d.name,''), c.is_active FROM cost_centers c LEFT JOIN departments d ON d.id=c.department_id WHERE c.company_id=$1 ORDER BY c.code`, companyID)
	if err != nil {
		h.reportError(w, err)
		return
	}
	defer centers.Close()
	var costs []map[string]any
	for centers.Next() {
		var id int64
		var code, name, department string
		var active bool
		if centers.Scan(&id, &code, &name, &department, &active) == nil {
			costs = append(costs, map[string]any{"ID": id, "Code": code, "Name": name, "Department": department, "Active": active})
		}
	}
	h.renderAdmin(w, r, "pages/finance/dimensions.html", "Reporting dimensions", map[string]any{"Departments": deps, "CostCenters": costs})
}

func (h *Handler) createDepartment(w http.ResponseWriter, r *http.Request) {
	h.createDimension(w, r, "departments")
}
func (h *Handler) createDimension(w http.ResponseWriter, r *http.Request, table string) {
	companyID := h.companyID(r)
	code := strings.TrimSpace(r.PostFormValue("code"))
	name := strings.TrimSpace(r.PostFormValue("name"))
	if companyID == 0 || code == "" || name == "" {
		shared.WriteHTTPError(w, http.StatusBadRequest, "Company, code, dan nama wajib diisi")
		return
	}
	_, err := h.db.Exec(r.Context(), "INSERT INTO "+table+" (company_id, code, name) VALUES ($1,$2,$3)", companyID, code, name)
	if err != nil {
		h.reportError(w, err)
		return
	}
	h.auditRecord(r, "create", table, code)
	http.Redirect(w, r, "/accounting/dimensions", http.StatusSeeOther)
}
func (h *Handler) createCostCenter(w http.ResponseWriter, r *http.Request) {
	companyID := h.companyID(r)
	code := strings.TrimSpace(r.PostFormValue("code"))
	name := strings.TrimSpace(r.PostFormValue("name"))
	departmentID, _ := strconv.ParseInt(r.PostFormValue("department_id"), 10, 64)
	if companyID == 0 || code == "" || name == "" {
		shared.WriteHTTPError(w, http.StatusBadRequest, "Company, code, dan nama wajib diisi")
		return
	}
	_, err := h.db.Exec(r.Context(), `INSERT INTO cost_centers (company_id, department_id, code, name) VALUES ($1,NULLIF($2,0),$3,$4)`, companyID, departmentID, code, name)
	if err != nil {
		h.reportError(w, err)
		return
	}
	h.auditRecord(r, "create", "cost_centers", code)
	http.Redirect(w, r, "/accounting/dimensions", http.StatusSeeOther)
}

func (h *Handler) handleReportSchedules(w http.ResponseWriter, r *http.Request) {
	companyID := h.companyID(r)
	if companyID == 0 {
		shared.WriteHTTPError(w, http.StatusBadRequest, "Pilih perusahaan aktif terlebih dahulu")
		return
	}
	rows, err := h.db.Query(r.Context(), `SELECT id,report_type,recipients,frequency,is_active,last_sent_at FROM report_schedules WHERE company_id=$1 ORDER BY created_at DESC`, companyID)
	if err != nil {
		h.reportError(w, err)
		return
	}
	defer rows.Close()
	var schedules []map[string]any
	for rows.Next() {
		var id int64
		var typ, frequency string
		var recipients []string
		var active bool
		var lastSent *time.Time
		if rows.Scan(&id, &typ, &recipients, &frequency, &active, &lastSent) == nil {
			schedules = append(schedules, map[string]any{"ID": id, "Type": typ, "Recipients": strings.Join(recipients, ", "), "Frequency": frequency, "Active": active, "LastSent": lastSent})
		}
	}
	h.renderAdmin(w, r, "pages/finance/report_schedules.html", "Report schedules", map[string]any{"Schedules": schedules})
}
func (h *Handler) createReportSchedule(w http.ResponseWriter, r *http.Request) {
	companyID := h.companyID(r)
	typ := r.PostFormValue("report_type")
	frequency := r.PostFormValue("frequency")
	recipients := []string{}
	for _, value := range strings.Split(r.PostFormValue("recipients"), ",") {
		if email := strings.TrimSpace(value); email != "" {
			recipients = append(recipients, email)
		}
	}
	if companyID == 0 || len(recipients) == 0 || (typ != "PNL" && typ != "BUDGET_VS_ACTUAL") || (frequency != "DAILY" && frequency != "WEEKLY" && frequency != "MONTHLY") {
		shared.WriteHTTPError(w, http.StatusBadRequest, "Konfigurasi schedule tidak valid")
		return
	}
	_, err := h.db.Exec(r.Context(), `INSERT INTO report_schedules (company_id,report_type,recipients,frequency) VALUES ($1,$2,$3,$4)`, companyID, typ, recipients, frequency)
	if err != nil {
		h.reportError(w, err)
		return
	}
	h.auditRecord(r, "create", "report_schedule", typ)
	http.Redirect(w, r, "/accounting/report-schedules", http.StatusSeeOther)
}
func (h *Handler) toggleReportSchedule(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	companyID := h.companyID(r)
	_, err := h.db.Exec(r.Context(), `UPDATE report_schedules SET is_active=NOT is_active,updated_at=NOW() WHERE id=$1 AND company_id=$2`, id, companyID)
	if err != nil {
		h.reportError(w, err)
		return
	}
	h.auditRecord(r, "toggle", "report_schedule", strconv.FormatInt(id, 10))
	http.Redirect(w, r, "/accounting/report-schedules", http.StatusSeeOther)
}
func (h *Handler) auditRecord(r *http.Request, action, entity, entityID string) {
	if h.audit == nil {
		return
	}
	sess := shared.SessionFromContext(r.Context())
	actorID := int64(0)
	if sess != nil {
		actorID, _ = strconv.ParseInt(sess.User(), 10, 64)
	}
	_ = h.audit.Record(r.Context(), shared.AuditLog{ActorID: actorID, Action: "reporting." + action, Entity: entity, EntityID: entityID, Meta: map[string]any{"company_id": h.companyID(r)}})
}

func (h *Handler) handleGeneralLedger(w http.ResponseWriter, r *http.Request) {
	// Proxy to Account Service for List (MVP behavior)
	h.accountHandler.List(w, r)
}

func (h *Handler) handleTrialBalance(w http.ResponseWriter, r *http.Request) {
	h.accountHandler.List(w, r)
}

func (h *Handler) handleBalanceSheet(w http.ResponseWriter, r *http.Request) {
	h.accountHandler.List(w, r)
}

func (h *Handler) handleProfitLoss(w http.ResponseWriter, r *http.Request) {
	year, month, err := budgetPeriod(r, time.Now())
	if err != nil {
		shared.WriteHTTPError(w, http.StatusBadRequest, "Periode harus berformat YYYY-MM")
		return
	}
	filter := dimensionFilter(r)
	balances, err := h.accountService.ListBalancesForPeriodAndDimensions(r.Context(), year, month, filter)
	if err != nil {
		h.reportError(w, err)
		return
	}
	h.renderProfitLoss(w, r, reports.BuildProfitAndLoss(balances), year, month, filter)
}

func (h *Handler) handleNotImplemented(w http.ResponseWriter, _ *http.Request) {
	h.logger.Info("ledger handler invoked", slog.String("path", "finance"))
	shared.WriteHTTPError(w, http.StatusNotImplemented, "")
}

func (h *Handler) handleCashFlow(w http.ResponseWriter, r *http.Request) {
	balances, err := h.accountService.ListBalances(r.Context())
	if err != nil {
		h.logger.Error("list balances for cash flow", slog.Any("error", err))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "Gagal memuat data laporan")
		return
	}

	cf := reports.BuildCashFlow(balances)
	viewData := view.TemplateData{
		Title: "Cash Flow",
		Data: map[string]any{
			"Report": cf,
		},
	}
	if err := h.templates.Render(w, "pages/finance/cashflow.html", viewData); err != nil {
		h.logger.Error("render cash flow", slog.Any("error", err))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "")
	}
}

func (h *Handler) handleBudget(w http.ResponseWriter, r *http.Request) {
	year, month, err := budgetPeriod(r, time.Now())
	if err != nil {
		shared.WriteHTTPError(w, http.StatusBadRequest, "Periode harus berformat YYYY-MM")
		return
	}
	filter := dimensionFilter(r)
	balances, err := h.accountService.ListBalancesForPeriodAndDimensions(r.Context(), year, month, filter)
	if err != nil {
		h.logger.Error("list balances for budget", slog.Any("error", err))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "Gagal memuat data laporan")
		return
	}

	rows, err := h.budgets.ListBudgetsByPeriod(r.Context(), sqlc.ListBudgetsByPeriodParams{PeriodYear: int32(year), PeriodMonth: int32(month)})
	if err != nil {
		h.logger.Error("list budgets", slog.Any("error", err))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "Gagal memuat data laporan")
		return
	}
	budgetData := make(reports.BudgetData, len(rows))
	for _, row := range rows {
		amount, err := row.Amount.Float64Value()
		if err != nil || !amount.Valid {
			h.logger.Error("invalid budget amount", slog.Int64("budget_id", row.ID))
			shared.WriteHTTPError(w, http.StatusInternalServerError, "Gagal memuat data laporan")
			return
		}
		budgetData[row.AccountID] = amount.Float64
	}
	bva := reports.BuildBudgetVsActual(balances, budgetData)

	viewData := view.TemplateData{
		Title: "Budget vs Actual",
		Data: map[string]any{
			"Report":      bva,
			"Period":      time.Date(year, month, 1, 0, 0, 0, 0, time.UTC).Format("January 2006"),
			"PeriodValue": time.Date(year, month, 1, 0, 0, 0, 0, time.UTC).Format("2006-01"),
			"Filter":      filter,
			"Departments": h.listDimensions(r, "departments"),
			"CostCenters": h.listDimensions(r, "cost_centers"),
		},
	}
	if err := h.templates.Render(w, "pages/finance/budget.html", viewData); err != nil {
		h.logger.Error("render budget", slog.Any("error", err))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "")
	}
}

type reportDimension struct {
	ID   int64
	Name string
}

func dimensionFilter(r *http.Request) reports.DimensionFilter {
	department, _ := strconv.ParseInt(r.URL.Query().Get("department_id"), 10, 64)
	costCenter, _ := strconv.ParseInt(r.URL.Query().Get("cost_center_id"), 10, 64)
	return reports.DimensionFilter{DepartmentID: department, CostCenterID: costCenter}
}

func (h *Handler) listDimensions(r *http.Request, table string) []reportDimension {
	if table != "departments" && table != "cost_centers" {
		return nil
	}
	rows, err := h.db.Query(r.Context(), "SELECT id, name FROM "+table+" WHERE is_active = true ORDER BY name")
	if err != nil {
		return nil
	}
	defer rows.Close()
	result := []reportDimension{}
	for rows.Next() {
		var item reportDimension
		if rows.Scan(&item.ID, &item.Name) == nil {
			result = append(result, item)
		}
	}
	return result
}

func (h *Handler) renderProfitLoss(w http.ResponseWriter, r *http.Request, report reports.ProfitAndLoss, year int, month time.Month, filter reports.DimensionFilter) {
	period := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	data := map[string]any{"Report": report, "Period": period.Format("January 2006"), "PeriodValue": period.Format("2006-01"), "Filter": filter, "Departments": h.listDimensions(r, "departments"), "CostCenters": h.listDimensions(r, "cost_centers")}
	if err := h.templates.Render(w, "pages/finance/pnl.html", view.TemplateData{Title: "Profit and Loss", Data: data}); err != nil {
		h.reportError(w, err)
	}
}

func (h *Handler) handleProfitLossExcel(w http.ResponseWriter, r *http.Request) {
	year, month, err := budgetPeriod(r, time.Now())
	if err != nil {
		shared.WriteHTTPError(w, http.StatusBadRequest, "Periode harus berformat YYYY-MM")
		return
	}
	balances, err := h.accountService.ListBalancesForPeriodAndDimensions(r.Context(), year, month, dimensionFilter(r))
	if err != nil {
		h.reportError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=profit-loss-%04d-%02d.xlsx", year, month))
	if err := reports.WriteProfitAndLossXLSX(w, reports.BuildProfitAndLoss(balances), fmt.Sprintf("%04d-%02d", year, month)); err != nil {
		h.logger.Error("write P&L workbook", slog.Any("error", err))
	}
}

func (h *Handler) handleBudgetExcel(w http.ResponseWriter, r *http.Request) {
	year, month, err := budgetPeriod(r, time.Now())
	if err != nil {
		shared.WriteHTTPError(w, http.StatusBadRequest, "Periode harus berformat YYYY-MM")
		return
	}
	balances, err := h.accountService.ListBalancesForPeriodAndDimensions(r.Context(), year, month, dimensionFilter(r))
	if err != nil {
		h.reportError(w, err)
		return
	}
	rows, err := h.budgets.ListBudgetsByPeriod(r.Context(), sqlc.ListBudgetsByPeriodParams{PeriodYear: int32(year), PeriodMonth: int32(month)})
	if err != nil {
		h.reportError(w, err)
		return
	}
	budgets := reports.BudgetData{}
	for _, row := range rows {
		value, _ := row.Amount.Float64Value()
		if value.Valid {
			budgets[row.AccountID] = value.Float64
		}
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=budget-vs-actual-%04d-%02d.xlsx", year, month))
	_ = reports.WriteBudgetVsActualXLSX(w, reports.BuildBudgetVsActual(balances, budgets), fmt.Sprintf("%04d-%02d", year, month))
}

func (h *Handler) reportError(w http.ResponseWriter, err error) {
	h.logger.Error("load accounting report", slog.Any("error", err))
	shared.WriteHTTPError(w, http.StatusInternalServerError, "Gagal memuat data laporan")
}

func budgetPeriod(r *http.Request, now time.Time) (int, time.Month, error) {
	value := r.URL.Query().Get("period")
	if value == "" {
		return now.Year(), now.Month(), nil
	}
	parsed, err := time.Parse("2006-01", value)
	if err != nil {
		return 0, 0, err
	}
	return parsed.Year(), parsed.Month(), nil
}
