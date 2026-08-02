# Approval Engine and HR Core

The shared approval capability has two functional layers: the approval engine
and HR Core consumers. Payroll is documented separately.

## Configurable approvals

Migration `000043_approval_engine` adds policies, ordered steps, requests,
assignments, decisions, and time-bounded delegations. The legacy `approvals`
table remains an audit compatibility log for existing modules.

Policy resolution uses:

1. module (`PO`, `LEAVE`, or another registered module);
2. exact company before a company-neutral fallback;
3. matching minimum/maximum amount range, preferring the highest matching
   minimum threshold.

Steps can resolve a named user, all active users in a role, or—after migration
`000044`—the requesting employee's manager. Active delegations replace the
original assignee while retaining `delegated_from` on the assignment.

The `/approvals` inbox exposes only the authenticated user's actionable current
step. `/approvals/policies` manages ranges and up to two ordered steps through
the initial SSR interface. RBAC permissions are:

- `approvals.inbox`
- `approvals.policy.admin`
- `approvals.delegate`

PO submission now calculates its approval amount from `qty × price`, resolves a
policy, and creates assignments. Approval advances the current step; only the
last required step sets the PO to `APPROVED`. Rejection cancels the PO.

Notifications are emitted for assignment, overdue escalation, final approval,
and rejection. Escalation currently re-alerts overdue current-step assignees
and advances their due timestamp by 24 hours.

## Release B: HR Core

Migration `000044_hr_core` adds:

- departments, positions, employees, linked user accounts, and manager
  relationships;
- leave types, annual balances, and leave requests;
- attendance import batches and daily attendance records.

The HR modules are intentionally separated:

- `internal/hr/employees` — employee directory and employee creation;
- `internal/hr/leave` — self-service requests, balance reservation, and approval
  finalization;
- `internal/hr/attendance` — CSV parsing, validation, import batches, and
  idempotent daily upserts.

Routes:

| Route | Permission | Purpose |
|---|---|---|
| `/hr/employees` | `hr.employee.view` or `hr.employee.admin` | Employee directory. |
| `/hr/leave` | `hr.leave.request` or `hr.leave.admin` | Own leave requests. |
| `/hr/attendance` | `hr.attendance.import` | CSV import and batch results. |

### Leave setup

Before employees request leave:

1. Link the employee and their manager to active Odyssey user accounts.
2. Create a leave type and annual `hr_leave_balances` row.
3. Create an active `LEAVE` policy in `/approvals/policies`; choose
   **Employee manager** as its first-step selector.
4. Grant employees `hr.leave.request` and managers `approvals.inbox` through
   their roles.

On submission, requested days move into `pending`. Manager approval moves those
days from `pending` to `used`; rejection releases `pending`. Finalization writes
an `audit_logs` record before the approval-completion notification is sent.

### Attendance CSV

Required headers are `employee_number,date`. Optional headers are
`check_in,check_out,status`. Dates use `YYYY-MM-DD`; timestamps accept RFC3339
or `YYYY-MM-DD HH:MM`. Status is `PRESENT`, `ABSENT`, or `LEAVE`.

Invalid rows are recorded on the import batch. Valid rows are upserted by
employee and attendance date. Direct time-clock integrations remain out of
scope.

## Planned talent-workflow depth

Recruitment, candidate and offer workflows, structured performance cycles, and
training/certification management are not currently supported. The implementation
boundary, lifecycles, permissions, privacy controls, and rollout plan are defined in
the [`Product Workflow Depth Execution Plan`](product-workflow-depth-plan.md).

## Verification

```bash
ODYSSEY_TEST_MODE=1 GOTENBERG_URL=http://127.0.0.1:0 \
  go test ./internal/approvals ./internal/hr/... ./internal/procurement
```

The approval tests cover different PO policies below and above an amount
threshold, delegation, and the leave flow from manager assignment through
balance finalization, audit callback, and notifications.
