package shared

import (
	"context"
	"strconv"
)

type sessionContextKey struct{}

// ContextWithSession stores the session in context.
func ContextWithSession(ctx context.Context, sess *Session) context.Context {
	return context.WithValue(ctx, sessionContextKey{}, sess)
}

// SessionFromContext extracts the session from context.
func SessionFromContext(ctx context.Context) *Session {
	if ctx == nil {
		return nil
	}
	sess, _ := ctx.Value(sessionContextKey{}).(*Session)
	return sess
}

// RequestIdentity is the authenticated tenant boundary for request-scoped
// application code. Handlers should pass this identity into services instead
// of accepting company IDs from query strings or form fields.
type RequestIdentity struct {
	UserID    int64
	CompanyID int64
}

// IdentityFromContext extracts and validates the user and active company from
// the session. A session without both values is not a tenant-scoped identity.
func IdentityFromContext(ctx context.Context) (RequestIdentity, bool) {
	sess := SessionFromContext(ctx)
	if sess == nil {
		return RequestIdentity{}, false
	}

	userID, err := strconv.ParseInt(sess.User(), 10, 64)
	if err != nil || userID <= 0 {
		return RequestIdentity{}, false
	}
	companyID, err := strconv.ParseInt(sess.Get("company_id"), 10, 64)
	if err != nil || companyID <= 0 {
		return RequestIdentity{}, false
	}

	return RequestIdentity{UserID: userID, CompanyID: companyID}, true
}

// CompanyIDFromContext returns the active company only when the request has a
// complete authenticated tenant identity.
func CompanyIDFromContext(ctx context.Context) (int64, bool) {
	identity, ok := IdentityFromContext(ctx)
	return identity.CompanyID, ok
}
