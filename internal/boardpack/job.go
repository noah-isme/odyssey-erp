package boardpack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/hibiken/asynq"

	"github.com/odyssey-erp/odyssey-erp/jobs"
)

// JobConfig wires dependencies required by the worker job.
type JobConfig struct {
	Service    *Service
	Builder    *Builder
	Renderer   *Renderer
	StorageDir string
	Logger     *slog.Logger
}

// Job processes board pack generation requests coming from the queue.
type Job struct {
	service    *Service
	builder    *Builder
	renderer   *Renderer
	storageDir string
	logger     *slog.Logger
}

// NewJob constructs a Job handler.
func NewJob(cfg JobConfig) *Job {
	return &Job{service: cfg.Service, builder: cfg.Builder, renderer: cfg.Renderer, storageDir: cfg.StorageDir, logger: cfg.Logger}
}

// Handle fulfils the asynq.HandlerFunc contract.
func (j *Job) Handle(ctx context.Context, task *asynq.Task) error {
	if j == nil || j.service == nil || j.builder == nil || j.renderer == nil {
		return fmt.Errorf("boardpack job not configured")
	}
	var payload jobs.BoardPackPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return asynq.SkipRetry
	}
	if payload.BoardPackID == 0 {
		return asynq.SkipRetry
	}
	pack, err := j.service.Get(ctx, payload.BoardPackID)
	if err != nil {
		if errors.Is(err, ErrBoardPackNotFound) {
			return asynq.SkipRetry
		}
		return err
	}
	if pack.Status == StatusReady {
		return nil
	}
	if err := j.service.MarkInProgress(ctx, pack.ID); err != nil {
		if errors.Is(err, ErrInvalidStatus) {
			current, loadErr := j.service.Get(ctx, pack.ID)
			if loadErr == nil && (current.Status == StatusInProgress || current.Status == StatusReady) {
				return nil
			}
		}
		return err
	}
	data, err := j.builder.Build(ctx, pack)
	if err != nil {
		_ = j.service.MarkFailed(ctx, pack.ID, err.Error())
		return err
	}
	rendered, err := j.renderer.Render(ctx, data)
	if err != nil {
		_ = j.service.MarkFailed(ctx, pack.ID, err.Error())
		return err
	}
	path, err := j.save(ctx, pack.ID, rendered.PDF)
	if err != nil {
		_ = j.service.MarkFailed(ctx, pack.ID, err.Error())
		return err
	}
	meta := map[string]any{}
	if len(data.Warnings) > 0 {
		meta["warnings"] = data.Warnings
	}
	meta["generated_at"] = data.GeneratedAt
	if _, err := j.service.MarkReady(ctx, pack, path, rendered.Length, nil, meta); err != nil {
		return err
	}
	if j.logger != nil {
		j.logger.Info("board pack ready", slog.Int64("board_pack_id", pack.ID), slog.String("file", path))
	}
	return nil
}

func (j *Job) save(_ context.Context, id int64, pdf []byte) (string, error) {
	dir := j.storageDir
	if strings.TrimSpace(dir) == "" {
		dir = filepath.Join(os.TempDir(), "boardpacks")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("board-pack-%d.pdf", id)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, pdf, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
