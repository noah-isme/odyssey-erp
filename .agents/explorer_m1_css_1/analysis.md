# Milestone 1 (M1_CSS) Implementation Specification & Technical Plan

**Target System**: Odyssey ERP — Midnight Ledger Industrial Enterprise Design System  
**Author**: `explorer_m1_css_1` (Teamwork Explorer)  
**Date**: 2026-07-29  
**Status**: Ready for Implementation  

---

## 1. Executive Summary

This document specifies the exact, line-by-line implementation spec for **Milestone 1 (M1_CSS)** of the Odyssey ERP Midnight Ledger UI Refactor. Milestone 1 establishes the design system foundation by:
1. Refactoring core border radii tokens in `web/static/css/core/tokens.css` from soft SaaS curves (6px/10px/14px/pill) to sharp industrial enterprise geometry (2px/4px/6px/2px).
2. Adding global `.font-mono` typography utility and enforcing `font-family: var(--font-mono)` on tabular numeric utilities (`.numeric` and `.numeric-right`) in `web/static/css/core/utilities.css`.
3. Consolidating legacy orphaned CSS files (`web/static/css/close.css` and `web/static/css/analytics.css`) into modular BEM page stylesheets (`web/static/css/pages/close.css` and `web/static/css/pages/analytics.css`), bundling them into `web/static/css/main.css`, and updating HTML template references.

---

## 2. Detailed Technical Specifications

### Task 1: Refactor Border Radii Tokens (`web/static/css/core/tokens.css`)

#### Rationale & Impact
Currently, `tokens.css` defines soft rounded corners (`--radius-1: 6px;`, `--radius-2: 10px;`, `--radius-3: 14px;`, `--badge-radius: var(--radius-pill);`). Per Midnight Ledger design principles, enterprise high-density UI requires crisp 2px primary control edges and 4px/6px container corners.
- `--radius-1`: Used for controls, sharp badges, table outer edges, nav items (`2px`).
- `--radius-2`: Used for buttons (`--btn-radius`), inputs (`--input-radius`), medium containers (`4px`).
- `--radius-3`: Used for cards (`--card-radius`), float panels (`--float-radius`), modals (`--modal-radius`), tables (`6px`).
- `--badge-radius`: Changed from `var(--radius-pill)` (9999px soft pill) to `var(--radius-1)` (2px rectangular badge).

#### Target File
`/home/noah/project/odyssey-erp/web/static/css/core/tokens.css`

#### Line-by-Line Changes
1. **Lines 59–68**:
```css
<<<<
    /* Radius */
    --radius-1: 6px;
    /* controls */
    --radius-2: 10px;
    /* buttons/inputs */
    --radius-3: 14px;
    /* cards/panels */
    --radius-pill: 9999px;
    --radius-md: var(--radius-2);
    --radius-lg: var(--radius-3);
====
    /* Radius (Midnight Ledger Industrial Sharp Radii) */
    --radius-1: 2px;
    /* controls / sharp badges / nav */
    --radius-2: 4px;
    /* buttons / inputs / sub-containers */
    --radius-3: 6px;
    /* cards / dialogs / panels */
    --radius-pill: 9999px;
    --radius-md: var(--radius-2);
    --radius-lg: var(--radius-3);
>>>>
```

2. **Lines 255–257**:
```css
<<<<
    /* Badges */
    --badge-radius: var(--radius-pill);
    --badge-font: var(--text-xs);
====
    /* Badges (Industrial Sharp BEM Pills) */
    --badge-radius: var(--radius-1);
    --badge-font: var(--text-xs);
>>>>
```

---

### Task 2: Core Typography & Numeric Utilities (`web/static/css/core/utilities.css`)

#### Rationale & Impact
Monospaced font rendering for tabular metrics, reference codes, and amounts is a core requirement of Midnight Ledger.
- `.font-mono` is missing in `utilities.css` (it was previously mis-scoped in `landing.css`). Adding it globally allows all templates to format codes using `var(--font-mono)`.
- `.numeric` and `.numeric-right` currently specify `font-variant-numeric: var(--numeric)` and `font-feature-settings: "tnum" 1, "lnum" 1`, but omit `font-family: var(--font-mono)`. Adding explicit monospacing guarantees consistent tabular column alignment across all browsers.

