# Browser E2E Testing (Playwright / Cypress)

## Overview
This guide describes the setup and best practices for browser-based End-to-End (E2E) testing of the Odyssey ERP web interfaces using Playwright (or Cypress). While the standard HTTP-based API regression suite (`go test ./tests/e2e`) runs quickly against the backend, the browser E2E suite ensures that all user-facing workflows, JavaScript interactions, and UI rendering function correctly across supported browsers.

## 1. Setup and Installation

### Playwright Initialization
Playwright is recommended for full multi-browser testing of the web application.

```bash
# Navigate to the e2e testing directory
cd testing/browser-e2e

# Install dependencies (assuming a package.json is present)
npm install

# Install Playwright browsers
npx playwright install --with-deps
```

### Running Tests Locally
Ensure that the Odyssey application is running locally before executing the tests.

```bash
# Start the local development server (in another terminal)
make dev

# Run the Playwright test suite
npx playwright test

# Run tests in UI mode for debugging
npx playwright test --ui
```

## 2. Test Data Management Strategy

Managing test data effectively is critical for reliable E2E tests, avoiding state leakage between test runs.

### Dedicated Test Database
- Tests should **never** run against a production or staging database.
- Use a dedicated `odyssey_e2e` database, reset before the test suite runs.

### Seed Scripts
- Use the `make seed` command (or an equivalent SQL script) to load a deterministic baseline of data (e.g., standard users, an initial chart of accounts, master data).
- The baseline data ensures a consistent starting point.

### Idempotency and Isolation
- Test scenarios should generate dynamic unique identifiers (e.g., random document numbers, timestamps) when creating transactions to prevent unique constraint failures.
- Alternatively, tests can use API endpoints to clean up created entities in a `afterEach` hook.
- Run tests in isolated Playwright worker processes, with each worker potentially using a separate tenant/company context if parallel execution is required.

## 3. CI Integration

The browser E2E suite should be integrated into the CI pipeline to run against PRs and the main branch.

### GitHub Actions Example

```yaml
name: Browser E2E Tests
on:
  push:
    branches: [ main ]
  pull_request:
    branches: [ main ]

jobs:
  e2e-tests:
    timeout-minutes: 15
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v4
    
    - name: Set up Go
      uses: actions/setup-go@v5
      with:
        go-version: '1.22'
        
    - name: Set up Node.js
      uses: actions/setup-node@v4
      with:
        node-version: 18
        
    - name: Start Database & Dependencies
      run: docker compose -f docker-compose.yml up -d db redis
      
    - name: Seed E2E Database
      run: make seed
      
    - name: Build and Start Application
      run: |
        make build
        ./bin/odyssey &
      env:
        PG_DSN: postgres://postgres:postgres@localhost:5432/odyssey_e2e?sslmode=disable
        PORT: 8080
        
    - name: Install Playwright & Browsers
      working-directory: ./testing/browser-e2e
      run: |
        npm ci
        npx playwright install --with-deps
        
    - name: Run Playwright Tests
      working-directory: ./testing/browser-e2e
      run: npx playwright test
      env:
        BASE_URL: http://localhost:8080
        
    - uses: actions/upload-artifact@v4
      if: always()
      with:
        name: playwright-report
        path: testing/browser-e2e/playwright-report/
        retention-days: 15
```

## 4. Best Practices
- **Locators:** Use data-test-id attributes (e.g., `data-testid="submit-button"`) instead of brittle CSS selectors or text matches where possible.
- **Wait Strategies:** Rely on Playwright's auto-waiting mechanisms. Avoid using explicit hardcoded timeouts (`page.waitForTimeout()`).
- **Auth State:** Authenticate once per worker and reuse the signed-in state (cookies/local storage) to speed up test execution, avoiding UI login for every single test.
