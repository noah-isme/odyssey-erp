# Reporting Catalog

This catalog maps the requested report/KPI inventory to current documented routes and outputs. “Not mapped” means the requirement should not be advertised as available until a route and data contract are added.

| Report / KPI | Status | Route | Data source | Filters | Export |
|---|---|---|---|---|---|
| Trial balance | Implemented | `/accounting/reports/trial-balance` | Posted GL journals | Company, period, account | PDF route exists; verify current handler before advertising CSV |
| Balance sheet | Implemented | `/finance/consol/bs` | Consolidated/posting snapshots | Group, period, FX | HTML, CSV, PDF |
| Profit & loss | Implemented | `/finance/consol/pl` | Consolidated/posting snapshots | Group, period, FX, dimensions | HTML, CSV, PDF |
| Cash flow | Implemented | `/finance/cashflow` | Bank transactions and accounting cash movements | Company, period | HTML; export contract needs explicit documentation |
| Budget vs actual | Implemented | `/accounting/budget` | `accounting_budgets` plus posted journal actuals | Company, month, department/cost center | Scheduled email and Excel are documented |
| AR aging / customer balances | Implemented | `/finance/ar/aging` | AR invoices, payments, allocations | Company, customer, as-of date | HTML/PDF behavior should be documented per handler |
| AP aging / vendor balances | Implemented | `/finance/ap/aging` | AP invoices, payments, allocations | Company, vendor, as-of date | HTML/PDF behavior should be documented per handler |
| Consolidated P&L / BS | Implemented | `/finance/consol/pl`, `/finance/consol/bs` | Company-group consolidation, FX and eliminations | Group, period, FX | CSV, PDF |
| Finance insights | Implemented | `/finance/insights` | Cached finance metrics | Company, date range | Dashboard/export behavior documented in insights guide |
| Board pack | Implemented | `/board-packs` | Selected finance reports and KPIs | Template, period, group | PDF |
| Sales by customer/product | Not mapped | — | Sales orders/deliveries/invoices | — | — |
| Monthly revenue | Partial | Analytics/dashboard surfaces | AR and sales sources | Company, period | Analytics CSV/PDF |
| Stock valuation | Implemented | Inventory valuation surface | Inventory movements, AVG/FIFO costing | Company, warehouse, product | Export not catalogued |
| Dead stock / fast-moving items | Not mapped | — | Inventory movements | — | — |
| Attendance | Partial | `/hr/attendance` | Attendance records/imports | Company, employee, period | Not catalogued |
| Payroll | Partial | `/payroll` | Payroll runs and payslips | Company, period, employee | Payslip PDF; run export is CSV |
| Leave report | Partial | `/hr/leave` | Leave requests and balances | Company, employee, period, status | Not catalogued |
| Gross margin | Partial | Analytics/insights | Revenue and cost/valuation data | Company, period | Analytics CSV/PDF |
| ROI | Not mapped | — | — | — | — |
| Inventory turnover | Not mapped | — | — | — | — |
| Cash flow KPI | Partial | Dashboard/finance cash flow | Bank and accounting cash movements | Company, period | Not catalogued |
| Net profit | Implemented | P&L / analytics | Posted GL journals | Company, period, dimensions | CSV/PDF via report surfaces |

Route and export claims should be verified against handlers when a report is changed. The catalog intentionally distinguishes “implemented report” from “requested business report”; unsupported rows are documentation backlog items, not implied features.
