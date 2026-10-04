package main

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

// The --iso004 extension writes certification fixtures into a shared staging
// database, so every precondition below fails closed: any mismatch aborts
// before a connection is used for anything other than reading
// current_database().

const (
	iso004RequiredAppEnv   = "staging"
	iso004DefaultDenyHosts = "prod"
)

// iso004KeyPattern restricts the run key to characters that are safe inside
// fixture codes, storage keys and prefix comparisons (no LIKE wildcards).
var iso004KeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,39}$`)

// iso004Confirmation is the parsed value of --confirm-staging <db>@<host>.
type iso004Confirmation struct {
	Database string
	Host     string
}

func parseISO004Confirmation(raw string) (iso004Confirmation, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return iso004Confirmation{}, errors.New("--confirm-staging <db-name>@<db-host> is required with --iso004")
	}
	at := strings.LastIndex(raw, "@")
	if at <= 0 || at == len(raw)-1 {
		return iso004Confirmation{}, fmt.Errorf("--confirm-staging %q must have the form <db-name>@<db-host>", raw)
	}
	return iso004Confirmation{Database: raw[:at], Host: raw[at+1:]}, nil
}

// iso004GuardInput collects everything the static (pre-connection) guard
// needs, so it can be unit tested without a database.
type iso004GuardInput struct {
	DSN            string
	Confirm        string
	AppEnv         string
	DenyHostRegex  string
	Key            string
	ExplicitDSNSet bool
}

// iso004Target is the database the guard approved, before the
// current_database() check.
type iso004Target struct {
	Host     string
	Database string
	Confirm  iso004Confirmation
}

// checkISO004Static validates flags and environment without touching the
// database. It returns the approved target or the first refusal reason.
func checkISO004Static(in iso004GuardInput) (iso004Target, error) {
	if !in.ExplicitDSNSet || strings.TrimSpace(in.DSN) == "" {
		return iso004Target{}, errors.New("--iso004 requires an explicit --dsn or PG_DSN; the localhost default is not used")
	}
	if in.AppEnv != iso004RequiredAppEnv {
		return iso004Target{}, fmt.Errorf("--iso004 requires APP_ENV=%s in the process environment (got %q)", iso004RequiredAppEnv, in.AppEnv)
	}
	if !iso004KeyPattern.MatchString(in.Key) {
		return iso004Target{}, fmt.Errorf("--iso004-key %q is required and must match %s (pass the ISO-004 run ID)", in.Key, iso004KeyPattern.String())
	}
	confirm, err := parseISO004Confirmation(in.Confirm)
	if err != nil {
		return iso004Target{}, err
	}
	denyExpr := in.DenyHostRegex
	if strings.TrimSpace(denyExpr) == "" {
		return iso004Target{}, errors.New("--deny-host-regex must not be empty")
	}
	deny, err := regexp.Compile("(?i)" + denyExpr)
	if err != nil {
		return iso004Target{}, fmt.Errorf("compile --deny-host-regex %q: %w", denyExpr, err)
	}

	cfg, err := pgx.ParseConfig(in.DSN)
	if err != nil {
		return iso004Target{}, fmt.Errorf("parse DSN: %w", err)
	}
	host := cfg.Host
	// pgx adds same-host fallbacks for sslmode=prefer; only a different host
	// means the DSN could reach a second server.
	for _, fb := range cfg.Fallbacks {
		if fb.Host != host {
			return iso004Target{}, errors.New("DSN lists more than one host; --iso004 requires exactly one host")
		}
	}
	if host == "" {
		return iso004Target{}, errors.New("DSN has no host")
	}
	if deny.MatchString(host) {
		return iso004Target{}, fmt.Errorf("DSN host %q matches --deny-host-regex %q; refusing to write fixtures", host, denyExpr)
	}
	if !strings.EqualFold(host, confirm.Host) {
		return iso004Target{}, fmt.Errorf("DSN host %q does not match --confirm-staging host %q", host, confirm.Host)
	}
	if cfg.Database == "" {
		return iso004Target{}, errors.New("DSN must name the database explicitly (dbname) with --iso004")
	}
	if cfg.Database != confirm.Database {
		return iso004Target{}, fmt.Errorf("DSN database %q does not match --confirm-staging database %q", cfg.Database, confirm.Database)
	}
	return iso004Target{Host: host, Database: cfg.Database, Confirm: confirm}, nil
}

// checkISO004CurrentDatabase compares the server-reported database name with
// the confirmation string. It runs after connecting and before any write.
func checkISO004CurrentDatabase(target iso004Target, currentDatabase string) error {
	if currentDatabase != target.Confirm.Database {
		return fmt.Errorf("current_database() is %q but --confirm-staging names %q", currentDatabase, target.Confirm.Database)
	}
	return nil
}
