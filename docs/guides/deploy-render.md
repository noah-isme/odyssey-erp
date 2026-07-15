# Deploy ke Render

Repository menyediakan `render.yaml` untuk membuat enam komponen production di region Singapore:

- web service `odyssey-web`;
- background worker `odyssey-worker`;
- private Gotenberg service;
- Render Key Value untuk session dan Asynq;
- Render PostgreSQL;
- object storage S3-compatible eksternal untuk PDF board-pack.

Object storage wajib karena filesystem web dan worker Render tidak dibagi. Jangan menggunakan persistent disk pada salah satu service untuk `BOARD_PACK_STORAGE`.

## 1. Siapkan object storage

Buat private bucket sebelum membuat Blueprint. Kredensial cukup diberi izin membaca dan menulis object pada bucket tersebut.

Contoh Cloudflare R2:

- `BOARD_PACK_S3_ENDPOINT=https://<ACCOUNT_ID>.r2.cloudflarestorage.com`
- `BOARD_PACK_S3_REGION=auto`
- `BOARD_PACK_S3_BUCKET=<nama-bucket>`
- access key dan secret key dari R2 API token

Contoh AWS S3:

- `BOARD_PACK_S3_ENDPOINT=https://s3.<region>.amazonaws.com`
- `BOARD_PACK_S3_REGION=<region>`
- `BOARD_PACK_S3_BUCKET=<nama-bucket>`
- access key dan secret key dari IAM principal khusus bucket

Aktifkan versioning/lifecycle sesuai kebijakan retensi perusahaan. Bucket tidak boleh public.

Jika database berasal dari instalasi lama yang memakai local filesystem, unggah PDF lama ke bucket dan perbarui `board_packs.file_path` menjadi object key sebelum mengaktifkan driver S3.

## 2. Buat Blueprint

1. Di Render Dashboard pilih **New > Blueprint**.
2. Hubungkan repository Odyssey dan pilih branch deployment.
3. Render membaca `render.yaml` dari root repository.
4. Isi lima nilai S3 yang ditandai `sync: false`.
5. Tinjau estimasi biaya, kemudian apply Blueprint.

`SESSION_SECRET` dan `CSRF_SECRET` dibuat otomatis. PostgreSQL dan Key Value hanya menerima koneksi dari private network Render. Deploy web baru berjalan setelah CI branch berhasil.

## 3. Migration dan data awal

Web service menjalankan migration berikut sebelum setiap deploy:

```sh
/app/migrate -path /app/migrations -database "$PG_DSN" up
```

Migration tidak menjalankan `make seed`. Script seed repository berisi akun dan data demo sehingga **tidak boleh dijalankan pada database production**.

Untuk membuat administrator pertama, tambahkan sementara `BOOTSTRAP_ADMIN_EMAIL` dan `BOOTSTRAP_ADMIN_PASSWORD` pada environment web service, lalu jalankan melalui Render Shell:

```sh
/app/bootstrap-admin
```

Password minimal 12 karakter. Hapus kedua environment variable setelah command berhasil. Command bersifat idempotent, tidak membuat transaksi/master data demo, dan dapat digunakan kembali untuk merotasi password administrator secara terkontrol.

## 4. Smoke test

Setelah semua service healthy:

1. buka `/healthz` dan pastikan respons `200`;
2. login dengan akun yang diprovisikan, bukan akun demo;
3. buat transaksi uji dan pastikan worker memproses job;
4. generate lalu download board-pack PDF;
5. periksa log web, worker, Gotenberg, dan Key Value;
6. uji restore PostgreSQL dan object storage sebelum menerima data riil.

## 5. Operasional

- Jangan mengubah plan PostgreSQL ke free untuk production.
- Pertahankan `noeviction` dan persistence pada Key Value karena instance menyimpan session serta antrean job.
- Worker diberi shutdown window 120 detik agar job aktif dapat selesai.
- Rotasi kredensial object storage dan secret aplikasi secara berkala.
- Pasang custom domain, alert health check, backup/PITR, dan pembatasan akses operasional sebelum go-live.

## Konfigurasi local filesystem

Development tetap memakai konfigurasi lama:

```env
BOARD_PACK_STORAGE_DRIVER=local
BOARD_PACK_STORAGE=./var/boardpacks
```

Untuk deployment multi-service gunakan `BOARD_PACK_STORAGE_DRIVER=s3`.
