# Accounting Operations

The accounting area is mounted at `/accounting`. It provides chart-of-accounts,
general-ledger, trial-balance, profit-and-loss, balance-sheet, cash-flow, budget,
dimension, report-schedule, and fixed-asset workflows.

## Operating boundaries

- Post only to an open accounting period; period and journal controls are enforced
  by the accounting services.
- Use the journal workflow for manual entries. Void or reverse a posted entry
  through the available journal actions instead of changing records directly.
- Reconciliation and statement import are managed under `/accounting/banks`.
- Use the reports at `/accounting/trial-balance`, `/accounting/pnl`,
  `/accounting/balance-sheet`, `/accounting/cash-flow`, and `/accounting/budget`.

## Related references

- [Account mapping](../reference/account-mapping.md)
- [Period policy](../reference/period-policy.md)
- [Reporting catalog](../reference/reporting-catalog.md)

For database maintenance, use the supported Make targets in
`QUICK_REFERENCE.md`; do not run undocumented jobs or alter accounting data
directly in production.
