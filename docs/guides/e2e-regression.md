# HTTP E2E regression

The regression suite uses Go's standard `net/http` client—no browser or test
framework dependency is required. It validates login/session/CSRF, core report
pages, reporting administration pages, and native Excel downloads against a
running application.

Run against a local or staging instance:

```bash
ODYSSEY_E2E_URL=http://127.0.0.1:8080 \
ODYSSEY_E2E_EMAIL=admin@odyssey.local \
ODYSSEY_E2E_PASSWORD=admin123 \
go test ./tests/e2e -run TestRegressionFlow -v
```

Without `ODYSSEY_E2E_URL`, the suite skips intentionally. Configure a dedicated
seeded test account in CI; do not use production credentials.
