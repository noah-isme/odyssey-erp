package shared

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestHRScopesAreWellFormedUniqueAndMigrated(t *testing.T) {
	format := regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

	files, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no migrations/*.up.sql files found")
	}
	var all strings.Builder
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		all.Write(b)
	}
	sql := all.String()

	scopes := HRScopes()
	if len(scopes) == 0 {
		t.Fatal("HRScopes is empty")
	}
	seen := map[string]bool{}
	for _, p := range scopes {
		if !format.MatchString(p) {
			t.Errorf("permission %q must be lowercase and dot-separated", p)
		}
		if seen[p] {
			t.Errorf("duplicate permission %q", p)
		}
		seen[p] = true
		if !strings.Contains(sql, "'"+p+"'") {
			t.Errorf("permission %q is not seeded by any migrations/*.up.sql", p)
		}
	}
}
