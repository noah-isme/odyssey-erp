# Phase 6 – Insights, Audit & Monitoring (Timeboxed MVS)

## Scope
- Fokus pada tiga inisiatif utama: Finance Insights, Audit Timeline, dan Observability.
- Kerjakan fitur secara bertahap sesuai timebox T0–T4, dengan T5 (anomaly detection) hanya jika waktu memungkinkan.
- Pastikan seluruh pekerjaan mengikuti batasan maksimal 25 file dan semua perubahan lulus lint, test, dan build.

## Deliverables per Role
- **Tech Lead**: koordinasi lintasan kerja, review cakupan, dan dokumentasikan progres harian.
- **Backend – Insights**: view model dan query perbandingan MoM/YoY dari `mv_pl_monthly`, handler SSR, serta renderer SVG multi-seri.
- **Backend – Audit**: query timeline audit + join jurnal, handler SSR dengan filter dasar, serta ekspor CSV dengan rate limit.
- **Observability**: endpoint `/metrics` dengan metrik dasar HTTP dan job queue.
- **QA**: uji handler (200/403), snapshot SVG tidak kosong, verifikasi MIME CSV.

## Timebox Rencana
- **T0 – Scaffolding**: siapkan stub route, handler, template kosong, serta dokumentasi awal.
- **T1 – Insights**: implementasi query, view model, chart SVG, dan halaman SSR.
- **T2 – Audit**: implementasi timeline, paging, dan ekspor CSV termasuk guard RBAC + rate limit.
- **T3 – Observability**: wiring metrik Prometheus dan integrasi dengan router utama.
- **T4 – Hardening**: perkuat RBAC, dokumentasi tambahan, dan catat TODO sisa.
- **T5 – Opsional**: anomaly rule berbasis threshold jika kapasitas tersedia.

## Risiko & Mitigasi
- **Data kosong / rentang besar**: fallback placeholder chart dan batasi rentang default.
- **Performa query**: gunakan indeks atau materialized view bila diperlukan; catat TODO bila belum sempat.
- **Kompleksitas SVG**: prioritaskan pengujian renderer sedini mungkin agar mudah di-refactor.

## Catatan Koordinasi
- Gunakan commit kecil per timebox.
- Update dokumen ini saat deliverable selesai atau ditunda untuk transparansi tim.
