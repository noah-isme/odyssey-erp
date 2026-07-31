package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestPhase6RBACRepairMigrationIsCaseInsensitiveAndIdempotent(t *testing.T) {
	sql, err := os.ReadFile("000052_phase6_rbac_permissions.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	contents := strings.ToLower(string(sql))
	for _, fragment := range []string{
		"lower(trim(r.name)) in ('admin', 'administrator')",
		"on conflict (name) do update",
		"on conflict do nothing",
		"role_permissions",
	} {
		if !strings.Contains(contents, fragment) {
			t.Errorf("repair migration missing %q", fragment)
		}
	}
}
