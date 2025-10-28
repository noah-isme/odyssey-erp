package shared

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditLog represents a record stored in audit_logs.
type AuditLog struct {
	ActorID  int64
	Action   string
	Entity   string
	EntityID string
	Meta     map[string]any
	At       time.Time
}

// AuditLogger writes records into audit_logs.
type AuditLogger struct {
	pool *pgxpool.Pool
}

// NewAuditLogger returns a new AuditLogger.
func NewAuditLogger(pool *pgxpool.Pool) *AuditLogger {
	return &AuditLogger{pool: pool}
}

// Record persists the log entry.
func (l *AuditLogger) Record(ctx context.Context, log AuditLog) error {
	if l == nil {
		return errors.New("audit logger not initialised")
	}
	if log.Action == "" || log.Entity == "" || log.EntityID == "" {
		return errors.New("audit log requires action/entity/entity_id")
	}
	metaJSON, err := json.Marshal(log.Meta)
	if err != nil {
		return err
	}
	_, err = l.pool.Exec(ctx, `INSERT INTO audit_logs (actor_id, action, entity, entity_id, meta, occurred_at) VALUES ($1, $2, $3, $4, $5, COALESCE($6, NOW()))`, log.ActorID, log.Action, log.Entity, log.EntityID, metaJSON, log.At)
	return err
}
