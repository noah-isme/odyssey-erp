package shared

import "context"

type sessionContextKey struct{}

// ContextWithSession stores the session in context.
func ContextWithSession(ctx context.Context, sess *Session) context.Context {
	return context.WithValue(ctx, sessionContextKey{}, sess)
}

// SessionFromContext extracts the session from context.
func SessionFromContext(ctx context.Context) *Session {
	sess, _ := ctx.Value(sessionContextKey{}).(*Session)
	return sess
}
