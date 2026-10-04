package app

import (
	"os"
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
