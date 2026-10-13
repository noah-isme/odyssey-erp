# Approval Engine and HR Core

The shared approval capability has two functional layers: the approval engine
and HR Core consumers. Payroll is documented separately.

## Configurable approvals

Migration `000043_approval_engine` adds policies, ordered steps, requests,
assignments, decisions, and time-bounded delegations. The legacy `approvals`
table remains an audit compatibility log for existing modules.

Policy resolution uses:

1. module (`PO`, `LEAVE`, `OVERTIME`, or another registered module);
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
| `/hr/employees/{id}` | `hr.employee.view` or `hr.employee.admin`; sensitive and compensation sections need the HRIS permissions below | Employee profile, emergency contacts, and job history. |
| `/hr/leave` | `hr.leave.request` or `hr.leave.admin` | Own leave requests. |
| `/hr/attendance` | `hr.attendance.import` | CSV import and batch results. |
| `/hr/holidays` | `hr.schedule.admin` | Company holiday calendar. |
| `/hr/shifts` | `hr.schedule.admin` | Shift definitions and employee shift assignment. |
| `/hr/overtime` | `hr.overtime.request` (own requests) or `hr.overtime.admin` (All requests tab, Retry apply) | Overtime requests. |
| `/hr/expenses` | `hr.expense.admin` | All expense claims, linked documents, Retry apply. |
| `/hr/profile-changes` | `hr.profile_change.admin` (payload masked without `hr.employee.sensitive`) | Profile change requests, Retry apply. |

### HRIS permissions

The HRIS expansion (migration `000130`) adds `hr.compensation.view`,
`hr.compensation.admin`, and `hr.employee.sensitive`, declared in
`internal/shared/authz_hr.go`. All new HR permissions are granted to Admin
only. Assign them to HR, IT, or manager roles explicitly; nothing else receives
them automatically.

| Permission | Grants | Default grant |
|---|---|---|
| `hr.compensation.view` | View employee compensation and bank details | Admin only |
| `hr.compensation.admin` | Change employee compensation and bank details | Admin only |
| `hr.employee.sensitive` | View and edit sensitive employee profile fields | Admin only |
| `hr.schedule.admin` | Manage holidays, shifts, and shift assignments (migration `000131`) | Admin only |
| `hr.overtime.request` | Submit and cancel own overtime requests (migration `000131`) | Admin only |
| `hr.overtime.admin` | View all overtime requests and Retry apply (migration `000131`) | Admin only |
| `hr.expense.admin` | View all expense claims and Retry apply (migration `000132`) | Admin only |
| `hr.profile_change.admin` | View all profile change requests and Retry apply (migration `000132`) | Admin only |

#### Job history and as-of semantics

`hr_employee_job_history` records department, position, manager, status,
employment type, and grade per employee over time. Each row has `valid_from`
and a nullable `valid_to`; `(employee_id, valid_from)` is unique and
`valid_to >= valid_from`. The current record is the row with `valid_to IS NULL`.
The state as of a date D is the row where `valid_from <= D` and
(`valid_to IS NULL` or `valid_to >= D`). Migration `000130` backfills one
`Initial record` row per existing employee from `hire_date`, and the realistic
seed (`make seed-realistic`) inserts the same row per seeded employee with
`ON CONFLICT (employee_id, valid_from) DO NOTHING`, so re-runs are safe.

"Today" for job changes (the mirror rule, the directory's default effective date
and the scheduled job) is the Asia/Jakarta calendar date, the same timezone the FX
daily-rates job uses.

`RecordJobChange` copies the new record onto `hr_employees` (department,
position, manager, status, employment type) only when `valid_from` is today or
earlier. If `valid_from` equals the start of the open latest record (a new
hire's `Hired` record, a pre-boarded hire with a future `hire_date`, or a second
edit on the same day), that record is corrected in place instead of a new one
being added (audited with `corrected: true`). The employee directory edit uses
today, or the start of a lone future record (a pre-boarded hire) if that is later, as
the default effective date. If a job change is already scheduled in the future
on top of an earlier record, the directory edit is refused with "a job change is
scheduled for <date>; edit it from the employee's Job tab" and nothing is saved,
because the directory form shows only the current values and would otherwise
overwrite the scheduled change. It saves the profile fields and the job change in a single transaction,
so a rejected job change leaves no partial edit. A change that sets status
ACTIVE after the employee's `termination_date` is a rehire: it clears
`termination_date` and `termination_reason` in the same transaction and audits
`EMPLOYEE_REHIRED`.

