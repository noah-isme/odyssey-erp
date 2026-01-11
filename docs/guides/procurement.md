# SOP – Procurement Phase 3

## Tujuan
Menjamin alur pengadaan PR → PO → GRN → AP berjalan konsisten, terdokumentasi, dan dapat diaudit.

## Prasyarat
* Role dengan permission `procurement.edit` dan `inventory.edit` untuk operasi create/post.
* Master data (produk, supplier, gudang) sudah tersedia.
* Session CSRF aktif via aplikasi web.

## Langkah Operasional
1. **Buat Purchase Request (PR)**
   - Navigasi ke `/procurement/prs` dan isi form PR minimal satu baris.
   - Submit form untuk menyimpan PR dengan status `DRAFT`.
   - Ajukan PR menggunakan endpoint `POST /procurement/prs/{id}/submit` agar status berubah ke `SUBMITTED`.

2. **Konversi ke Purchase Order (PO)**
   - Buka `/procurement/pos` dan isi nomor PR yang akan dikonversi.
   - Sistem menyalin baris PR ke PO baru dengan status `DRAFT`.
   - Gunakan endpoint `POST /procurement/pos/{id}/submit` untuk masuk ke tahap approval.
   - Approver mengeksekusi `POST /procurement/pos/{id}/approve`; approval dicatat di tabel `approvals`.

3. **Terima Barang (GRN)**
   - Form di `/procurement/grns` memungkinkan input gudang, supplier, dan rincian barang.
   - Setelah form tersimpan, status GRN `DRAFT`.
   - Tekan tombol/endpoint `POST /procurement/grns/{id}/post` untuk mem-posting.
   - Posting GRN memanggil service inventory (`PostInbound`) sehingga qty dan avg cost diperbarui atomik.

4. **Buat Invoice AP**
   - Akses `/procurement/ap/invoices`, masukkan GRN yang sudah diposting dan tanggal jatuh tempo.
   - Invoice dibuat dalam status `DRAFT`. Gunakan `POST /procurement/ap/invoices/{id}/post` untuk mengubah ke `POSTED`.

5. **Catat Pembayaran**
   - Form `/procurement/ap/payments` mencatat pembayaran terhadap invoice.
   - Jika jumlah bayar ≥ total invoice maka status diubah menjadi `PAID`.

6. **Laporan PDF**
   - Stock card: `GET /report/stock-card/pdf?warehouse_id=...&product_id=...`.
   - GRN: `GET /report/grn/pdf?number=...`.

## Kontrol & Audit
* Semua mutasi inventory menulis log ke `audit_logs` dengan entity `inventory_tx`.
* Approval PO tersimpan di tabel `approvals` dan dapat ditelusur berdasarkan UUID referensi.
* Idempotency key diterapkan pada GRN posting dan transaksi inventory untuk mencegah duplikasi saat retry.

## Troubleshooting
* **Error 403** – pastikan role memiliki permission yang sesuai.
* **Average cost tidak sesuai** – jalankan job `TaskInventoryRevaluation` atau periksa baris GRN untuk unit cost yang salah.
* **PDF kosong** – pastikan layanan Gotenberg berjalan dan endpoint `/report/ping` mengembalikan status OK.
