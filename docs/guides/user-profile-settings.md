# Profil dan Pengaturan Pengguna

Halaman personal workspace tersedia untuk setiap pengguna yang sudah masuk.

| Halaman | URL | Fungsi |
|---|---|---|
| Profil | `/profile` | Mengubah nama tampilan dan melihat email akun. |
| Pengaturan | `/settings` | Mengatur tema, bahasa, notifikasi, dan password. |

## Profil

1. Buka menu pengguna di header, lalu pilih **Profil**.
2. Ubah **Nama tampilan**.
3. Pilih **Simpan profil**.

Email tidak dapat diubah dari halaman ini karena dikelola administrator. Role dan hak akses juga tetap dikelola melalui Administrasi/RBAC.

## Tampilan dan bahasa

Di **Pengaturan**, pilih salah satu opsi berikut lalu simpan.

| Preferensi | Pilihan | Perilaku |
|---|---|---|
| Tema | Sistem, Terang, Gelap | Sistem mengikuti pengaturan OS; pilihan lain memaksa tema yang dipilih. |
| Bahasa | Bahasa Indonesia, English | Aplikasi menampilkan satu bahasa aktif, bukan copy bilingual. |
| Notifikasi | Aktif/nonaktif | Menampilkan atau menyembunyikan kontrol notifikasi di header. |

Preferensi disimpan pada akun dan digunakan kembali pada sesi berikutnya. Browser menyimpan bahasa terakhir untuk mencegah teks bilingual terlihat sesaat saat halaman dimuat; preferensi akun tetap menjadi sumber kebenaran dan akan menyinkronkan browser bila berbeda.

## Mengubah password

1. Buka **Pengaturan** dan bagian **Keamanan akun**.
2. Isi password saat ini, password baru, dan konfirmasi password baru.
3. Password baru harus minimal 8 karakter.
4. Pilih **Ubah password**.

Sesi saat ini tetap aktif setelah password berhasil diubah. Pengguna harus segera keluar dan masuk kembali pada perangkat lain bila kebijakan organisasi mengharuskan rotasi kredensial di semua perangkat.

## Notifikasi hasil simpan

Setiap penyimpanan profil, pengaturan, atau password mengarahkan kembali ke halaman terkait dan menampilkan toast sukses atau gagal. Jika toast tidak terlihat setelah pembaruan aplikasi, lakukan hard refresh (`Ctrl+Shift+R` atau `Cmd+Shift+R`) agar JavaScript terbaru dimuat.

## Analytics

**Analytics** pada sidebar membuka `/analytics`, yang merupakan dashboard KPI kanonis. Rute `/analytics/kpi` hanya alias kompatibilitas dan tidak lagi ditampilkan sebagai menu terpisah.

## Perusahaan aktif dan Banking

Gunakan pemilih perusahaan di header untuk mengganti konteks perusahaan aktif. Pilihan tersebut tersimpan di sesi dan digunakan oleh halaman Banking.

- Saldo akun bank dihitung dari saldo awal ditambah seluruh transaksi bank tercatat.
- Transaksi manual dan transfer hanya dapat memakai periode akuntansi **OPEN** yang mencakup tanggal transaksi.
- Pada detail akun bank, pilih **Impor statement** untuk mengunggah CSV atau OFX/QFX maksimal 5 MB. CSV memerlukan kolom tanggal dan jumlah; deskripsi serta referensi opsional.
- Baris statement hasil impor berstatus **Pending** dan belum membuat jurnal GL. Lengkapi akun lawan melalui proses finance sebelum transaksi dianggap lengkap secara akuntansi.
