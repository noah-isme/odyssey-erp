# Fixed Assets operations

## Prerequisites

Create the required chart-of-account entries, then open **Accounting → Fixed
Assets → Categories**. Every category requires:

* Asset account
* Accumulated depreciation account
* Depreciation expense account
* Useful life in months and residual rate

For disposal with proceeds or a gain/loss, also select the cash-proceeds,
disposal-gain, and disposal-loss accounts. Do not register an asset until its
category is configured.

## Registering an asset

Open **Accounting → Fixed Assets → New asset**, select a category, and enter
the asset number, name, in-service date, acquisition cost, and useful life.
The register records the asset as `ACTIVE`; acquisition capitalization remains
an accounting entry handled through the journal workflow.

## Monthly depreciation

The worker runs at **02:10 UTC on the first day of every month**. For each
eligible active asset it posts straight-line depreciation:

* Debit: depreciation expense
* Credit: accumulated depreciation

The process uses a deterministic source reference per asset and month, so it
does not intentionally post duplicate depreciation. The matching accounting
period must be `OPEN`; a locked or missing period prevents posting.

To investigate a failed run, inspect the Jobs dashboard and worker log, correct
the account/period configuration, then requeue the worker task.

## Disposal

From the asset register enter the disposal date and proceeds. The system posts
derecognition of the asset and accumulated depreciation, records proceeds, and
posts the resulting gain or loss to the configured category account. Disposal
requires an open accounting period and the relevant proceeds/gain/loss accounts
when the calculation produces those amounts.

## Manual journals and reporting dimensions

Use **Accounting → Journals → New journal entry** to post a balanced manual
journal. Each line can carry an optional Department and Cost Center, allowing
Profit and Loss and Budget vs Actual reports to be filtered by those dimensions.

## Report schedules

Report schedules can select current or previous month, Department, Cost Center,
frequency, and recipients. The worker records queued deliveries in
`report_schedule_deliveries`. Use **Retry** to reset a schedule for the next
hourly scan; schedule changes and retry actions are recorded in the audit log.
