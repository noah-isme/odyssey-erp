package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/odyssey-erp/odyssey-erp/internal/platform/cache"
	"github.com/odyssey-erp/odyssey-erp/internal/platform/db"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	fxcli "github.com/odyssey-erp/odyssey-erp/cmd/odyssey/cli"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/mappings"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/periods"
	"github.com/odyssey-erp/odyssey-erp/internal/analytics"
	"github.com/odyssey-erp/odyssey-erp/internal/analytics/export"
	analytichttp "github.com/odyssey-erp/odyssey-erp/internal/analytics/http"
	"github.com/odyssey-erp/odyssey-erp/internal/analytics/svg"
	"github.com/odyssey-erp/odyssey-erp/internal/ap"
	apihttp "github.com/odyssey-erp/odyssey-erp/internal/api"
	"github.com/odyssey-erp/odyssey-erp/internal/app"
	approvalengine "github.com/odyssey-erp/odyssey-erp/internal/approvals"
	"github.com/odyssey-erp/odyssey-erp/internal/ar"
	"github.com/odyssey-erp/odyssey-erp/internal/audit"
	audithttp "github.com/odyssey-erp/odyssey-erp/internal/audit/http"
	"github.com/odyssey-erp/odyssey-erp/internal/auth"
	boardpacksvc "github.com/odyssey-erp/odyssey-erp/internal/boardpack"
	boardpackhttp "github.com/odyssey-erp/odyssey-erp/internal/boardpack/http"
	closepkg "github.com/odyssey-erp/odyssey-erp/internal/close"
	closehttp "github.com/odyssey-erp/odyssey-erp/internal/close/http"
	"github.com/odyssey-erp/odyssey-erp/internal/cmms"
	cmmshttp "github.com/odyssey-erp/odyssey-erp/internal/cmms/http"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/awss3"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/dhl"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/midtrans"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/mockpay"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/oidc"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/openai"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/shopify"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/stripe"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/whatsapp"
	"github.com/odyssey-erp/odyssey-erp/internal/consol"
	consolhttp "github.com/odyssey-erp/odyssey-erp/internal/consol/http"
	"github.com/odyssey-erp/odyssey-erp/internal/crm"
	"github.com/odyssey-erp/odyssey-erp/internal/dashboard"
	deliveryorders "github.com/odyssey-erp/odyssey-erp/internal/delivery/orders"
	"github.com/odyssey-erp/odyssey-erp/internal/documents"
	documentshttp "github.com/odyssey-erp/odyssey-erp/internal/documents/http"
	eliminationpkg "github.com/odyssey-erp/odyssey-erp/internal/elimination"
	eliminationhttp "github.com/odyssey-erp/odyssey-erp/internal/elimination/http"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/bankfeeds"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/banking"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/forecasting"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/treasury"
	"github.com/odyssey-erp/odyssey-erp/internal/freight"
	fxservice "github.com/odyssey-erp/odyssey-erp/internal/fx"
	hrattendance "github.com/odyssey-erp/odyssey-erp/internal/hr/attendance"
	hremployees "github.com/odyssey-erp/odyssey-erp/internal/hr/employees"
	hrleave "github.com/odyssey-erp/odyssey-erp/internal/hr/leave"
	"github.com/odyssey-erp/odyssey-erp/internal/insights"
	insightshhtp "github.com/odyssey-erp/odyssey-erp/internal/insights/http"
	"github.com/odyssey-erp/odyssey-erp/internal/integration"
	"github.com/odyssey-erp/odyssey-erp/internal/inventory"
	jobmetrics "github.com/odyssey-erp/odyssey-erp/internal/jobs"
	"github.com/odyssey-erp/odyssey-erp/internal/logistics"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata"
	"github.com/odyssey-erp/odyssey-erp/internal/mrp"
	"github.com/odyssey-erp/odyssey-erp/internal/notifications"
	"github.com/odyssey-erp/odyssey-erp/internal/observability"
	"github.com/odyssey-erp/odyssey-erp/internal/outbox"
	"github.com/odyssey-erp/odyssey-erp/internal/payroll"
	"github.com/odyssey-erp/odyssey-erp/internal/portal"
	"github.com/odyssey-erp/odyssey-erp/internal/pos"
	"github.com/odyssey-erp/odyssey-erp/internal/procurement"
	"github.com/odyssey-erp/odyssey-erp/internal/projects"
	"github.com/odyssey-erp/odyssey-erp/internal/qms"
	qmshttp "github.com/odyssey-erp/odyssey-erp/internal/qms/http"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/roles"
	"github.com/odyssey-erp/odyssey-erp/internal/sales"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/storage"
	"github.com/odyssey-erp/odyssey-erp/internal/tax"
	"github.com/odyssey-erp/odyssey-erp/internal/users"
	variancepkg "github.com/odyssey-erp/odyssey-erp/internal/variance"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
	"github.com/odyssey-erp/odyssey-erp/internal/wms"
	"github.com/odyssey-erp/odyssey-erp/jobs"
	"github.com/odyssey-erp/odyssey-erp/report"
)

