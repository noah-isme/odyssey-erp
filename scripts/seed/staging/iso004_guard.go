package main

import (
	"fmt"
	"regexp"
)

// The --iso004 extension writes certification fixtures into a shared staging
// database. Its guard is the shared staging guard (staging_guard.go) plus the
// run-key check below.

const (
	iso004RequiredAppEnv   = stagingRequiredAppEnv
	iso004DefaultDenyHosts = stagingDefaultDenyHosts
)

// iso004KeyPattern restricts the run key to characters that are safe inside
// fixture codes, storage keys and prefix comparisons (no LIKE wildcards).
var iso004KeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,39}$`)

// iso004Confirmation is the parsed value of --confirm-staging <db>@<host>.
type iso004Confirmation = stagingConfirmation

// iso004Target is the database the guard approved, before the
// current_database() check.
type iso004Target = stagingTarget

func parseISO004Confirmation(raw string) (iso004Confirmation, error) {
	return parseStagingConfirmation(raw, "--iso004")
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

// checkISO004Static validates flags and environment without touching the
// database. It returns the approved target or the first refusal reason.
func checkISO004Static(in iso004GuardInput) (iso004Target, error) {
	return checkStagingStatic(stagingGuardInput{
		Label:          "--iso004",
		DSN:            in.DSN,
		Confirm:        in.Confirm,
		AppEnv:         in.AppEnv,
		DenyHostRegex:  in.DenyHostRegex,
		ExplicitDSNSet: in.ExplicitDSNSet,
		CheckKey: func() error {
			if !iso004KeyPattern.MatchString(in.Key) {
				return fmt.Errorf("--iso004-key %q is required and must match %s (pass the ISO-004 run ID)", in.Key, iso004KeyPattern.String())
			}
			return nil
		},
	})
}

// checkISO004CurrentDatabase compares the server-reported database name with
// the confirmation string. It runs after connecting and before any write.
func checkISO004CurrentDatabase(target iso004Target, currentDatabase string) error {
	return checkStagingCurrentDatabase(target, currentDatabase)
}
