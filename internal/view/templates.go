package view

import (
	"bytes"
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

// stringify renders any value as a string, including named string types whose
// underlying kind is string.
func stringify(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

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
	Lang        string
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
		"formatCurrency": func(currency string, v any) string {
			var val float64
			if n, ok := v.(pgtype.Numeric); ok {
				f, _ := n.Float64Value()
				val = f.Float64
			} else if f, ok := v.(float64); ok {
				val = f
			} else if i, ok := v.(int64); ok {
				val = float64(i)
			} else if i, ok := v.(int); ok {
				val = float64(i)
			}
			return printer.Sprintf("%s %.2f", currency, val)
		},
		"toFloat": func(v any) float64 {
			if n, ok := v.(pgtype.Numeric); ok {
				f, _ := n.Float64Value()
				return f.Float64
			}
			if f, ok := v.(float64); ok {
				return f
			}
			return 0
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
		"formatDatePtr": func(v any) string {
			var t time.Time
			if tv, ok := v.(*time.Time); ok && tv != nil {
				t = *tv
			} else if tv, ok := v.(time.Time); ok {
				t = tv
			}
			if t.IsZero() {
				return ""
			}
			return t.Format("02 Jan 2006")
		},
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
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
		// lower/upper take any value rather than string: Go templates do not
		// convert a named string type (ap.APInvoiceStatus, orders.Status, ...)
		// to its underlying string when binding a func(string) parameter, so a
		// typed status passed to these helpers fails at execution time.
		"lower": func(v any) string { return strings.ToLower(stringify(v)) },
		"upper": func(v any) string { return strings.ToUpper(stringify(v)) },
		"default": func(val, def string) string {
			if strings.TrimSpace(val) == "" {
				return def
			}
			return val
		},
		"isActive": func(currentPath, href string) bool {
			if currentPath == href {
				return true
			}
			if href != "/" && strings.HasPrefix(currentPath, href) {
				return true
			}
			return false
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

// Render executes a named template with TemplateData, responding with 200.
func (e *Engine) Render(w http.ResponseWriter, name string, data TemplateData) error {
	return e.RenderStatus(w, name, data, http.StatusOK)
}

// RenderStatus executes a named template and responds with the given status.
//
// The body is rendered into a buffer before anything is written, so callers
// must not set the status themselves: doing so commits the response on the
// first write and a template failure part-way through would then be served as
// a success with truncated or empty HTML.
//
// When this returns a non-nil error, nothing has been written to w and the
// caller is free to send an error response instead.
func (e *Engine) RenderStatus(w http.ResponseWriter, name string, data TemplateData, status int) error {
	if e == nil {
		return fmt.Errorf("template engine not initialised")
	}
	tpl, ok := e.templates[name]
	if !ok {
		return fmt.Errorf("template %s not found", name)
	}
	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, name, data); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	// The response is committed at this point. A write failure here means the
	// client went away, which no caller can act on, so it is not reported.
	_, _ = buf.WriteTo(w)
	return nil
}
