package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	"github.com/odyssey-erp/odyssey-erp/internal/ap"
	"github.com/odyssey-erp/odyssey-erp/internal/analytics"
	apihttp "github.com/odyssey-erp/odyssey-erp/internal/api"
	"github.com/odyssey-erp/odyssey-erp/internal/app"
	"github.com/odyssey-erp/odyssey-erp/internal/boardpack"
	"github.com/odyssey-erp/odyssey-erp/internal/cmms"
	"github.com/odyssey-erp/odyssey-erp/internal/consol"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/bankfeeds"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/banking"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/forecasting"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/mockpay"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/oidc"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/stripe"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/whatsapp"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/openai"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/dhl"
	"github.com/odyssey-erp/odyssey-erp/internal/crm"
	"github.com/odyssey-erp/odyssey-erp/internal/documents"
	"github.com/odyssey-erp/odyssey-erp/internal/fixedassets"
	fxservice "github.com/odyssey-erp/odyssey-erp/internal/fx"
	"github.com/odyssey-erp/odyssey-erp/internal/notifications"
	"github.com/odyssey-erp/odyssey-erp/internal/outbox"
	"github.com/odyssey-erp/odyssey-erp/internal/payroll"
	"github.com/odyssey-erp/odyssey-erp/internal/platform/cache"
	"github.com/odyssey-erp/odyssey-erp/internal/platform/db"
	"github.com/odyssey-erp/odyssey-erp/internal/qms"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
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

	analyticsRepo := sqlc.New(pool)
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
	notificationDispatcher := notifications.NewDispatcher(notificationService, notificationRepo, notificationEmailQueue{client: asynqClient})
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

	connectorsRegistry := connectors.NewRegistry()
	connectorsRegistry.Register("mockpay", mockpay.NewAdapter(logger))
	connectorsRegistry.Register("stripe", stripe.NewAdapter(logger))
	connectorsRegistry.Register("oidc", oidc.NewAdapter(logger))
	connectorsRegistry.Register("whatsapp", whatsapp.NewAdapter(logger))
	connectorsRegistry.Register("openai", openai.NewAdapter(logger))
	connectorsRegistry.Register("dhl", dhl.NewAdapter(logger))
	connectorsOutboxWorker := connectors.NewOutboxWorker(sqlc.New(pool), connectorsRegistry)

	outboxRepo := outbox.NewRepository(pool)
	outboxDispatcher := outbox.NewDispatcher(pool, outboxRepo, logger)
	cmms.RegisterOutboxHandlers(outboxDispatcher, cmmsService, logger)
	qms.RegisterOutboxHandlers(outboxDispatcher, qmsService, logger)

	bankfeedsRepo := bankfeeds.NewPGRepository(pool)
	bankingRepo := banking.NewRepository(pool)
	// We don't have a poster in worker, but ImportStatement doesn't post to GL directly.
	bankingService := banking.NewService(bankingRepo, logger, nil)
	bankfeedsService := bankfeeds.NewService(bankfeedsRepo, bankingService, nil)
	bankFeedsProcessor := jobs.NewBankFeedsProcessor(bankfeedsService, logger)

	forecastRepo := forecasting.NewPGRepository(pool)
	forecastReaders := []forecasting.SourceReader{
		forecasting.NewMockReader("mock_ar", forecasting.SourceTypeOpenAR, false),
		forecasting.NewMockReader("mock_ap", forecasting.SourceTypePostedAP, true),
		forecasting.NewMockReader("mock_payroll", forecasting.SourceTypeApprovedPayroll, true),
	}
	forecastService := forecasting.NewService(forecastRepo, forecastReaders, logger)
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
			{Type: jobs.TypeBankFeedsSync, Handler: bankFeedsProcessor.ProcessSyncTask},
			{Type: jobs.TypeBankFeedsEvent, Handler: bankFeedsProcessor.ProcessEventTask},
			{Type: jobs.TypeCashForecastRefresh, Handler: forecastProcessor.ProcessRefreshTask},
			{Type: jobs.TypeCMMSPMGeneratorScan, Handler: jobs.HandleCMMSPMGeneratorScanTask(logger, func(ctx context.Context) error {
				_, err := cmmsService.GenerateAllPMWorkOrders(ctx)
				return err
			})},
			{Type: jobs.TaskDocumentDisposition, Handler: jobs.HandleDocumentDisposition(documentsService)},
			{Type: jobs.TaskConnectorOutboxSweep, Handler: jobs.HandleConnectorOutboxSweep(connectorsOutboxWorker)},
			{Type: jobs.TaskProcessAPInvoice, Handler: jobs.HandleProcessAPInvoice(apOrchestrator.ProcessInvoice)},
		},
		FXFetcher:   fxJobFetcher{service: fxDailyService},
		FXCompanies: fxRepo,
		FXLocation:  mustLocation("Asia/Jakarta"),
		FXLogger:    logger,
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
			// Run CMMS PM generator hourly
			{Spec: "0 * * * *", Task: asynq.NewTask(jobs.TypeCMMSPMGeneratorScan, nil), Options: []asynq.Option{asynq.MaxRetry(3)}},
			{Spec: "5 0 * * *", Task: func() *asynq.Task { task, _ := jobs.NewFXDailyRatesTask(time.Time{}, false); return task }(), Options: []asynq.Option{asynq.MaxRetry(5)}},
			{Spec: "0 1 * * *", Task: asynq.NewTask(jobs.TaskDocumentDisposition, nil), Options: []asynq.Option{asynq.MaxRetry(3)}},
			{Spec: "* * * * *", Task: asynq.NewTask(jobs.TaskConnectorOutboxSweep, nil), Options: []asynq.Option{asynq.MaxRetry(3)}},
		},
	})
	if err != nil {
		logger.Error("init worker", slog.Any("error", err))
		os.Exit(1)
	}

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