The worker job `hr:apply_job_changes` runs daily at 17:10 UTC (cron `10 17 * * *`,
00:10 in Asia/Jakarta) and applies the record effective today to every employee whose `hr_employees`
row differs, so future-dated changes take effect on their start date. It is
idempotent (employees already in sync are not written or audited) and audits
each applied change as `EMPLOYEE_JOB_CHANGED` with `source: scheduled`, attributed
to the user who recorded the change. A record with no author falls back to
user id 1, the system placeholder the CMMS and documents modules also use;
`audit_logs.actor_id` is NOT NULL and references `users`, so **user id 1 must
exist** in every environment or the job fails. It skips
employees that have a termination date, including those with an OPEN offboarding
checklist, because offboarding manages their status. A failure for one employee
does not stop the others; the run fails so asynq retries it.

#### PII boundary and retention

Sensitive profile columns are stored as plain text in `hr_employees`; access is
controlled by `hr.employee.sensitive` and `hr.compensation.*`, not by
encryption. Employees with any `payroll_run_lines` row can never be deleted
because that reference is `ON DELETE RESTRICT`. PII erasure for former staff
therefore requires a future "scrub personal data" admin action that blanks the
sensitive columns and emergency contacts while keeping the employee row. This
is a documented gap and is not built yet. The `000130` down-migration drops the
new columns and tables irreversibly, including any data entered in them.

### Leave setup

Before employees request leave:

1. Link the employee and their manager to active Odyssey user accounts.
2. Create a leave type and annual `hr_leave_balances` row.
3. Create an active `LEAVE` policy in `/approvals/policies`; choose
   **Employee manager** as its first-step selector.
4. Grant employees `hr.leave.request` and managers `approvals.inbox` through
   their roles.
5. Optionally define company holidays and shifts (`hr_holidays`, `hr_shifts`,
   `hr_employee_shifts`; managed through the schedule service). Without them,
   leave counts Monday-Friday with no holidays.

Requested days are **working days**, not calendar days: the count uses the
employee's shift work-week as of the start date (Monday-Friday when no shift is
assigned) and excludes company holidays in the range. The request dialog has
"start date is a half day" and "end date is a half day" checkboxes; each flag
removes 0.5 day when that boundary day is a working day, and a single-day
request with either flag counts as 0.5. A range with no working days is
rejected ("hr: no working days in range"). The flags are stored in
`hr_leave_requests.start_half_day` / `end_half_day` and shown next to the days
in the request lists. Because holidays and shifts are read at submission time,
later calendar changes do not recount existing requests. Requests cannot span
two calendar years; split them at year end.

On submission, requested days move into `pending`. Manager approval moves those
days from `pending` to `used`; rejection releases `pending`. Finalization writes
an `audit_logs` record before the approval-completion notification is sent.

### Overtime setup

Overtime reuses the approval engine with module `OVERTIME`. Setup mirrors
leave setup, except no type or balance is needed:

1. Link the employee and their manager to active Odyssey user accounts.
2. Create an active `OVERTIME` policy in `/approvals/policies`; choose
   **Employee manager** as its first-step selector. The policy `Amount` is the
   request's **overtime hours** (minutes / 60), so amount ranges are in hours.
3. Grant employees `hr.overtime.request`, HR admins `hr.overtime.admin`, and
   managers `approvals.inbox` through their roles. Migration `000131` grants
   the new permissions to Admin only.

