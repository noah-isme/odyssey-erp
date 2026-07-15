package view_test

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/web"
)

var (
	buttonTag   = regexp.MustCompile(`(?is)<button\b[^>]*>`)
	buttonBlock = regexp.MustCompile(`(?is)<button\b([^>]*)>(.*?)</button>`)
	htmlTag     = regexp.MustCompile(`(?is)<[^>]+>`)
	tableBlock  = regexp.MustCompile(`(?is)<table\b([^>]*)>(.*?)</table>`)
	inputTag    = regexp.MustCompile(`(?is)<input\b[^>]*>`)
	selectTag   = regexp.MustCompile(`(?is)<select\b[^>]*>`)
	textareaTag = regexp.MustCompile(`(?is)<textarea\b[^>]*>`)
)

func TestBusinessTemplatesUseCanonicalUIContracts(t *testing.T) {
	err := fs.WalkDir(web.Templates, "templates", func(path string, entry fs.DirEntry, walkErr error) error {
		require.NoError(t, walkErr)
		if entry.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}

		source, readErr := fs.ReadFile(web.Templates, path)
		require.NoError(t, readErr)
		html := string(source)

		for _, tag := range buttonTag.FindAllString(html, -1) {
			require.Truef(t, hasClass(tag, "btn"), "%s contains a non-canonical button: %s", path, tag)
		}
		require.NotContainsf(t, strings.ToLower(html), `role="button"`, "%s uses role=button instead of .btn", path)

		tables := tableBlock.FindAllStringSubmatch(html, -1)
		for _, table := range tables {
			require.Truef(t, hasClass(table[1], "table"), "%s contains a table without .table", path)
		}
		isReport := strings.Contains(path, "/reports/")
		if len(tables) > 0 && !isReport {
			require.Truef(t, containsAny(html, "table-responsive", "table-wrap", "responsive-table", "overflow-x-auto"), "%s contains a table without a responsive wrapper", path)
		}

		if !isReport && !strings.HasSuffix(path, "pages/login.html") {
			for _, tag := range inputTag.FindAllString(html, -1) {
				lowerTag := strings.ToLower(tag)
				if strings.Contains(lowerTag, `type="hidden"`) {
					continue
				}
				if strings.Contains(lowerTag, `type="checkbox"`) || strings.Contains(lowerTag, `type="radio"`) {
					require.Truef(t, hasAnyClass(tag, "check__input", "checkbox-input"), "%s contains a non-canonical choice control: %s", path, tag)
					continue
				}
				require.Truef(t, hasClass(tag, "form-input"), "%s contains an input without .form-input: %s", path, tag)
			}
			for _, tag := range selectTag.FindAllString(html, -1) {
				require.Truef(t, hasClass(tag, "form-select"), "%s contains a select without .form-select: %s", path, tag)
			}
			for _, tag := range textareaTag.FindAllString(html, -1) {
				require.Truef(t, hasClass(tag, "form-textarea"), "%s contains a textarea without .form-textarea: %s", path, tag)
			}
		}
		return nil
	})
	require.NoError(t, err)
}

func TestMigratedModulesRejectLegacyMarkup(t *testing.T) {
	migratedPaths := []string{
		"/pages/sales/", "/pages/delivery/", "/pages/inventory/", "/pages/accounting/",
		"/pages/finance/", "/pages/masterdata/", "/pages/ap/", "/pages/ar/",
		"/pages/close/", "/pages/eliminations/", "/pages/variance/",
	}

	err := fs.WalkDir(web.Templates, "templates", func(path string, entry fs.DirEntry, walkErr error) error {
		require.NoError(t, walkErr)
		if entry.IsDir() || !strings.HasSuffix(path, ".html") || !containsAny(path, migratedPaths...) {
			return nil
		}
		source, readErr := fs.ReadFile(web.Templates, path)
		require.NoError(t, readErr)
		html := strings.ToLower(string(source))
		require.NotContainsf(t, html, `style="`, "%s contains inline styling", path)
		require.NotContainsf(t, html, "<script", "%s contains an inline script", path)
		require.NotRegexpf(t, regexp.MustCompile(`(?:^|\s)(?:[a-z]+:)?grid-cols-(?:2|3|4)(?:\s|$)`), html, "%s contains a fixed grid utility", path)
		return nil
	})
	require.NoError(t, err)
}

func TestIconOnlyButtonsHaveAccessibleLabels(t *testing.T) {
	err := fs.WalkDir(web.Templates, "templates", func(path string, entry fs.DirEntry, walkErr error) error {
		require.NoError(t, walkErr)
		if entry.IsDir() || !strings.HasSuffix(path, ".html") || strings.HasSuffix(path, "pages/landing.html") || strings.HasSuffix(path, "pages/login.html") {
			return nil
		}
		source, readErr := fs.ReadFile(web.Templates, path)
		require.NoError(t, readErr)
		for _, button := range buttonBlock.FindAllStringSubmatch(string(source), -1) {
			visibleText := strings.TrimSpace(htmlTag.ReplaceAllString(button[2], ""))
			if visibleText == "" {
				require.Containsf(t, strings.ToLower(button[1]), "aria-label=", "%s contains an icon-only button without aria-label", path)
			}
		}
		return nil
	})
	require.NoError(t, err)
}

func TestFocusVisibleFallbackIsPreserved(t *testing.T) {
	styles, err := fs.ReadFile(web.Static, "static/css/core/utilities.css")
	require.NoError(t, err)
	css := strings.ToLower(string(styles))
	require.Contains(t, css, ":focus-visible")
	require.NotRegexp(t, regexp.MustCompile(`(?s):focus-visible\s*\{[^}]*outline\s*:\s*none`), css)
}

func TestReportTablesHaveAccessibleNames(t *testing.T) {
	reportFragments := []string{"/reports/", "aging_report", "customer_statement", "/finance/consol_", "/finance/budget", "/finance/cashflow", "/finance/insights", "/finance/audit_timeline", "/partials/finance/"}
	err := fs.WalkDir(web.Templates, "templates", func(path string, entry fs.DirEntry, walkErr error) error {
		require.NoError(t, walkErr)
		if entry.IsDir() || !strings.HasSuffix(path, ".html") || !containsAny(path, reportFragments...) {
			return nil
		}
		source, readErr := fs.ReadFile(web.Templates, path)
		require.NoError(t, readErr)
		for _, table := range tableBlock.FindAllStringSubmatch(string(source), -1) {
			named := strings.Contains(strings.ToLower(table[1]), "aria-labelledby=") || strings.Contains(strings.ToLower(table[2]), "<caption")
			require.Truef(t, named, "%s contains a report table without caption or aria-labelledby", path)
		}
		return nil
	})
	require.NoError(t, err)
}

func hasAnyClass(tag string, classes ...string) bool {
	for _, class := range classes {
		if hasClass(tag, class) {
			return true
		}
	}
	return false
}

func hasClass(tag, expected string) bool {
	classAttr := regexp.MustCompile(`(?is)class\s*=\s*["']([^"']*)["']`).FindStringSubmatch(tag)
	if len(classAttr) != 2 {
		return false
	}
	for _, class := range strings.Fields(classAttr[1]) {
		if class == expected {
			return true
		}
	}
	return false
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}
