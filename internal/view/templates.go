package view

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/web"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// Engine renders HTML templates.
type Engine struct {
	templates map[string]*template.Template
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
	printer := message.NewPrinter(language.Indonesian)
	funcMap := template.FuncMap{
		"formatDate": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format("02 Jan 2006 15:04")
		},
		"formatDecimal": func(v float64) string {
			return printer.Sprintf("%.2f", v)
		},
		"now": func() time.Time {
			return time.Now()
		},
		"countByStatus": func(items interface{}, status string) int {
			count := 0
			if items == nil {
				return count
			}
			// Use reflection to handle different types
			return count
		},
		"sub": func(a, b int) int {
			return a - b
		},
		"add": func(a, b int) int {
			return a + b
		},
		"addf": func(a, b float64) float64 {
			return a + b
		},
		"mul": func(a, b int) int {
			return a * b
		},
		"div": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"lower": strings.ToLower,
		"upper": strings.ToUpper,
	}

	base, err := template.New("root").Funcs(funcMap).ParseFS(web.Templates,
		"templates/layouts/*.html",
		"templates/partials/*.html",
		"templates/partials/*/*.html",
	)
	if err != nil {
		return nil, err
	}

	patterns := []string{
		"templates/pages/*.html",
		"templates/pages/*/*.html",
	}

	templates := make(map[string]*template.Template)
	for _, pattern := range patterns {
		matches, err := fs.Glob(web.Templates, pattern)
		if err != nil {
			return nil, err
		}
		sort.Strings(matches)
		for _, match := range matches {
			clone, err := base.Clone()
			if err != nil {
				return nil, err
			}
			if _, err := clone.ParseFS(web.Templates, match); err != nil {
				return nil, err
			}
			name := strings.TrimPrefix(match, "templates/")
			templates[name] = clone
		}
	}

	if len(templates) == 0 {
		return nil, fmt.Errorf("no page templates found")
	}

	return &Engine{templates: templates}, nil
}

// Render executes a named template with TemplateData.
func (e *Engine) Render(w http.ResponseWriter, name string, data TemplateData) error {
	if e == nil {
		return fmt.Errorf("template engine not initialised")
	}
	tpl, ok := e.templates[name]
	if !ok {
		return fmt.Errorf("template %s not found", name)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return tpl.ExecuteTemplate(w, name, data)
}
