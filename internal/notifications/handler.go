package notifications

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

type Handler struct {
	service *Service
	userID  func(*http.Request) (int64, bool)
}

func NewHandler(service *Service) *Handler { return &Handler{service: service, userID: recipientID} }

func (h *Handler) MountRoutes(r chi.Router) {
	r.Get("/api/notifications", h.list)
	r.Get("/api/notifications/unread-count", h.unreadCount)
	r.Post("/api/notifications/{id}/read", h.markRead)
	r.Post("/api/notifications/read-all", h.markAllRead)
}

func recipientID(r *http.Request) (int64, bool) {
	sess := shared.SessionFromContext(r.Context())
	if sess == nil || sess.User() == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(sess.User(), 10, 64)
	return id, err == nil && id > 0
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	var items []Notification
	var err error
	if r.URL.Query().Get("unread") == "true" {
		items, err = h.service.ListUnread(r.Context(), uid, limit)
	} else {
		items, err = h.service.ListRecent(r.Context(), uid, limit)
	}
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": items})
}

func (h *Handler) unreadCount(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}
	count, err := h.service.UnreadCount(r.Context(), uid)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"count": count})
}

func (h *Handler) markRead(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	err = h.service.MarkRead(r.Context(), uid, id)
	if errors.Is(err, ErrNotFound) {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) markAllRead(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}
	count, err := h.service.MarkAllRead(r.Context(), uid)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"updated": count})
}
