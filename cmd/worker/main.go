package main

import (
	"context"
	"github.com/odyssey-erp/odyssey-erp/internal/notifications"
	"github.com/odyssey-erp/odyssey-erp/internal/platform/cache"
	"github.com/odyssey-erp/odyssey-erp/internal/platform/db"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	"github.com/odyssey-erp/odyssey-erp/internal/analytics"
	"github.com/odyssey-erp/odyssey-erp/internal/app"
	"github.com/odyssey-erp/odyssey-erp/internal/boardpack"
	"github.com/odyssey-erp/odyssey-erp/internal/consol"
	"github.com/odyssey-erp/odyssey-erp/internal/fixedassets"
	"github.com/odyssey-erp/odyssey-erp/internal/variance"
	"github.com/odyssey-erp/odyssey-erp/jobs"
	"github.com/odyssey-erp/odyssey-erp/report"
)

type notificationEmailQueue struct{ client *asynq.Client }

func (q notificationEmailQueue) EnqueueEmail(ctx context.Context, email notifications.Email) error {
	task, err := jobs.NewSendEmailTask(jobs.SendEmailPayload{To: email.To, Subject: email.Subject, Body: email.Body})
	if err != nil {
		return err
	}
	_, err = q.client.EnqueueContext(ctx, task, asynq.Queue(jobs.QueueDefault))
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
	boardpackJob.SetNotificationDispatcher(notificationDispatcher)

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
		},
		Cron: []jobs.CronRegistration{
			{Spec: "15 1 * * *", Task: warmupTask, Options: []asynq.Option{asynq.MaxRetry(3)}},
			{Spec: "30 1 * * *", Task: anomalyTask, Options: []asynq.Option{asynq.MaxRetry(3)}},
			{Spec: "0 2 * * *", Task: consolidateTask, Options: []asynq.Option{asynq.MaxRetry(3)}},
			// Run overdue invoice scan every day at 8:00 AM
			{Spec: "0 8 * * *", Task: asynq.NewTask(jobs.TypeOverdueInvoicesScan, nil), Options: []asynq.Option{asynq.MaxRetry(3)}},
			{Spec: "5 * * * *", Task: asynq.NewTask(jobs.TypeReportScheduleScan, nil), Options: []asynq.Option{asynq.MaxRetry(3)}},
			{Spec: "10 2 1 * *", Task: asynq.NewTask(jobs.TaskFixedAssetDepreciation, nil), Options: []asynq.Option{asynq.MaxRetry(3)}},
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
