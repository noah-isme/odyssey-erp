# Reporting Catalog

This catalog maps the requested report/KPI inventory to current documented routes and
outputs. Its `Status` column describes route/code availability only. The
[authoritative feature matrix](feature-matrix.md) controls integration and production
certification. “Not mapped” means the requirement should not be advertised as available
until a route and data contract are added.

Governed datasets, custom report definitions, dashboard widgets, broader operational
and HR coverage, and their certification sequence are planned in the
[`Reporting and Administration Depth Execution Plan`](../guides/reporting-administration-depth-plan.md).

| Report / KPI | Status | Route | Data source | Filters | Export |
|---|---|---|---|---|---|
| Trial balance | Implemented | `/accounting/reports/trial-balance` | Posted GL journals | Company, period, account | PDF route exists; verify current handler before advertising CSV |
| Balance sheet | Implemented route; PDF tag required | `/finance/consol/bs` | Consolidated/posting snapshots | Group, period, FX | HTML, CSV; PDF requires `production pdf` build tags |
| Profit & loss | Implemented route; PDF tag required | `/finance/consol/pl` | Consolidated/posting snapshots | Group, period, FX, dimensions | HTML, CSV; PDF requires `production pdf` build tags |
| Cash flow | Implemented | `/finance/cashflow` | Bank transactions and accounting cash movements | Company, period | HTML; export contract needs explicit documentation |
| Budget vs actual | Implemented | `/accounting/budget` | `accounting_budgets` plus posted journal actuals | Company, month, department/cost center | Scheduled email and Excel are documented |
| AR aging / customer balances | Implemented | `/finance/ar/aging` | AR invoices, payments, allocations | Company, customer, as-of date | HTML/PDF behavior should be documented per handler |
| AP aging / vendor balances | Implemented | `/finance/ap/aging` | AP invoices, payments, allocations | Company, vendor, as-of date | HTML/PDF behavior should be documented per handler |
| Consolidated P&L / BS | Implemented route; PDF tag required | `/finance/consol/pl`, `/finance/consol/bs` | Company-group consolidation, FX and eliminations | Group, period, FX | CSV; PDF requires `production pdf` build tags |
| Finance insights | Implemented | `/finance/insights` | Cached finance metrics | Company, date range | Dashboard/export behavior documented in insights guide |
| Board pack | Implemented | `/board-packs` | Selected finance reports and KPIs | Template, period, group | PDF |
| Sales by customer/product | Not mapped | — | Sales orders/deliveries/invoices | — | — |
| Monthly revenue | Partial | Analytics/dashboard surfaces | AR and sales sources | Company, period | Analytics CSV/PDF |
| Stock valuation | Implemented | Inventory valuation surface | Inventory movements, AVG/FIFO costing | Company, warehouse, product | Export not catalogued |
| Dead stock / fast-moving items | Not mapped | — | Inventory movements | — | — |
| Attendance | Partial | `/hr/attendance` | Attendance records/imports | Company, employee, period | Not catalogued |
| Payroll | Partial | `/payroll` | Payroll runs and payslips | Company, period, employee | Payslip PDF; run export is CSV |
| Leave report | Partial | `/hr/leave` | Leave requests and balances | Company, employee, period, status | Not catalogued |
| HR dashboard | Implemented | `/hr/dashboard` | Employees, job history, attendance, leave balances, payroll runs | Company; year-to-date and 30-day windows | HTML; requires `hr.reports.view` |
| HR headcount | Implemented | `/hr/reports/headcount.csv` | Employees and effective-dated job history | `as_of` (default today) | CSV; requires `hr.reports.view` |
| HR hires and leavers | Implemented | `/hr/reports/hires-leavers.csv` | Employee hire and termination dates | `from`, `to` (default year-to-date) | CSV; requires `hr.reports.view` |
| HR turnover | Implemented | `/hr/reports/turnover.csv` | Leavers and average headcount | `from`, `to` (default year-to-date) | CSV; requires `hr.reports.view` |
| HR absenteeism | Implemented | `/hr/reports/absenteeism.csv` | ABSENT attendance over scheduled working days | `from`, `to` (default year-to-date) | CSV; requires `hr.reports.view` |
| HR leave liability | Implemented | `/hr/reports/leave-liability.csv` | Leave balances and base salary | `year` (default current year) | CSV; requires `hr.reports.view` and `hr.compensation.view` |
| HR overtime cost | Implemented | `/hr/reports/overtime-cost.csv` | POSTED payroll run overtime plus approved unrun overtime | `from`, `to` (default year-to-date) | CSV; requires `hr.reports.view` and `hr.compensation.view` |
| HR contract and probation expiries | Implemented | `/hr/reports/expiries.csv` | Employee contract and probation end dates | `days` (default 30) | CSV; requires `hr.reports.view` |
| Gross margin | Partial | Analytics/insights | Revenue and cost/valuation data | Company, period | Analytics CSV/PDF |
| ROI | Not mapped | — | — | — | — |
| Inventory turnover | Not mapped | — | — | — | — |
| Cash flow KPI | Partial | Dashboard/finance cash flow | Bank and accounting cash movements | Company, period | Not catalogued |
| Net profit | Implemented | P&L / analytics | Posted GL journals | Company, period, dimensions | CSV/PDF via report surfaces |

Route and export claims should be verified against handlers when a report is changed. The catalog intentionally distinguishes “implemented report” from “requested business report”; unsupported rows are documentation backlog items, not implied features.

## HR dashboard and reports

All HR report routes require `hr.reports.view`, which migration 000134 grants to the Admin role only; grant it to other roles deliberately. Leave liability and overtime cost, and their dashboard cards, additionally require `hr.compensation.view`. The handler checks this server-side: without it the dashboard omits those cards and never computes them, and the two CSV routes return 403.

- Filters: `as_of` (headcount), `year` (leave liability), `days` (expiries, default 30), and `from`/`to` for the remaining reports (default year-to-date). Dates use `YYYY-MM-DD`; an unparsable or invalid filter returns 400.
- Every export is audited as `HR_REPORT_EXPORT` with the report name and filter. Auditing is best effort: a failed audit write is logged and the export still succeeds.
- No CSV contains national ID, tax ID, date of birth or address.
- CSV injection protection: any cell that starts with `=`, `+`, `-`, `@`, tab or CR (after trimming leading spaces) is prefixed with a single quote. Server-formatted negative numbers are therefore exported with a leading apostrophe (for example `'-12.5`); consumers that need the numeric value must strip it. A department named `=HYPERLINK(...)` exports as `'=HYPERLINK(...)`.
- Leave liability is available days x base salary / 21, summed per department. It counts all leave types because leave types have no paid-leave flag.
- Overtime cost combines overtime on POSTED payroll run lines with approved overtime not yet covered by a POSTED run, priced with the current company payroll policy. The unrun portion is an estimate and may differ from the eventual payslip.