#### Target File
`/home/noah/project/odyssey-erp/web/static/css/core/utilities.css`

#### Line-by-Line Changes
**Lines 282–295**:
```css
<<<<
/* -----------------------------
   Numeric Utilities (finance-friendly)
----------------------------- */
.numeric {
    font-variant-numeric: var(--numeric);
    font-feature-settings: "tnum" 1, "lnum" 1;
}

.numeric-right {
    font-variant-numeric: var(--numeric);
    font-feature-settings: "tnum" 1, "lnum" 1;
    text-align: right;
}
====
/* -----------------------------
   Numeric & Monospace Utilities (Midnight Ledger core)
----------------------------- */
.font-mono {
    font-family: var(--font-mono);
}

.numeric {
    font-family: var(--font-mono);
    font-variant-numeric: var(--numeric);
    font-feature-settings: "tnum" 1, "lnum" 1;
}

.numeric-right {
    font-family: var(--font-mono);
    font-variant-numeric: var(--numeric);
    font-feature-settings: "tnum" 1, "lnum" 1;
    text-align: right;
}
>>>>
```

---

### Task 3: Legacy CSS Consolidation & HTML Template Link Updates

#### 3.1 Migration: `close.css` to `pages/close.css`
- **Current Path**: `web/static/css/close.css`
- **New Path**: `web/static/css/pages/close.css`
- **Changes**:
  - Replace Pico CSS fallback variables (`var(--pico-muted-color, #6b7280)`, `var(--pico-card-background-color, #ffffff)`) with design tokens (`var(--text-muted)`, `var(--card-bg)`).
  - Replace hardcoded hex colors (`#b42318`, `#e0f2fe`, `#075985`, `#fef3c7`, `#92400e`, `#d1fae5`, `#065f46`, `#fee2e2`, `#b91c1c`, `#e5e7eb`, `#374151`) with Midnight Ledger semantic tokens (`var(--info-bg)`, `var(--info-600)`, `var(--warning-bg)`, `var(--warning-600)`, `var(--success-bg)`, `var(--success-600)`, `var(--error-bg)`, `var(--error-600)`, `var(--badge-neutral-bg)`, `var(--badge-neutral-fg)`).
  - Convert `.status-badge` radius from soft pill (`999px`) to sharp industrial token (`var(--badge-radius)` / 2px) and add `font-family: var(--font-mono)`.
  - Replace hardcoded `rem` values with `--space-*` tokens.

