# Milestone 1 (M1_CSS) Changes Log

## Summary of Changes
Refactored core CSS tokens, added font-mono utilities, migrated legacy un-tokenized `close.css` and `analytics.css` to modular BEM stylesheets in `web/static/css/pages/`, updated `main.css` imports, updated HTML template links, removed legacy files, and verified via build and test.

## Modified & Created Files

1. **`web/static/css/core/tokens.css`**:
   - Refactored border radii tokens to sharp industrial geometry:
     - `--radius-1: 2px;` (controls / sharp badges / nav)
     - `--radius-2: 4px;` (buttons / inputs / sub-containers)
     - `--radius-3: 6px;` (cards / dialogs / panels)
   - Updated `--badge-radius` from `var(--radius-pill)` to `var(--radius-1)` (2px sharp rectangular badge).

2. **`web/static/css/core/utilities.css`**:
   - Added global typography utility `.font-mono { font-family: var(--font-mono); }`.
   - Updated tabular numeric utilities `.numeric` and `.numeric-right` with explicit `font-family: var(--font-mono);`.

3. **`web/static/css/pages/close.css`** (New File):
   - Created tokenized BEM page stylesheet for period close workflows.
   - Replaced Pico CSS fallbacks (`var(--pico-muted-color, #6b7280)`, `var(--pico-card-background-color, #ffffff)`) with Midnight Ledger design tokens (`var(--text-muted)`, `var(--card-bg)`).
   - Replaced hardcoded hex colors with semantic color tokens (`var(--info-bg)`, `var(--info-600)`, `var(--warning-bg)`, `var(--warning-600)`, `var(--success-bg)`, `var(--success-600)`, `var(--error-bg)`, `var(--error-600)`, `var(--badge-neutral-bg)`, `var(--badge-neutral-fg)`).
   - Updated `.status-badge` border-radius to `var(--badge-radius)` (2px sharp) and enforced monospaced typography `font-family: var(--font-mono)`.

4. **`web/static/css/pages/analytics.css`** (New File):
   - Created tokenized BEM page stylesheet for analytics and finance dashboard.
   - Replaced soft `0.75rem` card border-radius with `var(--card-radius)` (6px sharp industrial token).
   - Enforced monospaced font family `font-family: var(--font-mono)` on KPI card values (`.kpi-card .value`).

5. **`web/static/css/main.css`**:
   - Added `@import url('./pages/close.css');` and `@import url('./pages/analytics.css');` under `/* Pages */` imports block.

6. **Template Stylesheet References**:
   - `web/templates/pages/close/periods.html`: Updated `<link rel="stylesheet" href="/static/css/close.css">` to `/static/css/pages/close.css`.
   - `web/templates/pages/close/run.html`: Updated `<link rel="stylesheet" href="/static/css/close.css">` to `/static/css/pages/close.css`.
   - `web/templates/pages/finance/dashboard.html`: Updated `<link rel="stylesheet" href="/static/css/analytics.css">` to `/static/css/pages/analytics.css`.

7. **Removed Legacy Files**:
   - `web/static/css/close.css`: Removed via git rm.
   - `web/static/css/analytics.css`: Removed via git rm.