Employees request one entry per day (`/hr/overtime`); a second PENDING or
APPROVED request for the same day is refused. The worked day cannot be in the
future (Asia/Jakarta date): "hr: overtime can only be requested for days already
worked". Approved minutes feed payroll
(see [`payroll.md`](payroll.md)).

#### Payroll-lock rule

A payroll run's lines freeze once it leaves DRAFT, so overtime is checked
against REGULAR runs in `APPROVAL` or `POSTED` status covering the day:

- At submit, overtime for such a day is refused.
- If an approval lands on a covering `POSTED` run, the finalizer converts it to
  `REJECTED` with `apply_error` "payroll already posted" and notifies the
  requester.
- If it lands on a covering run in `APPROVAL`, the request stays `PENDING` with
  `apply_error` "payroll run awaiting approval" until an HR admin uses Retry
  apply.

Recalculate DRAFT payroll runs before submitting them for approval so newly
approved overtime is included.

#### Finalizer failure

`store.Decide` commits the approval as `APPROVED` before the finalizer runs. If
the finalizer fails, the approval stays `APPROVED` while the overtime row stays
`PENDING` with the failure in `apply_error`. An HR admin (`hr.overtime.admin`)
opens `/hr/overtime`, **All requests** tab, and uses **Retry apply** to re-run
the finalizer.

Deciding a request that is no longer pending (a double click, a late second approver) shows "This request has already been decided."; a request not assigned to the user shows "This request is not assigned to you."

The approver sees "Approved, but applying the result failed: <reason>. HR can
retry from the request page." (or "Rejected, ..." for a rejection). The reason
is the finalizer's text only when it is a plain `hr: ` message; anything else
(database errors, wrapped errors) is replaced by "an unexpected error occurred".

### Approval modules

| Module | Source | Policy `Amount` | Seeded policy | Recommended first step |
|---|---|---|---|---|
| `LEAVE` | Leave request | Requested days, as configured today | No | Employee manager |
| `OVERTIME` | Overtime request | Overtime **hours** (minutes / 60) | No | Employee manager |
| `EXPENSE` | Expense claim | Claim amount | No | Employee manager, then a finance role step |
| `HR_PROFILE` | Profile change request | `0` (always) | No | **Role step for HR/finance** |

Policies for `LEAVE`, `OVERTIME`, `EXPENSE`, and `HR_PROFILE` are **not seeded**; create them
at `/approvals/policies`. Without a resolving policy, submission fails with "no
approval policy configured" and the leave request ends `CANCELLED` (its reserved
pending days are released and `LEAVE_SUBMIT_FAILED` is audited); the row is kept,
not deleted.

`HR_PROFILE` policies should use a role step for HR or finance, **not**
"Employee manager": a bank change redirects salary payments, so it is a payroll
control and should not be approved by the employee's line manager alone. Because
the amount is always `0`, a single policy with no range covers every change type.

### Expenses and profile changes

Employees submit expense claims and profile change requests from the portal (see
[`portal.md`](portal.md)). Expense claims stop at `APPROVED`: there is no AP or
payroll posting. A profile change applies on approval in one transaction: `ADDRESS`
updates the employee, `EMERGENCY_CONTACT` upserts the contact, and `BANK` creates a
new compensation assignment effective on the decision date (the Asia/Jakarta calendar
date of the approval), with components moved to the new assignment; future-dated
assignments inherit the new bank details. An `ADDRESS` change overwrites only the
fields that are present and non-empty in the request; blank or omitted fields keep
their current values (so a field cannot be cleared this way). The portal form is
prefilled with the employee's current address.

As with overtime, `store.Decide` commits the approval as `APPROVED` before the
finalizer runs. If the finalizer fails, the approval stays `APPROVED` and the HR
row stays `PENDING` with the failure in `apply_error`. HR admins use **Retry apply**
on `/hr/overtime` (All requests tab; `hr.overtime.admin`), `/hr/expenses`
(`hr.expense.admin`), or `/hr/profile-changes` (`hr.profile_change.admin`). Migrations
`000131` and `000132` grant these permissions to Admin only.

### Attendance CSV

