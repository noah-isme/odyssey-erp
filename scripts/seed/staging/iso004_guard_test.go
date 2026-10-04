package main

import (
	"strings"
	"testing"
)

func TestCheckISO004Static(t *testing.T) {
	const dsn = "postgres://seed:secret@staging-db.internal:5432/odyssey_staging?sslmode=require"
	valid := iso004GuardInput{
		DSN:            dsn,
		Confirm:        "odyssey_staging@staging-db.internal",
		AppEnv:         "staging",
		DenyHostRegex:  iso004DefaultDenyHosts,
		Key:            "18123456789",
		ExplicitDSNSet: true,
	}

	tests := []struct {
		name    string
		mutate  func(in *iso004GuardInput)
		wantErr string
	}{
		{name: "valid", mutate: func(*iso004GuardInput) {}},
		{name: "host comparison is case-insensitive", mutate: func(in *iso004GuardInput) { in.Confirm = "odyssey_staging@STAGING-DB.internal" }},
		{name: "missing APP_ENV", mutate: func(in *iso004GuardInput) { in.AppEnv = "" }, wantErr: "APP_ENV=staging"},
		{name: "APP_ENV production", mutate: func(in *iso004GuardInput) { in.AppEnv = "production" }, wantErr: "APP_ENV=staging"},
		{name: "APP_ENV wrong case", mutate: func(in *iso004GuardInput) { in.AppEnv = "Staging" }, wantErr: "APP_ENV=staging"},
		{name: "database name mismatch", mutate: func(in *iso004GuardInput) { in.Confirm = "odyssey@staging-db.internal" }, wantErr: "does not match --confirm-staging database"},
		{name: "host mismatch", mutate: func(in *iso004GuardInput) { in.Confirm = "odyssey_staging@other-db.internal" }, wantErr: "does not match --confirm-staging host"},
		{name: "host alias is not accepted", mutate: func(in *iso004GuardInput) {
			in.DSN = "postgres://seed@127.0.0.1:5432/odyssey_staging"
			in.Confirm = "odyssey_staging@localhost"
		}, wantErr: "does not match --confirm-staging host"},
		{name: "denied host default regex", mutate: func(in *iso004GuardInput) {
			in.DSN = "postgres://seed@db.prod.internal:5432/odyssey_staging"
			in.Confirm = "odyssey_staging@db.prod.internal"
		}, wantErr: "matches --deny-host-regex"},
		{name: "denied host default regex is case-insensitive", mutate: func(in *iso004GuardInput) {
			in.DSN = "postgres://seed@ODYSSEY-PROD-1:5432/odyssey_staging"
			in.Confirm = "odyssey_staging@ODYSSEY-PROD-1"
		}, wantErr: "matches --deny-host-regex"},
		{name: "denied host custom regex", mutate: func(in *iso004GuardInput) { in.DenyHostRegex = `^staging-db\.` }, wantErr: "matches --deny-host-regex"},
		{name: "empty deny regex", mutate: func(in *iso004GuardInput) { in.DenyHostRegex = " " }, wantErr: "must not be empty"},
		{name: "invalid deny regex", mutate: func(in *iso004GuardInput) { in.DenyHostRegex = "(" }, wantErr: "compile --deny-host-regex"},
		{name: "missing confirmation", mutate: func(in *iso004GuardInput) { in.Confirm = "" }, wantErr: "--confirm-staging <db-name>@<db-host> is required"},
		{name: "confirmation without host", mutate: func(in *iso004GuardInput) { in.Confirm = "odyssey_staging@" }, wantErr: "must have the form"},
		{name: "confirmation without database", mutate: func(in *iso004GuardInput) { in.Confirm = "@staging-db.internal" }, wantErr: "must have the form"},
		{name: "confirmation without separator", mutate: func(in *iso004GuardInput) { in.Confirm = "odyssey_staging" }, wantErr: "must have the form"},
		{name: "missing key", mutate: func(in *iso004GuardInput) { in.Key = "" }, wantErr: "--iso004-key"},
		{name: "key with LIKE wildcard", mutate: func(in *iso004GuardInput) { in.Key = "run_1" }, wantErr: "--iso004-key"},
		{name: "key with percent", mutate: func(in *iso004GuardInput) { in.Key = "run%1" }, wantErr: "--iso004-key"},
		{name: "no explicit DSN", mutate: func(in *iso004GuardInput) { in.ExplicitDSNSet = false }, wantErr: "explicit --dsn"},
		{name: "DSN without database", mutate: func(in *iso004GuardInput) { in.DSN = "postgres://seed@staging-db.internal:5432/" }, wantErr: "name the database explicitly"},
		{name: "multi-host DSN", mutate: func(in *iso004GuardInput) {
			in.DSN = "postgres://seed@staging-db.internal:5432,db.prod.internal:5432/odyssey_staging"
			in.DenyHostRegex = "^nomatch$"
		}, wantErr: "more than one host"},
		{name: "unparseable DSN", mutate: func(in *iso004GuardInput) { in.DSN = "postgres://%zz" }, wantErr: "parse DSN"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := valid
			tc.mutate(&in)
			target, err := checkISO004Static(in)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if target.Confirm.Database != "odyssey_staging" {
					t.Fatalf("target database = %q", target.Confirm.Database)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestCheckISO004CurrentDatabase(t *testing.T) {
	target := iso004Target{Host: "staging-db.internal", Database: "odyssey_staging", Confirm: iso004Confirmation{Database: "odyssey_staging", Host: "staging-db.internal"}}
	if err := checkISO004CurrentDatabase(target, "odyssey_staging"); err != nil {
		t.Fatalf("matching current_database rejected: %v", err)
	}
	err := checkISO004CurrentDatabase(target, "odyssey")
	if err == nil || !strings.Contains(err.Error(), "current_database()") {
		t.Fatalf("expected current_database mismatch error, got %v", err)
	}
}

func TestParseISO004ConfirmationUsesLastAt(t *testing.T) {
	c, err := parseISO004Confirmation("db@name@host")
	if err != nil {
		t.Fatal(err)
	}
	if c.Database != "db@name" || c.Host != "host" {
		t.Fatalf("got %+v", c)
	}
}
