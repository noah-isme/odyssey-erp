package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	"github.com/odyssey-erp/odyssey-erp/internal/analytics"
	"github.com/odyssey-erp/odyssey-erp/internal/ap"
	apihttp "github.com/odyssey-erp/odyssey-erp/internal/api"
	"github.com/odyssey-erp/odyssey-erp/internal/app"
	"github.com/odyssey-erp/odyssey-erp/internal/ar"
	"github.com/odyssey-erp/odyssey-erp/internal/boardpack"
	"github.com/odyssey-erp/odyssey-erp/internal/cmms"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/awss3"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/dhl"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/midtrans"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/midtransiris"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/mockpay"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/oidc"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/openai"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/shopify"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/stripe"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/whatsapp"
	"github.com/odyssey-erp/odyssey-erp/internal/consol"
	"github.com/odyssey-erp/odyssey-erp/internal/crm"
	"github.com/odyssey-erp/odyssey-erp/internal/documents"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/bankfeeds"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/banking"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/forecasting"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/payments"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/treasury"
	"github.com/odyssey-erp/odyssey-erp/internal/fixedassets"
	fxservice "github.com/odyssey-erp/odyssey-erp/internal/fx"
	"github.com/odyssey-erp/odyssey-erp/internal/notifications"
	"github.com/odyssey-erp/odyssey-erp/internal/observability"
	"github.com/odyssey-erp/odyssey-erp/internal/outbox"
	"github.com/odyssey-erp/odyssey-erp/internal/payroll"
	"github.com/odyssey-erp/odyssey-erp/internal/platform/cache"
	"github.com/odyssey-erp/odyssey-erp/internal/platform/db"
	"github.com/odyssey-erp/odyssey-erp/internal/qms"
	"github.com/odyssey-erp/odyssey-erp/internal/sales/customers"
	"github.com/odyssey-erp/odyssey-erp/internal/sales/orders"
	"github.com/odyssey-erp/odyssey-erp/internal/sales/quotations"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/storage"
	"github.com/odyssey-erp/odyssey-erp/internal/tax"
	"github.com/odyssey-erp/odyssey-erp/internal/variance"
	"github.com/odyssey-erp/odyssey-erp/jobs"
	"github.com/odyssey-erp/odyssey-erp/report"
)

type notificationEmailQueue struct{ client *asynq.Client }

type payrollDeliveryQueue struct{ client *asynq.Client }

type fxJobFetcher struct{ service *fxservice.Service }

type webhookDispatcher struct{ handler *apihttp.Handler }

type paymentAlertSink struct {
	pool       *pgxpool.Pool
	dispatcher interface {
		Dispatch(context.Context, notifications.Message) error
	}
	logger *slog.Logger
}

func startWorkerMetricsServer(addr string, logger *slog.Logger) (func(context.Context) error, error) {
	if addr == "" {
		return func(context.Context) error { return nil }, nil
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen for worker metrics on %s: %w", addr, err)
	}
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	server := &http.Server{
		Handler:           metricsMux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) && logger != nil {
			logger.Error("worker metrics server stopped", slog.Any("error", err))
		}
	}()
	return server.Shutdown, nil
}

