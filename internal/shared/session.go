package shared

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// FlashMessage represents a one-time notification stored in session.
type FlashMessage struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// SessionManager orchestrates cookie based sessions backed by Redis.
type SessionManager struct {
	client     *redis.Client
	cookieName string
	ttl        time.Duration
	secure     bool
	secret     []byte
}

// Session holds per-request session data.
type Session struct {
	ID        string
	values    map[string]string
	userID    string
	flashes   []FlashMessage
	manager   *SessionManager
	isNew     bool
	dirty     bool
	destroyed bool
}

type sessionPayload struct {
	Values  map[string]string `json:"values"`
	UserID  string            `json:"user_id"`
	Flashes []FlashMessage    `json:"flashes"`
}

// NewSessionManager constructs a SessionManager.
func NewSessionManager(client *redis.Client, cookieName string, secret string, ttl time.Duration, secure bool) *SessionManager {
	return &SessionManager{
		client:     client,
		cookieName: cookieName,
		ttl:        ttl,
		secure:     secure,
		secret:     []byte(secret),
	}
}

// Load loads or creates a new session for request.
func (sm *SessionManager) Load(ctx context.Context, r *http.Request) (*Session, error) {
	cookie, err := r.Cookie(sm.cookieName)
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			return sm.newSession(), nil
		}
		return nil, err
	}

	payload, err := sm.client.Get(ctx, sm.redisKey(cookie.Value)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			sess := sm.newSession()
			sess.ID = cookie.Value
			sess.isNew = true
			return sess, nil
		}
		return nil, err
	}

	var stored sessionPayload
	if err := json.Unmarshal(payload, &stored); err != nil {
		return nil, err
	}

	sess := sm.newSession()
	sess.ID = cookie.Value
	sess.values = stored.Values
	sess.userID = stored.UserID
	sess.flashes = stored.Flashes
	sess.isNew = false
	return sess, nil
}

// Commit persists the session and writes cookie headers as needed.
func (sm *SessionManager) Commit(ctx context.Context, w http.ResponseWriter, r *http.Request, sess *Session) error {
	if sess == nil {
		return nil
	}

	if sess.destroyed {
		if err := sm.client.Del(ctx, sm.redisKey(sess.ID)).Err(); err != nil && !errors.Is(err, redis.Nil) {
			return err
		}
		http.SetCookie(w, &http.Cookie{
			Name:     sm.cookieName,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   sm.secure,
			SameSite: http.SameSiteStrictMode,
		})
		return nil
	}

	if sess.isNew && sess.ID == "" {
		sess.ID = sm.generateSessionID()
	}

	if sess.dirty || sess.isNew {
		payload := sessionPayload{Values: sess.values, UserID: sess.userID, Flashes: sess.flashes}
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if err := sm.client.Set(ctx, sm.redisKey(sess.ID), data, sm.ttl).Err(); err != nil {
			return err
		}
		sess.dirty = false
	}

	if sess.ID != "" {
		cookie := &http.Cookie{
			Name:     sm.cookieName,
			Value:    sess.ID,
			Path:     "/",
			HttpOnly: true,
			Secure:   sm.secure,
			SameSite: http.SameSiteStrictMode,
			Expires:  time.Now().Add(sm.ttl),
		}
		http.SetCookie(w, cookie)
	}

	// Clear flashes after they have been persisted once.
	if len(sess.flashes) > 0 {
		sess.flashes = nil
		sess.dirty = true
		_ = sm.client.Set(ctx, sm.redisKey(sess.ID), mustJSON(sessionPayload{Values: sess.values, UserID: sess.userID, Flashes: sess.flashes}), sm.ttl).Err()
	}

	return nil
}

// Destroy marks the session for deletion.
func (sm *SessionManager) Destroy(sess *Session) {
	if sess == nil {
		return
	}
	sess.destroyed = true
}

// TTL exposes the configured session lifetime.
func (sm *SessionManager) TTL() time.Duration {
	return sm.ttl
}

// CookieName returns the cookie identifier used for sessions.
func (sm *SessionManager) CookieName() string {
	return sm.cookieName
}

// Session helpers

// Set stores a key-value pair.
func (s *Session) Set(key, value string) {
	if s.values == nil {
		s.values = make(map[string]string)
	}
	s.values[key] = value
	s.dirty = true
}

// Get retrieves a value.
func (s *Session) Get(key string) string {
	if s.values == nil {
		return ""
	}
	return s.values[key]
}

// Delete removes a value.
func (s *Session) Delete(key string) {
	if s.values == nil {
		return
	}
	delete(s.values, key)
	s.dirty = true
}

// SetUser associates the session with a user ID.
func (s *Session) SetUser(id string) {
	s.userID = id
	s.dirty = true
}

// User returns the current user ID.
func (s *Session) User() string {
	return s.userID
}

// AddFlash queues a flash message.
func (s *Session) AddFlash(msg FlashMessage) {
	s.flashes = append(s.flashes, msg)
	s.dirty = true
}

// PopFlash retrieves and clears the oldest flash message.
func (s *Session) PopFlash() *FlashMessage {
	if len(s.flashes) == 0 {
		if value := s.Get("flash"); value != "" {
			s.Delete("flash")
		}
		return nil
	}
	msg := s.flashes[0]
	s.flashes = s.flashes[1:]
	s.dirty = true
	return &msg
}

func (sm *SessionManager) newSession() *Session {
	return &Session{
		ID:      sm.generateSessionID(),
		values:  make(map[string]string),
		manager: sm,
		isNew:   true,
		dirty:   true,
	}
}

func (sm *SessionManager) redisKey(id string) string {
	return "session:" + id
}

func (sm *SessionManager) generateSessionID() string {
	if id, err := uuid.NewRandom(); err == nil {
		return id.String()
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return base64.RawURLEncoding.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	if len(sm.secret) > 0 {
		for i := range b {
			b[i] ^= sm.secret[i%len(sm.secret)]
		}
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func mustJSON(v sessionPayload) []byte {
	data, _ := json.Marshal(v)
	return data
}
