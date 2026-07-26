package accounting

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting/accounts"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/assets"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/banks"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/dimensions"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/reports"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/schedules"
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
	assets         *assets.Service
	dimensions     *dimensions.Service
	schedules      *schedules.Service
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
	assetRepo := assets.NewRepository(db)
	assetService := assets.NewService(assetRepo)
	dimRepo := dimensions.NewRepository(db)
	dimService := dimensions.NewService(dimRepo)
	schedRepo := schedules.NewRepository(db)
	schedService := schedules.NewService(schedRepo)

	// Handlers
	accountHandler := accounts.NewHandler(logger, accountService, templates)
	journalHandler := journals.NewHandler(logger, journalService, templates, db, csrf)
	banksHandler := banks.NewHandler(logger, bankService, templates, csrf)

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
		assets:         assetService,
		dimensions:     dimService,
		schedules:      schedService,
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
	r.Post("/report-schedules/{id}/retry", h.retryReportSchedule)
	r.Get("/fixed-assets", h.handleFixedAssets)
	r.Get("/fixed-assets/new", h.showFixedAssetForm)
	r.Post("/fixed-assets", h.createFixedAsset)
	r.Get("/fixed-assets/categories", h.handleFixedAssetCategories)
	r.Post("/fixed-assets/categories", h.createFixedAssetCategory)
	r.Post("/fixed-assets/{id}/dispose", h.disposeFixedAsset)
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
	deps, err := h.dimensions.ListDepartments(r.Context(), companyID)
	if err != nil {
		h.reportError(w, err)
		return
	}
	centers, err := h.dimensions.ListCostCenters(r.Context(), companyID)
	if err != nil {
		h.reportError(w, err)
		return
	}
	h.renderAdmin(w, r, "pages/finance/dimensions.html", "Reporting dimensions", map[string]any{"Departments": deps, "CostCenters": centers})
}

