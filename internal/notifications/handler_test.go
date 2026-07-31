package notifications

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPUnreadCountAndMarkRead(t *testing.T) {
	store := &memoryStore{items: []Notification{{ID: 1, RecipientID: 9}, {ID: 2, RecipientID: 9}}}
	router := chi.NewRouter()
	handler := NewHandler(NewService(store))
	handler.userID = func(*http.Request) (int64, bool) { return 9, true }
	handler.MountRoutes(router)

	countResponse := httptest.NewRecorder()
	router.ServeHTTP(countResponse, httptest.NewRequest(http.MethodGet, "/api/notifications/unread-count", nil))
	require.Equal(t, http.StatusOK, countResponse.Code)
	require.JSONEq(t, `{"count":2}`, countResponse.Body.String())

	readResponse := httptest.NewRecorder()
	router.ServeHTTP(readResponse, httptest.NewRequest(http.MethodPost, "/api/notifications/1/read", nil))
	require.Equal(t, http.StatusNoContent, readResponse.Code)

	countResponse = httptest.NewRecorder()
	router.ServeHTTP(countResponse, httptest.NewRequest(http.MethodGet, "/api/notifications/unread-count", nil))
	require.JSONEq(t, `{"count":1}`, countResponse.Body.String())
}

func TestHTTPNotificationListMarkAllAndIsolation(t *testing.T) {
	store := &memoryStore{items: []Notification{
		{ID: 1, RecipientID: 9, Type: TypeInvoiceIssued, Title: "Invoice issued"},
		{ID: 2, RecipientID: 9, Type: TypePasswordReset, Title: "Password changed", ReadAt: func() *time.Time { ts := time.Now(); return &ts }()},
		{ID: 3, RecipientID: 10, Type: TypeInvoiceIssued, Title: "Other user"},
	}}
	router := chi.NewRouter()
	handler := NewHandler(NewService(store))
	handler.userID = func(*http.Request) (int64, bool) { return 9, true }
	handler.MountRoutes(router)

	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/notifications?unread=true&limit=10", nil))
	require.Equal(t, http.StatusOK, listResponse.Code)

	var payload struct {
		Notifications []Notification `json:"notifications"`
	}
	require.NoError(t, json.Unmarshal(listResponse.Body.Bytes(), &payload))
	require.Len(t, payload.Notifications, 1)
	require.Equal(t, int64(1), payload.Notifications[0].ID)

	markAllResponse := httptest.NewRecorder()
	router.ServeHTTP(markAllResponse, httptest.NewRequest(http.MethodPost, "/api/notifications/read-all", nil))
	require.Equal(t, http.StatusOK, markAllResponse.Code)
	require.JSONEq(t, `{"updated":1}`, markAllResponse.Body.String())

	handler.userID = func(*http.Request) (int64, bool) { return 0, false }
	unauthorizedResponse := httptest.NewRecorder()
	router.ServeHTTP(unauthorizedResponse, httptest.NewRequest(http.MethodGet, "/api/notifications", nil))
	require.Equal(t, http.StatusUnauthorized, unauthorizedResponse.Code)
}
