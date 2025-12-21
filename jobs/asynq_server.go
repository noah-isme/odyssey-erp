package jobs

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
)

// Worker wraps the Asynq server and optional scheduler.
type Worker struct {
	server    *asynq.Server
	mux       *asynq.ServeMux
	scheduler *asynq.Scheduler
	logger    *slog.Logger
}

// TaskHandler allows injecting custom Asynq handlers during worker setup.
type TaskHandler struct {
	Type    string
	Handler asynq.HandlerFunc
}

// CronRegistration wires a cron expression to a prepared task.
type CronRegistration struct {
	Spec    string
	Task    *asynq.Task
	Options []asynq.Option
}

// WorkerConfig collects dependencies required to bootstrap the worker.
type WorkerConfig struct {
	RedisOpts asynq.RedisClientOpt
	Logger    *slog.Logger
	Handlers  []TaskHandler
	Cron      []CronRegistration
}

// NewWorker constructs a Worker instance.
func NewWorker(cfg WorkerConfig) (*Worker, error) {
	srv := asynq.NewServer(cfg.RedisOpts, asynq.Config{
		Concurrency: 5,
		Queues: map[string]int{
			QueueDefault: 1,
		},
	})
	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskTypeSendEmail, HandleSendEmailTask)
	mux.HandleFunc(TaskInventoryRevaluation, HandleInventoryRevaluationTask)
	mux.HandleFunc(TaskProcurementReindex, HandleProcurementReindexTask)
	for _, h := range cfg.Handlers {
		if h.Type == "" || h.Handler == nil {
			continue
		}
		mux.HandleFunc(h.Type, h.Handler)
	}

	var scheduler *asynq.Scheduler
	if len(cfg.Cron) > 0 {
		scheduler = asynq.NewScheduler(cfg.RedisOpts, &asynq.SchedulerOpts{Location: time.UTC})
		for _, entry := range cfg.Cron {
			if entry.Spec == "" || entry.Task == nil {
				continue
			}
			if _, err := scheduler.Register(entry.Spec, entry.Task, entry.Options...); err != nil {
				return nil, err
			}
		}
	}

	return &Worker{server: srv, mux: mux, scheduler: scheduler, logger: cfg.Logger}, nil
}

// Run starts processing jobs until context cancellation.
func (w *Worker) Run(ctx context.Context) error {
	if w == nil {
		return errors.New("worker: not configured")
	}
	if w.scheduler != nil {
		if err := w.scheduler.Start(); err != nil {
			return err
		}
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- w.server.Run(w.mux)
	}()
	select {
	case <-ctx.Done():
		if w.scheduler != nil {
			w.scheduler.Shutdown()
		}
		w.server.Shutdown()
		return ctx.Err()
	case err := <-errCh:
		if w.scheduler != nil {
			w.scheduler.Shutdown()
		}
		return err
	}
}

// Client submits jobs to the queue.
type Client struct {
	client *asynq.Client
}

// NewClient constructs an Asynq client.
func NewClient(redisOpts asynq.RedisClientOpt) (*Client, error) {
	client := asynq.NewClient(redisOpts)
	return &Client{client: client}, nil
}

// EnqueueSendEmail enqueues a send-email task.
func (c *Client) EnqueueSendEmail(ctx context.Context, payload SendEmailPayload) (*asynq.TaskInfo, error) {
	task, err := NewSendEmailTask(payload)
	if err != nil {
		return nil, err
	}
	return c.client.EnqueueContext(ctx, task, asynq.Queue(QueueDefault))
}

// EnqueueVarianceSnapshot enqueues a variance snapshot task.
func (c *Client) EnqueueVarianceSnapshot(ctx context.Context, snapshotID int64) (*asynq.TaskInfo, error) {
	task, err := NewVarianceSnapshotTask(snapshotID)
	if err != nil {
		return nil, err
	}
	return c.client.EnqueueContext(ctx, task, asynq.Queue(QueueDefault))
}

// EnqueueBoardPack enqueues a board pack generation task.
func (c *Client) EnqueueBoardPack(ctx context.Context, boardPackID int64) (*asynq.TaskInfo, error) {
	task, err := NewBoardPackTask(boardPackID)
	if err != nil {
		return nil, err
	}
	return c.client.EnqueueContext(ctx, task, asynq.Queue(QueueDefault))
}

// Close releases client resources.
func (c *Client) Close() error {
	return c.client.Close()
}

// Handler exposes HTTP endpoints for job observability.
type Handler struct {
	inspector *asynq.Inspector
	logger    *slog.Logger
	templates TemplateRenderer
}

// TemplateRenderer abstracts template rendering.
type TemplateRenderer interface {
	Render(w http.ResponseWriter, name string, data any) error
}

