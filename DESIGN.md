# Odyssey ERP Design System

## 1. Atmosphere & Identity

Odyssey is a quiet operational command center. It prioritizes dense, repeatable finance and logistics work with sharp industrial geometry, restrained authority-blue actions, tabular financial data, and clear document status.

## 2. Color

All UI colors come from `web/static/css/core/tokens.css`. Components use semantic tokens such as `--bg-app`, `--bg-surface`, `--text-primary`, `--text-secondary`, `--border-subtle`, `--brand`, `--success-600`, `--warning-600`, and `--error-600`. Light and dark theme values are defined in that file; raw palette tokens are not used directly in templates.

## 3. Typography

The primary stack is `--font-sans`; finance values use `--font-mono` or tabular numeric settings. The active scale is `--text-xs` 12px, `--text-sm` 13px, `--text-md` 14px, `--text-lg` 16px, `--text-xl` 20px, and `--text-2xl` 22px. Body copy does not drop below 13px and letter spacing remains zero except existing uppercase table headings.

## 4. Spacing & Layout

Spacing follows the existing 4px token scale from `--space-1` through `--space-16`. Authenticated pages use the application shell and a constrained page container up to `--app-max` 1440px. Operational forms use responsive grids; document lines and financial values use horizontally scrollable tables.

## 5. Components

### Page Header
- Structure: eyebrow/status, document title, concise metadata, command actions.
- States: responsive wrapping; actions remain keyboard reachable.

### Card
- Structure: optional header, body, and footer; cards are never nested.
- Variants: default, muted, floating, small, large.
- Surface: semantic background, subtle border, 6px maximum radius.

### Button
- Variants: primary, secondary, ghost, danger, icon, small, large.
- States: hover, active, focus-visible, disabled, loading.
- Motion: tokenized transform/opacity/color transitions only.

### Form Controls
- Structure: label, input/select/textarea, help or error text.
- States: hover, focus-visible, invalid, disabled.
- Accessibility: explicit labels and semantic input types.

### Data Table
- Structure: uppercase compact header, tabular numeric columns, hover row, empty state.
- Behavior: horizontal overflow on narrow screens; actions use links or buttons.

### Status Badge
- Variants: neutral, info, warning, success, error derived from document state.
- Accessibility: status is always present as text, not color alone.

## 6. Motion & Interaction

Motion uses `--dur-1` 120ms, `--dur-2` 180ms, or `--dur-3` 240ms with `--ease-standard`. Only transform, opacity, colors, borders, and shadows transition. `prefers-reduced-motion` disables transitions globally.

## 7. Depth & Surface

The strategy is mixed but restrained: borders define most surfaces; `--shadow-1` is reserved for controls or lightly elevated content and stronger shadows for overlays. Page sections remain unframed; cards represent actual documents, tools, or repeated records.

## 8. Accessibility Constraints & Accepted Debt

Target WCAG 2.2 AA. Every control has a visible focus state, forms have labels, status is textual, tables have accessible labels, and layouts remain usable from 375px upward.

Accepted debt: the current embedded SSR component set has no isolated primitive showcase route; existing page and template render tests are the state harness until a dedicated design-system catalog is added.