type lineRenderer struct{}

func (lineRenderer) Line(width, height int, series []float64, labels []string, opts svg.LineOpts) (template.HTML, error) {
	return svg.Line(width, height, series, labels, opts)
}

type barRenderer struct{}

func (barRenderer) Bars(width, height int, seriesA, seriesB []float64, labels []string, opts svg.BarOpts) (template.HTML, error) {
	return svg.Bars(width, height, seriesA, seriesB, labels, opts)
}

type notificationEmailQueue struct{ client *jobs.Client }

type payrollDeliveryQueue struct{ client *jobs.Client }

func (q payrollDeliveryQueue) EnqueuePayslip(ctx context.Context, line payroll.RunLine) error {
	task, err := jobs.NewPayrollPayslipTask(line.PayslipID)
	if err != nil {
		return err
	}
	_, err = q.client.AsynqClient().EnqueueContext(ctx, task, asynq.Queue(jobs.QueueDefault), asynq.MaxRetry(5), asynq.TaskID(fmt.Sprintf("payroll-payslip-%d", line.PayslipID)))
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	return err
}

func (q notificationEmailQueue) EnqueueEmail(ctx context.Context, email notifications.Email) error {
	_, err := q.client.EnqueueSendEmail(ctx, jobs.SendEmailPayload{To: email.To, Subject: email.Subject, Body: email.Body, CorrelationID: email.CorrelationID})
	return err
}

type analyticsPeriodValidator struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func (v analyticsPeriodValidator) ValidatePeriod(ctx context.Context, period string) error {
	if v.pool == nil || period == "" {
		return nil
	}
	const query = "SELECT status FROM accounting_periods WHERE period = $1 AND status IN ('OPEN','CLOSED') LIMIT 1"
	var status string
	if err := v.pool.QueryRow(ctx, query, period).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("analytics: period %s not accessible", period)
		}
		if v.logger != nil {
			v.logger.Warn("validate period fallback", slog.Any("error", err))
		}
		return nil
	}
	return nil
}

// jobsTemplates adapts the view engine to the jobs package's TemplateRenderer,
// which is deliberately decoupled from internal/view. Without this adapter the
// jobs handler receives no renderer and silently serves a standalone fallback
// page instead of the application shell.
type jobsTemplates struct {
	engine *view.Engine
}

