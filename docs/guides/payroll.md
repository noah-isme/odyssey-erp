# Payroll engine

Phase 4 adds Indonesian payroll as a versioned calculation engine. Regulatory values are data in migration `000046_payroll_engine`; no tax, BPJS, overtime, or rounding rate is compiled into the Go service.

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

The December/last-tax-period annual PPh 21 reconciliation required by PMK 168/2023 must be represented as a separately reviewed tax rule/calculation strategy before a December production payroll is released; the current engine intentionally blocks policy sign-off from being confused with that reconciliation.
