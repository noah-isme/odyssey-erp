# Original User Request

## 2026-07-29T08:15:13Z

Eliminate all remaining AI design slop across Odyssey ERP pages, establishing high-density Midnight Ledger industrial enterprise aesthetics across all remaining application templates.

Working directory: /home/noah/project/odyssey-erp
Integrity mode: benchmark

## Requirements

### R1. Full-Suite UI Audit & Design Slop Elimination
Audit and refactor all remaining domain page templates (`web/templates/pages/`: Sales, Procurement, Inventory, Accounting, Auth/Login, Master Data, Delivery, Roles/Permissions). Remove all generic SaaS icon bubbles, soft rounded shadows, and ad-hoc inline styles.

### R2. Midnight Ledger Design Tokens & BEM Architecture
Ensure 100% token usage from `web/static/css/core/tokens.css` and `web/static/css/components/`. Apply sharp 2px border radii (`var(--radius-1)`), monospace tabular numeric formatting (`font-family: var(--font-mono)`, `font-variant-numeric: tabular-nums lining-nums`), industrial state badges (`.sys-badge`), and clear visual hierarchies without CSS redundancy.

### R3. Build & Test Suite Integrity
Maintain complete system stability. All refactored pages must parse Go templates error-free, and all Go compilation (`make build`) and automated test suites (`ODYSSEY_TEST_MODE=1 go test ./...`) must pass cleanly with 0 regressions.

## Acceptance Criteria

### Aesthetic & Token Compliance
- [ ] No hardcoded hex colors, ad-hoc inline styles, or 48px pastel icon bubbles remain across `web/templates/` and `web/static/css/`.
- [ ] All table amounts, reference numbers, and metrics use monospaced fonts (`.font-mono`, `.numeric`).
- [ ] All state indicators use standardized BEM-lite badge components (`.sys-badge`, `.status-badge`).

### System Verification
- [ ] `make build` compiles binary packages without errors.
- [ ] `ODYSSEY_TEST_MODE=1 go test ./...` passes all unit and integration test suites cleanly.
