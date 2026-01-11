# Testing Runbook

This document summarises the steps required to run the Odyssey ERP automated
checks without contacting external infrastructure.

## Test mode environment

The application now honours the `ODYSSEY_TEST_MODE` environment flag. When the
flag is set to `1` the runtime skips expensive side effects such as opening
PostgreSQL/Redis connections or initialising background workers. The helper
package located at `github.com/odyssey-erp/odyssey-erp/testing` enables the flag
for unit tests, and CI also exports it before executing the suite.

Set the following variables when running tools manually:

```bash
export ODYSSEY_TEST_MODE=1
export GOTENBERG_URL="http://127.0.0.1:0"
```

`GOTENBERG_URL` is pointed at a non-routable address so that any code paths that
accidentally reach the HTTP client fail fast instead of hanging on network
timeouts.

## Local lint, test, and build

After exporting the environment variables above you can execute the complete Go
workflow:

```bash
go vet ./...
go test ./...
go build ./...
```

These commands should finish in a few seconds now that runtime hooks are
suppressed in test mode.

## Troubleshooting

If vet or test still appears to hang:

- Verify that `ODYSSEY_TEST_MODE` is set to `1` in the shell.
- Ensure that no process is attempting to connect to PostgreSQL or Redis by
  checking `ps` output for `psql` or `redis-cli` commands.
- Run packages individually with `go test -run ^$ <package>` to identify any
  remaining integration-style code paths that require further guards.

Document any new findings in this runbook so that the next engineer can resolve
similar issues quickly.
