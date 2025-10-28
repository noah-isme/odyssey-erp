package view

import (
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/web"
)

// Engine renders HTML templates.
type Engine struct {
	templates *template.Template
}

// TemplateData contains values shared across templates.
type TemplateData struct {
	Title       string
	CSRFToken   string
	Flash       *shared.FlashMessage
	CurrentPath string
	Data        any
}

// NewEngine parses templates at build-time.
func NewEngine() (*Engine, error) {
	funcMap := template.FuncMap{
		"formatDate": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format("02 Jan 2006 15:04")
		},
	}
	tpl, err := template.New("root").Funcs(funcMap).ParseFS(web.Templates, "templates/layouts/*.html", "templates/partials/*.html", "templates/pages/*.html")
	if err != nil {
		return nil, err
	}
	return &Engine{templates: tpl}, nil
}

// Render executes a named template with TemplateData.
func (e *Engine) Render(w http.ResponseWriter, name string, data TemplateData) error {
	if e == nil {
		return fmt.Errorf("template engine not initialised")
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return e.templates.ExecuteTemplate(w, name, data)
}
