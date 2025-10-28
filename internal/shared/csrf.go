package shared

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"time"
)

const (
	// CSRFSessionKey is the key used to persist tokens in the session store.
	CSRFSessionKey = "csrf_token"
	// CSRFFormField is the form field name carrying the CSRF token.
	CSRFFormField = "csrf_token"
)

// CSRFManager issues and verifies CSRF tokens bound to a session.
type CSRFManager struct {
	secret []byte
}

// NewCSRFManager returns a CSRFManager using the provided secret key.
func NewCSRFManager(secret string) *CSRFManager {
	return &CSRFManager{secret: []byte(secret)}
}

// EnsureToken retrieves or generates a CSRF token for the session.
func (m *CSRFManager) EnsureToken(ctx context.Context, sess *Session) (string, error) {
	if sess == nil {
		return "", errors.New("session missing")
	}
	if token := sess.Get(CSRFSessionKey); token != "" {
		return token, nil
	}
	token := m.generateToken(sess.ID)
	sess.Set(CSRFSessionKey, token)
	return token, nil
}

// VerifyToken compares the supplied token with the session token.
func (m *CSRFManager) VerifyToken(ctx context.Context, sess *Session, token string) error {
	if sess == nil {
		return ErrCSRFTokenMissing
	}
	expected := sess.Get(CSRFSessionKey)
	if expected == "" {
		return ErrCSRFTokenMissing
	}
	if token == "" {
		return ErrCSRFTokenMissing
	}
	if !hmac.Equal([]byte(expected), []byte(token)) {
		return ErrCSRFTokenMismatch
	}
	return nil
}

func (m *CSRFManager) generateToken(sessionID string) string {
	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(sessionID))
	_, _ = mac.Write([]byte{'|'})
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(time.Now().UnixNano()))
	_, _ = mac.Write(buf)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