func (s *paymentAlertSink) recipients(ctx context.Context, companyID int64) ([]int64, error) {
	if s == nil || s.pool == nil {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT user_id
		FROM (
			SELECT u.id AS user_id
			FROM users u
			JOIN user_roles ur ON ur.user_id = u.id
			JOIN roles r ON r.id = ur.role_id
			WHERE u.is_active AND LOWER(r.name) IN ('admin', 'administrator')
			UNION
			SELECT sur.user_id
			FROM scoped_user_roles sur
			JOIN company_roles cr ON cr.id = sur.role_id
			JOIN users u ON u.id = sur.user_id AND u.is_active
			WHERE sur.company_id = $1 AND LOWER(cr.name) IN ('admin', 'administrator')
		) recipients
		ORDER BY user_id`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *paymentAlertSink) AlertUnmatchedPayment(ctx context.Context, issue connectors.PaymentReconciliationIssue) error {
	if s == nil || s.dispatcher == nil {
		return nil
	}
	recipients, err := s.recipients(ctx, issue.CompanyID)
	if err != nil {
		return err
	}
	if len(recipients) == 0 {
		if s.logger != nil {
			s.logger.Warn("payment reconciliation issue has no administrator recipient", slog.Int64("company_id", issue.CompanyID), slog.String("provider_reference", issue.ProviderReference))
		}
		return nil
	}
	var failures []error
	for _, recipientID := range recipients {
		err := s.dispatcher.Dispatch(ctx, notifications.Message{
			RecipientID: recipientID,
			DedupeKey:   fmt.Sprintf("payment-reconciliation:%d:%d:%s:%s:%d", issue.CompanyID, issue.ConnectionID, issue.ProviderReference, issue.IssueType, time.Now().UTC().Truncate(time.Hour).Unix()),
			Type:        notifications.TypePaymentReconciliationUnmatched,
			Title:       "Payment reconciliation requires review",
			Body:        fmt.Sprintf("Provider %s reported %q for payment %s; local status is %q. %s", issue.Provider, issue.ObservedStatus, issue.ProviderReference, issue.ExpectedStatus, issue.Details),
			URL:         "/settings/integrations",
		})
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (s *paymentAlertSink) AlertConnectorDeadLetter(ctx context.Context, deadLetter connectors.ConnectorDeadLetter) error {
	if s == nil || s.dispatcher == nil {
		return nil
	}
	recipients, err := s.recipients(ctx, deadLetter.CompanyID)
	if err != nil {
		return err
	}
	if len(recipients) == 0 {
		if s.logger != nil {
			s.logger.Warn("connector dead letter has no administrator recipient", slog.Int64("company_id", deadLetter.CompanyID), slog.Int64("dead_letter_id", deadLetter.ID))
		}
		return nil
	}
	var failures []error
	for _, recipientID := range recipients {
		err := s.dispatcher.Dispatch(ctx, notifications.Message{
			RecipientID: recipientID,
			DedupeKey:   fmt.Sprintf("connector-dead-letter:%d:%d", deadLetter.ID, time.Now().UTC().Truncate(time.Hour).Unix()),
			Type:        notifications.TypeConnectorDeadLetter,
			Title:       "Connector command moved to dead letter",
			Body:        fmt.Sprintf("Provider %s command %s (%s) exhausted retries: %s", deadLetter.Provider, deadLetter.CommandType, deadLetter.CorrelationID, deadLetter.ErrorMessage),
			URL:         "/settings/integrations",
		})
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (d webhookDispatcher) DispatchWebhookDeliveries(ctx context.Context) error {
	_, err := d.handler.DeliverDue(ctx, http.DefaultClient, 100)
	return err
}

func (f fxJobFetcher) FetchDailyRates(ctx context.Context, base string, date time.Time, force bool) error {
	return f.service.FetchDailyRatesForJob(ctx, base, date, force)
}

func (q payrollDeliveryQueue) EnqueuePayslip(ctx context.Context, line payroll.RunLine) error {
	task, err := jobs.NewPayrollPayslipTask(line.PayslipID)
	if err != nil {
		return err
	}
	_, err = q.client.EnqueueContext(ctx, task, asynq.Queue(jobs.QueueDefault), asynq.MaxRetry(5), asynq.TaskID(fmt.Sprintf("payroll-payslip-%d", line.PayslipID)))
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	return err
}

func (q notificationEmailQueue) EnqueueEmail(ctx context.Context, email notifications.Email) error {
	task, err := jobs.NewSendEmailTask(jobs.SendEmailPayload{To: email.To, Subject: email.Subject, Body: email.Body, CorrelationID: email.CorrelationID})
	if err != nil {
		return err
	}
	options := []asynq.Option{asynq.Queue(jobs.QueueDefault), asynq.MaxRetry(5)}
	if email.CorrelationID != "" {
		options = append(options, asynq.TaskID(email.CorrelationID))
	}
	_, err = q.client.EnqueueContext(ctx, task, options...)
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	return err
}

func main() {
	if app.InTestMode() {
		slog.Default().Info("test mode detected, skipping worker startup")
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
	mailClient := shared.NewMailClient(shared.MailConfig{Host: cfg.SMTPHost, Port: cfg.SMTPPort, From: cfg.SMTPFrom, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword})

	pool, err := db.New(ctx, cfg.PGDSN)
	if err != nil {
		logger.Error("connect database", slog.Any("error", err))
		os.Exit(1)
	}
	defer pool.Close()

	redisClient, err := cache.New(ctx, cfg.RedisAddr)
	if err != nil {
		logger.Error("connect redis", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() {
		if err := redisClient.Close(); err != nil {
			logger.Warn("redis close", slog.Any("error", err))
		}
	}()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Warn("redis ping", slog.Any("error", err))
	}
	redisOpts, err := cache.AsynqOptions(cfg.RedisAddr)
	if err != nil {
		logger.Error("parse redis configuration", slog.Any("error", err))
		os.Exit(1)
	}

	analyticsRepo := analytics.NewRepository(pool)
	analyticsCache := analytics.NewCache(redisClient, 10*time.Minute)
	analyticsService := analytics.NewService(analyticsRepo, analyticsCache)

	warmupJob := jobs.NewInsightsWarmupJob(analyticsService, pool, logger, nil)
	anomalyJob := jobs.NewAnomalyScanJob(pool, logger, nil)
	consolRepo := consol.NewRepository(pool)
	consolService := consol.NewService(consolRepo)
	consolidator := jobs.NewConsolidateRefreshJob(consolService, consolRepo, logger, nil)
	varianceRepo := variance.NewRepository(pool)
	varianceService := variance.NewService(varianceRepo)
	fixedAssetService := fixedassets.NewService(pool, journals.NewService(journals.NewRepository(pool), nil, nil))
	varianceJob := variance.NewSnapshotJob(varianceService, logger)

	boardpackRepo := boardpack.NewRepository(pool)
	boardpackService := boardpack.NewService(boardpackRepo)
	boardpackBuilder := boardpack.NewBuilder(boardpackRepo, varianceService, analyticsService)
	pdfClient := report.NewClient(cfg.GotenbergURL)
	payrollRepo := payroll.NewRepository(pool)
	payslipProcessor := payroll.NewPayslipProcessor(payrollRepo, pdfClient, mailClient)
	boardpackRenderer, err := boardpack.NewRenderer(pdfClient)
	if err != nil {
		logger.Error("init board pack renderer", slog.Any("error", err))
		os.Exit(1)
	}
	boardpackStorage, err := boardpack.NewStorage(ctx, boardpack.StorageConfig{
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
	boardpackJob := boardpack.NewJob(boardpack.JobConfig{
		Service:  boardpackService,
		Builder:  boardpackBuilder,
		Renderer: boardpackRenderer,
		Storage:  boardpackStorage,
		Logger:   logger,
	})

	warmupTask, err := jobs.NewInsightsWarmupTask("active")
	if err != nil {
		logger.Error("build warmup task", slog.Any("error", err))
		os.Exit(1)
	}
	anomalyTask, err := jobs.NewAnomalyScanTask(12, 2.5)
	if err != nil {
		logger.Error("build anomaly task", slog.Any("error", err))
		os.Exit(1)
	}
	consolidateTask, err := jobs.NewConsolidateRefreshTask("all", "active")
	if err != nil {
		logger.Error("build consolidate task", slog.Any("error", err))
		os.Exit(1)
	}

	asynqClient := asynq.NewClient(redisOpts)
	defer func() {
		if err := asynqClient.Close(); err != nil {
			logger.Warn("close asynq client", slog.Any("error", err))
		}
	}()
	notificationRepo := notifications.NewRepository(pool)
	notificationService := notifications.NewService(notificationRepo)
	notificationDispatcher := notifications.NewDispatcher(notificationService, notificationRepo, notificationEmailQueue{client: asynqClient}, nil)
	payrollOutbox := payroll.NewOutboxDispatcher(payrollRepo, payrollDeliveryQueue{client: asynqClient})
	taxService := tax.NewService(tax.NewRepository(pool), nil)
	crmService := crm.NewService(crm.NewRepository(pool), nil, nil, crm.NewNotificationAdapter(notificationDispatcher))
	cmmsService := cmms.NewService(cmms.NewRepository(pool))
	fxRepo := fxservice.NewRepository(pool)
	fxProvider := fxservice.NewExchangeRateAPI(fxservice.ProviderConfig{BaseURL: cfg.FXAPIBaseURL, APIKey: cfg.FXAPIKey, Timeout: cfg.FXFetchTimeout})
	fxDailyService := &fxservice.Service{Provider: fxProvider, Repo: fxRepo, MaxRateAge: cfg.FXMaxRateAge}
	apiHandler := apihttp.NewHandler(pool, []byte(cfg.SessionSecret))
	boardpackJob.SetNotificationDispatcher(notificationDispatcher)

	qmsService := qms.NewService(qms.NewRepository(pool))

	docStorage, err := storage.NewStorage(ctx, storage.StorageConfig{
		Driver:   cfg.BoardPackStorageDriver, // Share storage config for now
		LocalDir: cfg.BoardPackStorageDir,
	})
	if err != nil {
		logger.Error("init documents storage", slog.Any("error", err))
		os.Exit(1)
	}
	documentsService := documents.NewService(documents.NewRepository(pool), docStorage)

	vault, err := shared.NewVault()
	if err != nil {
		logger.Error("init vault", slog.Any("error", err))
		os.Exit(1)
	}

	connectorsRegistry := connectors.NewRegistry()
	if cfg.ConnectorDevelopmentMode {
		connectorsRegistry.Register("mockpay", mockpay.NewAdapter(logger))
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
	connectorsRepo := connectors.NewRepository(pool)
	treasuryRepo := treasury.NewPGRepository(pool)
	irisAdapter := midtransiris.NewAdapter(logger, vault, midtransiris.Options{
		ProviderOptions: providerOptions,
		ConnectionResolver: func(ctx context.Context, ref automation.ConnectionRef) (*connectors.Connection, error) {
			conn, err := connectorsRepo.GetConnection(ctx, ref.CompanyID, ref.ConnectionID)
			if err != nil {
				return nil, err
			}
			if conn.CompanyID != ref.CompanyID {
				return nil, fmt.Errorf("midtrans iris: connection company mismatch")
			}
			return &conn, nil
		},
		ScopedBeneficiaryResolver: func(ctx context.Context, connection automation.ConnectionRef, ref string) (midtransiris.Beneficiary, error) {
			const prefix = "bank-account:"
			value := strings.TrimPrefix(strings.TrimSpace(ref), prefix)
			accountID, parseErr := strconv.ParseInt(value, 10, 64)
			if parseErr != nil || accountID <= 0 {
				return midtransiris.Beneficiary{}, fmt.Errorf("invalid bank account reference")
			}
			account, accountErr := treasuryRepo.GetSupplierBankAccount(ctx, accountID)
			if accountErr != nil {
				return midtransiris.Beneficiary{}, accountErr
			}
			if account.CompanyID != connection.CompanyID {
				return midtransiris.Beneficiary{}, fmt.Errorf("bank account is outside connection company scope")
			}
			bank := account.RoutingNumber
			if strings.TrimSpace(bank) == "" {
				bank = account.BankName
			}
			return midtransiris.Beneficiary{
				Name:    fmt.Sprintf("Supplier %d", account.SupplierID),
				Account: account.AccountNumber,
				Bank:    bank,
			}, nil
		},
	})
	paymentRouter := payments.NewProviderRouter(map[string]payments.ExecutionPort{
		midtransiris.Provider: irisAdapter,
		"midtrans-iris":       irisAdapter,
		"midtransiris":        irisAdapter,
		"iris":                irisAdapter,
	})
	paymentCoordinator := payments.NewCoordinator(paymentRouter, payments.NewPostgresStore(pool), payments.NewSeparationAuthorizer(automation.Settings{}))
	connectorsOutboxWorker := connectors.NewOutboxWorker(connectorsRepo, connectorsRegistry, connectors.WithOutboxWorkerLogger(logger))
	connectorsService := connectors.NewService(connectorsRepo, vault, connectorsRegistry)
	recoveryMetrics := observability.NewPaymentRecoveryMetrics(nil)
	paymentReconciliation := connectors.NewPaymentReconciliationService(
		connectorsRepo,
		connectorsRegistry,
		logger,
		recoveryMetrics,
		&paymentAlertSink{pool: pool, dispatcher: notificationDispatcher, logger: logger},
	)

	outboxRepo := outbox.NewRepository(pool)
	outboxDispatcher := outbox.NewDispatcher(pool, outboxRepo, logger)
	cmms.RegisterOutboxHandlers(outboxDispatcher, cmmsService, logger)
	qms.RegisterOutboxHandlers(outboxDispatcher, qmsService, logger)

	arRepo := ar.NewRepository(pool)
	arService := ar.NewService(arRepo)
	ar.RegisterOutboxHandlers(outboxDispatcher, arService, logger)

	// Marketplace outbox routing
	salesCustRepo := customers.NewRepository(pool)
	salesQuoteRepo := quotations.NewRepository(pool)
	salesOrdersRepo := orders.NewRepository(pool)
	salesOrdersSvc := orders.NewService(salesOrdersRepo, salesCustRepo, salesQuoteRepo)
	marketplaceProc := orders.NewMarketplaceProcessor(logger, salesOrdersSvc, orders.NewMappingRepository(pool))
	orders.RegisterOutboxHandlers(outboxDispatcher, marketplaceProc)

	bankfeedsRepo := bankfeeds.NewPGRepository(pool)
	bankingRepo := banking.NewRepository(pool)
	// We don't have a poster in worker, but ImportStatement doesn't post to GL directly.
	bankingService := banking.NewService(bankingRepo, logger, nil)
	bankfeedsService := bankfeeds.NewService(bankfeedsRepo, bankingService, nil)
	bankFeedsProcessor := jobs.NewBankFeedsProcessor(bankfeedsService, logger)
	financeAutomationOutbox := automation.NewOutboxRepository(pool)
	financeAutomationDispatcher := automation.NewDispatcher(
		financeAutomationOutbox,
		fmt.Sprintf("finance-worker-%d", os.Getpid()),
		logger,
	)
	if strings.EqualFold(strings.TrimSpace(cfg.ReleaseProfile), string(app.ReleaseProfileV011Finance)) {
		financeSettlementService := payments.NewPostgresSettlementService(pool, treasury.NewTreasurySettlementEffects())
		if err := payments.RegisterPaymentExecutionHandlers(financeAutomationDispatcher, paymentCoordinator, financeSettlementService); err != nil {
			logger.Error("register payment execution handlers", slog.Any("error", err))
			os.Exit(1)
		}
	} else {
		logger.Info("finance payment execution handlers disabled by release profile", slog.String("release_profile", cfg.ReleaseProfile))
	}

	forecastRepo := forecasting.NewPGRepository(pool)
	forecastReaders := forecasting.NewDatabaseReaders(pool)
	forecastService := forecasting.NewServiceWithFXResolver(
		forecastRepo,
		forecastReaders,
		fxservice.Resolver{Repo: fxRepo, MaxAge: cfg.FXMaxRateAge},
		logger,
	)
	forecastProcessor := jobs.NewCashForecastProcessor(forecastService, logger)

	apRepo := ap.NewRepository(pool)
	apService := ap.NewService(apRepo, nil) // Dependencies omitted for simplicity in worker
	matchingService := ap.NewMatchingService(apRepo)
	exceptionService := ap.NewExceptionService(apRepo)
	apOrchestrator := ap.NewOrchestrator(matchingService, exceptionService, apService)

	worker, err := jobs.NewWorker(jobs.WorkerConfig{
		RedisOpts: redisOpts,
		Logger:    logger,
		Mailer:    mailClient,
		Handlers: []jobs.TaskHandler{
			{Type: jobs.TaskAnalyticsInsightsWarmup, Handler: warmupJob.Handle},
			{Type: jobs.TaskAnalyticsAnomalyScan, Handler: anomalyJob.Handle},
			{Type: jobs.TaskConsolidateRefresh, Handler: consolidator.Handle},
			{Type: jobs.TaskVarianceSnapshotProcess, Handler: varianceJob.Handle},
			{Type: jobs.TaskBoardPackGenerate, Handler: boardpackJob.Handle},
			{Type: jobs.TypeOverdueInvoicesScan, Handler: jobs.HandleOverdueInvoicesScanTask(logger, pool, asynqClient)},
			{Type: jobs.TypeReportScheduleScan, Handler: jobs.HandleReportScheduleScanTask(logger, pool, asynqClient)},
			{Type: jobs.TaskFixedAssetDepreciation, Handler: jobs.HandleFixedAssetDepreciation(fixedAssetService)},
			{Type: jobs.TaskPayrollPayslipEmail, Handler: jobs.HandlePayrollPayslipEmail(payslipProcessor)},
			{Type: jobs.TaskPayrollPayslipDispatch, Handler: jobs.HandlePayrollPayslipDispatch(payrollOutbox)},
			{Type: jobs.TaskTaxCaptureDispatch, Handler: jobs.HandleTaxCaptureDispatch(taxService)},
			{Type: jobs.TaskCRMReminderDispatch, Handler: jobs.HandleCRMReminderDispatch(crmService)},
			{Type: jobs.TaskWebhookDeliveryDispatch, Handler: jobs.HandleWebhookDeliveryDispatch(webhookDispatcher{handler: apiHandler})},
			{Type: jobs.TaskOutboxSweep, Handler: jobs.HandleOutboxSweep(outboxDispatcher)},
			{Type: jobs.TaskFinanceAutomationDispatch, Handler: jobs.HandleFinanceAutomationDispatch(financeAutomationDispatcher)},
			{Type: jobs.TypeBankFeedsSync, Handler: bankFeedsProcessor.ProcessSyncTask},
			{Type: jobs.TypeBankFeedsEvent, Handler: bankFeedsProcessor.ProcessEventTask},
			{Type: jobs.TypeCashForecastRefresh, Handler: forecastProcessor.ProcessRefreshTask},
			{Type: jobs.TypeCMMSPMGeneratorScan, Handler: jobs.HandleCMMSPMGeneratorScanTask(logger, func(ctx context.Context) error {
				_, err := cmmsService.GenerateAllPMWorkOrders(ctx)
				return err
			})},
			{Type: jobs.TaskDocumentOCR, Handler: jobs.HandleDocumentOCR(documentsService)},
			{Type: jobs.TaskDocumentDisposition, Handler: jobs.HandleDocumentDisposition(documentsService)},
			{Type: jobs.TaskConnectorOutboxSweep, Handler: jobs.HandleConnectorOutboxSweep(connectorsOutboxWorker)},
			{Type: jobs.TaskPaymentReconciliation, Handler: jobs.HandlePaymentReconciliation(paymentReconciliation)},
			{Type: jobs.TaskConnectorDeadLetterAudit, Handler: jobs.HandleConnectorDeadLetterAudit(paymentReconciliation)},
			{Type: jobs.TaskProcessAPInvoice, Handler: jobs.HandleProcessAPInvoice(apOrchestrator.ProcessInvoice)},
		},
		FXFetcher:   fxJobFetcher{service: fxDailyService},
		FXCompanies: fxRepo,
		FXLocation:  mustLocation("Asia/Jakarta"),
		FXLogger:    logger,
		Analytics:   analyticsService,
		Connectors:  connectorsService,
		Cron: []jobs.CronRegistration{
			{Spec: "15 1 * * *", Task: warmupTask, Options: []asynq.Option{asynq.MaxRetry(3)}},
			{Spec: "30 1 * * *", Task: anomalyTask, Options: []asynq.Option{asynq.MaxRetry(3)}},
			{Spec: "0 2 * * *", Task: consolidateTask, Options: []asynq.Option{asynq.MaxRetry(3)}},
			// Run overdue invoice scan every day at 8:00 AM
			{Spec: "0 8 * * *", Task: asynq.NewTask(jobs.TypeOverdueInvoicesScan, nil), Options: []asynq.Option{asynq.MaxRetry(3)}},
			{Spec: "5 * * * *", Task: asynq.NewTask(jobs.TypeReportScheduleScan, nil), Options: []asynq.Option{asynq.MaxRetry(3)}},
			{Spec: "10 2 1 * *", Task: asynq.NewTask(jobs.TaskFixedAssetDepreciation, nil), Options: []asynq.Option{asynq.MaxRetry(3)}},
			{Spec: "*/5 * * * *", Task: asynq.NewTask(jobs.TaskPayrollPayslipDispatch, nil), Options: []asynq.Option{asynq.MaxRetry(3)}},
			{Spec: "*/5 * * * *", Task: asynq.NewTask(jobs.TaskTaxCaptureDispatch, nil), Options: []asynq.Option{asynq.MaxRetry(3)}},
			{Spec: "* * * * *", Task: asynq.NewTask(jobs.TaskCRMReminderDispatch, nil), Options: []asynq.Option{asynq.MaxRetry(3)}},
			{Spec: "* * * * *", Task: asynq.NewTask(jobs.TaskWebhookDeliveryDispatch, nil), Options: []asynq.Option{asynq.MaxRetry(3)}},
			{Spec: "* * * * *", Task: asynq.NewTask(jobs.TaskOutboxSweep, nil), Options: []asynq.Option{asynq.MaxRetry(3)}},
			{Spec: "* * * * *", Task: asynq.NewTask(jobs.TaskFinanceAutomationDispatch, nil), Options: []asynq.Option{asynq.MaxRetry(3)}},
			// Run CMMS PM generator hourly
			{Spec: "0 * * * *", Task: asynq.NewTask(jobs.TypeCMMSPMGeneratorScan, nil), Options: []asynq.Option{asynq.MaxRetry(3)}},
			{Spec: "5 0 * * *", Task: func() *asynq.Task { task, _ := jobs.NewFXDailyRatesTask(time.Time{}, false); return task }(), Options: []asynq.Option{asynq.MaxRetry(5)}},
			{Spec: "0 1 * * *", Task: asynq.NewTask(jobs.TaskDocumentDisposition, nil), Options: []asynq.Option{asynq.MaxRetry(3)}},
			{Spec: "* * * * *", Task: asynq.NewTask(jobs.TaskConnectorOutboxSweep, nil), Options: []asynq.Option{asynq.MaxRetry(3)}},
			{Spec: "*/5 * * * *", Task: asynq.NewTask(jobs.TaskPaymentReconciliation, nil), Options: []asynq.Option{asynq.MaxRetry(3)}},
			{Spec: "*/5 * * * *", Task: asynq.NewTask(jobs.TaskConnectorDeadLetterAudit, nil), Options: []asynq.Option{asynq.MaxRetry(3)}},
		},
	})
	if err != nil {
		logger.Error("init worker", slog.Any("error", err))
		os.Exit(1)
	}
	stopMetrics, err := startWorkerMetricsServer(cfg.WorkerMetricsAddr, logger)
	if err != nil {
		logger.Error("init worker metrics server", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := stopMetrics(shutdownCtx); err != nil {
			logger.Warn("shutdown worker metrics server", slog.Any("error", err))
		}
	}()

	if err := worker.Run(ctx); err != nil && err != context.Canceled {
		logger.Error("worker run", slog.Any("error", err))
		os.Exit(1)
	}
}

func mustLocation(name string) *time.Location {
	location, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return location
}