**Proposed Code Content for `web/static/css/pages/close.css`**:
```css
/* ==========================================================================
   Odyssey ERP - Close Module Page Stylesheet
   Path: web/static/css/pages/close.css
   ========================================================================== */

.page-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-end;
    flex-wrap: wrap;
    gap: var(--space-6);
    margin-bottom: var(--space-8);
}

.page-header .filters {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-4);
    align-items: flex-end;
}

.page-header .filters label {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    font-size: var(--text-sm);
}

.eyebrow {
    font-size: var(--text-xs);
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-muted);
    margin-bottom: var(--space-1);
}

.muted {
    color: var(--text-muted);
    font-size: var(--text-sm);
}

.text-danger {
    color: var(--error-600);
}

.note {
    background: var(--info-bg);
    color: var(--info-600);
    border: 1px solid rgba(37, 99, 235, 0.2);
    border-radius: var(--radius-1);
    padding: var(--space-2) var(--space-3);
    margin-top: var(--space-2);
    font-size: var(--text-sm);
}

.card {
    background: var(--card-bg);
    border: 1px solid var(--card-border);
    border-radius: var(--card-radius);
    padding: var(--card-pad);
    margin-bottom: var(--space-6);
}

.card-heading {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
    margin-bottom: var(--space-4);
}

.responsive-table {
    overflow-x: auto;
}

.period-table,
.checklist-table {
    width: 100%;
    border-collapse: collapse;
}

.period-table th,
.period-table td,
.checklist-table th,
.checklist-table td {
    padding: var(--table-cell-pad-y) var(--table-cell-pad-x);
    border-bottom: 1px solid var(--table-border);
    vertical-align: top;
}

.inline-form {
    display: flex;
    gap: var(--space-2);
    align-items: center;
}

.status-badge {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    padding: 2px 8px;
    border-radius: var(--badge-radius);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    font-weight: var(--weight-semibold);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    white-space: nowrap;
    border: 1px solid var(--border-subtle);
}

.status-badge--info {
    background: var(--info-bg);
    color: var(--info-600);
    border-color: rgba(37, 99, 235, 0.2);
}

.status-badge--warning {
    background: var(--warning-bg);
    color: var(--warning-600);
    border-color: rgba(178, 106, 0, 0.2);
}

.status-badge--success {
    background: var(--success-bg);
    color: var(--success-600);
    border-color: rgba(31, 122, 77, 0.2);
}

.status-badge--danger {
    background: var(--error-bg);
    color: var(--error-600);
    border-color: rgba(180, 35, 24, 0.2);
}

.status-badge--muted {
    background: var(--badge-neutral-bg);
    color: var(--badge-neutral-fg);
    border-color: var(--border-subtle);
}

.status-stack {
    display: flex;
    gap: var(--space-2);
    flex-wrap: wrap;
    margin-bottom: var(--space-2);
}

.summary-card progress {
    width: 100%;
    height: 8px;
    border-radius: var(--radius-1);
    accent-color: var(--brand);
}

.summary-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
    gap: var(--space-4);
    margin: var(--space-4) 0 0;
}

.summary-grid dt {
    font-size: var(--text-xs);
    font-weight: var(--weight-medium);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
    margin-bottom: var(--space-1);
}

.summary-grid dd {
    font-family: var(--font-mono);
    font-size: var(--text-xl);
    font-weight: var(--weight-semibold);
    color: var(--text-primary);
    margin: 0;
}

.checklist-form {
    display: grid;
    gap: var(--space-2);
    grid-template-columns: minmax(160px, 1fr);
}

.checklist-form select,
.checklist-form textarea {
    width: 100%;
}

.checklist-form textarea {
    resize: vertical;
}

.close-actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-6);
}

.close-actions form {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
}
```

#### 3.2 Migration: `analytics.css` to `pages/analytics.css`
- **Current Path**: `web/static/css/analytics.css`
- **New Path**: `web/static/css/pages/analytics.css`
- **Changes**:
  - Update `border-radius: 0.75rem;` (12px) to `var(--card-radius)` (6px sharp).
  - Add `font-family: var(--font-mono)` to `.kpi-card .value`.
  - Replace hardcoded rem metrics with spacing/typography tokens.

**Proposed Code Content for `web/static/css/pages/analytics.css`**:
```css
/* ==========================================================================
   Odyssey ERP - Analytics & Finance Dashboard Page Stylesheet
   Path: web/static/css/pages/analytics.css
   ========================================================================== */

.dashboard-wrapper {
    display: grid;
    gap: var(--space-6);
}

.dashboard-section {
    background: var(--card-bg);
    border: 1px solid var(--border-subtle);
    border-radius: var(--card-radius);
    padding: var(--space-5);
}

.filters-form {
    display: grid;
    gap: var(--space-4);
}

.filters-form .filter-grid {
    display: grid;
    gap: var(--space-4);
}

@media (min-width: 640px) {
    .filters-form .filter-grid {
        grid-template-columns: repeat(3, minmax(0, 1fr));
    }
}

.kpi-grid {
    display: grid;
    gap: var(--space-4);
}

@media (min-width: 640px) {
    .kpi-grid {
        grid-template-columns: repeat(3, minmax(0, 1fr));
    }
}

.kpi-card {
    border-radius: var(--card-radius);
    border: 1px solid var(--border-subtle);
    padding: var(--space-4);
    background: var(--card-bg);
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
}

.kpi-card .label {
    font-size: var(--text-sm);
    color: var(--text-secondary);
}

.kpi-card .value {
    font-family: var(--font-mono);
    font-size: var(--text-xl);
    font-weight: var(--weight-semibold);
    color: var(--text-primary);
}

.chart-container {
    overflow-x: auto;
}

.chart-frame {
    min-width: 280px;
}

.aging-grid {
    display: grid;
    gap: var(--space-5);
}

@media (min-width: 720px) {
    .aging-grid {
        grid-template-columns: repeat(2, minmax(0, 1fr));
    }
}

.table-responsive {
    overflow-x: auto;
}

.table-responsive table {
    width: 100%;
}

.table-responsive th,
.table-responsive td {
    text-align: right;
}

.table-responsive th:first-child,
.table-responsive td:first-child {
    text-align: left;
}

.chart-description {
    font-size: var(--text-xs);
    color: var(--text-muted);
    margin-top: var(--space-2);
}
```

