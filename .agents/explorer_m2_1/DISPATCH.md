## 2026-07-29T08:45:11Z
You are explorer_m2_1, a teamwork_preview_explorer subagent.
Your working directory is /home/noah/project/odyssey-erp/.agents/explorer_m2_1 (metadata only, do NOT write source code here).

Context & Inputs:
- Read ORIGINAL_REQUEST: /home/noah/project/odyssey-erp/.agents/ORIGINAL_REQUEST.md
- Read PROJECT.md: /home/noah/project/odyssey-erp/.agents/orchestrator/PROJECT.md
- Read Template Survey Analysis: /home/noah/project/odyssey-erp/.agents/explorer_survey_templates_1/analysis.md

Objective:
Investigate and formulate the exact line-by-line implementation spec for Milestone 2 (M2_Sales_Procurement_Delivery):
1. `web/templates/pages/sales/` (orders, quotations, customers, list/detail/form/edit pages):
   - Replace all `rounded-md` / `rounded-lg` with sharp 2px tokenized classes.
   - Add `.font-mono` to document numbers (`.DocNumber`, `.Code`, order IDs).
   - Add `.numeric` / `.numeric-right` to monetary values, unit prices, totals.
   - Standardize state indicators to `.sys-badge` or `.status-badge`.
2. `web/templates/pages/procurement/` (POs, GRNs, vendors):
   - Remove inline styles (e.g. `style="display: none;"` context menus -> CSS classes).
   - Add `.font-mono` to PO numbers, GRN numbers, vendor codes.
   - Add `.numeric` / `.numeric-right` to purchase amounts and quantities.
   - Standardize state badges to `.sys-badge`.
3. `web/templates/pages/delivery/` (delivery orders list/detail/form/edit):
   - Remove `rounded-md` soft boxes (e.g., line-items, address blocks).
   - Add `.font-mono` to delivery order numbers, tracking numbers.
   - Add `.numeric` / `.numeric-right` to delivered quantities.
   - Standardize state badges to `.sys-badge`.

Deliverables:
1. Write detailed line-by-line refactoring plan to `/home/noah/project/odyssey-erp/.agents/explorer_m2_1/analysis.md`.
2. Write handoff report to `/home/noah/project/odyssey-erp/.agents/explorer_m2_1/handoff.md`.
3. Send message back to parent when completed.
