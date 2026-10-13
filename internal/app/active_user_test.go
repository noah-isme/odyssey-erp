package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// TestActiveUserMiddlewareRejectsDeactivatedSession proves that a session
// opened before the user was deactivated (users.is_active = false, e.g. by
// offboarding) is rejected with 401 and destroyed on its next request.
func TestActiveUserMiddlewareRejectsDeactivatedSession(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	sessions := shared.NewSessionManager(client, "session", "secret", time.Hour, false)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	activeUsers := map[int64]bool{42: true, 43: false}
	var lookups []int64
	lookup := func(_ context.Context, id int64) (bool, error) {
		lookups = append(lookups, id)
		if id == 44 {
			return false, errors.New("db down")
		}
		return activeUsers[id], nil
	}
	reached := 0
	stack := append(MiddlewareStack(MiddlewareConfig{Logger: logger, SessionManager: sessions, CSRFManager: shared.NewCSRFManager("csrfsecret")}),
		activeUserMiddleware(lookup, sessions, logger))
	handler := applyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached++
		w.WriteHeader(http.StatusOK)
	}), stack)

	login := func(userID string) string {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		sess, err := sessions.Load(context.Background(), req)
		require.NoError(t, err)
		sess.SetUser(userID)
		require.NoError(t, sessions.Commit(context.Background(), httptest.NewRecorder(), req, sess))
		require.NotEmpty(t, mr.Keys())
		return sess.ID
	}
	get := func(path, sessionID string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if sessionID != "" {
			req.AddCookie(&http.Cookie{Name: sessions.CookieName(), Value: sessionID})
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	t.Run("active user passes", func(t *testing.T) {
		id := login("42")
		res := get("/hr/employees", id)
		require.Equal(t, http.StatusOK, res.Code)
	})

	t.Run("deactivated user is rejected and the session destroyed", func(t *testing.T) {
		before := reached
		id := login("43")
		keysBefore := len(mr.Keys())
		res := get("/hr/employees", id)
		require.Equal(t, http.StatusUnauthorized, res.Code)
		require.Equal(t, before, reached, "handler must not run")
		require.Len(t, mr.Keys(), keysBefore-1, "session removed from redis")
		var cleared bool
		for _, c := range res.Result().Cookies() {
			if c.Name == sessions.CookieName() && c.MaxAge < 0 {
				cleared = true
			}
		}
		require.True(t, cleared, "session cookie expired")

		// The same cookie no longer carries a user: the request is anonymous.
		res = get("/welcome", id)
		require.Equal(t, http.StatusOK, res.Code)
	})

	t.Run("unknown user id is rejected", func(t *testing.T) {
		id := login("not-a-number")
		res := get("/", id)
		require.Equal(t, http.StatusUnauthorized, res.Code)
	})

	t.Run("lookup error fails closed", func(t *testing.T) {
		id := login("44")
		res := get("/", id)
		require.Equal(t, http.StatusInternalServerError, res.Code)
	})

	t.Run("anonymous and static requests skip the lookup", func(t *testing.T) {
		n := len(lookups)
		require.Equal(t, http.StatusOK, get("/welcome", "").Code)
		id := login("43")
		require.Equal(t, http.StatusOK, get("/static/css/app.css", id).Code)
		require.Len(t, lookups, n)
	})

	t.Run("nil lookup is a no-op", func(t *testing.T) {
		h := activeUserMiddleware(nil, sessions, logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }))
		res := httptest.NewRecorder()
		h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/", nil))
		require.Equal(t, http.StatusTeapot, res.Code)
		require.Nil(t, poolUserActiveLookup(nil))
	})
}
