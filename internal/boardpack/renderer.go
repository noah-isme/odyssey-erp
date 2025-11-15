package boardpack

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"math"
	"time"

	"github.com/odyssey-erp/odyssey-erp/web"
)

// PDFClient exposes the subset of the report client used by the renderer.
type PDFClient interface {
	RenderHTML(ctx context.Context, html string) ([]byte, error)
}

// Renderer transforms DocumentData into PDF artefacts via html/template + PDF conversion.
type Renderer struct {
	tpl    *template.Template
	client PDFClient
}

// NewRenderer parses the board pack PDF template and wires the PDF client.
func NewRenderer(client PDFClient) (*Renderer, error) {
	if client == nil {
		return nil, fmt.Errorf("boardpack renderer: pdf client required")
	}
	funcMap := template.FuncMap{
		"formatDate": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format("02 Jan 2006")
		},
		"formatDecimal": func(v float64) string {
			return fmt.Sprintf("%0.2f", v)
		},
		"formatPercent": func(v float64) string {
			return fmt.Sprintf("%0.2f%%", v)
		},
		"abs": math.Abs,
	}
	tpl, err := template.New("boardpack_standard.html").Funcs(funcMap).ParseFS(web.Templates, "templates/reports/boardpack_standard.html")
	if err != nil {
		return nil, err
	}
	return &Renderer{tpl: tpl, client: client}, nil
}

// Render executes the template and converts the HTML to PDF bytes.
func (r *Renderer) Render(ctx context.Context, data DocumentData) (RenderResult, error) {
	if r == nil || r.tpl == nil || r.client == nil {
		return RenderResult{}, fmt.Errorf("boardpack renderer not initialised")
	}
	buf := &bytes.Buffer{}
	if err := r.tpl.Execute(buf, data); err != nil {
		return RenderResult{}, err
	}
	pdf, err := r.client.RenderHTML(ctx, buf.String())
	if err != nil {
		return RenderResult{}, err
	}
	return RenderResult{HTML: buf.String(), PDF: pdf, Length: int64(len(pdf))}, nil
}
