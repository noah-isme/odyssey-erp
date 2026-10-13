package migrations_test

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

func TestMakefileMigrateTargetsDependOnGuard(t *testing.T) {
	data, err := os.ReadFile("../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")
	for _, target := range []string{"migrate-up", "migrate-down", "test-migrate"} {
		re := regexp.MustCompile(`^` + regexp.QuoteMeta(target) + `:(.*)$`)
		found := false
		for _, line := range lines {
			m := re.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			found = true
			if !strings.Contains(" "+m[1]+" ", " migrate-guard ") {
				t.Errorf("%s: line %q must depend on migrate-guard", target, line)
			}
		}
		if !found {
			t.Errorf("Makefile has no %s: line", target)
		}
	}
	for _, target := range []string{"migrate-up-remote", "migrate-down-remote"} {
		for _, line := range lines {
			if strings.HasPrefix(line, target+":") && strings.Contains(line, "migrate-guard") {
				t.Errorf("%s must stay unguarded: %q", target, line)
			}
		}
	}
}

// TestMigrateGuardRecipe runs the real recipe (it only inspects PG_DSN; no
// migration or database connection happens).
func TestMigrateGuardRecipe(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make is not installed")
	}
	tests := []struct {
		dsn  string
		pass bool
	}{
		{"postgres://u:p@localhost:5434/db?sslmode=disable", true},
		{"postgres://u:p@127.0.0.1/db", true},
		{"postgres://u:p@[::1]:5432/db", true},
		{"postgres://u:p@prod.example.com:5432/db", false},
		{"postgres://u:p@localhost.evil.example/db", false},
		{"postgres://localhost:x@prod.example.com/db", false},
		{"postgres://u:p@localhost,prod.example.com/db", false},
		// libpq/pq: query parameters override the URL host.
		{"postgres://u:p@localhost/db?host=prod.example.com", false},
		{"postgres://u:p@localhost/db?sslmode=disable&host=prod.example.com", false},
		{"postgres://u:p@localhost/db?HOST=prod.example.com", false},
		{"postgres://u:p@localhost/db?hostaddr=10.0.0.5", false},
		{"postgres://u:p@localhost/db?service=prod", false},
		{"host=prod.example.com dbname=odyssey user=u", false},
		{"", false},
	}
	for _, tc := range tests {
		cmd := exec.Command("make", "-s", "-C", "..", "migrate-guard")
		cmd.Env = append(os.Environ(), "PG_DSN="+tc.dsn)
		out, err := cmd.CombinedOutput()
		if tc.pass && err != nil {
			t.Errorf("PG_DSN=%q should pass the guard: %v\n%s", tc.dsn, err, out)
		}
		if !tc.pass && err == nil {
			t.Errorf("PG_DSN=%q must be refused by the guard", tc.dsn)
		}
	}
}
