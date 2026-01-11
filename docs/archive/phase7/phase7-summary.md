# Phase 7 Close-Out Summary

## Scope akhir & fitur utama

- Konsolidasi Profit & Loss serta Balance Sheet mencapai GA dengan peringatan (FX gap, neraca tidak seimbang, filter entitas terpotong) tersinkronisasi di SSR, CSV, dan PDF.
- Exporter PDF menggunakan Gotenberg v8 dengan retry dua kali, timeout 10 detik, dan validasi ukuran minimum 1 KiB.
- Caching view-model lima menit, pembatasan laju 10 req/menit, dan header `X-Consol-Warning` distandarkan untuk seluruh channel.
- Runbook, tooling FX (`make fx-tools`), dan automasi cache busting disiapkan untuk operasi hari-ke-dua.

## Risiko & mitigasi

- **Ketergantungan Gotenberg** — Pantau `FinanceExportHighErrorRate`, sediakan fallback ke mode CSV-only bila PDF gagal secara terus-menerus.
- **Data FX tidak lengkap** — Gunakan `odyssey fx validate` setiap akhir minggu; jika ada gap, jalankan backfill `--mode apply` setelah dry-run.
- **Cache stale** — Operator harus menjalankan `BustConsolViewCache()` setelah refresh job sukses atau ketika peringatan tidak selaras.
- **Rate limit terlampaui** — 429 akan muncul di percobaan ke-11; koordinasikan dengan tim front-end untuk batching permintaan bulk.

## Hal penting untuk Phase 8

- Integrasikan metrik `odyssey_consol_warnings_total` ke dashboard anomaly Phase 8.
- Kaji otomatisasi seeding data konsolidasi untuk lingkungan staging agar regresi mudah diuji.
- Perluas observability untuk memetakan korelasi peringatan terhadap kualitas data sumber (AP/AR, inventori, integrasi procurement).
- Rencanakan migrasi exporter ke pipeline multi-tenant termasuk isolasi cache per entitas grup.

## Retro singkat

- **Yang berjalan baik** — Kolaborasi lintas tim pada warning parity dan penulisan runbook menghasilkan proses QA yang cepat; Gotenberg v8 stabil sepanjang uji beban.
- **Yang perlu ditingkatkan** — Provisioning data contoh membutuhkan koordinasi manual; perlu skrip standar untuk sprint awal Phase 8.
- **Insight tambahan** — Observability stack (Grafana + Prometheus) memberi sinyal dini saat PDF timeout, memudahkan troubleshooting.
