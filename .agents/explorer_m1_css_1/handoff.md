# Handoff Report — Milestone 1 (M1_CSS) Implementation Specification

**Agent**: `explorer_m1_css_1`  
**Role**: Teamwork Explorer (Read-only CSS Investigation & Spec Formulation)  
**Target Milestone**: M1_CSS (Tokens, Utilities, & Legacy CSS Migration)  
**Date**: 2026-07-29  

---

## 1. Observation

1. **`web/static/css/core/tokens.css`**:
   - Lines 60–64 define soft SaaS border radii:
     ```css
     60:    --radius-1: 6px;
     62:    --radius-2: 10px;
     64:    --radius-3: 14px;
     ```
   - Line 256 defines soft pill badge radius:
     ```css
     256:    --badge-radius: var(--radius-pill);
     ```

2. **`web/static/css/core/utilities.css`**:
   - Lines 285–294 define `.numeric` and `.numeric-right` but lack `font-family: var(--font-mono)`:
     ```css
     285: .numeric {
     286:     font-variant-numeric: var(--numeric);
     287:     font-feature-settings: "tnum" 1, "lnum" 1;
     288: }
     289: 
     290: .numeric-right {
     291:     font-variant-numeric: var(--numeric);
     292:     font-feature-settings: "tnum" 1, "lnum" 1;
     293:     text-align: right;
     294: }
     ```
   - `.font-mono` is absent in `utilities.css` (previously found only in `web/static/css/pages/landing.css:42` using `--dev-mono`).

3. **Legacy Out-of-Spec CSS Files**:
   - `web/static/css/close.css`: Un-tokenized file containing Pico CSS fallback variables (`var(--pico-muted-color, #6b7280)`, `var(--pico-card-background-color, #ffffff)`), hardcoded hex colors (`#b42318`, `#e0f2fe`, `#075985`, `#fef3c7`, `#92400e`, `#d1fae5`, `#065f46`, `#fee2e2`, `#b91c1c`, `#e5e7eb`), and soft pill badge radius (`border-radius: 999px`). Referenced in `web/templates/pages/close/periods.html:8` and `web/templates/pages/close/run.html:8`.
   - `web/static/css/analytics.css`: Contains soft card border radii (`border-radius: 0.75rem;` / 12px) and un-tokenized spacing metrics. Referenced in `web/templates/pages/finance/dashboard.html:8`.
   - `web/static/css/main.css`: Missing `@import` statements for `pages/close.css` and `pages/analytics.css`.

---

## 2. Logic Chain

1. **Tokens Refactoring**:
   - *Observation*: `tokens.css:60-64` defines `--radius-1: 6px`, `--radius-2: 10px`, `--radius-3: 14px`, and `tokens.css:256` defines `--badge-radius: var(--radius-pill)`.
   - *Reasoning*: Midnight Ledger design system requires sharp 2px control radii and 4px/6px container corners. Updating `--radius-1: 2px;`, `--radius-2: 4px;`, `--radius-3: 6px;`, and `--badge-radius: var(--radius-1);` propagates sharp geometry across controls, buttons, forms, tables, cards, and badges automatically without breaking dependent tokens (`--radius-md`, `--radius-lg`, `--card-radius`, `--btn-radius`, `--input-radius`).

2. **Typography & Monospace Utility Refactoring**:
   - *Observation*: `utilities.css:285-294` sets tabular number features but leaves `font-family` unspecified, while `.font-mono` is missing from `utilities.css`.
   - *Reasoning*: Adding `.font-mono { font-family: var(--font-mono); }` and adding `font-family: var(--font-mono);` directly to `.numeric` and `.numeric-right` ensures tabular numeric formatting (numbers, amounts, currencies) always renders in monospaced font across all browsers and templates.

3. **Legacy CSS File Consolidation**:
   - *Observation*: `close.css` and `analytics.css` sit outside `main.css` bundles and use un-tokenized colors and soft radii. They are referenced via `<link rel="stylesheet">` tags in `periods.html`, `run.html`, and `dashboard.html`.
   - *Reasoning*: Moving them to `web/static/css/pages/close.css` and `web/static/css/pages/analytics.css`, replacing hardcoded hex/pico values with Midnight Ledger tokens, adding them to `main.css` `@import` declarations, and updating the template link references completes full tokenization of Milestone 1 CSS architecture.

---

## 3. Caveats

- **No source code was modified during this investigation**: This report and `analysis.md` provide read-only specifications for the implementer agent.
- **Template dependencies**: `periods.html`, `run.html`, and `dashboard.html` `<link>` tags must be updated in sync with creating `pages/close.css` and `pages/analytics.css` to prevent browser 404 console errors.

---

## 4. Conclusion

Milestone 1 (M1_CSS) has a complete, self-contained, line-by-line implementation spec detailed in `/home/noah/project/odyssey-erp/.agents/explorer_m1_css_1/analysis.md`. Implementing these changes will successfully establish Midnight Ledger design tokens, global monospace numeric utilities, and modular BEM page stylesheets across the codebase.

---

## 5. Verification Method

To independently verify the implementation after code changes:
1. **File Inspection**:
   - Check `web/static/css/core/tokens.css` lines 59-68 and 255-257.
   - Check `web/static/css/core/utilities.css` lines 282-295.
   - Check creation of `web/static/css/pages/close.css` and `web/static/css/pages/analytics.css`.
   - Check `@import` lines in `web/static/css/main.css`.
   - Check template link updates in `periods.html`, `run.html`, and `dashboard.html`.
2. **Build Verification**:
   ```bash
   make build
   ```
3. **Test Suite Verification**:
   ```bash
   ODYSSEY_TEST_MODE=1 go test ./...
   ```