func (j jobsTemplates) Render(w http.ResponseWriter, name string, data any) error {
	viewData, ok := data.(jobs.ViewData)
	if !ok {
		return fmt.Errorf("jobs template data has unexpected type %T", data)
	}
	return j.engine.Render(w, name, view.TemplateData{
		Title:       viewData.Title,
		CurrentPath: viewData.CurrentPath,
		CSRFToken:   viewData.CSRFToken,
		Data:        viewData.Data,
	})
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "fx" {
		if err := runFXCommand(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if app.InTestMode() {
		slog.Default().Info("test mode detected, skipping runtime startup")
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := app.LoadConfig()
	if err != nil {
		slog.Default().Error("load config", slog.Any("error", err))
		os.Exit(1)
	}

	logger := app.NewLogger(cfg)

	dbpool, err := db.New(ctx, cfg.PGDSN)
	if err != nil {
		logger.Error("connect postgres", slog.Any("error", err))
		os.Exit(1)
	}
	defer dbpool.Close()

	redisClient, err := cache.New(ctx, cfg.RedisAddr)
	if err != nil {
		logger.Error("connect redis", slog.Any("error", err))
		os.Exit(1)
	}
	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Warn("redis ping", slog.Any("error", err))
	}
	defer func() {
		if err := redisClient.Close(); err != nil {
			logger.Warn("redis close", slog.Any("error", err))
		}
	}()
	redisOpts, err := cache.AsynqOptions(cfg.RedisAddr)
	if err != nil {
		logger.Error("parse redis configuration", slog.Any("error", err))
		os.Exit(1)
	}

	sessionManager := shared.NewSessionManager(redisClient, "odyssey_session", cfg.SessionSecret, cfg.SessionTTL, cfg.IsProduction())
	csrfManager := shared.NewCSRFManager(cfg.CSRFSecret)

	templates, err := view.NewEngine()
	if err != nil {
		logger.Error("parse templates", slog.Any("error", err))
		os.Exit(1)
	}

	authRepo := auth.NewRepository(dbpool)
	authService := auth.NewService(authRepo)
	authHandler := auth.NewHandler(logger, authService, templates, sessionManager, csrfManager, authRepo)

	auditLogger := shared.NewAuditLogger(dbpool)
	approvalRecorder := shared.NewApprovalRecorder(dbpool, logger)
	idempotencyStore := shared.NewIdempotencyStore(dbpool)
	closeRepo := closepkg.NewRepository(dbpool)
	closeService := closepkg.NewService(closeRepo)

	journalRepo := journals.NewRepository(dbpool)
	periodRepo := periods.NewRepository(dbpool)
	mappingRepo := mappings.NewRepository(dbpool)

	journalService := journals.NewService(journalRepo, auditLogger, closeService)
	accountingHandler := accounting.NewHandler(logger, dbpool, templates, csrfManager, auditLogger, closeService)
	integrationHooks := integration.NewHooks(journalService, periodRepo, mappingRepo)

	bankingRepo := banking.NewRepository(dbpool)
	bankingService := banking.NewService(bankingRepo, logger, journalService)
	bankingHandler := banking.NewHandler(logger, bankingService, templates, csrfManager)

	bankfeedsRepo := bankfeeds.NewPGRepository(dbpool)
	bankfeedsService := bankfeeds.NewService(bankfeedsRepo, bankingService, nil)
	bankFeedsHandler := bankfeeds.NewHandler(bankfeedsService, logger)

	forecastRepo := forecasting.NewPGRepository(dbpool)
	forecastReaders := forecasting.NewDatabaseReaders(dbpool)
	forecastFXRepo := fxservice.NewRepository(dbpool)
	forecastService := forecasting.NewServiceWithFXResolver(
		forecastRepo,
		forecastReaders,
		fxservice.Resolver{Repo: forecastFXRepo, MaxAge: cfg.FXMaxRateAge},
		logger,
	)
	forecastHandler := forecasting.NewHandler(forecastService)

	treasuryRepo := treasury.NewPGRepository(dbpool)
	treasuryService := treasury.NewService(treasuryRepo, nil, logger)
	treasuryHandler := treasury.NewHandler(treasuryService)

	inventoryRepo := inventory.NewRepository(dbpool)
	inventoryService := inventory.NewService(inventoryRepo, auditLogger, idempotencyStore, inventory.ServiceConfig{}, integrationHooks)

	procurementRepo := procurement.NewRepository(dbpool)
	procurementService := procurement.NewService(logger, procurementRepo, inventoryService, approvalRecorder, auditLogger, idempotencyStore, integrationHooks)
	inventoryService.SetReorderRequestCreator(func(ctx context.Context, request inventory.ReorderRequest) error {
		lines := make([]procurement.PRLineInput, 0, len(request.Lines))
		for _, line := range request.Lines {
			lines = append(lines, procurement.PRLineInput{ProductID: line.ProductID, Qty: line.Qty, Note: line.Note})
		}
		_, err := procurementService.CreatePurchaseRequest(ctx, procurement.CreatePRInput{SupplierID: request.SupplierID, RequestBy: request.RequestedBy, Note: request.Note, Lines: lines})
		return err
	})

	rbacService := rbac.NewService(rbac.NewRepository(dbpool))
	rbacMiddleware := rbac.Middleware{Service: rbacService, Logger: logger}

	usersRepo := users.NewRepository(dbpool)
	usersService := users.NewService(usersRepo)
	usersHandler := users.NewHandler(logger, usersService, templates, csrfManager, sessionManager, rbacMiddleware)

	rolesRepo := roles.NewRepository(dbpool)
	rolesService := roles.NewService(rolesRepo)
	rolesHandler := roles.NewHandler(logger, rolesService, templates, csrfManager, sessionManager, rbacMiddleware)

	permissionsHandler := rbac.NewPermissionsHandler(logger, rbacService, templates, csrfManager, sessionManager, rbacMiddleware)

	jobClient, err := jobs.NewClient(redisOpts)
	if err != nil {
		logger.Error("init job client", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() {
		if err := jobClient.Close(); err != nil {
			logger.Warn("close job client", slog.Any("error", err))
		}
	}()
	bankFeedsHandler.SetEventEnqueuer(func(ctx context.Context, eventID int64) error {
		_, err := jobClient.EnqueueBankFeedsEvent(ctx, eventID)
		return err
	})
	outboxRepo := outbox.NewRepository(dbpool)
	connectorsRegistry := connectors.NewRegistry()
	if cfg.ConnectorDevelopmentMode {
		connectorsRegistry.Register("mockpay", mockpay.NewAdapter(logger))
	}
	vault, err := shared.NewVault()
	if err != nil {
		logger.Error("init vault", slog.Any("error", err))
		if cfg.IsProduction() {
			os.Exit(1)
		}
	}
	providerOptions := connectors.ProviderOptions{
		Vault:           vault,
		HTTPClient:      &http.Client{Timeout: 15 * time.Second},
		DevelopmentMode: cfg.ConnectorDevelopmentMode,
	}
	connectorsRegistry.Register("stripe", stripe.NewAdapter(logger, providerOptions))
	connectorsRegistry.Register("oidc", oidc.NewAdapter(logger, providerOptions))
	connectorsRegistry.Register("shopify", shopify.NewAdapter(logger, providerOptions))
	connectorsRegistry.Register("whatsapp", whatsapp.NewAdapter(logger, vault, providerOptions))
	connectorsRegistry.Register("openai", openai.NewAdapter(logger))
	connectorsRegistry.Register("dhl", dhl.NewAdapter(logger, providerOptions))
	connectorsRegistry.Register("awss3", awss3.NewAdapter(logger, vault, providerOptions))
	connectorsRegistry.Register("midtrans", midtrans.NewAdapter(logger, vault, providerOptions))
	connectorsProcessor := connectors.NewInboxProcessor(connectors.NewRepository(dbpool), connectorsRegistry, outboxRepo, logger)
	connectorsHandler := connectors.NewWebhookHandler(connectorsProcessor)

	connectorsService := connectors.NewService(connectors.NewRepository(dbpool), vault, connectorsRegistry)
	integrationHooks.SetConnectorsService(connectorsService)
	connectorsAdminHandler := connectors.NewAdminHandler(connectorsService, logger, templates)

	notificationRepo := notifications.NewRepository(dbpool)
	notificationService := notifications.NewService(notificationRepo)
	notificationDispatcher := notifications.NewDispatcher(notificationService, notificationRepo, notificationEmailQueue{client: jobClient}, messagingAdapter{svc: connectorsService})
	notificationHandler := notifications.NewHandler(notificationService)
	approvalRepo := approvalengine.NewRepository(dbpool)
	approvalService := approvalengine.NewService(approvalRepo, approvalengine.NewNotificationAdapter(notificationDispatcher))
	approvalService.RegisterFinalizer("PO", procurementService)
	procurementService.SetApprovalEngine(approvalService)
	sourcingService := procurement.NewSourcingService(procurement.NewSourcingRepository(dbpool), approvalService, auditLogger, idempotencyStore, procurement.NewAsynqRFQEmailQueue(jobClient.AsynqClient()))
	approvalService.RegisterFinalizer("RFQ_AWARD", sourcingService)
	approvalsHandler := approvalengine.NewHandler(logger, approvalService, templates, csrfManager, rbacMiddleware, dbpool)
	hrEmployeeService := hremployees.NewService(dbpool)
	hrEmployeesHandler := hremployees.NewHandler(logger, hrEmployeeService, templates, csrfManager, rbacMiddleware)
	hrLeaveService := hrleave.NewService(dbpool, approvalService, auditLogger)
	approvalService.RegisterFinalizer("LEAVE", hrLeaveService)
	hrLeaveHandler := hrleave.NewHandler(logger, hrLeaveService, templates, csrfManager, rbacMiddleware)
	hrAttendanceService := hrattendance.NewService(dbpool)
	hrAttendanceHandler := hrattendance.NewHandler(logger, hrAttendanceService, templates, csrfManager, rbacMiddleware)
	payrollRepo := payroll.NewRepository(dbpool)
	payrollService := payroll.NewService(payrollRepo, approvalService, journalService, payrollDeliveryQueue{client: jobClient})
	approvalService.RegisterFinalizer("PAYROLL", payrollService)
	taxRepo := tax.NewRepository(dbpool)
	taxService := tax.NewService(taxRepo, tax.ReviewedSchemaValidator{})
	taxHandler := tax.NewHandler(logger, taxService, templates, csrfManager, rbacMiddleware)

	arRepo := ar.NewRepository(dbpool)
	arService := ar.NewService(arRepo)
	arInvoicing := deliveryorders.NewInvoicingAdapter(dbpool)
	arService.SetDeliveryService(arInvoicing)
	arService.SetReturnDeliveryService(arInvoicing)
	arService.SetAccountingService(integrationHooks)
	arService.SetTaxService(taxService)
	arService.SetTaxService(taxService)

	arHandler := ar.NewHandler(logger, arService, templates, csrfManager, sessionManager, rbacMiddleware, jobClient.AsynqClient(), checkoutAdapter{svc: connectorsService})
	arHandler.SetNotificationDispatcher(notificationDispatcher)

	apRepo := ap.NewRepository(dbpool)
	apService := ap.NewService(apRepo, procurementService)
	apService.SetIntegrationHandler(integrationHooks)
	apService.SetTaxService(taxService)
	apHandler := ap.NewHandler(logger, apService, templates, csrfManager, sessionManager, rbacMiddleware)
	apHandler.SetEnqueueJob(func(invoiceID, createdBy int64) error {
		return jobs.EnqueueProcessAPInvoice(jobClient.AsynqClient(), invoiceID, createdBy)
	})

	closeHandler := closehttp.NewHandler(logger, closeService, templates, csrfManager, rbacMiddleware)
	eliminationRepo := eliminationpkg.NewRepository(dbpool)
	eliminationService := eliminationpkg.NewService(eliminationRepo, journalService)
	eliminationHandler := eliminationhttp.NewHandler(logger, eliminationService, templates, csrfManager, rbacMiddleware)

	analyticsRepo := analytics.NewRepository(dbpool)
	analyticsCache := analytics.NewCache(redisClient, 10*time.Minute)
	analyticsService := analytics.NewService(analyticsRepo, analyticsCache)
	pdfExporter := &export.PDFExporter{Endpoint: cfg.GotenbergURL, Client: http.DefaultClient}
	analyticsValidator := analyticsPeriodValidator{pool: dbpool, logger: logger}
	analyticsHandler := analytichttp.NewHandler(
		logger,
		analyticsService,
		templates,
		lineRenderer{},
		barRenderer{},
		pdfExporter,
		rbacService,
		analyticsValidator,
	)

	insightsRepo := insights.NewRepository(dbpool)
	insightsService := insights.NewService(insightsRepo)
	insightsHandler := insightshhtp.NewHandler(logger, insightsService, templates, rbacService)
	auditRepo := audit.NewRepository(dbpool)
	auditService := audit.NewService(auditRepo)
	auditExporter := audit.NewExporter(templates)
	auditHandler := audithttp.NewHandler(logger, auditService, templates, auditExporter, rbacService)
	metrics := observability.NewMetrics()
	jobmetrics.NewMetrics(metrics.Registerer())
	if err := consolhttp.SetupCacheMetrics(metrics.Registerer()); err != nil {
		logger.Warn("register consol cache metrics", slog.Any("error", err))
	}

	inventoryHandler := inventory.NewHandler(logger, inventoryService, templates, csrfManager, sessionManager, rbacMiddleware, dbpool)
	procurementHandler := procurement.NewHandler(logger, procurementService, templates, csrfManager, sessionManager, rbacMiddleware, jobClient.AsynqClient())
	procurementHandler.SetSourcingService(sourcingService)

	salesService := sales.NewService(dbpool)
	salesHandler := sales.NewHandler(logger, salesService, templates, csrfManager, sessionManager, rbacMiddleware, jobClient.AsynqClient())
	crmService := crm.NewService(crm.NewRepository(dbpool), salesService.Customers, salesService.Quotations, crm.NewNotificationAdapter(notificationDispatcher))
	crmHandler := crm.NewHandler(logger, crmService, templates, csrfManager, rbacMiddleware)

	masterdataHandler := masterdata.NewHandler(logger, dbpool, templates, csrfManager, sessionManager, rbacMiddleware)

	reportClient := report.NewClient(cfg.GotenbergURL)
	payrollProcessor := payroll.NewPayslipProcessor(payrollRepo, reportClient, nil)
	payrollHandler := payroll.NewHandler(logger, payrollService, payrollProcessor, templates, csrfManager, rbacMiddleware)
	reportHandler := report.NewHandler(reportClient, logger)
	creditNotePDF, err := ar.NewCreditNotePDFRenderer(reportClient)
	if err != nil {
		logger.Error("init AR credit note PDF renderer", slog.Any("error", err))
		os.Exit(1)
	}
	arHandler.SetCreditNotePDFRenderer(creditNotePDF)
	debitNotePDF, err := ap.NewDebitNotePDFRenderer(reportClient)
	if err != nil {
		logger.Error("init AP debit note PDF renderer", slog.Any("error", err))
		os.Exit(1)
	}
	apHandler.SetDebitNotePDFRenderer(debitNotePDF)

	consolPDFClient, err := consolhttp.NewPDFRenderClient(cfg.GotenbergURL)
	if err != nil {
		logger.Error("init consol pdf client", slog.Any("error", err))
		os.Exit(1)
	}

	consolRepo := consol.NewRepository(dbpool)
	consolService := consol.NewService(consolRepo)
	consolBSService := consol.NewBalanceSheetService(consolRepo)
	consolPLService := consol.NewProfitLossService(consolRepo)

	consolBSHandler, err := consolhttp.NewBalanceSheetHandler(logger, consolBSService, templates, csrfManager, sessionManager, rbacMiddleware, consolPDFClient)
	if err != nil {
		logger.Error("init consol bs handler", slog.Any("error", err))
		os.Exit(1)
	}

	consolPLHandler, err := consolhttp.NewProfitLossHandler(logger, consolPLService, templates, csrfManager, sessionManager, rbacMiddleware, consolPDFClient)
	if err != nil {
		logger.Error("init consol pl handler", slog.Any("error", err))
		os.Exit(1)
	}

	consolHandler, err := consolhttp.NewHandler(logger, consolService, consolBSHandler, consolPLHandler, templates, csrfManager, sessionManager, rbacMiddleware, consolPDFClient)
	if err != nil {
		logger.Error("init consolidation handler", slog.Any("error", err))
		os.Exit(1)
	}
	varianceRepo := variancepkg.NewRepository(dbpool)
	varianceService := variancepkg.NewService(varianceRepo)
	boardpackRepo := boardpacksvc.NewRepository(dbpool)
	boardpackService := boardpacksvc.NewService(boardpackRepo)
	boardpackStorage, err := boardpacksvc.NewStorage(ctx, boardpacksvc.StorageConfig{
		Driver:          cfg.BoardPackStorageDriver,
		LocalDir:        cfg.BoardPackStorageDir,
		Endpoint:        cfg.BoardPackS3Endpoint,
		Region:          cfg.BoardPackS3Region,
		Bucket:          cfg.BoardPackS3Bucket,
		AccessKeyID:     cfg.BoardPackS3AccessKeyID,
		SecretAccessKey: cfg.BoardPackS3SecretAccessKey,
		UsePathStyle:    cfg.BoardPackS3UsePathStyle,
		AutoCreate:      cfg.BoardPackS3AutoCreate,
	})
	if err != nil {
		logger.Error("init board pack storage", slog.Any("error", err))
		os.Exit(1)
	}

	varianceHandler := variancepkg.NewHandler(logger, varianceService, templates, csrfManager, rbacMiddleware, jobClient)
	boardpackHandler := boardpackhttp.NewHandler(logger, boardpackService, templates, csrfManager, rbacMiddleware, jobClient, boardpackStorage)

	inspector := asynq.NewInspector(redisOpts)
	defer func() {
		if err := inspector.Close(); err != nil {
			logger.Warn("inspector close", slog.Any("error", err))
		}
	}()
	jobHandler := jobs.NewHandler(inspector, logger, jobsTemplates{engine: templates})
	dashboardService := dashboard.NewService(dbpool)
	dashboardHandler := dashboard.NewHandler(logger, dashboardService, templates, csrfManager)
	wmsHandler := wms.NewHandler(wms.NewService(wms.NewRepository(dbpool)), rbacMiddleware, dbpool)
	wmsHandler.SetInventoryService(inventoryService)
	apiHandler := apihttp.NewHandler(dbpool, []byte(cfg.SessionSecret))
	portalHandler := portal.NewHandler(dbpool, templates, rbacMiddleware)
	posHandler := pos.NewHandler(pos.NewService(pos.NewRepository(dbpool)), rbacMiddleware, dbpool)
	posHandler.SetInventoryService(inventoryService)
	posHandler.SetAccountingService(integrationHooks)
	projectsHandler := projects.NewHandler(projects.NewService(projects.NewRepository(dbpool)), rbacMiddleware, dbpool)
	mrpHandler := mrp.NewHandler(mrp.NewService(mrp.NewRepository(dbpool)), rbacMiddleware, dbpool)
	mrpHandler.SetInventoryService(inventoryService)
	mrpHandler.SetManufacturingAccounting(integrationHooks)
	mrpHandler.SetUI(templates, csrfManager)
	mrpHandler.SetOutboxRepository(outboxRepo)

	documentsRepo := documents.NewRepository(dbpool)
	sharedStorage, err := storage.NewStorage(ctx, storage.StorageConfig{
		Driver:   "local",
		LocalDir: "./data/storage", // Use a local directory for development
	})
	if err != nil {
		logger.Error("init shared storage", slog.Any("error", err))
	}
	documentsService := documents.NewService(documentsRepo, sharedStorage)
	portalHandler.SetDocumentsService(documentsService)
	documentsHandler := documentshttp.NewHandler(logger, documentsService, templates, csrfManager, rbacMiddleware, jobClient, nil)

	cmmsRepo := cmms.NewRepository(dbpool)
	cmmsService := cmms.NewService(cmmsRepo)
	cmmsHandler := cmmshttp.NewHandler(logger, cmmsService, templates, csrfManager, rbacMiddleware, dbpool)

	qmsRepo := qms.NewRepository(dbpool)
	qmsService := qms.NewService(qmsRepo)
	qmsHandler := qmshttp.NewHandler(logger, qmsService, templates, csrfManager, rbacMiddleware, dbpool, outboxRepo)

	mrpHandler.SetQMSService(qmsService)

	logisticsService := logistics.NewService(dbpool)
	distributionHandler := app.NewDistributionHandler(dbpool, logisticsService, inventoryService)

	// Init Freight Module
	freightQueries := sqlc.New(dbpool)
	freightRepo := freight.NewPostgresRepository(freightQueries)
	// freightGL is internally initialized in NewFreightService or separated. Actually let's just use NewFreightService
	freightService := freight.NewFreightService(freightRepo)

	router := app.NewRouter(app.RouterParams{
		Logger:                 logger,
		Config:                 cfg,
		Templates:              templates,
		SessionManager:         sessionManager,
		CSRFManager:            csrfManager,
		AuthHandler:            authHandler,
		AccountingHandler:      accountingHandler,
		ARHandler:              arHandler,
		APHandler:              apHandler,
		RolesHandler:           rolesHandler,
		UsersHandler:           usersHandler,
		CloseHandler:           closeHandler,
		EliminationHandler:     eliminationHandler,
		VarianceHandler:        varianceHandler,
		BoardPackHandler:       boardpackHandler,
		InventoryHandler:       inventoryHandler,
		ProcurementHandler:     procurementHandler,
		SalesHandler:           salesHandler,
		MasterDataHandler:      masterdataHandler,
		Pool:                   dbpool,
		RBACMiddleware:         rbacMiddleware,
		ReportHandler:          reportHandler,
		ConsolHandler:          consolHandler,
		JobHandler:             jobHandler,
		AnalyticsHandler:       analyticsHandler,
		InsightsHandler:        insightsHandler,
		AuditHandler:           auditHandler,
		PermissionsHandler:     permissionsHandler,
		BankingHandler:         bankingHandler,
		BankFeedsHandler:       bankFeedsHandler,
		ForecastingHandler:     forecastHandler,
		TreasuryHandler:        treasuryHandler,
		InventoryService:       inventoryService,
		LogisticsService:       logisticsService,
		DistributionHandler:    distributionHandler,
		FreightService:         freightService,
		Metrics:                metrics,
		DashboardHandler:       dashboardHandler,
		NotificationHandler:    notificationHandler,
		NotificationDispatcher: notificationDispatcher,
		ApprovalsHandler:       approvalsHandler,
		HREmployeesHandler:     hrEmployeesHandler,
		HRLeaveHandler:         hrLeaveHandler,
		HRAttendanceHandler:    hrAttendanceHandler,
		PayrollHandler:         payrollHandler,
		TaxHandler:             taxHandler,
		CRMHandler:             crmHandler,
		WMSHandler:             wmsHandler,
		APIHandler:             apiHandler,
		PortalHandler:          portalHandler,
		POSHandler:             posHandler,
		ProjectsHandler:        projectsHandler,
		MRPHandler:             mrpHandler,
		DocumentsHandler:       documentsHandler,
		CMMSHandler:            cmmsHandler,
		QMSHandler:             qmsHandler,
		ConnectorsHandler:      connectorsHandler,
		ConnectorsAdminHandler: connectorsAdminHandler,
	})

	// Route dump mode: print the real routing table and exit without serving.
	// The E2E suite uses this to derive its page coverage from the router
	// rather than a hand-maintained list.
	if os.Getenv("ODYSSEY_DUMP_ROUTES") != "" {
		if err := app.WriteRoutes(router, os.Stdout); err != nil {
			logger.Error("dump routes", slog.Any("error", err))
			os.Exit(1)
		}
		return
	}

	server := &http.Server{
		Addr:         cfg.AppAddr,
		Handler:      router,
		ReadTimeout:  cfg.AppReadTimeout,
		WriteTimeout: cfg.AppWriteTimeout,
	}

	go func() {
		logger.Info("starting http server", slog.String("addr", cfg.AppAddr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server", slog.Any("error", err))
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown", slog.Any("error", err))
	}
}

func runFXCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: odyssey fx fetch|status --date YYYY-MM-DD")
	}
	cfg, err := app.LoadConfig()
	if err != nil {
		return err
	}
	pool, err := db.New(ctx, cfg.PGDSN)
	if err != nil {
		return err
	}
	defer pool.Close()
	repo := fxservice.NewRepository(pool)
	provider := fxservice.NewExchangeRateAPI(fxservice.ProviderConfig{BaseURL: cfg.FXAPIBaseURL, APIKey: cfg.FXAPIKey, Timeout: cfg.FXFetchTimeout})
	service := &fxservice.Service{Provider: provider, Repo: repo, MaxRateAge: cfg.FXMaxRateAge}
	ops := fxservice.Operations{Service: service, Repository: repo}
	opts := fxcli.TransactionFXCommandOptions{}
	for i := 1; i < len(args); i++ {
		if args[i] == "--date" && i+1 < len(args) {
			opts.Date = args[i+1]
			i++
		} else if args[i] == "--force" {
			opts.Force = true
		} else {
			return fmt.Errorf("unknown fx option %q", args[i])
		}
	}
	switch args[0] {
	case "fetch":
		returnCode := fxcli.RunTransactionFXFetch(ctx, opsAdapter{ops}, opts)
		if returnCode != 0 {
			return fmt.Errorf("fx fetch failed")
		}
		return nil
	case "status":
		returnCode := fxcli.RunTransactionFXStatus(ctx, opsAdapter{ops}, opts)
		if returnCode != 0 {
			return fmt.Errorf("fx status failed")
		}
		return nil
	default:
		return fmt.Errorf("unknown fx command %q", args[0])
	}
}

type opsAdapter struct{ fxservice.Operations }

func (o opsAdapter) Status(ctx context.Context, date time.Time) ([]fxcli.FXRateStatus, error) {
	rows, err := o.Operations.Status(ctx, date)
	if err != nil {
		return nil, err
	}
	result := make([]fxcli.FXRateStatus, len(rows))
	for i, row := range rows {
		result[i] = fxcli.FXRateStatus{BaseCurrency: row.BaseCurrency, QuoteCurrency: row.QuoteCurrency, Source: row.Source, Status: row.Status, Rate: row.Rate, RateDate: row.RateDate}
	}
	return result, nil
}

type checkoutAdapter struct {
	svc *connectors.Service
}

func (c checkoutAdapter) CreateCheckoutIntent(ctx context.Context, companyID, connectionID int64, sourceType string, sourceID int64, amount float64, currency, customerName, customerEmail, orderID string) (string, error) {
	res, err := c.svc.CreateCheckoutIntent(ctx, connectors.CreateCheckoutIntentRequest{
		CompanyID:     companyID,
		ConnectionID:  connectionID,
		SourceType:    sourceType,
		SourceID:      sourceID,
		Amount:        amount,
		Currency:      currency,
		CustomerName:  customerName,
		CustomerEmail: customerEmail,
		OrderID:       orderID,
	})
	if err != nil {
		return "", err
	}
	return res.RedirectURL, nil
}

type messagingAdapter struct {
	svc *connectors.Service
}

func (m messagingAdapter) EnqueueMessage(ctx context.Context, msg notifications.OutboundMessage) error {
	// We'll assume CompanyID = 1 for system notifications.
	return m.svc.EnqueueMessage(ctx, 1, msg.Channel, msg.To, msg.Content, msg.CorrelationID)
}
