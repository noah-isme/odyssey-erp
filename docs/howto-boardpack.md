# How-to Board Pack

Dokumen ini memandu Finance Manager / Board Assistant untuk membuat dan mengunduh Board Pack PDF.

## Permission

- User harus memiliki permission `finance.boardpack` (sudah melekat di role `admin` dan `manager`).

## Langkah Generate

1. Masuk ke aplikasi lalu buka menu **Board Pack** dari navigasi utama.
2. Klik tombol **Generate Baru**.
3. Isi form:
   - **Company** – pilih perusahaan target.
   - **Accounting Period** – pilih periode (list menampilkan 36 periode terakhir sesuai Company).
   - **Template** – pilih konfigurasi Board Pack (mis. *Standard Executive Pack*).
   - **Variance Snapshot** (opsional) – pilih snapshot READY bila ingin menampilkan Top Variances.
   - **Catatan** – informasi singkat yang akan ikut tersimpan di metadata.
4. Submit form; sistem akan membuat record `PENDING` dan melempar job ke worker (Asynq).
5. Kembali ke daftar Board Pack; status akan berubah menjadi `IN_PROGRESS` lalu `READY` setelah worker selesai. Jika gagal, status `FAILED` dan kolom error menampilkan pesan.
6. Buka halaman detail (klik “Detail”), lalu tekan **Download PDF** saat status sudah `READY`.

## Catatan

- Generasi berjalan asynchronous; refresh halaman untuk mendapatkan status terbaru.
- Jika job gagal, klik tombol **Generate Baru** untuk membuat request ulang (re-run tidak dilakukan otomatis).
- File yang sudah READY tetap dapat diunduh sewaktu-waktu selama file masih tersimpan di direktori storage (`BOARD_PACK_STORAGE`).
