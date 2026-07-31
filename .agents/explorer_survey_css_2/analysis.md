# CSS Architecture & Design Token Audit Report

**Target System**: Odyssey ERP — Midnight Ledger Industrial Enterprise Design System  
**Auditor**: Explorer 2 (CSS Architecture & Token Audit Specialist)  
**Date**: 2026-07-29  
**Working Directory**: `/home/noah/project/odyssey-erp/.agents/explorer_survey_css_2`

---

## 1. Executive Summary

This audit evaluates the CSS architecture in `web/static/css/` across `core/`, `components/`, `layout/`, and `pages/` for 100% compliance with the **Midnight Ledger** design system.

### Core Key Findings:
1. **Border Radii Inconsistency (Soft SaaS vs. Industrial 2px)**:
   - `web/static/css/core/tokens.css` currently defines soft SaaS radii (`--radius-1: 6px;`, `--radius-2: 10px;`, `--radius-3: 14px;`, `--badge-radius: var(--radius-pill);`).
   - Per Requirement **R2**, Midnight Ledger specifies sharp **2px** border radii (`var(--radius-1: 2px)`). Radii scales must be refactored (`--radius-1: 2px; --radius-2: 4px; --radius-3: 6px;`).
2. **Missing Global `.font-mono` Utility**:
   - `.font-mono` is defined locally inside `web/static/css/pages/landing.css` (line 42) using non-standard variable `--dev-mono`.
   - Domain templates calling `.font-mono` on table amounts or reference codes fail to render monospace text unless `landing.css` is loaded. It MUST be moved to `web/static/css/core/utilities.css`.
   - `.numeric` and `.numeric-right` in `core/utilities.css` specify `font-variant-numeric: tabular-nums lining-nums`, but omit `font-family: var(--font-mono)`.
3. **State Badge Architecture Divergence**:
   - `web/static/css/components/misc.css` defines industrial terminal badges (`.sys-badge`) with `font-family: var(--font-mono)` and sharp 2px radii, but standard `.badge` uses rounded pill radii (`var(--radius-pill)` / 9999px).
   - `.status-badge` in loose `web/static/css/close.css` uses hardcoded soft colors (`#e0f2fe`, `#fef3c7`, `#d1fae5`, `#fee2e2`), rounded pill radii (`border-radius: 999px`), and Pico CSS fallback variables (`var(--pico-muted-color)`).
4. **Un-tokenized Loose CSS Files**:
   - `web/static/css/close.css` and `web/static/css/analytics.css` sit outside `main.css` bundles, contain hardcoded hex colors (`#b42318`, `#6b7280`, `#075985`, etc.) and soft `rem` border radii (`0.75rem`, `0.85rem`).

---

## 2. Comprehensive Token Audit (`web/static/css/core/tokens.css`)

| Token Category | Current Value in `tokens.css` | Required Midnight Ledger Value | Compliance Assessment & Required Refactor |
|---|---|---|---|
| `--radius-1` | `6px` | **`2px`** | **NON-COMPLIANT**. Controls & sharp containers require 2px. |
| `--radius-2` | `10px` | **`4px`** | **NON-COMPLIANT**. Inputs & buttons should be 4px (or 2px). |
| `--radius-3` | `14px` | **`6px`** | **NON-COMPLIANT**. Panels/cards should be 6px sharp industrial boxes. |
| `--badge-radius` | `var(--radius-pill)` (9999px) | **`var(--radius-1)`** (2px) | **NON-COMPLIANT**. State badges must be sharp rectangular pills. |
| `--card-radius` | `var(--radius-3)` (14px) | **`var(--radius-1)`** or **`var(--radius-2)`** | **NON-COMPLIANT**. Soft card corners destroy industrial aesthetic. |
| `--btn-radius` | `var(--radius-2)` (10px) | **`var(--radius-1)`** (2px) | **NON-COMPLIANT**. Buttons should have sharp 2px corners. |
| `--input-radius` | `var(--radius-2)` (10px) | **`var(--radius-1)`** (2px) | **NON-COMPLIANT**. Form inputs should have sharp 2px corners. |
| `--table-radius` | `var(--radius-3)` (14px) | **`var(--radius-1)`** (2px) | **NON-COMPLIANT**. Data tables should have sharp 2px container border. |
| `--modal-radius` | `var(--radius-3)` (14px) | **`var(--radius-2)`** (4px) | **NON-COMPLIANT**. Dialogs should be compact and sharp. |
| `--font-sans` | `ui-sans-serif, system-ui...` | Inter, system-ui | COMPLIANT. |
| `--font-mono` | `ui-monospace, SFMono-Regular...` | SFMono-Regular, Consolas, monospace | COMPLIANT. |
| `--numeric` | `tabular-nums lining-nums` | `tabular-nums lining-nums` | COMPLIANT. |

---

## 3. Core Utilities Audit (`web/static/css/core/utilities.css`)

### Issues Identified:
1. **Missing `.font-mono`**:
   `core/utilities.css` has `.font-normal`, `.font-medium`, `.font-semibold`, `.font-bold`, but lacks `.font-mono`.
   *Fix*: Add `.font-mono { font-family: var(--font-mono); }`.
