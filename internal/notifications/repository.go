package notifications

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type DB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Repository struct{ db DB }

func NewRepository(db DB) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, n Notification) (Notification, error) {
	if r == nil || r.db == nil {
		return Notification{}, errors.New("notifications: repository not configured")
	}
	var dedupeKey any
	if n.DedupeKey != "" {
		dedupeKey = n.DedupeKey
	}
	err := r.db.QueryRow(ctx, `INSERT INTO notifications (recipient_id, dedupe_key, type, title, body, url)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (recipient_id, type, dedupe_key) WHERE dedupe_key IS NOT NULL
		DO UPDATE SET dedupe_key = EXCLUDED.dedupe_key
		RETURNING id, recipient_id, COALESCE(dedupe_key, ''), type, title, body, url, read_at, created_at, updated_at`,
		n.RecipientID, dedupeKey, n.Type, n.Title, n.Body, n.URL).Scan(
		&n.ID, &n.RecipientID, &n.DedupeKey, &n.Type, &n.Title, &n.Body, &n.URL, &n.ReadAt, &n.CreatedAt, &n.UpdatedAt)
	return n, err
}

func (r *Repository) ListRecent(ctx context.Context, recipientID int64, limit int) ([]Notification, error) {
	rows, err := r.db.Query(ctx, `SELECT id, recipient_id, COALESCE(dedupe_key, ''), type, title, body, url, read_at, created_at, updated_at
		FROM notifications WHERE recipient_id=$1 ORDER BY created_at DESC LIMIT $2`, recipientID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Notification, 0)
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.RecipientID, &n.DedupeKey, &n.Type, &n.Title, &n.Body, &n.URL, &n.ReadAt, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, n)
	}
	return items, rows.Err()
}

func (r *Repository) ListUnread(ctx context.Context, recipientID int64, limit int) ([]Notification, error) {
	rows, err := r.db.Query(ctx, `SELECT id, recipient_id, COALESCE(dedupe_key, ''), type, title, body, url, read_at, created_at, updated_at
		FROM notifications WHERE recipient_id=$1 AND read_at IS NULL ORDER BY created_at DESC LIMIT $2`, recipientID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Notification, 0)
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.RecipientID, &n.DedupeKey, &n.Type, &n.Title, &n.Body, &n.URL, &n.ReadAt, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, n)
	}
	return items, rows.Err()
}

func (r *Repository) UnreadCount(ctx context.Context, recipientID int64) (int64, error) {
	var count int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE recipient_id=$1 AND read_at IS NULL`, recipientID).Scan(&count)
	return count, err
}

func (r *Repository) MarkRead(ctx context.Context, recipientID, id int64, at time.Time) (bool, error) {
	tag, err := r.db.Exec(ctx, `UPDATE notifications SET read_at=$3, updated_at=$3
		WHERE id=$1 AND recipient_id=$2 AND read_at IS NULL`, id, recipientID, at)
	return tag.RowsAffected() > 0, err
}

func (r *Repository) MarkAllRead(ctx context.Context, recipientID int64, at time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx, `UPDATE notifications SET read_at=$2, updated_at=$2
		WHERE recipient_id=$1 AND read_at IS NULL`, recipientID, at)
	return tag.RowsAffected(), err
}

func (r *Repository) Channels(ctx context.Context, recipientID int64, notificationType string) (Channels, error) {
	channels := Channels{InApp: true, Email: true, SMS: false, WhatsApp: false}
	err := r.db.QueryRow(ctx, `SELECT in_app_enabled, email_enabled, sms_enabled, whatsapp_enabled FROM notification_preferences
		WHERE user_id=$1 AND notification_type=$2`, recipientID, notificationType).Scan(&channels.InApp, &channels.Email, &channels.SMS, &channels.WhatsApp)
	if errors.Is(err, pgx.ErrNoRows) {
		return channels, nil
	}
	return channels, err
}

func (r *Repository) UserEmail(ctx context.Context, recipientID int64) (string, error) {
	var email string
	err := r.db.QueryRow(ctx, `SELECT email FROM users WHERE id=$1 AND is_active=TRUE`, recipientID).Scan(&email)
	return email, err
}

func (r *Repository) UserPhone(ctx context.Context, recipientID int64) (string, error) {
	var phone *string
	err := r.db.QueryRow(ctx, `SELECT phone FROM users WHERE id=$1 AND is_active=TRUE`, recipientID).Scan(&phone)
	if err != nil {
		return "", err
	}
	if phone == nil {
		return "", nil
	}
	return *phone, nil
}
