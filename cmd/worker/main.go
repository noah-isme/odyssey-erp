package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/hibiken/asynq"

	"github.com/odyssey-erp/odyssey-erp/internal/app"
	"github.com/odyssey-erp/odyssey-erp/jobs"
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

	worker := jobs.NewWorker(asynq.RedisClientOpt{Addr: cfg.RedisAddr}, logger)
	if err := worker.Run(ctx); err != nil && err != context.Canceled {
		logger.Error("worker run", slog.Any("error", err))
		os.Exit(1)
	}
}