// NewHandler constructs an HTTP handler for jobs endpoints.
// Templates parameter is optional - if nil, falls back to inline HTML.
func NewHandler(inspector *asynq.Inspector, logger *slog.Logger, templates ...TemplateRenderer) *Handler {
	var t TemplateRenderer
	if len(templates) > 0 {
		t = templates[0]
	}
	return &Handler{inspector: inspector, logger: logger, templates: t}
}

// MountRoutes attaches job routes.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Get("/", h.index)
	r.Get("/health", h.health)
}

func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	var pending, active, completed int
	queueName := QueueDefault

	if h.inspector != nil {
		info, _ := h.inspector.GetQueueInfo(QueueDefault)
		if info != nil {
			queueName = info.Queue
			pending = int(info.Pending)
			active = int(info.Active)
			completed = int(info.Completed)
		}
	}

	data := map[string]any{
		"Queue":     queueName,
		"Pending":   pending,
		"Active":    active,
		"Completed": completed,
	}

	if h.templates != nil {
		viewData := struct {
			Title       string
			CurrentPath string
			Data        any
		}{
			Title:       "Jobs Dashboard",
			CurrentPath: r.URL.Path,
			Data:        data,
		}
		if err := h.templates.Render(w, "pages/jobs/index.html", viewData); err != nil {
			h.logger.Error("render jobs template", slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
		return
	}

	// Fallback for when templates not configured - render styled HTML using main.css
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := `<!DOCTYPE html>
<html lang="id" data-theme="dark">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Jobs Dashboard - Odyssey ERP</title>
    <link rel="stylesheet" href="/static/css/main.css">
</head>
<body style="background: var(--bg-app); min-height: 100vh;">
    <main class="jobs-page">
        <a href="/" class="jobs-back">← Back to Dashboard</a>
        
        <header class="jobs-header">
            <h1 class="jobs-header__title">Jobs Dashboard</h1>
            <p class="jobs-header__subtitle">Background job monitoring and management</p>
        </header>

        <section class="jobs-stats">
            <article class="jobs-stat">
                <div class="jobs-stat__label">Queue</div>
                <div class="jobs-stat__value">` + queueName + `</div>
            </article>
            <article class="jobs-stat">
                <div class="jobs-stat__label">Pending</div>
                <div class="jobs-stat__value">` + itoa(pending) + `</div>
            </article>
            <article class="jobs-stat">
                <div class="jobs-stat__label">Active</div>
                <div class="jobs-stat__value">` + itoa(active) + `</div>
            </article>
            <article class="jobs-stat">
                <div class="jobs-stat__label">Completed</div>
                <div class="jobs-stat__value">` + itoa(completed) + `</div>
            </article>
        </section>

        <section class="jobs-section">
            <header class="jobs-section__header">
                <h2 class="jobs-section__title">Quick Actions</h2>
            </header>
            <div class="jobs-section__body">
                <a href="/jobs/health" class="jobs-action">
                    <div>
                        <div class="jobs-action__title">Health Check</div>
                        <div class="jobs-action__desc">View queue health status (JSON)</div>
                    </div>
                </a>
            </div>
        </section>

        <section class="jobs-section">
            <header class="jobs-section__header">
                <h2 class="jobs-section__title">Registered Task Types</h2>
            </header>
            <div class="jobs-section__body">
                <ul class="jobs-tasks">
                    <li class="jobs-tasks__item"><code class="jobs-tasks__code">task:send_email</code> Send email notifications</li>
                    <li class="jobs-tasks__item"><code class="jobs-tasks__code">task:inventory_revaluation</code> Recalculate inventory costs</li>
                    <li class="jobs-tasks__item"><code class="jobs-tasks__code">task:procurement_reindex</code> Reindex procurement data</li>
                    <li class="jobs-tasks__item"><code class="jobs-tasks__code">task:variance_snapshot</code> Generate variance snapshots</li>
                    <li class="jobs-tasks__item"><code class="jobs-tasks__code">task:board_pack</code> Generate board pack reports</li>
                    <li class="jobs-tasks__item"><code class="jobs-tasks__code">task:insights_warmup</code> Warm up insights cache</li>
                    <li class="jobs-tasks__item"><code class="jobs-tasks__code">task:consolidate_refresh</code> Refresh consolidation data</li>
                    <li class="jobs-tasks__item"><code class="jobs-tasks__code">task:anomaly_scan</code> Scan for anomalies</li>
                </ul>
            </div>
        </section>
    </main>
</body>
</html>`
	_, _ = w.Write([]byte(html))
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	if h.inspector == nil {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"queue":"default","pending":0}`))
		return
	}
	info, err := h.inspector.GetQueueInfo(QueueDefault)
	if err != nil {
		h.logger.Warn("jobs health", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	pending := 0
	queueName := QueueDefault
	if info != nil {
		pending = int(info.Pending)
		queueName = info.Queue
	}
	_, _ = w.Write([]byte(`{"queue":"` + queueName + `","pending":` + itoa(pending) + `}`))
}

func itoa(i int) string {
	return strconv.FormatInt(int64(i), 10)
}
