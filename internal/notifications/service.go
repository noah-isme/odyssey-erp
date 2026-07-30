package notifications

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidNotification = errors.New("notifications: invalid notification")
	ErrNotFound            = errors.New("notifications: notification not found")
)

type Store interface {
	Create(context.Context, Notification) (Notification, error)
	ListRecent(context.Context, int64, int) ([]Notification, error)
	ListUnread(context.Context, int64, int) ([]Notification, error)
	UnreadCount(context.Context, int64) (int64, error)
	MarkRead(context.Context, int64, int64, time.Time) (bool, error)
	MarkAllRead(context.Context, int64, time.Time) (int64, error)
}

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service { return &Service{store: store, now: time.Now} }

func (s *Service) Create(ctx context.Context, n Notification) (Notification, error) {
	if n.RecipientID <= 0 || strings.TrimSpace(n.Type) == "" || strings.TrimSpace(n.Title) == "" {
		return Notification{}, ErrInvalidNotification
	}
	return s.store.Create(ctx, n)
}

func (s *Service) ListRecent(ctx context.Context, recipientID int64, limit int) ([]Notification, error) {
	if recipientID <= 0 {
		return nil, ErrInvalidNotification
	}
	return s.store.ListRecent(ctx, recipientID, normalizeLimit(limit))
}

func (s *Service) ListUnread(ctx context.Context, recipientID int64, limit int) ([]Notification, error) {
	if recipientID <= 0 {
		return nil, ErrInvalidNotification
	}
	return s.store.ListUnread(ctx, recipientID, normalizeLimit(limit))
}

func (s *Service) UnreadCount(ctx context.Context, recipientID int64) (int64, error) {
	if recipientID <= 0 {
		return 0, ErrInvalidNotification
	}
	return s.store.UnreadCount(ctx, recipientID)
}

func (s *Service) MarkRead(ctx context.Context, recipientID, id int64) error {
	if recipientID <= 0 || id <= 0 {
		return ErrInvalidNotification
	}
	ok, err := s.store.MarkRead(ctx, recipientID, id, s.now())
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

func (s *Service) MarkAllRead(ctx context.Context, recipientID int64) (int64, error) {
	if recipientID <= 0 {
		return 0, ErrInvalidNotification
	}
	return s.store.MarkAllRead(ctx, recipientID, s.now())
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	if limit > 100 {
		return 100
	}
	return limit
}
