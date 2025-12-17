---
description: Panduan memperbaiki error parsing template HTML dan Go Templates
---

# Memperbaiki Error Template

Jika server gagal start dengan error seperti `template: ... function "..." not defined` atau `bad character`, ikuti langkah ini:

## 1. Identifikasi Error
Cek log error untuk mengetahui file dan jenis errornya.

### Contoh Error 1: Function Undefined
```
template: boardpack_standard.html:63: function "formatDecimal" not defined
```
**Penyebab:** Template menggunakan function yang belum didaftarkan di `FuncMap`.
**Solusi:**
1. Cari di mana template tersebut di-parse (misalnya `internal/view/templates.go` atau handler khusus seperti `report/sample.go`).
2. Tambahkan function yang hilang ke `template.FuncMap`.

### Contoh Error 2: Bad Character / Corrupted HTML
```
template: coa_list.html:39: bad character U+003C '<'
```
**Penyebab:** Biasanya terjadi karena copy-paste code yang menyisipkan tag HTML korup (misalnya `</th>` di tengah tag Go template).
**Solusi:**
1. Buka file yang bermasalah.
2. Cari tag yang aneh atau tidak pada tempatnya (seperti tag penutup di dalam `{{ }}`).
3. Hapus atau perbaiki struktur HTML.

## 2. Pengecekan (Validasi)
Gunakan unit test yang sudah dibuat untuk memvalidasi semua template tanpa harus menjalankan server.

```bash
go test -v ./internal/view/...
```

Jika tes pass, berarti semua template di folder standar sudah valid.

## 3. Handler Khusus
Beberapa handler (seperti `report/sample.go`) melakukan parsing template sendiri (tidak menggunakan global engine). Pastikan:
- Mereka mendaftarkan `FuncMap` yang diperlukan.
- Jalur file template benar.
