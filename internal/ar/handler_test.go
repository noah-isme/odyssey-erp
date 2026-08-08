package ar

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/notifications"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type arPermissionReader struct{ permissions []string }

func (p arPermissionReader) EffectivePermissions(context.Context, int64) ([]string, error) {
	return p.permissions, nil
}

func arRequest(t *testing.T, method, path string) *http.Request {
	t.Helper()
	manager := shared.NewSessionManager(nil, "session", "secret", time.Hour, false)
	req := httptest.NewRequest(method, path, strings.NewReader(""))
	session, err := manager.Load(req.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	session.SetUser("7")
	return req.WithContext(shared.ContextWithSession(req.Context(), session))
}

func TestCreditNoteRouteRequiresPermission(t *testing.T) {
	router := chi.NewRouter()
	handler := NewHandler(slog.Default(), &Service{}, nil, nil, nil, rbac.Middleware{Service: arPermissionReader{}}, nil, nil)
	router.Route("/finance/ar", handler.MountRoutes)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, arRequest(t, http.MethodGet, "/finance/ar/credit-notes/1"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestShowCreditNoteRendersSSR(t *testing.T) {
	templates, err := view.NewEngine()
	if err != nil {
		t.Fatal(err)
	}
	repo := newCreditNoteMemoryRepo()
	repo.notes[1] = &ARCreditNote{ID: 1, Number: "CN-1", CustomerID: 5, ARInvoiceID: 9, Currency: "IDR", Reason: "Damaged", Status: ARCreditNoteStatusDraft}
	repo.lines[1] = []ARCreditNoteLine{{ID: 11, ARCreditNoteID: 1, ProductID: 7, Description: "Returned part", Quantity: 2, UnitPrice: 15, Total: 30}}
	handler := NewHandler(slog.Default(), &Service{creditNotes: repo}, templates, nil, nil, rbac.Middleware{Service: arPermissionReader{permissions: []string{"finance.ar.credit_note.view"}}}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/finance/ar/credit-notes/1", nil)
	route := chi.NewRouteContext()
	route.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))

	rec := httptest.NewRecorder()
	handler.showCreditNote(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "CN-1") {
		t.Fatalf("rendered body did not include credit note number: %s", rec.Body.String())
	}
}

type creditNotePDFFake struct{}

func (creditNotePDFFake) RenderHTML(context.Context, string) ([]byte, error) {
	return []byte("pdf-bytes"), nil
}

func TestCreditNotePDFDownloadReturnsPDF(t *testing.T) {
	repo := newCreditNoteMemoryRepo()
	repo.notes[1] = &ARCreditNote{ID: 1, Number: "CN-2", CustomerID: 5, ARInvoiceID: 9, Currency: "IDR", Reason: "Damaged", Status: ARCreditNoteStatusDraft}
	repo.lines[1] = []ARCreditNoteLine{{ID: 11, ARCreditNoteID: 1, ProductID: 7, Description: "Returned part", Quantity: 2, UnitPrice: 15, Total: 30}}
	handler := NewHandler(slog.Default(), &Service{creditNotes: repo}, nil, nil, nil, rbac.Middleware{}, nil, nil)
	renderer, err := NewCreditNotePDFRenderer(creditNotePDFFake{})
	if err != nil {
		t.Fatal(err)
	}
	handler.SetCreditNotePDFRenderer(renderer)

	req := httptest.NewRequest(http.MethodGet, "/finance/ar/credit-notes/1/pdf", nil)
	route := chi.NewRouteContext()
	route.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
	rec := httptest.NewRecorder()

	handler.creditNotePDFDownload(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/pdf" {
		t.Fatalf("content-type=%q", got)
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="CN-2.pdf"` {
		t.Fatalf("content-disposition=%q", got)
	}
	if body := rec.Body.String(); body != "pdf-bytes" {
		t.Fatalf("pdf body=%q", body)
	}
}

func TestPostInvoiceDispatchesNotification(t *testing.T) {
	repo := newMemoryARRepo()
	repo.invoices[1] = &ARInvoice{ID: 1, Number: "INV-1", CustomerID: 5, Currency: "IDR", Status: ARStatusDraft}
	store := &arNotificationsStore{}
	dispatcher := notifications.NewDispatcher(notifications.NewService(store), arNotificationPrefs{channels: notifications.Channels{InApp: true, Email: false}}, nil, nil)
	handler := &Handler{service: &Service{repo: repo}, notifications: dispatcher}

	manager := shared.NewSessionManager(nil, "session", "secret", time.Hour, false)
	req := httptest.NewRequest(http.MethodPost, "/finance/ar/invoices/1/post", nil)
	sess, err := manager.Load(context.Background(), req)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	sess.SetUser("7")
	req = req.WithContext(shared.ContextWithSession(req.Context(), sess))
	route := chi.NewRouteContext()
	route.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))

	rec := httptest.NewRecorder()
	handler.postInvoice(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(store.items) != 1 {
		t.Fatalf("notifications=%d", len(store.items))
	}
	if store.items[0].Type != notifications.TypeInvoiceIssued {
		t.Fatalf("type=%s", store.items[0].Type)
	}
}

type arNotificationPrefs struct {
	channels notifications.Channels
}

func (p arNotificationPrefs) Channels(context.Context, int64, string) (notifications.Channels, error) {
	return p.channels, nil
}

func (arNotificationPrefs) UserEmail(context.Context, int64) (string, error) { return "", nil }
func (arNotificationPrefs) UserPhone(context.Context, int64) (string, error) { return "", nil }

type arNotificationsStore struct {
	items []notifications.Notification
}

func (s *arNotificationsStore) Create(_ context.Context, n notifications.Notification) (notifications.Notification, error) {
	n.ID = int64(len(s.items) + 1)
	s.items = append(s.items, n)
	return n, nil
}

func (s *arNotificationsStore) ListRecent(context.Context, int64, int) ([]notifications.Notification, error) {
	return nil, nil
}

func (s *arNotificationsStore) ListUnread(context.Context, int64, int) ([]notifications.Notification, error) {
	return nil, nil
}

func (s *arNotificationsStore) UnreadCount(context.Context, int64) (int64, error) {
	return 0, nil
}

func (s *arNotificationsStore) MarkRead(context.Context, int64, int64, time.Time) (bool, error) {
	return false, nil
}

func (s *arNotificationsStore) MarkAllRead(context.Context, int64, time.Time) (int64, error) {
	return 0, nil
}
