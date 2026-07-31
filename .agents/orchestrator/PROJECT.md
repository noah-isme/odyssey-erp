# Project: Odyssey ERP Midnight Ledger UI Refactor

## Architecture
- Modular monolith Go backend serving HTML templates rendered via `internal/view/templates.go` engine.
- CSS architecture: `web/static/css/core/tokens.css` (design tokens), `web/static/css/core/utilities.css` (global utility classes), `web/static/css/components/` (component stylesheets, BEM badges, buttons, tables), `web/static/css/pages/` (domain-specific page stylesheets), bundled into `web/static/css/main.css`.
- UI standards: Midnight Ledger high-density industrial enterprise design system (sharp 2px border radius `var(--radius-1)`, monospace tabular typography `var(--font-mono)`, `.font-mono`, `.numeric`, `.numeric-right`, standardized BEM state badges `.sys-badge`, `.status-badge`).

## Feature Inventory
| # | Feature | Description | Milestone | Source |
|---|---------|-------------|-----------|--------|
| 1 | CSS Tokens & Core Radii Scale | Update `tokens.css` radii: `--radius-1: 2px; --radius-2: 4px; --radius-3: 6px; --badge-radius: var(--radius-1);` | M1_CSS (DONE) | survey_css_2 |
| 2 | CSS Typography Utilities | Add `.font-mono { font-family: var(--font-mono); }` and update `.numeric` / `.numeric-right` with `font-family: var(--font-mono)` in `core/utilities.css` | M1_CSS (DONE) | survey_css_2 |
| 3 | CSS Legacy File Consolidation | Refactor `close.css` and `analytics.css` into BEM page stylesheets (`pages/close.css`, `pages/analytics.css`) and bundle into `main.css` | M1_CSS (DONE) | survey_css_2 |
| 4 | CSS Component Badges | Ensure standardized industrial state badges (`.sys-badge`, `.status-badge`) in `misc.css` | M1_CSS (DONE) | survey_css_2 |
| 5 | Sales Domain UI Refactoring | Refactor `web/templates/pages/sales/` (orders, quotations, customers) for 2px radii, `.font-mono` doc numbers, `.numeric` amounts, `.sys-badge` | M2_Sales_Procurement_Delivery | survey_templates_1 |
| 6 | Procurement Domain UI Refactoring | Refactor `web/templates/pages/procurement/` (POs, GRNs, vendors) removing inline styles, rounded borders, setting `.font-mono` / `.numeric` | M2_Sales_Procurement_Delivery | survey_templates_1 |
| 7 | Delivery Domain UI Refactoring | Refactor `web/templates/pages/delivery/` removing soft borders (`rounded-md`), inline margins/styles, adding `.font-mono` / `.numeric` | M2_Sales_Procurement_Delivery | survey_templates_1 |
| 8 | Accounting & Finance UI Refactoring | Refactor `web/templates/pages/accounting/`, `finance/`, `ap/`, `ar/`, `eliminations/`, `variance/`, `boardpacks/`, `consol/` for monospaced codes, tabular numbers, sharp borders | M3_Accounting_Finance_Close | survey_templates_1 |
| 9 | Close Module UI Refactoring | Refactor `web/templates/pages/close/` (`periods.html`, `run.html`, `bank_reconciliation.html`) removing Pico CSS fallbacks & hex colors | M3_Accounting_Finance_Close | survey_templates_1 |
| 10 | Inventory Domain UI Refactoring | Refactor `web/templates/pages/inventory/` (dashboard, stock, valuation) for monospaced SKUs, numeric quantities, sharp borders | M4_Inventory_MasterData_Auth_Layouts | survey_templates_1 |
| 11 | Master Data UI Refactoring | Refactor `web/templates/pages/masterdata/` (suppliers, companies, branches, warehouses, categories, units) for `.font-mono` codes and sharp radii | M4_Inventory_MasterData_Auth_Layouts | survey_templates_1 |
| 12 | Auth & Roles/Permissions UI Refactoring | Refactor `web/templates/pages/auth/`, `roles/`, `users/` for sharp 2px control edges and standardized BEM forms | M4_Inventory_MasterData_Auth_Layouts | survey_templates_1 |
| 13 | Layouts, Home, Landing & Partials UI Refactoring | Refactor `web/templates/pages/home.html`, `landing.html`, `web/templates/partials/sidebar.html`, `flash.html` removing inline styles and non-standard badges | M4_Inventory_MasterData_Auth_Layouts | survey_templates_1 |

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|------|-------|-------------|--------|
| 1 | M1_CSS | Update core tokens (`tokens.css`), typography utilities (`utilities.css`), consolidate legacy `close.css` & `analytics.css` into BEM `main.css` | none | DONE |
| 2 | M2_Sales_Procurement_Delivery | Refactor Sales, Procurement, and Delivery domain page templates in `web/templates/pages/{sales,procurement,delivery}/` | M1_CSS | IN_PROGRESS |
| 3 | M3_Accounting_Finance_Close | Refactor Accounting, Finance, AP, AR, Close, Eliminations, Variance, Boardpacks, and Consol page templates | M1_CSS | PLANNED |
| 4 | M4_Inventory_MasterData_Auth_Layouts | Refactor Inventory, Master Data, Auth, Roles, Users, Home, Landing, Layouts & Partials templates | M1_CSS | PLANNED |

## Code Layout
- CSS Core & Components: `web/static/css/core/`, `web/static/css/components/`, `web/static/css/pages/`, `web/static/css/main.css`
- Sales / Procurement / Delivery Templates: `web/templates/pages/sales/`, `web/templates/pages/procurement/`, `web/templates/pages/delivery/`
- Accounting / Finance / Close Templates: `web/templates/pages/accounting/`, `web/templates/pages/finance/`, `web/templates/pages/ap/`, `web/templates/pages/ar/`, `web/templates/pages/close/`, `web/templates/pages/eliminations/`, `web/templates/pages/variance/`, `web/templates/pages/boardpacks/`, `web/templates/pages/consol/`
- Inventory / Master Data / Auth / Layouts Templates: `web/templates/pages/inventory/`, `web/templates/pages/masterdata/`, `web/templates/pages/auth/`, `web/templates/pages/roles/`, `web/templates/pages/users/`, `web/templates/pages/home.html`, `web/templates/pages/landing.html`, `web/templates/partials/`, `web/templates/layouts/`