func (h *Handler) createDepartment(w http.ResponseWriter, r *http.Request) {
	companyID := h.companyID(r)
	code := strings.TrimSpace(r.PostFormValue("code"))
	name := strings.TrimSpace(r.PostFormValue("name"))
	if companyID == 0 || code == "" || name == "" {
		shared.WriteHTTPError(w, http.StatusBadRequest, "Company, code, dan nama wajib diisi")
		return
	}
	if err := h.dimensions.CreateDepartment(r.Context(), companyID, code, name); err != nil {
		h.reportError(w, err)
		return
	}
	h.auditRecord(r, "create", "departments", code)
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
	if err := h.dimensions.CreateCostCenter(r.Context(), companyID, code, name, departmentID); err != nil {
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
	scheds, err := h.schedules.List(r.Context(), companyID)
	if err != nil {
		h.reportError(w, err)
		return
	}
	deps, _ := h.dimensions.ListDepartments(r.Context(), companyID)
	centers, _ := h.dimensions.ListCostCenters(r.Context(), companyID)
	h.renderAdmin(w, r, "pages/finance/report_schedules.html", "Report schedules", map[string]any{"Schedules": scheds, "Departments": deps, "CostCenters": centers})
}

func (h *Handler) createReportSchedule(w http.ResponseWriter, r *http.Request) {
	companyID := h.companyID(r)
	typ := r.PostFormValue("report_type")
	frequency := r.PostFormValue("frequency")
	departmentID, _ := strconv.ParseInt(r.PostFormValue("department_id"), 10, 64)
	costCenterID, _ := strconv.ParseInt(r.PostFormValue("cost_center_id"), 10, 64)
	offset, _ := strconv.Atoi(r.PostFormValue("period_offset_months"))
	recipients := []string{}
	for _, value := range strings.Split(r.PostFormValue("recipients"), ",") {
		if email := strings.TrimSpace(value); email != "" {
			recipients = append(recipients, email)
		}
	}
	if err := h.schedules.Create(r.Context(), companyID, typ, frequency, departmentID, costCenterID, offset, recipients); err != nil {
		h.reportError(w, err)
		return
	}
	h.auditRecord(r, "create", "report_schedule", typ)
	http.Redirect(w, r, "/accounting/report-schedules", http.StatusSeeOther)
}

func (h *Handler) retryReportSchedule(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	companyID := h.companyID(r)
	if err := h.schedules.Retry(r.Context(), id, companyID); err != nil {
		h.reportError(w, err)
		return
	}
	h.auditRecord(r, "retry", "report_schedule", strconv.FormatInt(id, 10))
	http.Redirect(w, r, "/accounting/report-schedules", http.StatusSeeOther)
}

func (h *Handler) handleFixedAssets(w http.ResponseWriter, r *http.Request) {
	companyID := h.companyID(r)
	assets, err := h.assets.ListAssets(r.Context(), companyID)
	if err != nil {
		h.reportError(w, err)
		return
	}
	h.renderAdmin(w, r, "pages/finance/fixed_assets.html", "Fixed assets", map[string]any{"Assets": assets})
}

func (h *Handler) handleFixedAssetCategories(w http.ResponseWriter, r *http.Request) {
	companyID := h.companyID(r)
	cats, err := h.assets.ListCategories(r.Context(), companyID)
	if err != nil {
		h.reportError(w, err)
		return
	}
	accounts, _ := h.db.Query(r.Context(), `SELECT id,code,name FROM accounts WHERE is_active=true ORDER BY code`)
	defer func() {
		if accounts != nil {
			accounts.Close()
		}
	}()
	h.renderAdmin(w, r, "pages/finance/fixed_asset_categories.html", "Fixed asset categories", map[string]any{"Categories": cats, "Accounts": rowsToDimensionMaps(accounts)})
}

func (h *Handler) createFixedAssetCategory(w http.ResponseWriter, r *http.Request) {
	companyID := h.companyID(r)
	ids := []int64{}
	for _, key := range []string{"asset_account_id", "accumulated_depreciation_account_id", "depreciation_expense_account_id", "cash_proceeds_account_id", "disposal_gain_account_id", "disposal_loss_account_id"} {
		id, _ := strconv.ParseInt(r.PostFormValue(key), 10, 64)
		ids = append(ids, id)
	}
	life, _ := strconv.Atoi(r.PostFormValue("useful_life_months"))
	residual, _ := strconv.ParseFloat(r.PostFormValue("residual_rate"), 64)
	if companyID == 0 || life <= 0 || ids[0] == 0 || ids[1] == 0 || ids[2] == 0 {
		shared.WriteHTTPError(w, 400, "Kategori aset tidak valid")
		return
	}
	if err := h.assets.CreateCategory(r.Context(), companyID, r.PostFormValue("code"), r.PostFormValue("name"), ids[0], ids[1], ids[2], ids[3], ids[4], ids[5], life, residual); err != nil {
		h.reportError(w, err)
		return
	}
	h.auditRecord(r, "create", "fixed_asset_category", r.PostFormValue("code"))
	http.Redirect(w, r, "/accounting/fixed-assets/categories", 303)
}

func (h *Handler) showFixedAssetForm(w http.ResponseWriter, r *http.Request) {
	companyID := h.companyID(r)
	cats, err := h.assets.ListCategories(r.Context(), companyID)
	if err != nil {
		h.reportError(w, err)
		return
	}
	h.renderAdmin(w, r, "pages/finance/fixed_asset_form.html", "New fixed asset", map[string]any{"Categories": cats})
}

func rowsToDimensionMaps(rows interface {
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

func (h *Handler) createFixedAsset(w http.ResponseWriter, r *http.Request) {
	companyID := h.companyID(r)
	categoryID, _ := strconv.ParseInt(r.PostFormValue("category_id"), 10, 64)
	cost, _ := strconv.ParseFloat(r.PostFormValue("acquisition_cost"), 64)
	life, _ := strconv.Atoi(r.PostFormValue("useful_life_months"))
	date, err := time.Parse("2006-01-02", r.PostFormValue("in_service_date"))
	if err != nil || companyID == 0 || categoryID == 0 || cost <= 0 || life <= 0 {
		shared.WriteHTTPError(w, 400, "Data aset tidak valid")
		return
	}
	if err := h.assets.CreateAsset(r.Context(), companyID, categoryID, r.PostFormValue("number"), r.PostFormValue("name"), date, date, cost, life); err != nil {
		h.reportError(w, err)
		return
	}
	h.auditRecord(r, "create", "fixed_asset", r.PostFormValue("number"))
	http.Redirect(w, r, "/accounting/fixed-assets", 303)
}

func (h *Handler) disposeFixedAsset(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	proceeds, _ := strconv.ParseFloat(r.PostFormValue("proceeds"), 64)
	date, err := time.Parse("2006-01-02", r.PostFormValue("disposal_date"))
	if err != nil {
		shared.WriteHTTPError(w, 400, "Tanggal disposal tidak valid")
		return
	}
	if err := h.assets.DisposeAsset(r.Context(), id, date, proceeds); err != nil {
		h.reportError(w, err)
		return
	}
	h.auditRecord(r, "dispose", "fixed_asset", strconv.FormatInt(id, 10))
	http.Redirect(w, r, "/accounting/fixed-assets", 303)
}

func (h *Handler) toggleReportSchedule(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	companyID := h.companyID(r)
	if err := h.schedules.Toggle(r.Context(), id, companyID); err != nil {
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
	h.accountHandler.List(w, r)
}

func (h *Handler) handleTrialBalance(w http.ResponseWriter, r *http.Request) {
	balances, err := h.accountService.ListBalances(r.Context())
	if err != nil {
		h.reportError(w, err)
		return
	}
	tb := reports.BuildTrialBalance(balances)
	data := map[string]any{"Report": tb}
	if err := h.templates.Render(w, "pages/finance/trial_balance.html", view.TemplateData{Title: "Trial Balance", CSRFToken: h.csrfToken(r), Data: data}); err != nil {
		h.reportError(w, err)
	}
}

func (h *Handler) handleBalanceSheet(w http.ResponseWriter, r *http.Request) {
	balances, err := h.accountService.ListBalances(r.Context())
	if err != nil {
		h.reportError(w, err)
		return
	}
	bs := reports.BuildBalanceSheet(balances)
	data := map[string]any{"Report": bs, "AsOfDate": time.Now()}
	if err := h.templates.Render(w, "pages/finance/balance_sheet.html", view.TemplateData{Title: "Balance Sheet", CSRFToken: h.csrfToken(r), Data: data}); err != nil {
		h.reportError(w, err)
	}
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
		Title:     "Cash Flow",
		CSRFToken: h.csrfToken(r),
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
		Title:     "Budget vs Actual",
		CSRFToken: h.csrfToken(r),
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
	if err := h.templates.Render(w, "pages/finance/pnl.html", view.TemplateData{Title: "Profit and Loss", CSRFToken: h.csrfToken(r), Data: data}); err != nil {
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
	var buf bytes.Buffer
	if err := reports.WriteProfitAndLossXLSX(&buf, reports.BuildProfitAndLoss(balances), fmt.Sprintf("%04d-%02d", year, month)); err != nil {
		h.logger.Error("write P&L workbook", slog.Any("error", err))
		h.reportError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=profit-loss-%04d-%02d.xlsx", year, month))
	if _, err := w.Write(buf.Bytes()); err != nil {
		h.logger.Error("write P&L response", slog.Any("error", err))
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
	var buf bytes.Buffer
	if err := reports.WriteBudgetVsActualXLSX(&buf, reports.BuildBudgetVsActual(balances, budgets), fmt.Sprintf("%04d-%02d", year, month)); err != nil {
		h.logger.Error("write budget workbook", slog.Any("error", err))
		h.reportError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=budget-vs-actual-%04d-%02d.xlsx", year, month))
	if _, err := w.Write(buf.Bytes()); err != nil {
		h.logger.Error("write budget response", slog.Any("error", err))
	}
}

func (h *Handler) reportError(w http.ResponseWriter, err error) {
	h.logger.Error("load accounting report", slog.Any("error", err))
	shared.WriteHTTPError(w, http.StatusInternalServerError, "Gagal memuat data laporan")
}

func budgetPeriod(r *http.Request, now time.Time) (int, time.Month, error) {
	period := r.URL.Query().Get("period")
	if period == "" {
		return now.Year(), now.Month(), nil
	}
	var year int
	var month int
	if _, err := fmt.Sscanf(period, "%d-%d", &year, &month); err != nil {
		return 0, 0, err
	}
	if month < 1 || month > 12 {
		return 0, 0, fmt.Errorf("invalid month")
	}
	return year, time.Month(month), nil
}
