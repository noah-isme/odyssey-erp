package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/odyssey-erp/odyssey-erp/internal/analytics"
	analyticsdb "github.com/odyssey-erp/odyssey-erp/internal/analytics/db"
	"github.com/odyssey-erp/odyssey-erp/internal/app"
	"github.com/odyssey-erp/odyssey-erp/internal/boardpack"
	"github.com/odyssey-erp/odyssey-erp/internal/consol"
	"github.com/odyssey-erp/odyssey-erp/internal/variance"
	"github.com/odyssey-erp/odyssey-erp/jobs"
	"github.com/odyssey-erp/odyssey-erp/report"
)

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

	pool, err := pgxpool.New(ctx, cfg.PGDSN)
	if err != nil {
		logger.Error("connect database", slog.Any("error", err))
		os.Exit(1)
	}
	defer pool.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	defer func() {
		if err := redisClient.Close(); err != nil {
			logger.Warn("redis close", slog.Any("error", err))
		}
	}()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Warn("redis ping", slog.Any("error", err))
	}

	analyticsRepo := analyticsdb.New(pool)
	analyticsCache := analytics.NewCache(redisClient, 10*time.Minute)
	analyticsService := analytics.NewService(analyticsRepo, analyticsCache)

	warmupJob := jobs.NewInsightsWarmupJob(analyticsService, pool, logger, nil)
	anomalyJob := jobs.NewAnomalyScanJob(pool, logger, nil)
	consolRepo := consol.NewRepository(pool)
	consolService := consol.NewService(consolRepo)
	consolidator := jobs.NewConsolidateRefreshJob(consolService, consolRepo, logger, nil)
	varianceRepo := variance.NewRepository(pool)
	varianceService := variance.NewService(varianceRepo)
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
	boardpackJob := boardpack.NewJob(boardpack.JobConfig{
		Service:    boardpackService,
		Builder:    boardpackBuilder,
		Renderer:   boardpackRenderer,
		StorageDir: cfg.BoardPackStorageDir,
		Logger:     logger,
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

	worker, err := jobs.NewWorker(jobs.WorkerConfig{
		RedisOpts: asynq.RedisClientOpt{Addr: cfg.RedisAddr},
		Logger:    logger,
		Handlers: []jobs.TaskHandler{
			{Type: jobs.TaskAnalyticsInsightsWarmup, Handler: warmupJob.Handle},
			{Type: jobs.TaskAnalyticsAnomalyScan, Handler: anomalyJob.Handle},
			{Type: jobs.TaskConsolidateRefresh, Handler: consolidator.Handle},
			{Type: jobs.TaskVarianceSnapshotProcess, Handler: varianceJob.Handle},
			{Type: jobs.TaskBoardPackGenerate, Handler: boardpackJob.Handle},
		},
		Cron: []jobs.CronRegistration{
			{Spec: "15 1 * * *", Task: warmupTask, Options: []asynq.Option{asynq.MaxRetry(3)}},
			{Spec: "30 1 * * *", Task: anomalyTask, Options: []asynq.Option{asynq.MaxRetry(3)}},
			{Spec: "0 2 * * *", Task: consolidateTask, Options: []asynq.Option{asynq.MaxRetry(3)}},
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
