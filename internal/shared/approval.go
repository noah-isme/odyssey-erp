package shared

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ApprovalAction enumerates approval log actions.
type ApprovalAction string

const (
	// ApprovalSubmit marks a submit action.
	ApprovalSubmit ApprovalAction = "SUBMIT"
	// ApprovalApprove marks an approve action.
	ApprovalApprove ApprovalAction = "APPROVE"
	// ApprovalReject marks a reject action.
	ApprovalReject ApprovalAction = "REJECT"
)

// ApprovalLog represents a single approval record.
type ApprovalLog struct {
	ID      int64
	Module  string
	RefID   uuid.UUID
	ActorID int64
	Action  ApprovalAction
	Note    string
	At      time.Time
}

// ApprovalRecorder persists approval history.
type ApprovalRecorder struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewApprovalRecorder constructs ApprovalRecorder.
func NewApprovalRecorder(pool *pgxpool.Pool, logger *slog.Logger) *ApprovalRecorder {
	return &ApprovalRecorder{pool: pool, logger: logger}
}

// Record writes approval entry to database.
func (r *ApprovalRecorder) Record(ctx context.Context, log ApprovalLog) error {
	if r == nil {
		return errors.New("approval recorder not initialised")
	}
	if log.Module == "" {
		return errors.New("approval module required")
	}
	if log.ActorID == 0 {
		return errors.New("approval actor required")
	}
	if log.RefID == uuid.Nil {
		return errors.New("approval ref id required")
	}
	if log.Action == "" {
		return errors.New("approval action required")
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO approvals (module, ref_id, actor_id, action, note, at)
VALUES ($1, $2, $3, $4, $5, COALESCE($6, NOW()))`, log.Module, log.RefID, log.ActorID, string(log.Action), log.Note, log.At)
	if err != nil {
		r.logger.Error("record approval", slog.Any("error", err))
		return err
	}
	return nil
}

// List returns approvals for module/ref.
func (r *ApprovalRecorder) List(ctx context.Context, module string, ref uuid.UUID) ([]ApprovalLog, error) {
	if r == nil {
		return nil, errors.New("approval recorder not initialised")
	}
	rows, err := r.pool.Query(ctx, `SELECT id, module, ref_id, actor_id, action, note, at
FROM approvals WHERE module=$1 AND ref_id=$2 ORDER BY at ASC`, module, ref)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []ApprovalLog
	for rows.Next() {
		var l ApprovalLog
		var action string
		if err := rows.Scan(&l.ID, &l.Module, &l.RefID, &l.ActorID, &action, &l.Note, &l.At); err != nil {
			return nil, err
		}
		l.Action = ApprovalAction(action)
		logs = append(logs, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

// EnsureSubmit helper ensures a submit record exists else create.
func (r *ApprovalRecorder) EnsureSubmit(ctx context.Context, module string, ref uuid.UUID, actorID int64, note string) error {
	if r == nil {
		return errors.New("approval recorder not initialised")
	}
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT true FROM approvals WHERE module=$1 AND ref_id=$2 AND action='SUBMIT' LIMIT 1`, module, ref).Scan(&exists)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return r.Record(ctx, ApprovalLog{Module: module, RefID: ref, ActorID: actorID, Action: ApprovalSubmit, Note: note})
		}
		return err
	}
	return nil
}
