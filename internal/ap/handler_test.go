package ap

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/procurement"
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

// TestCreateAPInvoiceRunsInlineProcessorBeforeEnqueue: a created invoice is
// matched in the request with the invoice creator as actor before the retry
// job is enqueued; an inline failure still creates the invoice, still
// enqueues, and is surfaced as a warning flash.
func TestCreateAPInvoiceRunsInlineProcessorBeforeEnqueue(t *testing.T) {
	tests := []struct {
		name      string
		inline    bool
		inlineErr error
		wantCalls []string
		wantFlash string
	}{
		{name: "inline success", inline: true, wantCalls: []string{"inline", "enqueue"}, wantFlash: "success"},
		{name: "inline failure falls back to the queue", inline: true, inlineErr: ErrInvoiceProcessingBusy, wantCalls: []string{"inline", "enqueue"}, wantFlash: "warning"},
		{name: "no inline processor", wantCalls: []string{"enqueue"}, wantFlash: "success"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			apRepo := newMemoryAPRepo()
			procRepo := newStubProcRepo()
			procRepo.grns[1] = procurement.GoodsReceipt{ID: 1, SupplierID: 10, POID: 22, Status: procurement.GRNStatusPosted}
			procRepo.grnLines[1] = []procurement.GRNLine{{ID: 1, ProductID: 100, Qty: 2, UnitCost: 50}}
			procRepo.pos[22] = procurement.PurchaseOrder{ID: 22, SupplierID: 10, Status: procurement.POStatusApproved}
			procRepo.poLines[22] = []procurement.POLine{{ID: 220, ProductID: 100, Qty: 2, Price: 50}}
			handler := &Handler{logger: slog.Default(), service: NewService(apRepo, procurement.NewService(nil, procRepo, nil, nil, nil, nil, nil))}

			var calls []string
			type call struct{ invoiceID, createdBy int64 }
			var inlineCall, enqueueCall call
			if tc.inline {
				handler.SetInlineProcessor(func(_ context.Context, invoiceID, createdBy int64) error {
					calls = append(calls, "inline")
					inlineCall = call{invoiceID, createdBy}
					return tc.inlineErr
				})
			}
			handler.SetEnqueueJob(func(invoiceID, createdBy int64) error {
				calls = append(calls, "enqueue")
				enqueueCall = call{invoiceID, createdBy}
				return nil
			})

			req := apRequest(t, http.MethodPost, "/finance/ap/invoices")
			req.Body = io.NopCloser(strings.NewReader(url.Values{"number": {"CERT-AP-1"}, "source_type": {"grn"}, "source_id": {"1"}, "due_date": {"2026-12-31"}}.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec := httptest.NewRecorder()
			handler.createAPInvoice(rec, req)

			if rec.Code != http.StatusSeeOther {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if strings.Join(calls, ",") != strings.Join(tc.wantCalls, ",") {
				t.Fatalf("calls=%v want %v", calls, tc.wantCalls)
			}
			inv := apRepo.invoices[1]
			if inv.CreatedBy != 9 {
				t.Fatalf("created_by=%d want the session user 9", inv.CreatedBy)
			}
			if tc.inline && inlineCall != (call{inv.ID, inv.CreatedBy}) {
				t.Fatalf("inline call=%+v want invoice %d by creator %d", inlineCall, inv.ID, inv.CreatedBy)
			}
			if enqueueCall != (call{inv.ID, inv.CreatedBy}) {
				t.Fatalf("enqueue call=%+v want invoice %d by creator %d", enqueueCall, inv.ID, inv.CreatedBy)
			}
			flash := shared.SessionFromContext(req.Context()).PopFlash()
			if flash == nil || flash.Kind != tc.wantFlash {
				t.Fatalf("flash=%+v want kind %q", flash, tc.wantFlash)
			}
		})
	}
}