Required headers are `employee_number,date`. Optional headers are
`check_in,check_out,status`. Dates use `YYYY-MM-DD`; timestamps accept RFC3339
or `YYYY-MM-DD HH:MM`. Status is `PRESENT`, `ABSENT`, or `LEAVE`.

Invalid rows are recorded on the import batch. Valid rows are upserted by
employee and attendance date. Direct time-clock integrations remain out of
scope.

## Onboarding and offboarding

Checklists live in `internal/hr/checklists` and are served at `/hr/checklists` (My tasks, All checklists, Templates) and `/hr/checklists/{id}` (tasks, assets held, Complete button). Templates have a kind (`ONBOARDING` or `OFFBOARDING`) and ordered items with an assignee role (`HR`, `IT`, `MANAGER`), an optional default assignee user, and a due offset in days. Starting a checklist from the employee detail page instantiates the items as tasks: `MANAGER` resolves to the employee's manager's user as of today, `HR` and `IT` use the item's default assignee, and `due_date = started_on + due_offset_days`. Each distinct assignee gets one notification linking to the checklist. Only one OPEN checklist per employee and kind is allowed.

Starting an offboarding checklist records `termination_date` and `termination_reason` on the employee but does not change the employee status. The employee stays ACTIVE until offboarding is completed.

### Permissions

- `hr.checklist.admin` administers templates, starts, completes and cancels checklists, and assigns or returns assets. Migration `000133` grants it to the Admin role only. Grant it to HR roles explicitly.
- `hr.employee.view` is enough to open `/hr/checklists` and see **My tasks** (tasks assigned to you). Completing or skipping a task requires being its assignee or holding `hr.checklist.admin`.
- An assignee does not need either permission: a line manager assigned a `MANAGER` task can open `/hr/checklists`, view the checklist that contains their task (the other tasks are read-only) and mark their own task done. Anyone else without `hr.employee.view` or `hr.checklist.admin` gets 403, and the admin actions stay admin-only.
- The Assets tab on the employee detail page is shown only with `hr.checklist.admin`.

### Completing offboarding

`CompleteOffboarding` (`POST /hr/checklists/{id}/complete`) refuses unless all of these hold:

- The checklist is an OPEN offboarding checklist with a termination date.
- Today is on or after the termination date (otherwise "hr: termination date is in the future").
- No task is OPEN. Complete or skip each one.
- The employee holds no assets (`fixed_assets.custodian_employee_id`, scoped to the company).
- The employee has no direct reports as of today or as of the termination date ("hr: reassign N direct reports first"). Change the reports' manager through job history first.

When the preconditions hold, one transaction does all of the following. If any step fails, none of it is applied:

1. Cancels the employee's PENDING leave, overtime, expense and profile-change requests, with their approval requests and assignments.
2. Cancels PENDING approval assignments in this company where the leaver's user is the approver.
3. Sets `users.is_active = false` for the employee's linked user, if there is one.
4. Sets leave balances to `entitled = used` and `pending = 0`.
5. Sets the employee's benefits to `ENDED`.
6. Closes the latest job-history record at the termination date and inserts an `INACTIVE` record from the day after.
7. Sets the employee `INACTIVE`, marks the checklist `COMPLETED`, and audits `EMPLOYEE_OFFBOARDED`.

`CompleteOnboarding` only closes an onboarding checklist once no task is OPEN.

### User deactivation is global

`users` has no company column. Setting `is_active = false` disables the login for **every company in the instance**. A person employed by several companies in one Odyssey instance loses access to all of them when any one company completes their offboarding. Check for other employments first. Re-enable the user manually if needed; offboarding does not do this on its own.

Offboarding only cancels the leaver's pending approval assignments **in the company being offboarded**. Assignments they hold as an approver in any other company stay PENDING but can no longer be actioned (the login is disabled). Before completing, check `/approvals` for the leaver in each company, or list them with `SELECT ar.company_id, ar.module, aa.request_id FROM approval_assignments aa JOIN approval_requests ar ON ar.id = aa.request_id WHERE aa.approver_id = <user id> AND aa.status = 'PENDING'`. Reassign them manually: create a delegation from the leaver to a replacement at `/approvals/policies` (needs `approvals.delegate`; the replacement is then assigned for the delegation window), and change any policy step that names the leaver as a user step.

