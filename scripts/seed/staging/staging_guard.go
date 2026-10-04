package main

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Shared staging guard. Both the --iso004 extension and the base seeder write
// certification fixtures into a shared staging database, so every
// precondition below fails closed: any mismatch aborts before a connection is
// used for anything other than reading current_database().

const (
	stagingRequiredAppEnv   = "staging"
	stagingDefaultDenyHosts = "prod"
)

// stagingConfirmation is the parsed value of --confirm-staging <db>@<host>.
type stagingConfirmation struct {
	Database string
	Host     string
}

// stagingTarget is the database the guard approved, before the
// current_database() check.
type stagingTarget struct {
	Host     string
	Database string
	Confirm  stagingConfirmation
}

// stagingGuardInput collects everything the static (pre-connection) guard
// needs, so it can be unit tested without a database.
type stagingGuardInput struct {
	// Label names the command in refusal messages, for example "--iso004".
	Label          string
	DSN            string
	Confirm        string
	AppEnv         string
	DenyHostRegex  string
	ExplicitDSNSet bool
	// CheckKey, when set, runs after the APP_ENV check and may refuse.
	CheckKey func() error
}

func parseStagingConfirmation(raw, label string) (stagingConfirmation, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return stagingConfirmation{}, fmt.Errorf("--confirm-staging <db-name>@<db-host> is required with %s", label)
	}
	at := strings.LastIndex(raw, "@")
	if at <= 0 || at == len(raw)-1 {
		return stagingConfirmation{}, fmt.Errorf("--confirm-staging %q must have the form <db-name>@<db-host>", raw)
	}
	return stagingConfirmation{Database: raw[:at], Host: raw[at+1:]}, nil
}

// checkStagingStatic validates flags and environment without touching the
// database. It returns the approved target or the first refusal reason.
func checkStagingStatic(in stagingGuardInput) (stagingTarget, error) {
	label := in.Label
	if !in.ExplicitDSNSet || strings.TrimSpace(in.DSN) == "" {
		return stagingTarget{}, fmt.Errorf("%s requires an explicit --dsn or PG_DSN; the localhost default is not used", label)
	}
	if in.AppEnv != stagingRequiredAppEnv {
		return stagingTarget{}, fmt.Errorf("%s requires APP_ENV=%s in the process environment (got %q)", label, stagingRequiredAppEnv, in.AppEnv)
	}
	if in.CheckKey != nil {
		if err := in.CheckKey(); err != nil {
			return stagingTarget{}, err
		}
	}
	confirm, err := parseStagingConfirmation(in.Confirm, label)
	if err != nil {
		return stagingTarget{}, err
	}
	denyExpr := in.DenyHostRegex
	if strings.TrimSpace(denyExpr) == "" {
		return stagingTarget{}, errors.New("--deny-host-regex must not be empty")
	}
	deny, err := regexp.Compile("(?i)" + denyExpr)
	if err != nil {
		return stagingTarget{}, fmt.Errorf("compile --deny-host-regex %q: %w", denyExpr, err)
	}

	cfg, err := pgx.ParseConfig(in.DSN)
	if err != nil {
		return stagingTarget{}, fmt.Errorf("parse DSN: %w", err)
	}
	host := cfg.Host
	// pgx adds same-host fallbacks for sslmode=prefer; only a different host
	// means the DSN could reach a second server.
	for _, fb := range cfg.Fallbacks {
		if fb.Host != host {
			return stagingTarget{}, fmt.Errorf("DSN lists more than one host; %s requires exactly one host", label)
		}
	}
	if host == "" {
		return stagingTarget{}, errors.New("DSN has no host")
	}
	if deny.MatchString(host) {
		return stagingTarget{}, fmt.Errorf("DSN host %q matches --deny-host-regex %q; refusing to write fixtures", host, denyExpr)
	}
	if !strings.EqualFold(host, confirm.Host) {
		return stagingTarget{}, fmt.Errorf("DSN host %q does not match --confirm-staging host %q", host, confirm.Host)
	}
	if cfg.Database == "" {
		return stagingTarget{}, fmt.Errorf("DSN must name the database explicitly (dbname) with %s", label)
	}
	if cfg.Database != confirm.Database {
		return stagingTarget{}, fmt.Errorf("DSN database %q does not match --confirm-staging database %q", cfg.Database, confirm.Database)
	}
	return stagingTarget{Host: host, Database: cfg.Database, Confirm: confirm}, nil
}

// checkStagingCurrentDatabase compares the server-reported database name with
// the confirmation string. It runs after connecting and before any write.
func checkStagingCurrentDatabase(target stagingTarget, currentDatabase string) error {
	if currentDatabase != target.Confirm.Database {
		return fmt.Errorf("current_database() is %q but --confirm-staging names %q", currentDatabase, target.Confirm.Database)
	}
	return nil
}
