package ap

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/web"
)

type DebitNotePDFClient interface {
	RenderHTML(ctx context.Context, html string) ([]byte, error)
}

type DebitNotePDFRenderer interface {
	Render(ctx context.Context, note APDebitNoteWithDetails) ([]byte, error)
}

type debitNotePDFRenderer struct {
	template *template.Template
	client   DebitNotePDFClient
}

func NewDebitNotePDFRenderer(client DebitNotePDFClient) (DebitNotePDFRenderer, error) {
	if client == nil {
		return nil, errors.New("ap: PDF client required")
	}
	tpl, err := template.ParseFS(web.Templates, "templates/reports/ap_debit_note_pdf.html")
	if err != nil {
		return nil, err
	}
	return &debitNotePDFRenderer{template: tpl, client: client}, nil
}

func (r *debitNotePDFRenderer) Render(ctx context.Context, note APDebitNoteWithDetails) ([]byte, error) {
	var html bytes.Buffer
	if err := r.template.ExecuteTemplate(&html, "ap_debit_note_pdf.html", note); err != nil {
		return nil, fmt.Errorf("ap: render debit note HTML: %w", err)
	}
	return r.client.RenderHTML(ctx, html.String())
}

func (h *Handler) SetDebitNotePDFRenderer(renderer DebitNotePDFRenderer) { h.debitNotePDF = renderer }

func (h *Handler) debitNotePDFDownload(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid debit note ID", http.StatusBadRequest)
		return
	}
	if h.debitNotePDF == nil {
		http.Error(w, "PDF renderer unavailable", http.StatusServiceUnavailable)
		return
	}
	note, err := h.service.GetAPDebitNoteWithDetails(r.Context(), id)
	if err != nil {
		http.Error(w, shared.UserSafeMessage(err), http.StatusNotFound)
		return
	}
	pdf, err := h.debitNotePDF.Render(r.Context(), *note)
	if err != nil {
		http.Error(w, "Failed to render PDF", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.pdf"`, note.Number))
	_, _ = w.Write(pdf)
}
