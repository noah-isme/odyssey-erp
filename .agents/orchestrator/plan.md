# Master Execution Plan: Odyssey ERP Midnight Ledger UI Refactor

## Objective
Eliminate all remaining AI design slop across Odyssey ERP pages, establishing high-density Midnight Ledger industrial enterprise aesthetics across all remaining application templates (`web/templates/pages/`).

## Strategy & Pattern
We follow the **Project Orchestration Pattern**:
1. **Phase 0: Survey** — COMPLETED. 3 parallel Explorers audited templates, CSS tokens, and build/test integrity.
2. **Phase 1: Feature Inventory & Milestone Decomposition** — COMPLETED. Synthesized findings into `.agents/orchestrator/PROJECT.md`.
3. **Phase 2: Milestone Iteration Loop Execution (Explorer -> Worker -> Reviewer -> Challenger -> Auditor -> Gate)**
   - **Milestone 1 (M1_CSS)**: Core Design Tokens, Typography Utilities, Legacy `close.css`/`analytics.css` consolidation. — **DONE (Gate Result PASS)**
   - **Milestone 2 (M2_Sales_Procurement_Delivery)**: Sales, Procurement, and Delivery HTML templates. — **IN_PROGRESS**
   - **Milestone 3 (M3_Accounting_Finance_Close)**: Accounting, Finance, AP, AR, Close, Eliminations, Variance, Boardpacks, and Consol HTML templates. — **PLANNED**
   - **Milestone 4 (M4_Inventory_MasterData_Auth_Layouts)**: Inventory, Master Data, Auth, Roles/Permissions, Users, Home/Landing, Layouts & Partials. — **PLANNED**
4. **Phase 3: Final Verification & Audit** — System-wide forensic audit, compilation check (`make build`), and complete test suite validation (`ODYSSEY_TEST_MODE=1 go test ./...`).

## Schedule & Milestones
- Phase 0: Survey — DONE
- Phase 1: Feature Inventory & Decomposition — DONE
- Milestone 1 (M1_CSS) — DONE (Gate Result: PASS)
- Milestone 2 (M2_Sales_Procurement_Delivery) — IN_PROGRESS
- Milestone 3 (M3_Accounting_Finance_Close) — PLANNED
- Milestone 4 (M4_Inventory_MasterData_Auth_Layouts) — PLANNED
