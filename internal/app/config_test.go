package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigRequiresExplicitProfileForStagingAndProduction(t *testing.T) {
	for _, environment := range []string{"staging", "production"} {
		t.Run(environment, func(t *testing.T) {
			t.Setenv("APP_ENV", environment)
			t.Setenv("SESSION_SECRET", "test-session-secret")
			t.Setenv("CSRF_SECRET", "test-csrf-secret")
			t.Setenv("RELEASE_PROFILE", "")

			_, err := LoadConfig()
			if err == nil || !strings.Contains(err.Error(), "RELEASE_PROFILE must be set explicitly") {
				t.Fatalf("LoadConfig() error = %v, want explicit profile error", err)
			}
		})
	}
}

func TestLoadConfigAcceptsCoreProfile(t *testing.T) {
	t.Setenv("APP_ENV", "staging")
	t.Setenv("SESSION_SECRET", "test-session-secret")
	t.Setenv("CSRF_SECRET", "test-csrf-secret")
	t.Setenv("RELEASE_PROFILE", string(ReleaseProfileV010Core))

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.ReleaseProfile != string(ReleaseProfileV010Core) {
		t.Fatalf("ReleaseProfile = %q, want %q", config.ReleaseProfile, ReleaseProfileV010Core)
	}
}

func TestLoadConfigPGMaxConns(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		set     bool
		want    int32
		wantErr bool
	}{
		{name: "unset means zero", set: false, want: 0},
		{name: "empty rejected like other numeric settings", set: true, value: "", wantErr: true},
		{name: "positive value", set: true, value: "24", want: 24},
		{name: "explicit zero", set: true, value: "0", want: 0},
		{name: "negative rejected", set: true, value: "-1", wantErr: true},
		{name: "non numeric rejected", set: true, value: "many", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SESSION_SECRET", "test-session-secret")
			t.Setenv("CSRF_SECRET", "test-csrf-secret")
			if tc.set {
				t.Setenv("PG_MAX_CONNS", tc.value)
			} else {
				// t.Setenv registers the restore; Unsetenv then leaves it unset.
				t.Setenv("PG_MAX_CONNS", "")
				if err := os.Unsetenv("PG_MAX_CONNS"); err != nil {
					t.Fatalf("unset PG_MAX_CONNS: %v", err)
				}
			}

			config, err := LoadConfig()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("LoadConfig() error = nil, want error for %q", tc.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfig() error = %v", err)
			}
			if config.PGMaxConns != tc.want {
				t.Fatalf("PGMaxConns = %d, want %d", config.PGMaxConns, tc.want)
			}
		})
	}
}

func TestLoadConfigCMMSSystemUserID(t *testing.T) {
	t.Setenv("APP_ENV", "staging")
	t.Setenv("SESSION_SECRET", "test-session-secret")
	t.Setenv("CSRF_SECRET", "test-csrf-secret")
	t.Setenv("RELEASE_PROFILE", string(ReleaseProfileV010Core))

	// t.Setenv restores the original value; Unsetenv tests the default.
	t.Setenv("CMMS_SYSTEM_USER_ID", "")
	if err := os.Unsetenv("CMMS_SYSTEM_USER_ID"); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.CMMSSystemUserID != 0 {
		t.Fatalf("CMMSSystemUserID default = %d, want 0 (PM scan disabled)", config.CMMSSystemUserID)
	}

	t.Setenv("CMMS_SYSTEM_USER_ID", "42")
	config, err = LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.CMMSSystemUserID != 42 {
		t.Fatalf("CMMSSystemUserID = %d, want 42", config.CMMSSystemUserID)
	}
}

func TestLoadConfigRBACGlobalRoles(t *testing.T) {
	t.Setenv("APP_ENV", "staging")
	t.Setenv("SESSION_SECRET", "test-session-secret")
	t.Setenv("CSRF_SECRET", "test-csrf-secret")
	t.Setenv("RELEASE_PROFILE", string(ReleaseProfileV010Core))

	t.Setenv("RBAC_GLOBAL_ROLES", "")
	if err := os.Unsetenv("RBAC_GLOBAL_ROLES"); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got := normalizeGlobalRoles(config.RBACGlobalRoles); len(got) != 0 {
		t.Fatalf("RBACGlobalRoles default = %q, want none", got)
	}

	t.Setenv("RBAC_GLOBAL_ROLES", " Group Operator ,,auditor, group operator")
	config, err = LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	got := normalizeGlobalRoles(config.RBACGlobalRoles)
	if len(got) != 2 || got[0] != "group operator" || got[1] != "auditor" {
		t.Fatalf("normalized RBACGlobalRoles = %q, want [group operator auditor]", got)
	}
}

func TestDocumentStorageDir(t *testing.T) {
	explicit := filepath.Join(t.TempDir(), "shared-storage")
	tests := []struct {
		appEnv   string
		explicit string
	}{
		{appEnv: "development"},
		{appEnv: "development", explicit: explicit},
		{appEnv: "test"},
		{appEnv: "test", explicit: explicit},
		{appEnv: "staging"},
		{appEnv: "staging", explicit: explicit},
		{appEnv: "staging", explicit: "   "},
		{appEnv: "production"},
		{appEnv: "production", explicit: explicit},
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for _, tc := range tests {
		name := tc.appEnv + "/empty"
		if strings.TrimSpace(tc.explicit) != "" {
			name = tc.appEnv + "/explicit"
		} else if tc.explicit != "" {
			name = tc.appEnv + "/blank"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("APP_ENV", tc.appEnv)
			t.Setenv("SESSION_SECRET", "test-session-secret")
			t.Setenv("CSRF_SECRET", "test-csrf-secret")
			t.Setenv("RELEASE_PROFILE", string(ReleaseProfileV010Core))
			t.Setenv("DOCUMENT_STORAGE_DIR", tc.explicit)

			cfg, err := LoadConfig()
			if err != nil {
				t.Fatalf("LoadConfig() error = %v", err)
			}
			got := DocumentStorageDir(cfg)

			if strings.TrimSpace(tc.explicit) != "" {
				if got != tc.explicit {
					t.Fatalf("DocumentStorageDir() = %q, want explicit %q", got, tc.explicit)
				}
				return
			}
			switch tc.appEnv {
			case "staging", "production":
				if !filepath.IsAbs(got) {
					t.Fatalf("DocumentStorageDir() = %q, want an absolute path in %s", got, tc.appEnv)
				}
				if rel, err := filepath.Rel(cwd, got); err == nil && !strings.HasPrefix(rel, "..") {
					t.Fatalf("DocumentStorageDir() = %q is inside the working directory %q", got, cwd)
				}
			default:
				if filepath.IsAbs(got) {
					t.Fatalf("DocumentStorageDir() = %q, want a relative development path", got)
				}
				if filepath.Clean(got) != filepath.Join("data", "storage") {
					t.Fatalf("DocumentStorageDir() = %q, want data/storage", got)
				}
			}
		})
	}
}
