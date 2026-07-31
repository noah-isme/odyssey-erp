package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

func applyMiddleware(h http.Handler, middlewares []func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

func TestCSRFMiddlewareRejectsMissingTokenAndAllowsValidToken(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	sessionManager := shared.NewSessionManager(client, "session", "secret", time.Hour, false)
	csrfManager := shared.NewCSRFManager("csrfsecret")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := applyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), MiddlewareStack(MiddlewareConfig{Logger: logger, SessionManager: sessionManager, CSRFManager: csrfManager}))

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodPost, "/settings/security/password", strings.NewReader("current_password=old&new_password=newpass123&confirm_password=newpass123")))
	require.Equal(t, http.StatusForbidden, missing.Code)

	primeReq := httptest.NewRequest(http.MethodGet, "/", nil)
	sess, err := sessionManager.Load(context.Background(), primeReq)
	require.NoError(t, err)
	token, err := csrfManager.EnsureToken(context.Background(), sess)
	require.NoError(t, err)
	primeRes := httptest.NewRecorder()
	require.NoError(t, sessionManager.Commit(context.Background(), primeRes, primeReq, sess))

	allowedReq := httptest.NewRequest(http.MethodPost, "/settings/security/password", strings.NewReader("current_password=old&new_password=newpass123&confirm_password=newpass123"))
	allowedReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	allowedReq.Header.Set("X-CSRF-Token", token)
	allowedReq.AddCookie(&http.Cookie{Name: sessionManager.CookieName(), Value: sess.ID})
	allowed := httptest.NewRecorder()
	handler.ServeHTTP(allowed, allowedReq)
	require.Equal(t, http.StatusOK, allowed.Code)
}
