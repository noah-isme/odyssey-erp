package portal

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

func TestInvitationTokenReturnsHashOfPlainToken(t *testing.T) {
	plain, hash, err := invitationToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(plain) != 64 || len(hash) != 64 {
		t.Fatalf("token lengths plain=%d hash=%d", len(plain), len(hash))
	}
	sum := sha256.Sum256([]byte(plain))
	if hash != hex.EncodeToString(sum[:]) {
		t.Fatalf("hash does not match token: %q", hash)
	}
}

func TestPortalUserReadsOnlyValidSessionIdentity(t *testing.T) {
	if _, _, ok := portalUser(httptest.NewRequest(http.MethodGet, "/", nil)); ok {
		t.Fatal("request without session must not authenticate")
	}

	req := portalRequestWithSession(t, "12", "7")
	userID, companyID, ok := portalUser(req)
	if !ok || userID != 12 || companyID != 7 {
		t.Fatalf("identity user=%d company=%d ok=%v", userID, companyID, ok)
	}
}

func TestPortalRoutesRejectUnavailableOrUnauthorizedRequests(t *testing.T) {
	router := chi.NewRouter()
	NewHandler(nil, nil, rbac.Middleware{}).MountRoutes(router)

	for _, tc := range []struct {
		method string
		path   string
		status int
	}{
		{http.MethodPost, "/portal/admin/invitations", http.StatusForbidden},
		{http.MethodPost, "/portal/invitations/accept", http.StatusServiceUnavailable},
		{http.MethodPut, "/portal/profile", http.StatusUnauthorized},
		{http.MethodPost, "/portal/chat", http.StatusUnauthorized},
	} {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
			if rec.Code != tc.status {
				t.Fatalf("status=%d want %d body=%s", rec.Code, tc.status, rec.Body.String())
			}
		})
	}
}

func portalRequestWithSession(t *testing.T, userID, companyID string) *http.Request {
	t.Helper()
	manager := shared.NewSessionManager(nil, "session", "secret", time.Hour, false)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	session, err := manager.Load(req.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	session.SetUser(userID)
	session.Set("company_id", companyID)
	return req.WithContext(shared.ContextWithSession(req.Context(), session))
}
