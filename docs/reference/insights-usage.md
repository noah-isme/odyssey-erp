# Panduan Finance Insights

Halaman `/finance/insights` menyediakan perbandingan Net dan Revenue dalam rentang waktu bulanan.

## Filter Awal
- **from** dan **to** (`YYYY-MM`): default 12 bulan terakhir jika tidak diisi.
- **company_id** dan **branch_id**: opsional, mengikuti akses user.

## Catatan Implementasi
- Grafik menggunakan renderer SVG multi-seri tanpa dependensi JS.
- Data bersumber dari materialized view `mv_pl_monthly`.
- Respons fallback "no data" apabila tidak ada catatan pada rentang yang diminta.

Dokumen ini akan diperbarui setelah implementasi final selesai.
