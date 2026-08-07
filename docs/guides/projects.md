# Projects and Timesheets

## Current status

**Partial.** The Horizon foundation supports company-scoped projects, tasks, members, timesheets, and FX snapshots. The core logic is in `internal/projects/` and the public API is documented in [`integrations.md`](integrations.md).

## Supported scope

- **Projects & Tasks:** Project creation, company isolation, and hierarchical task assignment.
- **Members:** Project membership validation for timesheet entry.
- **Timesheets:** Time tracking at the project/task level with hours, descriptions, and billable flags.
- **Workflow & Approvals:** A complete timesheet state machine (DRAFT -> SUBMITTED -> APPROVED/REJECTED -> LOCKED) involving employees and project managers.
- **Financial Integration:** Timesheet persistence with explicit currency definitions, base amounts, and FX snapshot rate locking.
- **Idempotency:** Idempotent project creation through the public API.

## Gaps

Milestones, Kanban boards, Gantt charts, project budgets, comprehensive project cost accounting, and resource-capacity allocation are not currently supported. Their ownership, lifecycle, integration, and delivery milestones are defined in the [`Product Workflow Depth Execution Plan`](product-workflow-depth-plan.md).
