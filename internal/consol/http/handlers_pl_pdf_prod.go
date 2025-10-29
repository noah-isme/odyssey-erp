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

type plPDFRenderer struct {
	logger    *slog.Logger
	client    PDFRenderClient
	templates *template.Template
}

func newPLPDFExporter(logger *slog.Logger, client PDFRenderClient) (plPDFExporter, error) {
	if client == nil {
		return nil, nil
	}
	funcMap := template.FuncMap{
		"formatDecimal": func(v float64) string {
			return fmt.Sprintf("%.2f", v)
		},
	}
	tpl, err := template.New("consol_pl_pdf.html").Funcs(funcMap).ParseFS(web.Templates, "templates/reports/finance/consol_pl_pdf.html")
	if err != nil {
		return nil, err
	}
	return &plPDFRenderer{logger: logger, client: client, templates: tpl}, nil
}

func (p *plPDFRenderer) Ready() bool {
	return p.client != nil && p.templates != nil
}

func (p *plPDFRenderer) Serve(w http.ResponseWriter, r *http.Request, h *ProfitLossHandler) {
	filters, errors := h.parseFilters(r)
	if len(errors) > 0 {
		http.Error(w, strings.Join(mapValues(errors), "; "), http.StatusBadRequest)
		return
	}
	report, warnings, err := h.service.Build(r.Context(), filters)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	vm := NewConsolPLViewModel(report, warnings)
	buf := &bytes.Buffer{}
	if err := p.templates.ExecuteTemplate(buf, "consol_pl_pdf.html", vm); err != nil {
		p.logger.Error("render consol pl pdf", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	pdf, err := p.client.RenderHTML(r.Context(), buf.String())
	if err != nil {
		p.logger.Error("generate consol pl pdf", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	filename := fmt.Sprintf("consol_pl-%d-%s.pdf", report.Filters.GroupID, report.Filters.Period)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	_, _ = w.Write(pdf)
}
