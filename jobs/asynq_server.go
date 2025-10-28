package jobs

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
)

// Worker wraps the Asynq server.
type Worker struct {
	server *asynq.Server
	mux    *asynq.ServeMux
	logger *slog.Logger
}

// NewWorker constructs a Worker instance.
func NewWorker(redisOpts asynq.RedisClientOpt, logger *slog.Logger) *Worker {
	srv := asynq.NewServer(redisOpts, asynq.Config{
		Concurrency: 5,
		Queues: map[string]int{
			QueueDefault: 1,
		},
	})
	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskTypeSendEmail, HandleSendEmailTask)
	mux.HandleFunc(TaskInventoryRevaluation, HandleInventoryRevaluationTask)
	mux.HandleFunc(TaskProcurementReindex, HandleProcurementReindexTask)
	return &Worker{server: srv, mux: mux, logger: logger}
}

// Run starts processing jobs until context cancellation.
func (w *Worker) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- w.server.Run(w.mux)
	}()
	select {
	case <-ctx.Done():
		w.server.Shutdown()
		return ctx.Err()
	case err := <-errCh:
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
