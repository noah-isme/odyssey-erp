package notifications

import (
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
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