2. **Incomplete `.numeric` Utility**:
   Currently `core/utilities.css:285-294` defines:
   ```css
   .numeric {
       font-variant-numeric: var(--numeric);
       font-feature-settings: "tnum" 1, "lnum" 1;
   }
   ```
   *Fix*: Update to explicitly include `font-family: var(--font-mono)` so both font family and tabular formatting are guaranteed:
   ```css
   .numeric {
       font-family: var(--font-mono);
       font-variant-numeric: var(--numeric);
       font-feature-settings: "tnum" 1, "lnum" 1;
   }
   ```

---

## 4. Component BEM Architecture Audit (`web/static/css/components/`)

### 4.1 Buttons (`components/buttons.css`)
- Uses `var(--btn-radius)`, which currently resolves to 10px.
- Updating `--btn-radius: var(--radius-1);` (2px) automatically brings all `.btn`, `.btn--primary`, `.btn--secondary`, `.btn--sm`, `.btn--lg` into compliance.

### 4.2 Cards (`components/cards.css`)
- Uses `var(--card-radius)` (currently 14px).
- `.card` header, footer, title follow clear BEM conventions (`.card__header`, `.card__title`, `.card__desc`, `.card__footer`).
- Updating `--card-radius` to `var(--radius-1)` or `var(--radius-2)` aligns with high-density industrial surfaces.

### 4.3 Tables (`components/tables.css`)
- `table-wrap` and `.table` follow high-density enterprise formatting.
- Headings (`thead th`) use uppercase 11px/12px text.
- Numeric columns use `.numeric` and `.numeric-right`.
- `.table--compact` padding (`8px 10px`) is well-suited for high-density financial data.

### 4.4 Form Controls (`components/forms.css`)
- Uses `var(--input-radius)` (currently 10px).
- Radii should be updated to `var(--radius-1)` (2px).
- Hardcoded invalid state shadow `box-shadow: 0 0 0 3px rgba(180, 35, 24, 0.12)` should use token `--error-600` opacity or `--focus-ring` token variant.

### 4.5 State Badges (`components/misc.css`)
- `.sys-badge` (lines 102-150) is the canonical Midnight Ledger industrial badge component:
  ```css
  .sys-badge {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      font-family: var(--font-mono);
      font-size: 11px;
      font-weight: var(--weight-semibold);
      letter-spacing: 0.04em;
      padding: 2px 7px;
      border-radius: var(--radius-1, 2px);
      border: 1px solid var(--border-subtle);
      background: var(--bg-surface-muted);
      color: var(--text-muted);
      text-transform: uppercase;
      white-space: nowrap;
  }
  ```
- Standard `.badge` and `.status-badge` currently use `var(--badge-radius)` (pill shape). Updating `--badge-radius` to `var(--radius-1)` enforces sharp rectangular industrial badges across all state indicators.

---

## 5. Audit of Out-of-Spec & Legacy CSS Files

### 5.1 `web/static/css/close.css`
- **Status**: Orphaned/legacy CSS file linked directly in `web/templates/pages/close/periods.html` and `run.html`.
- **Violations**:
  - `var(--pico-muted-color, #6b7280)` (Pico CSS remnants)
  - `var(--pico-card-background-color, #ffffff)`
  - `border-radius: 0.5rem`, `border-radius: 0.85rem`, `border-radius: 999px`
  - Hardcoded hex colors (`#b42318`, `#e0f2fe`, `#075985`, `#fef3c7`, `#92400e`, `#d1fae5`, `#065f46`, `#fee2e2`, `#b91c1c`)
- **Recommendation**: Refactor to `web/static/css/pages/close.css`, replace all hardcoded values with design tokens, update `.status-badge` to use `.sys-badge` / `.status-badge` BEM tokens, and import `pages/close.css` into `main.css`.

### 5.2 `web/static/css/analytics.css`
- **Status**: Orphaned CSS file linked in `web/templates/pages/finance/dashboard.html`.
- **Violations**: `border-radius: 0.75rem;` (12px), un-tokenized grid and typography rules.
- **Recommendation**: Refactor into `web/static/css/pages/analytics.css`, convert radii to `var(--radius-1)`, replace hardcoded rem margins/padding with `--space-*` tokens, and import in `main.css`.

### 5.3 `web/static/css/pages/login.css`
- **Status**: Imported in `main.css`.
- **Violations**: `border-radius: 16px;`, `border-radius: 12px;`, `border-radius: 10px;` on login card, input, and feature items.
- **Recommendation**: Refactor card and input radii to use `var(--radius-2)` (4px) / `var(--radius-3)` (6px) to maintain Midnight Ledger crisp edges.

---

## 6. Proposed Code Changes (Patch Proposal)

### Proposal 1: Refactor `web/static/css/core/tokens.css` Border Radii Scale
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
    /* controls / sharp badges / tables */
    --radius-2: 4px;
    /* buttons / inputs / sub-containers */
    --radius-3: 6px;
    /* cards / dialogs / panels */
    --radius-pill: 9999px;
    --radius-md: var(--radius-2);
    --radius-lg: var(--radius-3);
    --badge-radius: var(--radius-1);
>>>>
```

### Proposal 2: Add `.font-mono` & Monospace Tabular Numerics to `web/static/css/core/utilities.css`
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

## 7. Verification Plan for Implementers

1. **Token Verification**: Verify that changing `--radius-1` to `2px` in `tokens.css` updates buttons, inputs, badges, and cards across all domain pages without breaking layouts.
2. **Build Verification**: Run `make build` to verify Go binaries compile cleanly.
3. **Test Suite Verification**: Run `ODYSSEY_TEST_MODE=1 go test ./...` to ensure no template parsing errors or handler tests fail.
