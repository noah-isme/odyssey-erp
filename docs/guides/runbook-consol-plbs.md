# Consolidated P&L / Balance Sheet Runbook

This runbook describes day-two operations for the consolidated Profit & Loss and Balance Sheet exporters that ship in Phase 7 Sprint 3.4.4.

## Prerequisites

- The application is built with the `prod` tag so the consolidation HTTP handlers use the Gotenberg-backed PDF client.
- `GOTENBERG_URL` points at a reachable v7 instance. The health endpoint (`/ping`) must respond with HTTP 200.
- The finance database is populated and consolidated groups exist for the target period.
- Operators have access to the CLI binary (`odyssey`) for FX maintenance commands.

## Export workflows

### CSV streaming

1. Run `make export-demo GROUP_ID=<group> PERIOD=<yyyy-mm>` to fetch both CSV and PDF artifacts into `/tmp`. The target uses buffered streaming with a flush every 200 rows, so large files should arrive without exhausting memory.
2. Inspect the first three lines of each CSV. They are comment lines (`# …`) that include the report name, filters, and a semicolon-separated warning list. This metadata mirrors the SSR banner and PDF footer.
3. Confirm the `X-Consol-Warning` response header contains the same messages exposed in the SSR view (FX gaps, unbalanced balance sheet, truncated entity filter, etc.).
4. Validate the header row retains Excel-friendly column names and that numeric columns use `.` as the decimal separator.

### PDF rendering

1. Use `make export-demo` (or direct `curl`) to regenerate the PDFs. Each request is wrapped in a 10-second timeout and allows two retries for transient 5xx responses from Gotenberg.
2. Verify the response headers report `Content-Type: application/pdf` and a payload larger than 1 KB. The exporter returns a `TooSmallError` when Gotenberg produces an empty document.
3. When timeouts occur, the exporter surfaces `TimeoutError`. Investigate network connectivity or Gotenberg saturation before retrying manually.

## Warning propagation

- Warnings returned by the consolidation services (missing FX rates, unbalanced balance sheet totals, large entity filters that truncate the response) are persisted in the cached view-model, rendered as an SSR banner, included in CSV metadata, and displayed in the PDF warning list.
- After running `make export-demo`, inspect `/tmp/consol-*.csv` to ensure the comment block includes the same warnings shown in the UI banner. The PDFs render the warnings in the header list.

## FX tools

- Run `make fx-tools GROUP_ID=<group> PERIOD=<yyyy-mm> FX_PAIR=<pair>` to print the curated CLI commands for FX gap validation and CSV backfill previews.
- Typical workflows:
  - `odyssey fx validate --group 1 --period 2025-08 --pair IDRUSD --json` – check for missing FX methods for the requested period and emit JSON for dashboards.
  - `odyssey fx backfill --pair IDRUSD --from 2024-01 --to 2025-12 --source ./rates.csv --mode dry` – preview import candidates without mutating storage. Switch `--mode apply` once the dry-run output is satisfactory.

## Cache refresh & data hygiene

- The consolidation handlers cache view-model payloads for five minutes. Trigger `BustConsolViewCache()` via the job dashboard (or call `/finance/consol/cache/bust` with admin credentials) after a data correction to avoid stale warnings.
- Nightly `jobs/consolidate_refresh` runs automatically invoke the cache buster once consolidation materialized views finish refreshing.
- For auditability, keep the exporter cache metrics (`odyssey_consol_cache_hits_total`, `odyssey_consol_cache_misses_total`) visible in Grafana and alert when miss ratio exceeds 20% for ten minutes.

## Observability & metrics

- The consolidation HTTP handlers emit Prometheus metrics for latency, rate limit denials, and warning propagation (`odyssey_consol_warnings_total` labelled by `channel=ssr|csv|pdf`). Use dashboard `Consol / Exporters` to confirm counts track within ±5% across channels.
- Alert `FinanceExportHighErrorRate` fires when 5xx responses exceed 2% for 15 minutes. Follow the alert annotation to inspect Gotenberg health and recent deploys.
- Structured logs include `warning_count` and `warning_types`. When incidents arise, export the logs to the observability bucket for handoff to Phase 8 analytics work.

## Troubleshooting

- **TimeoutError / InvalidResponse** – the PDF exporter logs the upstream status. Restart or scale the Gotenberg deployment and retry the request.
- **TooSmallError** – the generated PDF is <1 KB. Confirm the HTML payload is correct and that Gotenberg has the Chromium engine enabled.
- **Missing warnings** – call `BustConsolViewCache()` (available through the job runbook) before re-running the export to ensure cached view-models refresh after data changes.
