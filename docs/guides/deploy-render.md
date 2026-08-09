# Render Demo/Staging Blueprint

`render.yaml` menyediakan Blueprint minimal dengan hanya dua resource Render:

- web service `odyssey-web` (`plan: free`);
- PostgreSQL `odyssey-postgres` (`plan: free`).

Blueprint ini ditujukan untuk demo, staging ringan, atau evaluasi. Render Free bukan target production: web akan sleep setelah idle, sedangkan PostgreSQL Free berkapasitas 1 GB, tidak memiliki backup, dan kedaluwarsa setelah 30 hari.

Worker, Gotenberg, Render Key Value/Redis, dan cron sengaja tidak didefinisikan agar tidak membuat resource Render berbayar. Konsekuensinya, background job dan pembuatan PDF asynchronous tidak akan diproses.

## 1. Siapkan dependensi eksternal

Meskipun Blueprint hanya membuat dua resource Render, aplikasi masih membutuhkan layanan berikut:

1. Redis-compatible endpoint untuk session (`REDIS_ADDR`). Gunakan layanan eksternal yang memiliki free tier. Tanpa Redis, web service gagal start.
2. Private S3-compatible bucket untuk board-pack. Cloudflare R2 atau layanan sejenis dapat digunakan selama masih berada dalam kuota gratis.
3. Endpoint Gotenberg (`GOTENBERG_URL`). Untuk deployment demo tanpa fitur PDF, isi URL placeholder yang valid seperti `http://127.0.0.1:3000`; route PDF akan gagal sampai endpoint Gotenberg eksternal tersedia.

Nilai object storage yang diperlukan:

- `BOARD_PACK_S3_ENDPOINT`
- `BOARD_PACK_S3_REGION`
- `BOARD_PACK_S3_BUCKET`
- `BOARD_PACK_S3_ACCESS_KEY_ID`
- `BOARD_PACK_S3_SECRET_ACCESS_KEY`

Bucket harus private. Jangan menyimpan board-pack di filesystem web karena filesystem Render Free bersifat ephemeral.

## 2. Buat Blueprint

1. Di Render Dashboard pilih **New > Blueprint**.
2. Hubungkan repository Odyssey dan pilih branch deployment.
3. Render membaca `render.yaml` dari root repository.
4. Isi `REDIS_ADDR`, `GOTENBERG_URL`, dan lima nilai S3 yang bertanda `sync: false`.
5. Pastikan preview hanya menampilkan `odyssey-web` dan `odyssey-postgres`.
6. Apply Blueprint.

`SESSION_SECRET` dan `CSRF_SECRET` dibuat otomatis. Migration dijalankan oleh `dockerCommand` sebelum web server start karena `preDeployCommand` tidak tersedia pada web service Free.

## 3. Administrator awal

Render Free tidak menyediakan Shell/SSH. Tambahkan sementara IP publik operator ke inbound access PostgreSQL, lalu jalankan `/app/bootstrap-admin` dari lingkungan lokal atau container sementara dengan environment berikut:

```sh
PG_DSN=<external-postgres-url>
BOOTSTRAP_ADMIN_EMAIL=<email-admin>
BOOTSTRAP_ADMIN_PASSWORD=<password-minimal-12-karakter>
```

Jangan menjalankan `make seed` pada database yang menyimpan data nyata karena seed repository berisi akun dan data demo.
Hapus kembali inbound rule PostgreSQL setelah administrator berhasil dibuat.

## 4. Smoke test

Setelah deploy healthy:

1. buka `/healthz` dan pastikan respons `200`;
2. login dengan administrator yang diprovisikan;
3. uji transaksi synchronous dan koneksi session Redis;
4. generate board-pack hanya jika worker eksternal tersedia;
5. uji route PDF hanya jika Gotenberg eksternal tersedia.

## 5. Batasan mode $0

- Web sleep setelah 15 menit tanpa traffic dan cold start dapat memerlukan sekitar satu menit.
- PostgreSQL Free kedaluwarsa setelah 30 hari, maksimal 1 GB, dan tidak menyediakan backup.
- Tidak ada worker, sehingga email queue, variance snapshot, board-pack, dan job asynchronous lain tidak diproses.
- Tidak ada Gotenberg internal, sehingga PDF memerlukan endpoint eksternal.
- Biaya $0 bergantung pada penggunaan tetap di bawah kuota Render serta free tier provider Redis dan object storage eksternal.

Untuk production, gunakan database berbayar dengan backup, worker, Redis persisten, dan Gotenberg yang dikelola.

## Konfigurasi development

Development lokal tetap dapat menggunakan filesystem:

```env
BOARD_PACK_STORAGE_DRIVER=local
BOARD_PACK_STORAGE=./var/boardpacks
```
