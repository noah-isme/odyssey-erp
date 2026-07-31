package ap

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type apPermissionReader struct{ permissions []string }

func (p apPermissionReader) EffectivePermissions(context.Context, int64) ([]string, error) {
	return p.permissions, nil
}

func apRequest(t *testing.T, method, path string) *http.Request {
	t.Helper()
	manager := shared.NewSessionManager(nil, "session", "secret", time.Hour, false)
	req := httptest.NewRequest(method, path, strings.NewReader(""))
	session, err := manager.Load(req.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	session.SetUser("9")
	return req.WithContext(shared.ContextWithSession(req.Context(), session))
}

func TestDebitNoteRouteRequiresPermission(t *testing.T) {
	router := chi.NewRouter()
	handler := NewHandler(slog.Default(), &Service{}, nil, nil, nil, rbac.Middleware{Service: apPermissionReader{}})
	router.Route("/finance/ap", handler.MountRoutes)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, apRequest(t, http.MethodGet, "/finance/ap/debit-notes/1"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestShowDebitNoteRendersSSR(t *testing.T) {
	templates, err := view.NewEngine()
	if err != nil {
		t.Fatal(err)
	}
	repo := newMemoryAPRepo()
	repo.debitNotes[1] = &APDebitNote{ID: 1, Number: "DN-1", SupplierID: 10, APInvoiceID: 99, Currency: "IDR", Reason: "Damaged", Status: APDebitNoteStatusDraft}
	repo.debitLines[1] = []APDebitNoteLine{{ID: 11, APDebitNoteID: 1, ProductID: 7, Description: "Returned part", Quantity: 2, UnitPrice: 15, Total: 30}}
	handler := NewHandler(slog.Default(), NewService(repo, nil), templates, nil, nil, rbac.Middleware{Service: apPermissionReader{permissions: []string{"finance.ap.view"}}})

	req := httptest.NewRequest(http.MethodGet, "/finance/ap/debit-notes/1", nil)
	route := chi.NewRouteContext()
	route.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))

	rec := httptest.NewRecorder()
	handler.showDebitNote(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "DN-1") {
		t.Fatalf("rendered body did not include debit note number: %s", rec.Body.String())
	}
}

type debitNotePDFFake struct{}

func (f debitNotePDFFake) RenderHTML(context.Context, string) ([]byte, error) {
	return []byte("pdf-bytes"), nil
}

func TestDebitNotePDFDownloadReturnsPDF(t *testing.T) {
	repo := newMemoryAPRepo()
	repo.debitNotes[1] = &APDebitNote{ID: 1, Number: "DN-2", SupplierID: 10, APInvoiceID: 99, Currency: "IDR", Reason: "Damaged", Status: APDebitNoteStatusDraft}
	repo.debitLines[1] = []APDebitNoteLine{{ID: 11, APDebitNoteID: 1, ProductID: 7, Description: "Returned part", Quantity: 2, UnitPrice: 15, Total: 30}}
	handler := NewHandler(slog.Default(), NewService(repo, nil), nil, nil, nil, rbac.Middleware{})
	renderer, err := NewDebitNotePDFRenderer(debitNotePDFFake{})
	if err != nil {
		t.Fatal(err)
	}
	handler.SetDebitNotePDFRenderer(renderer)

	req := httptest.NewRequest(http.MethodGet, "/finance/ap/debit-notes/1/pdf", nil)
	route := chi.NewRouteContext()
	route.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
	rec := httptest.NewRecorder()

	handler.debitNotePDFDownload(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/pdf" {
		t.Fatalf("content-type=%q", got)
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="DN-2.pdf"` {
		t.Fatalf("content-disposition=%q", got)
	}
	if body := rec.Body.String(); body != "pdf-bytes" {
		t.Fatalf("pdf body=%q", body)
	}
}
