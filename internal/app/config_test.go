package app

import (
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

func TestLoadConfigAcceptsFinanceProfile(t *testing.T) {
	t.Setenv("APP_ENV", "staging")
	t.Setenv("SESSION_SECRET", "test-session-secret")
	t.Setenv("CSRF_SECRET", "test-csrf-secret")
	t.Setenv("RELEASE_PROFILE", string(ReleaseProfileV011Finance))

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.ReleaseProfile != string(ReleaseProfileV011Finance) {
		t.Fatalf("ReleaseProfile = %q, want %q", config.ReleaseProfile, ReleaseProfileV011Finance)
	}
}
