package shared

import "testing"

func TestPhase1To6PermissionsAreUniqueAndComplete(t *testing.T) {
	got := Phase1To6PermissionNames()
	wantLen := len(Phase1To6Permissions) + len(CMMSQMSDocumentsPermissions)
	if len(got) != wantLen {
		t.Fatalf("permission inventory has %d entries, want %d", len(got), wantLen)
	}
	seen := make(map[string]bool, len(got))
	for _, name := range got {
		if seen[name] {
			t.Fatalf("duplicate permission %q", name)
		}
		seen[name] = true
	}
}
