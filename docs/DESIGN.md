# Odyssey ERP Design System

This document outlines the design philosophy, design tokens, CSS architecture, component library, and interaction patterns used in the Odyssey ERP application.

## Design Philosophy

The Odyssey ERP interface is built on the following principles:

1. **Dark-Themed & Premium**: A modern, dark aesthetic designed for extended daily use, reducing eye strain while maintaining high contrast.
2. **SSR-First with Progressive Enhancement**: Core functionality works with standard HTML/HTTP. JavaScript (HTMX, Alpine.js) is layered on top to provide a snappy, SPA-like feel without SPA complexity.
3. **WCAG 2.2 AA Accessibility**: Commitment to keyboard navigability, screen reader support, and adequate color contrast.
4. **Strict Separation of Concerns**: Clean boundaries between HTML (structure), CSS (presentation), and JS (behavior). No inline styles or scripts in templates.

## Design Tokens

Our design tokens are defined in `web/static/css/main.css` as CSS variables (`--var-name`).

### Colors

*   **Primary Brand:** `#6C63FF` (rich purple) - Used for primary actions, active states, and emphasis.
*   **Surfaces:**
    *   `--bg-darkest`: `#0F1117` - App background.
    *   `--bg-sidebar`: `#1A1D27` - Sidebar navigation.
    *   `--bg-surface`: `#242836` - Card surfaces, modals, dropdowns.
    *   `--border-color`: `#2D3244` - Input borders, table dividers, card outlines.
*   **Status & Feedback:**
    *   `--color-success`: `#10B981`
    *   `--color-warning`: `#F59E0B`
    *   `--color-danger`: `#EF4444`
    *   `--color-info`: `#3B82F6`
*   **Text & Typography:**
    *   `--text-primary`: `#FFFFFF` - Headings, primary body text.
    *   `--text-secondary`: `#94A3B8` - Muted text, labels, secondary info.
    *   `--text-tertiary`: `#64748B` - Disabled states, placeholders.

### Typography

*   **Font Family:** Inter, with standard system fallbacks (sans-serif).
*   **Weights:** Normal (400), Medium (500), Semibold (600), Bold (700).
*   **Numeric Data:** Uses `font-variant-numeric: tabular-nums;` for alignment in tables and financial reports.

### Spacing

Based on a 4px grid.
*   `--space-1`: 0.25rem (4px)
*   `--space-2`: 0.5rem (8px)
*   `--space-3`: 0.75rem (12px)
*   `--space-4`: 1rem (16px)
*   `--space-6`: 1.5rem (24px)
*   `--space-8`: 2rem (32px)
*   `--space-12`: 3rem (48px)

### Borders & Radii

*   `--radius-sm`: 0.375rem
*   `--radius-md`: 0.5rem
*   `--radius-lg`: 0.75rem
*   `--radius-xl`: 1rem
*   `--radius-full`: 9999px

### Transitions & Animation

*   `--transition-fast`: 150ms ease
*   `--transition-base`: 200ms ease
*   `--transition-slow`: 300ms ease
*   *Note: Animations respect the `prefers-reduced-motion` media query.*

### Shadows

Subtle dark shadows to create depth between overlapping elements (e.g., modals, dropdowns).

## CSS Architecture

Styles are modularized within `web/static/css/`:

*   `main.css` — Core tokens, resets, typography, layout fundamentals, and base components (buttons, badges).
*   `sidebar.css` — Sidebar navigation layout and interactions.
*   `modal.css` — Modal dialogs and overlays.
*   `table.css` — Data tables, pagination, and sorting indicators.
*   `forms.css` — Form inputs, labels, validation states.
*   `kanban.css` — Kanban boards and drag-and-drop styles.
*   `dropdown.css` — Context menus and dropdowns.
*   `dashboard.css` — Dashboard widgets and KPI cards.
*   `calendar.css` — Calendar grid and event styles.
*   `governance.css` — Governance module specifics.
*   `reports.css` — Reports module specifics.
*   `toasts.css` — Toast notifications.
*   `print.css` — Print-specific media queries.

## Component Library

### Cards

Used to group related information.

```html
<div class="card">
    <div class="card-header">
        <h3 class="card-title">Card Title</h3>
    </div>
    <div class="card-body">
        <p>Card content goes here.</p>
    </div>
    <div class="card-footer">
        <button class="btn btn-secondary">Cancel</button>
        <button class="btn btn-primary">Save</button>
    </div>
</div>
```

### Buttons

```html
<button class="btn btn-primary">Primary Action</button>
<button class="btn btn-secondary">Secondary Action</button>
<button class="btn btn-danger">Destructive Action</button>
<button class="btn btn-primary btn-sm">Small Button</button>
```

### Forms

