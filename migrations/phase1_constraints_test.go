package migrations_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPhase1MigrationsCarryReturnConstraints(t *testing.T) {
	files := []string{
		"000040_p1_ar_credit_notes_and_returns.up.sql",
		"000041_p1_b_grn_return_and_debit_notes.up.sql",
	}
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(".", file))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		content := string(data)
		for _, want := range []string{
			"CHECK (quantity_returned > 0)",
			"CREATE UNIQUE INDEX",
			"enforce_",
			"status ",
		} {
			if !strings.Contains(content, want) {
				t.Fatalf("%s does not contain %q", file, want)
			}
		}
	}
}