Existing sessions end on the next request. `activeUserMiddleware` (`internal/app/active_user.go`) looks up `users.is_active` for every request with a logged-in session (static assets and anonymous requests are skipped). For an inactive or deleted user it destroys the session and returns 401. This costs one extra `users` query per authenticated request. Login already refuses inactive users.

### Cancelling offboarding

`CancelOffboarding` (`POST /hr/checklists/{id}/cancel`) works only while the checklist is OPEN. It sets the checklist `CANCELLED`, clears the employee's `termination_date` and `termination_reason`, leaves the tasks as they are, and audits `OFFBOARDING_CANCELLED` with the reason. After `CompleteOffboarding` it cannot be used. To rehire, record a job change with status ACTIVE effective after the termination date (employee detail, **Job history**); that clears `termination_date` and `termination_reason` in the same transaction and audits `EMPLOYEE_REHIRED`. The user login is not re-enabled automatically.

### Approver reassignment

Completing offboarding cancels PENDING approval assignments where the leaver is the approver. It does not choose a replacement. The affected requests still need a new approver: create a delegation (see Configurable approvals above) or reassign them before or right after completing offboarding, otherwise those requests have no pending assignment. Review approval policies that name the leaver as a user step.

### Asset custody

Custody is stored in `fixed_assets.custodian_employee_id`. On the employee detail **Assets** tab, an admin assigns a company fixed asset to an ACTIVE employee or returns it. Disposed assets cannot be assigned, an asset held by someone else must be returned first, and both actions are audited (`ASSET_ASSIGNED`, `ASSET_RETURNED`). Offboarding cannot complete while any asset is held.

### Overdue reminders

The worker job `hr:checklist_reminders` runs daily at 08:00 (cron `0 8 * * *`). It notifies the assignee of OPEN tasks, on OPEN checklists, whose due date is before today, oldest first, up to 200 per run. Only active assignee users are notified. The dedupe key `hr-checklist-overdue-<taskID>-<YYYY-MM-DD>` limits this to one notification per task per day, even with several worker replicas or reruns. Tasks beyond the cap are picked up on later runs. A delivery failure fails the run so it is retried, and the dedupe keys keep the retry safe.

### Final pay

