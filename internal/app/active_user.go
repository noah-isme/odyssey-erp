package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// userActiveLookup reports whether a user exists and is active.
type userActiveLookup func(ctx context.Context, userID int64) (bool, error)

// poolUserActiveLookup checks users.is_active through loadWorkspaceUser. A
// missing user counts as inactive.
func poolUserActiveLookup(pool *pgxpool.Pool) userActiveLookup {
	if pool == nil {
		return nil
	}
	return func(ctx context.Context, userID int64) (bool, error) {
		user, err := loadWorkspaceUser(ctx, pool, userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return user.IsActive, nil
	}
}

// activeUserMiddleware rejects every request whose session belongs to a
// deactivated (users.is_active = false) or deleted user: the session is
// destroyed and the response is 401. Login already refuses inactive users;
// this closes sessions that were opened before the user was deactivated
// (for example by offboarding). Requests without a logged-in session and
// static assets pass through unchecked.
func activeUserMiddleware(lookup userActiveLookup, sessions *shared.SessionManager, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if lookup == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/static/") {
				next.ServeHTTP(w, r)
				return
			}
			sess := shared.SessionFromContext(r.Context())
			if sess == nil || strings.TrimSpace(sess.User()) == "" {
				next.ServeHTTP(w, r)
				return
			}
			active := false
			if id, err := strconv.ParseInt(strings.TrimSpace(sess.User()), 10, 64); err == nil && id > 0 {
				active, err = lookup(r.Context(), id)
				if err != nil {
					if logger != nil {
						logger.Error("check session user is active", slog.Any("error", err))
					}
					shared.WriteHTTPError(w, http.StatusInternalServerError, "")
					return
				}
			}
			if !active {
				sessions.Destroy(sess)
				shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
