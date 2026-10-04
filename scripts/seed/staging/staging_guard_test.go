package main

import (
	"strings"
	"testing"
)

// TestCheckStagingStaticBaseSeeder covers the guard as the base seeder uses
// it: the same checks as --iso004, without a run key.
func TestCheckStagingStaticBaseSeeder(t *testing.T) {
	const dsn = "postgres://seed:secret@staging-db.internal:5432/odyssey_staging?sslmode=require"
	valid := stagingGuardInput{
		Label:          "the base seeder",
		DSN:            dsn,
		Confirm:        "odyssey_staging@staging-db.internal",
		AppEnv:         "staging",
		DenyHostRegex:  stagingDefaultDenyHosts,
		ExplicitDSNSet: true,
	}

	tests := []struct {
		name    string
		mutate  func(in *stagingGuardInput)
		wantErr string
	}{
		{name: "valid without a run key", mutate: func(*stagingGuardInput) {}},
		{name: "no explicit DSN (localhost default is not used)", mutate: func(in *stagingGuardInput) { in.ExplicitDSNSet = false }, wantErr: "the base seeder requires an explicit --dsn"},
		{name: "empty DSN", mutate: func(in *stagingGuardInput) { in.DSN = " " }, wantErr: "explicit --dsn"},
		{name: "missing APP_ENV", mutate: func(in *stagingGuardInput) { in.AppEnv = "" }, wantErr: "APP_ENV=staging"},
		{name: "APP_ENV production", mutate: func(in *stagingGuardInput) { in.AppEnv = "production" }, wantErr: "APP_ENV=staging"},
		{name: "missing confirmation", mutate: func(in *stagingGuardInput) { in.Confirm = "" }, wantErr: "is required with the base seeder"},
		{name: "confirmation without separator", mutate: func(in *stagingGuardInput) { in.Confirm = "odyssey_staging" }, wantErr: "must have the form"},
		{name: "database mismatch", mutate: func(in *stagingGuardInput) { in.Confirm = "odyssey@staging-db.internal" }, wantErr: "does not match --confirm-staging database"},
		{name: "host mismatch", mutate: func(in *stagingGuardInput) { in.Confirm = "odyssey_staging@other-db.internal" }, wantErr: "does not match --confirm-staging host"},
		{name: "denied host", mutate: func(in *stagingGuardInput) {
			in.DSN = "postgres://seed@db.prod.internal:5432/odyssey_staging"
			in.Confirm = "odyssey_staging@db.prod.internal"
		}, wantErr: "matches --deny-host-regex"},
		{name: "empty deny regex", mutate: func(in *stagingGuardInput) { in.DenyHostRegex = "" }, wantErr: "must not be empty"},
		{name: "DSN without database", mutate: func(in *stagingGuardInput) { in.DSN = "postgres://seed@staging-db.internal:5432/" }, wantErr: "with the base seeder"},
		{name: "multi-host DSN", mutate: func(in *stagingGuardInput) {
			in.DSN = "postgres://seed@staging-db.internal:5432,db.prod.internal:5432/odyssey_staging"
			in.DenyHostRegex = "^nomatch$"
		}, wantErr: "more than one host"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := valid
			tt.mutate(&in)
			target, err := checkStagingStatic(in)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected refusal: %v", err)
				}
				if target.Database != "odyssey_staging" || target.Host != "staging-db.internal" {
					t.Fatalf("target = %+v", target)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestCheckStagingStaticRunsKeyCheck(t *testing.T) {
	in := stagingGuardInput{
		Label: "x", DSN: "postgres://seed@staging-db.internal:5432/d", Confirm: "d@staging-db.internal",
		AppEnv: "staging", DenyHostRegex: stagingDefaultDenyHosts, ExplicitDSNSet: true,
		CheckKey: func() error { return errTestKey },
	}
	if _, err := checkStagingStatic(in); err != errTestKey {
		t.Fatalf("expected the key check refusal, got %v", err)
	}
}

type testKeyError struct{}

func (testKeyError) Error() string { return "key refused" }

var errTestKey error = testKeyError{}
