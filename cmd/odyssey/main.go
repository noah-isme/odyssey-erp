package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/odyssey-erp/odyssey-erp/internal/app"
	"github.com/odyssey-erp/odyssey-erp/internal/auth"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
	"github.com/odyssey-erp/odyssey-erp/jobs"
	"github.com/odyssey-erp/odyssey-erp/report"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := app.LoadConfig()
	if err != nil {
		slog.Default().Error("load config", slog.Any("error", err))
		os.Exit(1)
	}

	logger := app.NewLogger(cfg)

	dbpool, err := pgxpool.New(ctx, cfg.PGDSN)
	if err != nil {
		logger.Error("connect postgres", slog.Any("error", err))
		os.Exit(1)
	}
	defer dbpool.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Warn("redis ping", slog.Any("error", err))
	}
	defer func() {
		if err := redisClient.Close(); err != nil {
			logger.Warn("redis close", slog.Any("error", err))
		}
	}()

	sessionManager := shared.NewSessionManager(redisClient, "odyssey_session", cfg.SessionSecret, cfg.SessionTTL, cfg.IsProduction())
	csrfManager := shared.NewCSRFManager(cfg.CSRFSecret)

	templates, err := view.NewEngine()
	if err != nil {
		logger.Error("parse templates", slog.Any("error", err))
		os.Exit(1)
	}

	authRepo := auth.NewRepository(dbpool)
	authService := auth.NewService(authRepo)
	authHandler := auth.NewHandler(logger, authService, templates, sessionManager, csrfManager)

	reportClient := report.NewClient(cfg.GotenbergURL)
	reportHandler := report.NewHandler(reportClient, logger)

	inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: cfg.RedisAddr})
	defer func() {
		if err := inspector.Close(); err != nil {
			logger.Warn("inspector close", slog.Any("error", err))
		}
	}()
	jobHandler := jobs.NewHandler(inspector, logger)

	router := app.NewRouter(app.RouterParams{
		Logger:         logger,
		Config:         cfg,
		Templates:      templates,
		SessionManager: sessionManager,
		CSRFManager:    csrfManager,
		AuthHandler:    authHandler,
		ReportHandler:  reportHandler,
		JobHandler:     jobHandler,
	})

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
