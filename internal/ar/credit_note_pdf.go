package ar

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"

	"github.com/odyssey-erp/odyssey-erp/web"
)

type CreditNotePDFClient interface {
	RenderHTML(ctx context.Context, html string) ([]byte, error)
}

type CreditNotePDFRenderer interface {
	Render(ctx context.Context, note ARCreditNoteWithDetails) ([]byte, error)
}

type creditNotePDFRenderer struct {
	template *template.Template
	client   CreditNotePDFClient
}

func NewCreditNotePDFRenderer(client CreditNotePDFClient) (CreditNotePDFRenderer, error) {
	if client == nil {
		return nil, errors.New("ar: PDF client required")
	}
	tpl, err := template.ParseFS(web.Templates, "templates/reports/ar_credit_note_pdf.html")
	if err != nil {
		return nil, err
	}
	return &creditNotePDFRenderer{template: tpl, client: client}, nil
}

func (r *creditNotePDFRenderer) Render(ctx context.Context, note ARCreditNoteWithDetails) ([]byte, error) {
	var html bytes.Buffer
	if err := r.template.ExecuteTemplate(&html, "ar_credit_note_pdf.html", note); err != nil {
		return nil, fmt.Errorf("ar: render credit note HTML: %w", err)
	}
	return r.client.RenderHTML(ctx, html.String())
}

func (h *Handler) SetCreditNotePDFRenderer(renderer CreditNotePDFRenderer) {
	h.creditNotePDF = renderer
}