A leaver is still paid for the final period (the one that contains the termination date) because completion keeps `termination_date`. See [Final-period pay in the payroll guide](payroll.md#as-of-department-and-final-period-pay).

## Performance review cycles

Performance reviews run in company-wide cycles (migration `000134_hris_performance_cycles`). Pages live under `/hr/performance`: the index (tabs **My reviews**, **Team reviews**, and **Cycles** for administrators), `/hr/performance/cycles/{id}` (administrators), and `/hr/performance/reviews/{pid}` (one participant's review).

### Permissions

| Permission | Grants |
| --- | --- |
| `hr.performance.view` | Open `/hr/performance` and the reviews the user is a participant or reviewer in. |
| `hr.performance.admin` | Create, activate, and close cycles, enroll participants, view every review, submit a manager review on behalf of any reviewer, edit any participant's goals, and complete reviews. |

Migration `000134` also seeds `hr.reports.view`, which the HR dashboard phase consumes. All three permissions are granted only to the Admin role (`admin` / `administrator`); grant `hr.performance.view` to other roles explicitly so employees and managers can reach their reviews.

### Cycle lifecycle

A cycle has a name (unique per company), a review period, a self-review due date, and a manager-review due date (period start <= self due <= manager due, and period end >= period start).

`DRAFT -> ACTIVE -> CLOSED`

1. **Draft.** An administrator creates the cycle (`PERF_CYCLE_CREATED`) and enrolls employees. Draft cycles are visible only to administrators.
2. **Activate.** Needs at least one participant (`PERF_CYCLE_ACTIVATED`). Goals and reviews can be entered only while the cycle is ACTIVE.
3. **Close.** Moves an ACTIVE cycle to CLOSED (`PERF_CYCLE_CLOSED`). Participants that are not completed keep their stage, no further goal edits or reviews are accepted, and an administrator can still complete a participant whose manager review was already submitted. Enrollment is refused on a closed cycle.

### Participants and reviewers

Administrators enroll employees (up to a per-request cap) while the cycle is DRAFT or ACTIVE. Every employee must belong to the company and be ACTIVE (an offboarded employee is refused with "hr: only active employees can be added to a performance cycle"), otherwise nothing is added. Employees already enrolled are skipped. The reviewer is the employee's manager on their job record effective at the cycle's `period_end`; if no record is effective the participant has no reviewer, and an administrator handles the manager step.

Participant stages: `ENROLLED -> SELF_SUBMITTED -> MANAGER_SUBMITTED -> COMPLETED`.

### Goals

The participant (or an administrator) adds, edits, and deletes goals while the cycle is ACTIVE and the participant is not yet completed. A goal has a title, description, KPI target, and a weight from 0 to 100 (two decimals); weights are not required to sum to 100. Goal changes are audited as `PERF_GOAL_CHANGED`.

### Review flow

1. **Self review.** Only the participant's own user can submit it (administrators cannot submit on their behalf), while the participant is `ENROLLED`: an overall rating (1-5), comments, and optional per-goal scores (1-5). Audited `PERF_SELF_SUBMITTED`.
2. **Manager review.** The reviewer, or an administrator, submits an overall rating, comments, and per-goal scores. It requires `SELF_SUBMITTED`; if the participant has not submitted a self review, it is allowed once the cycle's self-review due date has passed. Audited `PERF_MANAGER_SUBMITTED`.
3. **Complete.** An administrator (`hr.performance.admin`) sets the final rating (1-5) on a `MANAGER_SUBMITTED` participant of an ACTIVE or CLOSED cycle. This writes a summary row into `hr_performance_reviews` (with `company_id`, the cycle name as the review period, the final rating, and the manager comments), links it through the participant's `review_id`, and marks the participant `COMPLETED` (`PERF_COMPLETED`). The summary row's reviewer is the participant's reviewer, or the completing administrator's employee record when there is none; completion fails if neither exists. Completed cycles therefore appear in the existing employee review history.

Audit metadata for all `PERF_*` events holds ids and stage only, never ratings or comments.

### Who sees what

- Participant: their own goals, self rating, comments, and scores while reviewing. Manager ratings, comments, and scores stay hidden; the final rating appears only after the review is `COMPLETED`.
- Reviewer and administrators: everything, except that the self-review data appears only once it has been submitted.
- List pages never show self or manager ratings or comments. The final rating shows to reviewers and administrators, and to the participant once completed.
- Anyone who is not the participant, the reviewer, or an administrator gets a 404 on `/hr/performance/reviews/{pid}`, and non-administrators get a 404 on reviews in DRAFT cycles. The cycle page and cycle actions require `hr.performance.admin`.

### Tenant data

`hr_performance_reviews` gained a `company_id` column. The migration backfills it from the employee's company before making it NOT NULL, and new summary rows always set it. All cycle, participant, and goal tables carry `company_id` as well.

Recruitment, candidate and offer workflows, and training/certification management are not currently supported. Their scope and rollout are defined in the [`Product Workflow Depth Execution Plan`](product-workflow-depth-plan.md); its performance section (4.2) describes a broader target (templates, calibration, peer input) than the cycle flow above.

## Verification

```bash
ODYSSEY_TEST_MODE=1 GOTENBERG_URL=http://127.0.0.1:0 \
  go test ./internal/approvals ./internal/hr/... ./internal/procurement
```

The approval tests cover different PO policies below and above an amount
threshold, delegation, and the leave flow from manager assignment through
balance finalization, audit callback, and notifications.

### End-to-end scenario

`TestStagingHRCoreJourney` in `tests/staging/hr_core_test.go` drives a running
deployment over HTTP and verifies every step through a read-only database
session. Steps 1–6 are setup and stop the run on failure; steps 7–10 always all
run.

1. Admin route smoke: `/hr/employees`, `/hr/leave`, `/hr/attendance`,
   `/approvals`, and `/approvals/policies` return 200.
2. The no-access user is denied the HR pages, the approvals inbox, and a
   department POST.
3. Admin creates a run-unique department and position in the configured
   company.
4. Admin links a manager employee and an employee (managed by the manager) to
   the two HR users, reusing existing rows through `/hr/employees/{id}/update`.
5. Admin creates a run-unique leave type and a 12-day balance. The `LEAVE`
   policy that resolves for the company must be a single **Employee manager**
   step; if none resolves, the test creates one through `/approvals/policies`.
6. An end date before the start date is rejected without a request row.
7. Approve: submission reserves `pending` and assigns the manager; the employee
   cannot decide it; the manager approves from `/approvals`, which moves the
   days to `used` and writes the audit row and employee notification. A
   duplicate approval changes nothing. Builds before the `Decide` fix
   (`fix(approvals): return post-decision request from Decide`, which affects
   v0.10.0-rc.1 through rc.9) fail here: the approved request is finalized as
   `REJECTED`.
8. Reject: the manager rejects a second request and `pending` is released.
9. Cancel: the employee cancels a third request; `pending` is released and the
   approval leaves the inbox.
10. Attendance: template header, a CSV import with valid, unknown-employee, and
    invalid-status rows, an idempotent re-import with changed times, and manual
    records (a check-out before check-in is rejected).

Environment, with no defaults: `ODYSSEY_E2E_URL`, `PG_DSN`,
`ODYSSEY_E2E_ADMIN_EMAIL`/`_PASSWORD`, `ODYSSEY_E2E_NO_ACCESS_EMAIL`/`_PASSWORD`,
`ODYSSEY_E2E_COMPANY_ID`, `ODYSSEY_E2E_HR_EMPLOYEE_EMAIL`/`_PASSWORD` (an
existing user with `hr.leave.request`), and
`ODYSSEY_E2E_HR_MANAGER_EMAIL`/`_PASSWORD` (an existing user with
`approvals.inbox`). The test skips when all four `ODYSSEY_E2E_HR_*` variables
are unset and fails when only some are set.

```bash
ODYSSEY_EVIDENCE_STRICT=0 ODYSSEY_EVIDENCE_INDEX=/tmp/hr-evidence-index.json \
  go test -tags staging ./tests/staging -run TestStagingHRCoreJourney -v
```

`ODYSSEY_EVIDENCE_STRICT=0` is required when the test runs alone: whenever
`ODYSSEY_E2E_URL` is set, `TestMain` otherwise fails the run because the
v0.10-core evidence rows are missing. This scenario emits no certification
evidence IDs. `ODYSSEY_EVIDENCE_INDEX` keeps the index file out of the
repository; without it, `TestMain` writes `evidence-index.json` in the package
directory.

## Release notes: migration ordering and local-only guard

- 000130+ must not reach main or staging until 000125-000129 are applied there; verify with `SELECT version FROM schema_migrations` >= 129 before deploying; if you applied 000130+ locally before pulling finance-prep, run `migrate down` to 000124 first.
- `make migrate-up`, `make migrate-down`, and `make test-migrate` now refuse any
  `PG_DSN` host other than `localhost`, `127.0.0.1`, or `::1`. Deliberate deploys
  use `make migrate-up-remote` / `make migrate-down-remote` (see
  [DEPLOYMENT.md](../DEPLOYMENT.md)).

### Pull request template text

Include this in every PR that adds a migration numbered 000130 or higher:

> 000130+ must not reach main or staging until 000125-000129 are applied there; verify with `SELECT version FROM schema_migrations` >= 129 before deploying; if you applied 000130+ locally before pulling finance-prep, run `migrate down` to 000124 first.
