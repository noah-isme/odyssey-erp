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

// Close releases client resources.
func (c *Client) Close() error {
	return c.client.Close()
}

// Handler exposes HTTP endpoints for job observability.
type Handler struct {
	inspector *asynq.Inspector
	logger    *slog.Logger
}

// NewHandler constructs an HTTP handler for jobs endpoints.
func NewHandler(inspector *asynq.Inspector, logger *slog.Logger) *Handler {
	return &Handler{inspector: inspector, logger: logger}
}

// MountRoutes attaches job routes.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Get("/health", h.health)
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
