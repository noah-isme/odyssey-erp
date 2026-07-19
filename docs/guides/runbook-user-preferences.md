# Runbook Operasional: Preferensi Pengguna dan Notifikasi UI

Panduan ini mencakup deployment, verifikasi, dan penanganan masalah untuk Profil, Pengaturan pengguna, tema, bahasa, dan toast UI.

## Perubahan skema

Migration `000033_user_preferences` menambahkan kolom berikut pada tabel `users`.

| Kolom | Nilai default | Kegunaan |
|---|---|---|
| `ui_theme` | `system` | Preferensi tema pengguna. |
| `ui_language` | `id` | Bahasa aktif (`id` atau `en`). |
| `ui_notifications` | `true` | Visibilitas kontrol notifikasi. |

## Prosedur deployment

1. Pastikan backup database terbaru tersedia.
2. Terapkan migration:

   ```bash
   make migrate-up
   ```

3. Build atau restart aplikasi agar route `/profile`, `/settings`, dan `/api/me` tersedia.

   ```bash
   ~/go/bin/air
   ```

   Untuk Docker:

   ```bash
   docker compose up -d --build
   ```

4. Lakukan hard refresh pada browser pengguna setelah deploy aset JavaScript/CSS baru.
5. Masuk dengan akun uji dan verifikasi profil, pengaturan, perubahan password, dan toast sukses/gagal.

## Pemeriksaan pascadeploy

| Pemeriksaan | Hasil yang diharapkan |
|---|---|
| `GET /profile` saat belum login | Redirect ke `/auth/login`. |
| `GET /profile` saat login | Profil akun aktif ditampilkan. |
| Simpan `/settings` | Redirect kembali dan toast sukses/error tampil satu kali. |
| Ubah bahasa | Hanya satu bahasa tampil setelah halaman selesai dimuat. |
| Ubah tema | Tema diterapkan saat refresh halaman berikutnya. |
| `GET /api/me` saat login | JSON akun dan preferensi aktif. |
| `/analytics` | Dashboard Analytics terbuka. |
| `/analytics/kpi` | Tetap kompatibel sebagai alias dashboard, tanpa menu sidebar. |

## Troubleshooting

### Halaman Profil atau Pengaturan 404

Penyebab umum adalah proses aplikasi masih memakai binary lama. Restart Air atau rebuild container. Pastikan source deployment mencakup route baru di `internal/app/router.go`.

### Halaman Pengaturan error setelah deploy

Pastikan migration `000033_user_preferences` sudah diterapkan. Kolom yang belum ada akan membuat query preferensi pengguna gagal.

### Teks bilingual terlihat saat perpindahan halaman

Pastikan browser memuat `/static/js/core/ui.js` dan `/static/js/main.js` versi terbaru. Jalankan hard refresh. Bahasa terakhir dibaca sebelum halaman ditampilkan; bila preferensi akun dan browser berbeda, aplikasi melakukan satu reload sinkronisasi yang terkontrol.

### Toast tidak muncul atau browser memakai CPU/memori tinggi

Pastikan aset `web/static/js/features/toast/index.js` versi terbaru telah dideploy. Implementasi lama memakai snapshot state antrian toast yang tidak diperbarui di dalam loop, sehingga toast pertama dapat diproses berulang. Versi saat ini membaca state baru setiap iterasi dan membatasi jumlah toast aktif.

### Tidak bisa mengubah password

Pastikan password saat ini benar, password baru minimal 8 karakter, dan konfirmasi sama. Kesalahan ditampilkan melalui toast tanpa mengekspos detail password.

## Rollback

Jika rollback aplikasi diperlukan, rollback migration hanya setelah memastikan binary lama tidak lagi menulis kolom preferensi:

```bash
make migrate-down
```

Perintah tersebut menghapus kolom preferensi dari `users`; ekspor nilai preferensi terlebih dahulu bila perlu dipertahankan.

## Referensi

- [Panduan Profil dan Pengaturan](user-profile-settings.md)
- [Arsitektur Finance Analytics](../reference/ui-analytics-architecture.md)
- [Troubleshooting umum](../getting-started/troubleshooting.md)
