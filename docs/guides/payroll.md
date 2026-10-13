# Payroll engine

Indonesian payroll is a versioned calculation engine. Regulatory values are data in migration `000046_payroll_engine`; no tax, BPJS, overtime, or rounding rate is compiled into the Go service.

## Regulatory provenance

The reviewed migration records its source URL/reference and effective date in `payroll_rule_versions`. A run stores `tax_rule_version_id`, `bpjs_rule_version_id`, and `company_policy_id`; its JSON line breakdown repeats those IDs for payslip/audit evidence.

Release data was checked on 30 July 2026 against:

- [PP 58/2023 official JDIH copy](https://jdih.kemenkeu.go.id/download/e47c3fc4-a912-4bf1-bcad-335fee3f71f8/2023pp058.pdf): PTKP-to-TER categories and all monthly TER A/B/C brackets, effective 1 January 2024.
- [PMK 168/2023 official JDIH page](https://www.jdih.kemenkeu.go.id/dok/pmk-168-tahun-2023/summary): monthly TER operation and last-tax-period annual reconciliation guidance.
- [DJP worked example](https://www.pajak.go.id/id/siaran-pers/perhitungan-pph-21-lebih-mudah-berikut-ketentuannya): category mapping and the permanent-employee monthly/last-period distinction.
- [BPJS Ketenagakerjaan Penerima Upah](https://www.bpjsketenagakerjaan.go.id/penerima-upah.html): JHT, JP, JKK, and JKM contribution splits/risk classes.
- BPJS Ketenagakerjaan letter `B/1226/022026`, effective 1 March 2026: JP maximum wage Rp11,086,300. This also reconciles to the statutory annual-ceiling formula using the previous Rp10,547,400 ceiling and [BPS's official 2025 GDP growth of 5.11%](https://www.bps.go.id/id/pressrelease/2026/02/05/2546/pertumbuhan-ekonomi.html), truncated to the published hundred-rupiah ceiling.
- Perpres 64/2020: PPU health contribution (employee 1%, employer 4%) and Rp12,000,000 wage ceiling.

Do not edit an effective rule version. Add a new version and brackets/rates, retain its official document reference, and set `reviewed_at` only after payroll/legal review. Migration `000047_payroll_review_fixes` rejects overlapping reviewed rule ranges by rule type and overlapping company-policy ranges by company. `CreateDraft` refuses periods without a reviewed effective tax/BPJS version or effective company policy. The selected PTKP annual amount is retained in each line breakdown as regulatory evidence even though monthly TER calculation uses its mapped category.

## Setup

1. Apply migrations `000046_payroll_engine` and `000047_payroll_review_fixes`.
2. Add a `COMPANY` rule version and an effective `payroll_company_policies` row for each company. This controls overtime divisor/multipliers, JKK risk class, currency, and rounding unit.
3. Create payroll periods and effective employee compensation assignments. Bank code, account number, PTKP status, base salary, and BPJS participation are effective-dated with the assignment.
4. Configure the five `payroll_account_mappings`: `SALARY_EXPENSE`, `EMPLOYER_BPJS_EXPENSE`, `PAYROLL_PAYABLE`, `PPH21_PAYABLE`, and `BPJS_PAYABLE`.
5. Create a shared approval policy for module `PAYROLL` and grant the payroll RBAC permissions.

## As-of department and final-period pay

Draft calculation resolves each line's department as of the period `pay_date`: `COALESCE(jh.department_id, e.department_id)`, where `jh` is the `hr_employee_job_history` row valid on that date (latest `valid_from` wins). Employees without a matching history row fall back to the current `hr_employees.department_id`, so existing data behaves as before.

Employee selection is `(e.termination_date IS NULL OR e.termination_date >= p.starts_on) AND (e.status='ACTIVE' OR e.termination_date IS NOT NULL)`. A leaver is therefore paid for the period in which they leave (and any period starting on or before the termination date), is excluded from later periods, and an INACTIVE employee without a termination date stays excluded.

Offboarding relies on this rule. Starting an offboarding checklist records `termination_date` and `termination_reason` on the employee and leaves the status ACTIVE. Completing it (on or after the termination date) sets the employee INACTIVE and keeps `termination_date`. The leaver is therefore still paid for the final period, the one that contains the termination date, and is not paid for any period that starts after it. Cancelling the offboarding clears `termination_date` again.

`Repository.CreatePeriod` creates OPEN periods and rejects any period that overlaps an existing period of the same company (`ErrPeriodOverlap`), checked in the same transaction. There is no database exclusion constraint, so pre-existing overlapping data cannot break migrations.

**Final-period pay is not prorated.** A leaver whose termination date falls inside a period receives the full base salary and allowances for that period; the engine does not scale by days worked. If a partial month is owed, enter a one-off deduction adjustment for the difference on that period before the run is submitted for approval. The same applies to a rehire that starts mid-period.

**Rehire.** Recording an ACTIVE job change effective after the termination date clears `termination_date` and `termination_reason` (audited `EMPLOYEE_REHIRED`), so the employee is selected again by the rule above. The scheduled `hr:apply_job_changes` job skips employees that still have a termination date.

**Future-dated rehire edge case.** A rehire recorded with a start date in the future clears `termination_date` immediately, while the status stays INACTIVE until that start date (the scheduled job applies it then). During that gap the payroll filter no longer matches the employee (no termination date, not ACTIVE), so recalculating the run for the leaving period drops the leaver's final pay. Finalize (post) the leaving period's payroll **before** recording a future-dated rehire; a posted run is immutable and is not affected.

## Bank changes and back-dated salary changes

A bank change (`ChangeBank`) splits the compensation assignment on its effective date: the current assignment ends the day before and a new one starts on the effective date with the new bank details and the same salary terms. A later salary change dated **before** that effective date is therefore rejected with `ErrLaterChangeExists` ("a later change exists"). To correct salary retroactively, date the salary change on or after the latest assignment start, or have the bank change re-entered afterwards.

## Overtime sources and leave days

Draft calculation sums approved overtime minutes from two sources per employee and period: legacy `payroll_overtime` rows with `approved_by IS NOT NULL` for the period, plus HR overtime requests (`hr_overtime_requests`) with `status='APPROVED'` and `worked_on BETWEEN p.starts_on AND p.ends_on`. Do not record the same employee-day in both sources; the two are added, so a duplicate is paid twice. Prefer HR overtime requests for new data.

HR overtime approval policies (module `OVERTIME`) use `Amount = hours` (minutes / 60), not minutes. Recalculate DRAFT runs after overtime is approved and before submitting them for approval, since lines freeze once a run leaves DRAFT.

Approved leave days are summed as NUMERIC so half days are preserved. `Input.LeaveDays` (int64) keeps the rounded value (0.5 becomes 1, 1.5 becomes 2) and `Input.LeaveDaysExact` carries the exact string (for example `"1.5"`) into the line breakdown JSON as `LeaveDaysExact`. Leave days are informational and never affect pay.

## Deployment order

- Apply migrations **before** deploying the new code (new code reads the HRIS tables and columns added by `000130`-`000134`). Roll the **code back first** if you need to reverse a release, and only then run `migrate down`; older code cannot run against a schema that is missing tables the older build still queries, and newer code cannot run against a rolled-back schema.
- `CreatePeriod` refuses overlapping periods but there is no database constraint, so check existing data on staging and production **before** deploying. Any rows returned must be fixed by hand, otherwise an employee can be selected in two periods:

```sql
SELECT a.company_id, a.id AS period_a, b.id AS period_b, a.starts_on, a.ends_on, b.starts_on AS b_starts_on, b.ends_on AS b_ends_on
FROM payroll_periods a
JOIN payroll_periods b ON b.company_id = a.company_id AND b.id > a.id
  AND a.starts_on <= b.ends_on AND b.starts_on <= a.ends_on
ORDER BY a.company_id, a.id;
```

- Never point `make migrate-up` at a remote database; it is guarded and refuses any DSN that is not localhost or that sets `host=`, `hostaddr=` or `service=`. Use `make migrate-up-remote` deliberately.

## Lifecycle

`DRAFT → APPROVAL → POSTED` is enforced by conditional updates. Calculation uses repeatable-read isolation and atomically replaces only draft line snapshots. It records base salary, recurring/one-off components, overtime, THR, attendance/approved leave counts, department, PTKP/TER, BPJS, tax, and net pay. Approval rejection returns an unposted run to draft for correction and records its actor and note in `payroll_run_events`. Approval finalization posts it automatically.

Posting uses `payroll_runs.run_uuid` as the accounting source ID. Regular run UUIDs are deterministically derived from company and payroll period, while the partial unique index permits only one posted regular run per company/period. Posted lines are never recalculated or deleted; corrections use `ADJUSTMENT` or `REVERSAL` runs and `reversal_of_id`.

Journal debits salary and employer BPJS expense and credits payroll, PPh 21, and BPJS payables. Lines are grouped by mapped accounting department/cost center. A posted run creates one payment batch; `/payroll/{id}/bank.csv` exports employee bank instructions.

## Payslips and access

Posting creates immutable payslip records that also serve as a durable delivery outbox. Initial enqueue errors do not undo a posted run or stop later employees from being queued. The worker runs `payroll:payslip_dispatch` every five minutes to re-enqueue records without `delivered_at`, then `payroll:payslip_email` renders `payroll_payslip_pdf.html` through Gotenberg and sends the PDF with the shared SMTP mail client. The delivery task ID is `payroll-payslip-{payslipID}` and retries at most five times.

`/payroll/payslips/{id}.pdf` allows payroll staff, the linked employee user, or that employee's linked manager. Authorization is enforced in the repository query as well as route RBAC. Missing and unauthorized records both return HTTP 404 to prevent payslip-ID enumeration.

## Verification

Run:

```sh
ODYSSEY_TEST_MODE=1 GOTENBERG_URL=http://127.0.0.1:0 go test ./internal/payroll ./jobs ./internal/app
```

Calculator tests cover all TER categories, bracket boundaries, the PP 58 K/0 Rp10m example, BPJS health/JP caps, overtime, prorated/full THR, negative adjustment, and symmetric rounding. Service tests cover approval, balanced journals, deterministic source IDs, and repeated-post idempotency.

`CalculateAnnualPPh21` applies the PMK 168/2023 last-tax-period rule using
explicit annual policy bands, integer-rupiah rounding, prior-period withholding,
and a signed December/termination-month correction. The release suite loads the
official Tn A worked example from
[`internal/payroll/testdata/pph21_december_2024_tn_a.json`](../../internal/payroll/testdata/pph21_december_2024_tn_a.json)
and asserts the annual liability and final-period amount. Production activation
still requires payroll/legal review of the effective rule version and the
company's operational evidence.
