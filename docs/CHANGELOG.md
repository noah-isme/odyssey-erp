# Changelog

## Phase 8 Cycle 8.3 – Board Pack

### Added

- **Board Pack schema** — new tables `board_pack_templates` dan `board_packs` beserta enum status, siap dimigrasikan via `000010_phase8_board_pack`. Seed default "Standard Executive Pack" ditambahkan.
- **Service + job pipeline** — BoardPackService memvalidasi input + metadata, sedangkan worker Asynq mengeksekusi builder → HTML template → PDF (Gotenberg) → simpan file dengan logging dan retry-friendly errors.
- **Config & storage** — `BOARD_PACK_STORAGE` menentukan direktori penyimpanan PDF (default `./var/boardpacks`). Renderer memakai template `templates/reports/boardpack_standard.html` untuk layout PDF.
- **SSR UI** — halaman `/board-packs` (list + filter), `/board-packs/new` (form generate), detail, dan download protected, semuanya memakai permission baru `finance.boardpack` dan RBAC.
- **Docs** — `docs/howto-boardpack.md`, `docs/runbook-boardpack.md`, serta pembaruan `CHANGELOG.md` mencakup alur e2e, runbook worker, batasan versi pertama.

### Changed

- Nav bar menambahkan entry "Board Pack" di bawah Close & Insights.
- Seed RBAC kini menambahkan permission `finance.boardpack` ke admin & manager, plus menanam template default.

### Testing

- Unit test `internal/boardpack/builder_test.go` mencakup skenario dengan & tanpa variance snapshot. `go test` penuh gagal karena sandbox tidak mengizinkan akses GOPROXY; jalankan dengan akses network untuk verifikasi menyeluruh.

## Phase 7 Final (v0.7.0)

### Highlights

- Consolidated finance exporters reach GA with aligned warning propagation across SSR banners, CSV metadata, and PDF captions.
- Gotenberg-backed PDF pipeline promoted to production with retries, payload validation, and observability hooks.
- Export runbook, FX tooling helpers, and cache busting workflows finalized for operations handover.
- Release notes published alongside handover summary for downstream teams.

### Verification

- Manual walkthrough confirmed SSR warning banner parity with CSV header metadata and PDF footer caption lists.
- `make export-demo` executed against the reference stack to exercise CSV/PDF exporters end-to-end.
- RBAC (403) and rate limit (429) behaviours verified via automated and manual checks on export endpoints.

### Documentation

- `docs/phase7-summary.md` captured the closing brief for developers and ops, including Phase 8 outlook.
- `docs/runbook-consol-plbs.md` updated with FX, cache refresh, metrics, and observability procedures.
- `TESTING-PHASE7-S3.md` marked final with consolidated coverage notes for caching, warnings, and prod-tag PDF testing.

## Phase 7 Sprint 3.4.4

### Added

- Production-ready consolidation PDF exporter backed by Gotenberg with 10s timeout, two retries, and minimum-size validation.
- Streaming CSV exporter with buffered writes, metadata comment headers, and regression tests for P&L/Balance Sheet.
- `docs/runbook-consol-plbs.md` plus Makefile helpers (`export-demo`, `fx-tools`) for day-two export and FX operations.

### Changed

- Consolidation warnings now persist in cached view-models and render consistently across SSR banners, CSV metadata, and PDF warning lists.

### Testing

- Updated `TESTING-PHASE7-S3.md` to cover prod-tag PDF checks, CSV streaming, and warning parity.

## Phase 6 Final (v0.6.0-final)

### Added

- Grafana dashboards for finance and platform with latency, error, anomaly, and infrastructure panels.
- Prometheus finance alert rules covering high error rate, latency, and anomaly spikes with runbook annotations.
- Performance regression tests for HTTP latency, job throughput, and alert simulations.
- Finance SLO/SLA documentation, operations runbook, and release security checklist.
- Makefile targets for monitoring demo and alert simulations, plus Phase 6 release automation.

### Changed

- Prefixed HTTP metrics with `odyssey_` to align dashboards and alerts.
- Updated observability overview to include final dashboard variables and alert naming.

### Testing

- Documented automated, performance, and alert simulation coverage in `TESTING-PHASE6-S4.md`.
