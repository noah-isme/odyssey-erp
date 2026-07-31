# ADR-0005: Transaction-level FX

Transaction FX is separate from the monthly `fx_rates` consolidation table.
Documents keep the original currency amount and a locked invoice/payment rate;
revaluation journals use the remaining foreign-currency balance and do not
modify the original journal.

Rates use the quote convention `1 transaction currency = N base currency`.
The system uses the reversal model: the prior period's unrealized journal is
reversed at the beginning of the next period, then the open balance is valued
at the new closing rate. Daily rates are stored in `fx_daily_rates`, and fetch
attempts are audited in `fx_fetch_runs`.
