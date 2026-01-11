# Runbook – Board Pack Pipeline

## Overview

Board Pack generation terdiri dari beberapa komponen:

1. **HTTP handler** (`/board-packs`) menerima request user, membuat record `board_packs`, dan menjadwalkan job Asynq `boardpack:generate`.
2. **Worker** (binary `cmd/worker`) mengambil job, menjalankan Builder → Renderer (Gotenberg) → menyimpan PDF ke direktori `BOARD_PACK_STORAGE`, lalu meng-update status ke `READY` / `FAILED`.
3. **Renderer** menggunakan template `templates/reports/boardpack_standard.html` dan Gotenberg (`GOTENBERG_URL`).

## Prasyarat

- Environment variable
  - `BOARD_PACK_STORAGE` (default `./var/boardpacks`) – pastikan direktori dapat ditulis oleh aplikasi & worker.
  - `GOTENBERG_URL` – endpoint Gotenberg v8.
- Worker harus berjalan (`go run ./cmd/worker` atau proses systemd) dengan akses Redis.

## Operasional

### Menjalankan Worker

```
BOARD_PACK_STORAGE=/var/lib/odyssey/boardpacks \
GOTENBERG_URL=http://gotenberg:3000 \
go run ./cmd/worker
```

Worker otomatis mendaftarkan handler `boardpack:generate` dan akan menulis log:

- `board pack ready` saat sukses (menyertakan `board_pack_id` & path).
- Error builder / renderer akan di-log lalu status berubah ke `FAILED`.

### Troubleshooting

| Gejala | Penanganan |
| --- | --- |
| Status mentok di `PENDING` | Periksa worker (jalan atau tidak). Jalankan `jobs CLI` untuk melihat antrian `boardpack:generate`. |
| Status `FAILED` dengan pesan "variance snapshot ..." | Snapshot tidak READY atau bukan milik company; minta user memilih snapshot lain atau membuat ulang snapshot di modul Variance. |
| Status `FAILED` dengan pesan "render" / "save" | Periksa koneksi Gotenberg (`curl $GOTENBERG_URL/ping`) dan permission direktori storage. |
| Download 404 | File telah dihapus dari storage. Regenerasi board pack (request baru). |

### Monitoring

- Job queue dapat dicek via endpoint `/jobs/health` atau CLI `odyssey jobs list`.
- Tambahkan scraping OS-level untuk memastikan disk `BOARD_PACK_STORAGE` tidak penuh.

## Known Limitations (v1)

- Hanya tersedia satu template default (Standard Executive Pack).
- Variance section hanya menampilkan snapshot READY yang dipilih manual; tidak ada auto-refresh.
- Tidak ada retry otomatis untuk record `FAILED`; admin perlu membuat request baru.
- Metadata tambahan masih berupa map sederhana (`requested_by`, `variance_rule`, `warnings`).
