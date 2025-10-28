package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"

	"github.com/odyssey-erp/odyssey-erp/internal/auth"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
	_ "github.com/odyssey-erp/odyssey-erp/testing"
)

type stubRepo struct {
	user *auth.User
}

func (s *stubRepo) FindByEmail(ctx context.Context, email string) (*auth.User, error) {
	if s.user == nil {
		return nil, shared.ErrInvalidCredentials
	}
	return s.user, nil
}

func (s *stubRepo) CreateSession(ctx context.Context, id string, userID int64, expiresAt time.Time, ip, ua string) error {
	return nil
}

func (s *stubRepo) DeleteSession(ctx context.Context, id string) error {
	return nil
}

func newAuthHandler(t *testing.T, repo auth.Repository) (*auth.Handler, *shared.SessionManager, *shared.CSRFManager) {
	t.Helper()
	mr := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	sessionManager := shared.NewSessionManager(redisClient, "test_session", "secret", time.Hour, false)
	csrfManager := shared.NewCSRFManager("csrfsecret")
	templates, err := view.NewEngine()
	if err != nil {
		t.Fatalf("templates: %v", err)
	}
	handler := auth.NewHandler(nil, auth.NewService(repo), templates, sessionManager, csrfManager)
	return handler, sessionManager, csrfManager
}

func TestLoginPage(t *testing.T) {
	handler, sessionManager, _ := newAuthHandler(t, &stubRepo{})

	req := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	sess, err := sessionManager.Load(context.Background(), req)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	ctx := shared.ContextWithSession(req.Context(), sess)
	req = req.WithContext(ctx)

	res := httptest.NewRecorder()
	handler.ShowLoginForTest(res, req)
	if err := sessionManager.Commit(ctx, res, req, sess); err != nil {
		t.Fatalf("commit session: %v", err)
	}

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), "<form") {
		t.Fatalf("expected login form in body")
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	hashed, err := bcrypt.GenerateFromPassword([]byte("correctpass"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	handler, sessionManager, _ := newAuthHandler(t, &stubRepo{user: &auth.User{ID: 1, Email: "user@test.local", PasswordHash: string(hashed), IsActive: true}})

	// Prime session and CSRF token via GET.
	getReq := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	sess, err := sessionManager.Load(context.Background(), getReq)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	getCtx := shared.ContextWithSession(getReq.Context(), sess)
	getReq = getReq.WithContext(getCtx)
	getRes := httptest.NewRecorder()
	handler.ShowLoginForTest(getRes, getReq)
	if err := sessionManager.Commit(getCtx, getRes, getReq, sess); err != nil {
		t.Fatalf("commit session: %v", err)
	}

	token := sess.Get(shared.CSRFSessionKey)
	if token == "" {
		t.Fatalf("csrf token not set")
	}

	postData := url.Values{}
	postData.Set("email", "user@test.local")
	postData.Set("password", "wrongpass")
	postData.Set("csrf_token", token)

	postReq := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(postData.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Attach session cookie manually.
	postReq.AddCookie(&http.Cookie{Name: sessionManager.CookieName(), Value: sess.ID})

	loadedSess, err := sessionManager.Load(context.Background(), postReq)
	if err != nil {
		t.Fatalf("load session for post: %v", err)
	}
	postCtx := shared.ContextWithSession(postReq.Context(), loadedSess)
	postReq = postReq.WithContext(postCtx)

	res := httptest.NewRecorder()
	handler.HandleLoginForTest(res, postReq)
	if err := sessionManager.Commit(postCtx, res, postReq, loadedSess); err != nil {
		t.Fatalf("commit session post: %v", err)
	}

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), "Email atau password tidak valid") {
		t.Fatalf("expected error message in response")
	}
}
