# Handoff Report — CSS Architecture & Token Audit

**Agent**: Explorer 2 (CSS Architecture & Token Audit Specialist)  
**Working Directory**: `/home/noah/project/odyssey-erp/.agents/explorer_survey_css_2`  
**Date**: 2026-07-29  

---

## 1. Observation

1. **Border Radii Values in `web/static/css/core/tokens.css`**:
   - Lines 60-68 of `web/static/css/core/tokens.css`:
     ```css
     --radius-1: 6px;
     --radius-2: 10px;
     --radius-3: 14px;
     --radius-pill: 9999px;
     --radius-md: var(--radius-2);
     --radius-lg: var(--radius-3);
     ```
     `--badge-radius` line 256: `var(--radius-pill);`.

2. **Monospace & Tabular Number Classes in `web/static/css/core/utilities.css`**:
   - `web/static/css/core/utilities.css`: Has `.font-normal`, `.font-medium`, `.font-semibold`, `.font-bold` (lines 387-401), but does NOT contain `.font-mono`.
   - `.font-mono` is defined inside `web/static/css/pages/landing.css` line 42 (`.font-mono { font-family: var(--dev-mono) !important; }`).
   - `.numeric` and `.numeric-right` in `web/static/css/core/utilities.css:285-294` specify `font-variant-numeric: var(--numeric)` without `font-family: var(--font-mono)`.

3. **State Badges in `web/static/css/components/misc.css`**:
   - `.sys-badge` (lines 102-117) uses `font-family: var(--font-mono); border-radius: var(--radius-1, 2px); border: 1px solid var(--border-subtle); background: var(--bg-surface-muted); color: var(--text-muted); text-transform: uppercase;`.
   - `.badge` (lines 10-19) uses `--badge-radius` (which points to 9999px pill).

4. **Legacy & Un-tokenized Files (`close.css`, `analytics.css`)**:
   - `web/static/css/close.css` contains Pico CSS fallbacks (`var(--pico-muted-color, #6b7280)` lines 28, 33, 149), hardcoded hex colors (`#b42318` line 38, `#e0f2fe` line 102, `#075985` line 103, `#fef3c7` line 107, `#92400e` line 108, `#d1fae5` line 112, `#065f46` line 113, `#fee2e2` line 117, `#b91c1c` line 118), and soft radii (`border-radius: 0.5rem` line 43, `border-radius: 0.85rem` line 52, `border-radius: 999px` line 94).
   - `web/static/css/close.css` is linked in `web/templates/pages/close/periods.html:8` and `web/templates/pages/close/run.html:8`.
   - `web/static/css/analytics.css` contains `border-radius: 0.75rem;` (lines 9, 41) and is linked in `web/templates/pages/finance/dashboard.html:8`.
   - Neither `close.css` nor `analytics.css` are included in `web/static/css/main.css`.

---

## 2. Logic Chain

1. **From Observation 1**: Requirement **R2** in `ORIGINAL_REQUEST.md` mandates 100% token usage from `web/static/css/core/tokens.css` and sharp 2px border radii (`var(--radius-1)`). Because `tokens.css` defines `--radius-1` as 6px, `--radius-2` as 10px, and `--radius-3` as 14px, components inheriting these tokens exhibit soft rounded SaaS corners instead of Midnight Ledger industrial 2px sharp edges. Refactoring `--radius-1` to `2px`, `--radius-2` to `4px`, `--radius-3` to `6px`, and `--badge-radius` to `var(--radius-1)` aligns all derived component styles automatically.

2. **From Observation 2**: `ORIGINAL_REQUEST.md` requires monospace tabular numeric formatting (`font-family: var(--font-mono)`, `font-variant-numeric: tabular-nums lining-nums`, `.font-mono`, `.numeric`) across table amounts, reference numbers, and metrics. Because `.font-mono` is currently only defined in `pages/landing.css`, any template using `.font-mono` on pages that do not import `landing.css` will fail to render monospace typography. Adding `.font-mono` to `core/utilities.css` and attaching `font-family: var(--font-mono)` to `.numeric` and `.numeric-right` guarantees global availability and strict compliance.

3. **From Observation 3**: The industrial state badge `.sys-badge` in `misc.css` already implements Midnight Ledger specifications (monospaced uppercase font, 11px, 2px border radius, subtle border). However, standard badges and status badges using pill radii (9999px) violate the industrial aesthetic. Changing `--badge-radius` to `var(--radius-1)` (2px) standardizes badge appearance.

4. **From Observation 4**: `close.css` and `analytics.css` contain un-tokenized legacy styles, Pico CSS fallbacks, soft `rem` border radii, and hardcoded hex colors. In addition, they bypass `main.css`. Moving their styles into tokenized BEM page stylesheets (`pages/close.css` and `pages/analytics.css`) or standard component classes, and importing them into `main.css`, completes the tokenization and BEM architecture cleanup.

---

## 3. Caveats

- **Scope Boundary**: This investigation is read-only. No modifications were made directly to CSS or template source files. All recommendations and proposed patches are cataloged in `analysis.md`.
- **Domain Template Classes**: Refactoring `close.css` and `analytics.css` to BEM and design tokens will require minor template class updates in `web/templates/pages/close/*.html` and `web/templates/pages/finance/dashboard.html`.

---

## 4. Conclusion

The Odyssey ERP CSS architecture can achieve 100% Midnight Ledger Design Token & BEM Architecture compliance by executing the following actionable steps:
1. Update `web/static/css/core/tokens.css` radii scale: `--radius-1: 2px; --radius-2: 4px; --radius-3: 6px; --badge-radius: var(--radius-1);`.
2. Add `.font-mono { font-family: var(--font-mono); }` and update `.numeric` / `.numeric-right` in `web/static/css/core/utilities.css`.
3. Refactor legacy `close.css` and `analytics.css` to use BEM naming and tokens, and bundle them into `main.css`.
4. Update domain templates to standardize on `.sys-badge` / `.status-badge` for state indicators and `.font-mono` / `.numeric` for codes and monetary amounts.

---

## 5. Verification Method

### 5.1 Manual Verification Commands
- Check token definitions:
  `grep -n "radius" web/static/css/core/tokens.css`
- Check global utility definitions:
  `grep -n "font-mono" web/static/css/core/utilities.css`

### 5.2 Build & Test Verification Commands
- Compile project binaries:
  `make build`
- Run test suite in isolated test mode:
  `ODYSSEY_TEST_MODE=1 go test ./...`

### 5.3 Invalidation Conditions
- Any occurrence of hardcoded hex colors outside `core/tokens.css`.
- Any component retaining `border-radius: 12px`, `14px`, or `16px` after token refactoring.
- Any Go template parsing or test failures during `make build` or `go test`.
