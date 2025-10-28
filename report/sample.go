package report

import (
	"bytes"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/view"
	"github.com/odyssey-erp/odyssey-erp/web"
)

// Handler manages report endpoints.
type Handler struct {
	client    *Client
	logger    *slog.Logger
	templates *template.Template
}

// NewHandler creates a report handler.
func NewHandler(client *Client, logger *slog.Logger) *Handler {
	funcMap := template.FuncMap{
		"formatDate": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format("02 Jan 2006")
		},
	}
	tpl, err := template.New("reports").Funcs(funcMap).ParseFS(web.Templates, "templates/reports/*.html")
	if err != nil {
		logger.Error("parse report templates", slog.Any("error", err))
	}
	return &Handler{client: client, logger: logger, templates: tpl}
}

// MountRoutes registers report routes.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Get("/ping", h.ping)
	r.Post("/sample", h.sample)
	r.Get("/stock-card/pdf", h.stockCardPDF)
	r.Get("/grn/pdf", h.grnPDF)
}

func (h *Handler) ping(w http.ResponseWriter, r *http.Request) {
	if err := h.client.Ping(r.Context()); err != nil {
		h.logger.Warn("gotenberg ping failed", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (h *Handler) sample(w http.ResponseWriter, r *http.Request) {
	html := "" +
		"<html><head><title>Odyssey Report</title></head><body>" +
		"<h1>Odyssey ERP</h1><p>Generated at " + time.Now().Format(time.RFC1123) + "</p>" +
		"</body></html>"
	pdf, err := h.client.RenderHTML(r.Context(), html)
	if err != nil {
		h.logger.Error("render sample pdf", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=sample.pdf")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdf)
}

func (h *Handler) stockCardPDF(w http.ResponseWriter, r *http.Request) {
	if h.templates == nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	warehouseID, _ := strconv.ParseInt(r.URL.Query().Get("warehouse_id"), 10, 64)
	productID, _ := strconv.ParseInt(r.URL.Query().Get("product_id"), 10, 64)
	data := struct {
		WarehouseID int64
		ProductID   int64
		Entries     []struct {
			TxCode     string
			TxType     string
			PostedAt   time.Time
			QtyIn      float64
			QtyOut     float64
			BalanceQty float64
			UnitCost   float64
		}
	}{WarehouseID: warehouseID, ProductID: productID, Entries: nil}
	buf := &bytes.Buffer{}
	if err := h.templates.ExecuteTemplate(buf, "reports/stock_card_pdf.html", view.TemplateData{Data: data}); err != nil {
		h.logger.Error("render stock card report", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	pdf, err := h.client.RenderHTML(r.Context(), buf.String())
	if err != nil {
		h.logger.Error("render stock card pdf", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=stock-card.pdf")
	_, _ = w.Write(pdf)
}

func (h *Handler) grnPDF(w http.ResponseWriter, r *http.Request) {
	if h.templates == nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	number := r.URL.Query().Get("number")
	supplierID, _ := strconv.ParseInt(r.URL.Query().Get("supplier_id"), 10, 64)
	warehouseID, _ := strconv.ParseInt(r.URL.Query().Get("warehouse_id"), 10, 64)
	data := struct {
		Number      string
		SupplierID  int64
		WarehouseID int64
		ReceivedAt  time.Time
		Lines       []struct {
			ProductID int64
			Qty       float64
			UnitCost  float64
		}
	}{Number: number, SupplierID: supplierID, WarehouseID: warehouseID, Lines: nil}
	buf := &bytes.Buffer{}
	if err := h.templates.ExecuteTemplate(buf, "reports/grn_pdf.html", view.TemplateData{Data: data}); err != nil {
		h.logger.Error("render grn report", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	pdf, err := h.client.RenderHTML(r.Context(), buf.String())
	if err != nil {
		h.logger.Error("render grn pdf", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=grn.pdf")
	_, _ = w.Write(pdf)
}
