package view

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
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
		"formatDate": func(v any) string {
			var t time.Time
			if tv, ok := v.(time.Time); ok {
				t = tv
			} else if ts, ok := v.(pgtype.Timestamptz); ok {
				if ts.Valid {
					t = ts.Time
				}
			}
			if t.IsZero() {
				return ""
			}
			return t.Format("02 Jan 2006 15:04")
		},
		"formatDecimal": func(v any) string {
			if n, ok := v.(pgtype.Numeric); ok {
				f, _ := n.Float64Value()
				return printer.Sprintf("%.2f", f.Float64)
			}
			return printer.Sprintf("%.2f", v)
		},
		"formatCurrency": func(v any, currency string) string {
			var val float64
			if n, ok := v.(pgtype.Numeric); ok {
				f, _ := n.Float64Value()
				val = f.Float64
			} else if f, ok := v.(float64); ok {
				val = f
			}
			return printer.Sprintf("%s %.2f", currency, val)
		},
		"formatUUID": func(u any) string {
			if uid, ok := u.(pgtype.UUID); ok {
				if !uid.Valid {
					return ""
				}
				return uuid.UUID(uid.Bytes).String()
			}
			return fmt.Sprintf("%v", u)
		},
		"formatDateInput": func(v any) string {
			var t time.Time
			if tv, ok := v.(time.Time); ok {
				t = tv
			} else if ts, ok := v.(pgtype.Timestamptz); ok {
				if ts.Valid {
					t = ts.Time
				}
			}
			if t.IsZero() {
				return ""
			}
			return t.Format("2006-01-02")
		},
		"isNegative": func(v any) bool {
			if n, ok := v.(pgtype.Numeric); ok {
				f, _ := n.Float64Value()
				return f.Float64 < 0
			}
			if f, ok := v.(float64); ok {
				return f < 0
			}
			if i, ok := v.(int64); ok {
				return i < 0
			}
			return false
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
		"subf": func(a, b float64) float64 {
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
		"mulf": func(a, b float64) float64 {
			return a * b
		},
		"div": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"divf": func(a, b float64) float64 {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"lower": strings.ToLower,
		"upper": strings.ToUpper,
		"default": func(val, def string) string {
			if strings.TrimSpace(val) == "" {
				return def
			}
			return val
		},
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
		"templates/pages/*/*/*.html",
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
