//go:build production || pdf

package http

import (
	"bytes"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"github.com/odyssey-erp/odyssey-erp/web"
)

type productionPDFExporter struct {
	logger    *slog.Logger
	client    PDFRenderClient
	templates *template.Template
}

func newPDFExporter(logger *slog.Logger, client PDFRenderClient) (pdfExporter, error) {
	if client == nil {
		return nil, nil
	}
	funcMap := template.FuncMap{
		"formatDecimal": func(v float64) string {
			return fmt.Sprintf("%.2f", v)
		},
	}
	tpl, err := template.New("consol_tb_pdf.html").Funcs(funcMap).ParseFS(web.Templates, "templates/reports/finance/consol_tb_pdf.html")
	if err != nil {
		return nil, err
	}
	return &productionPDFExporter{logger: logger, client: client, templates: tpl}, nil
}

func (p *productionPDFExporter) Ready() bool {
	return p.client != nil && p.templates != nil
}

func (p *productionPDFExporter) Serve(w http.ResponseWriter, r *http.Request, h *Handler) {
	filter, errors := h.parseFilters(r)
	if len(errors) > 0 {
		http.Error(w, strings.Join(mapValues(errors), "; "), http.StatusBadRequest)
		return
	}
	tb, err := h.service.GetConsolidatedTB(r.Context(), filter)
	if err != nil {
		p.logger.Error("get consol tb pdf", slog.Any("error", err))
		http.Error(w, "Failed to generate report", http.StatusBadRequest)
		return
	}
	vm := FromDomain(tb)
	buf := &bytes.Buffer{}
	if err := p.templates.ExecuteTemplate(buf, "consol_tb_pdf.html", vm); err != nil {
		p.logger.Error("render consol tb pdf", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	pdf, err := p.client.RenderHTML(r.Context(), buf.String())
	if err != nil {
		p.logger.Error("generate consol tb pdf", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=consolidated_tb.pdf")
	_, _ = w.Write(pdf)
}
