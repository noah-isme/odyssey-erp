package main

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/redis/go-redis/v9"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting"
	"github.com/odyssey-erp/odyssey-erp/internal/analytics"
	analyticsdb "github.com/odyssey-erp/odyssey-erp/internal/analytics/db"
	"github.com/odyssey-erp/odyssey-erp/internal/analytics/export"
	analytichttp "github.com/odyssey-erp/odyssey-erp/internal/analytics/http"
	"github.com/odyssey-erp/odyssey-erp/internal/analytics/svg"
	"github.com/odyssey-erp/odyssey-erp/internal/app"
	"github.com/odyssey-erp/odyssey-erp/internal/audit"
	auditdb "github.com/odyssey-erp/odyssey-erp/internal/audit/db"
	audithttp "github.com/odyssey-erp/odyssey-erp/internal/audit/http"
	"github.com/odyssey-erp/odyssey-erp/internal/auth"
	"github.com/odyssey-erp/odyssey-erp/internal/insights"
	insightsdb "github.com/odyssey-erp/odyssey-erp/internal/insights/db"
	insightshhtp "github.com/odyssey-erp/odyssey-erp/internal/insights/http"
	"github.com/odyssey-erp/odyssey-erp/internal/integration"
	"github.com/odyssey-erp/odyssey-erp/internal/inventory"
	"github.com/odyssey-erp/odyssey-erp/internal/observability"
	"github.com/odyssey-erp/odyssey-erp/internal/procurement"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
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

func main() {
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

	auditLogger := shared.NewAuditLogger(dbpool)
	approvalRecorder := shared.NewApprovalRecorder(dbpool, logger)
	idempotencyStore := shared.NewIdempotencyStore(dbpool)

	accountingRepo := accounting.NewRepository(dbpool)
	accountingService := accounting.NewService(accountingRepo, auditLogger)
	integrationHooks := integration.NewHooks(accountingService, accountingRepo)

	inventoryRepo := inventory.NewRepository(dbpool)
	inventoryService := inventory.NewService(inventoryRepo, auditLogger, idempotencyStore, inventory.ServiceConfig{}, integrationHooks)

	procurementRepo := procurement.NewRepository(dbpool)
	procurementService := procurement.NewService(procurementRepo, inventoryService, approvalRecorder, auditLogger, idempotencyStore, integrationHooks)

	rbacService := rbac.NewService(dbpool)
	rbacMiddleware := rbac.Middleware{Service: rbacService, Logger: logger}

	analyticsRepo := analyticsdb.New(dbpool)
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

	insightsRepo := insightsdb.New(dbpool)
	insightsService := insights.NewService(insightsRepo)
	insightsHandler := insightshhtp.NewHandler(logger, insightsService, templates, rbacService)
	auditRepo := auditdb.New(dbpool)
	auditService := audit.NewService(auditRepo)
	auditExporter := audit.NewExporter(templates)
	auditHandler := audithttp.NewHandler(logger, auditService, templates, auditExporter, rbacService)
	metrics := observability.NewMetrics()

	inventoryHandler := inventory.NewHandler(logger, inventoryService, templates, csrfManager, sessionManager, rbacMiddleware)
	procurementHandler := procurement.NewHandler(logger, procurementService, templates, csrfManager, sessionManager, rbacMiddleware)

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
		Logger:             logger,
		Config:             cfg,
		Templates:          templates,
		SessionManager:     sessionManager,
		CSRFManager:        csrfManager,
		AuthHandler:        authHandler,
		InventoryHandler:   inventoryHandler,
		ProcurementHandler: procurementHandler,
		ReportHandler:      reportHandler,
		JobHandler:         jobHandler,
		AnalyticsHandler:   analyticsHandler,
		InsightsHandler:    insightsHandler,
		AuditHandler:       auditHandler,
		Metrics:            metrics,
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