#### 3.3 Bundling in `web/static/css/main.css`
Target File: `/home/noah/project/odyssey-erp/web/static/css/main.css`
**Lines 35–42**:
```css
<<<<
/* Pages */
@import url('./pages/dashboard.css');
@import url('./pages/landing.css');
@import url('./pages/login.css');
@import url('./pages/quotation-form.css');
@import url('./pages/jobs.css');
@import url('./pages/metrics.css');
@import url('./pages/ping.css');
====
/* Pages */
@import url('./pages/dashboard.css');
@import url('./pages/landing.css');
@import url('./pages/login.css');
@import url('./pages/quotation-form.css');
@import url('./pages/jobs.css');
@import url('./pages/metrics.css');
@import url('./pages/ping.css');
@import url('./pages/close.css');
@import url('./pages/analytics.css');
>>>>
```

#### 3.4 HTML Template Link Reference Updates
1. **`web/templates/pages/close/periods.html`**:
   - Line 8: Update `<link rel="stylesheet" href="/static/css/close.css">` to `<link rel="stylesheet" href="/static/css/pages/close.css">`.
2. **`web/templates/pages/close/run.html`**:
   - Line 8: Update `<link rel="stylesheet" href="/static/css/close.css">` to `<link rel="stylesheet" href="/static/css/pages/close.css">`.
3. **`web/templates/pages/finance/dashboard.html`**:
   - Line 8: Update `<link rel="stylesheet" href="/static/css/analytics.css">` to `<link rel="stylesheet" href="/static/css/pages/analytics.css">`.

---

## 3. Implementation Step-by-Step Order

To execute M1_CSS safely without disrupting build or development servers:

1. **Step 1**: Create `web/static/css/pages/close.css` and `web/static/css/pages/analytics.css` with tokenized content.
2. **Step 2**: Modify `web/static/css/core/tokens.css` radii definitions.
3. **Step 3**: Modify `web/static/css/core/utilities.css` `.font-mono`, `.numeric`, `.numeric-right` definitions.
4. **Step 4**: Update `web/static/css/main.css` with `@import` statements.
5. **Step 5**: Update HTML template stylesheet link paths in `periods.html`, `run.html`, and `dashboard.html`.
6. **Step 6**: Remove old legacy files `web/static/css/close.css` and `web/static/css/analytics.css`.
7. **Step 7**: Run `make build` and `ODYSSEY_TEST_MODE=1 go test ./...` to verify clean compilation and zero test failures.

---

## 4. Verification Checklist for Implementer

- [ ] `tokens.css`: `--radius-1` is `2px`, `--radius-2` is `4px`, `--radius-3` is `6px`, `--badge-radius` is `var(--radius-1)`.
- [ ] `utilities.css`: `.font-mono` exists and specifies `font-family: var(--font-mono)`. `.numeric` and `.numeric-right` specify `font-family: var(--font-mono)`.
- [ ] `web/static/css/pages/close.css` exists with zero Pico CSS fallbacks or hardcoded hex colors.
- [ ] `web/static/css/pages/analytics.css` exists with `border-radius: var(--card-radius)` and `font-family: var(--font-mono)` on `.kpi-card .value`.
- [ ] `main.css` imports both new page stylesheets.
- [ ] Template files `periods.html`, `run.html`, and `dashboard.html` resolve stylesheet paths cleanly without 404 errors.
- [ ] `make build` executes cleanly with 0 errors.
- [ ] `ODYSSEY_TEST_MODE=1 go test ./...` passes all tests cleanly.