```html
<div class="form-group">
    <label class="form-label" for="username">Username</label>
    <input type="text" class="form-input" id="username" name="username" required>
    <div class="form-error">This field is required.</div>
</div>

<div class="form-group">
    <label class="form-label" for="status">Status</label>
    <select class="form-select" id="status" name="status">
        <option>Active</option>
        <option>Inactive</option>
    </select>
</div>
```

### Data Tables

```html
<table class="data-table">
    <thead>
        <tr>
            <th>ID</th>
            <th>Name</th>
            <th>Status</th>
            <th>Actions</th>
        </tr>
    </thead>
    <tbody>
        <tr>
            <td>1001</td>
            <td>Widget A</td>
            <td><span class="badge badge-success">Active</span></td>
            <td>
                <a href="#" class="btn btn-sm btn-secondary">Edit</a>
            </td>
        </tr>
    </tbody>
</table>
```

### Badges

```html
<span class="badge badge-success">Approved</span>
<span class="badge badge-warning">Pending</span>
<span class="badge badge-danger">Rejected</span>
<span class="badge badge-info">Processing</span>
```

### Modals

Modal state is managed by Alpine.js (`x-data="{ open: false }"`).

```html
<div class="modal-overlay" x-show="open">
    <div class="modal-content" @click.away="open = false">
        <div class="modal-header">
            <h3>Confirm Action</h3>
        </div>
        <div class="modal-body">
            <p>Are you sure you want to proceed?</p>
        </div>
    </div>
</div>
```

### Toasts

Slide-in notifications that auto-dismiss. Handled via partial templates and minimal JS.

## Layout System & Template Hierarchy

*   **Layout:** A persistent sidebar (260px wide, collapsing to 70px) and a main content area.
*   **Template Hierarchy:**
    *   `base.html` - The outermost shell (HTML head, script includes).
    *   `authenticated.html` - Extends `base.html`. Includes the sidebar, top navigation, and flash messages.
    *   `public.html` - Extends `base.html`. Used for login/auth pages.
    *   `pages/*` - Individual page templates (e.g., `dashboard.html`, `sales/orders.html`) extending the appropriate layout.
*   **Partials:** Reusable snippets located in `partials/` (e.g., `_sidebar.html`, `_pagination.html`, `_flash_messages.html`).

## Interactivity Patterns

*   **HTMX:** Used for form submissions, partial page updates, and inline editing without full page reloads.
    *   *Example:* `<button hx-delete="/api/item/1" hx-target="#item-1" hx-swap="outerHTML">Delete</button>`
*   **Alpine.js:** Used for ephemeral UI state (dropdowns, modals, sidebar toggling, tabs).
    *   *Example:* `<div x-data="{ open: false }"><button @click="open = true">Open</button>...</div>`
*   **Chart.js:** Utilized for rendering dashboards and financial reports.
*   **SortableJS:** Provides drag-and-drop functionality for Kanban boards and list reordering.
*   **Vanilla JS:** Small, custom modules for behaviors that exceed Alpine's sweet spot.

## Navigation Structure

The sidebar is organized into core functional modules:
*   Dashboard
*   Sales (4 items)
*   Procurement (3 items)
*   Inventory (4 items)
*   Finance (6 items)
*   Reports
*   Governance (9 items)
*   Admin (1 item)

## Responsive Behavior

*   **Mobile:** The sidebar is hidden by default and accessible via a hamburger menu. Dashboard widgets stack into a single column.
*   **Tables:** Complex data tables gain a horizontal scrollbar on small viewports rather than breaking layout.
*   **Forms:** Inputs span 100% width on mobile, moving to multi-column grids on desktop.

## Accessibility Standards

*   **Semantic HTML:** Strict use of `<nav>`, `<main>`, `<header>`, `<footer>`, `<section>`, and correct heading hierarchy.
*   **Forms:** All `<input>` elements must have an associated `<label>`.
*   **Focus:** Explicit focus states are maintained for keyboard navigability.
*   **Contrast:** The dark theme is specifically tuned to meet WCAG AA contrast ratios.
*   **Current Gaps:** Limited ARIA attributes on complex custom components. *Note: A skip-to-content link should be added in future iterations.*

## Print Styles

`print.css` is used to strip out navigation, sidebar, and interactive elements when printing invoices, reports, or data tables, ensuring a clean, black-and-white optimized output.

## Guidelines for Contributors

1.  **No inline styles or scripts.** Keep HTML templates clean.
2.  **Use design tokens.** Do not hardcode colors, spacing, or fonts in CSS. Always reference variables from `main.css`.
3.  **Modular CSS.** Only create a new CSS file if styling a distinct, complex module. Otherwise, use existing component classes.
4.  **Leverage Partials.** Extract repeated UI elements into the `partials/` directory.
5.  **Standard Forms.** Rely on standard POST requests and server-side validation. Use HTMX for progressive enhancement only where it adds clear user value.
