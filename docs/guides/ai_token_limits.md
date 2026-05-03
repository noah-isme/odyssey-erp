# Panduan Menghindari Error "Token Limit Exceeded" pada Model AI

Untuk mencegah model AI menghasilkan teks yang melebihi batas token (*The model's generation exceeded the maximum output token limit*) saat berinteraksi dalam pengembangan Odyssey ERP, gunakan panduan berikut:

## 1. Gunakan Artefak (Artifacts) untuk Teks Panjang
- Jika model perlu membuat atau mengubah dokumen besar (seperti rencana implementasi, dokumen *walkthrough*, file konfigurasi panjang, atau catatan rilis), wajib gunakan tool `write_to_file` dengan mode Artifact (misalnya `implementation_plan.md`, `task.md`, atau `walkthrough.md`).
- **Jangan** pernah menulis ulang keseluruhan kode file atau log panjang langsung di dalam jendela *chat*.

## 2. Batasi Output Baris Kode di Chat
- Saat melaporkan hasil pencarian, diff kode, atau snippet kode, selalu batasi baris yang ditampilkan. Gunakan referensi baris (misal `internal/inventory/service.go:400-450`) ketimbang menempel seluruh blok fungsi besar di teks *chat*.
- Hindari menyuruh model untuk "Print full file" atau "Tampilkan semua isi kode". Gunakan tool `view_file` atau `grep_search` dengan opsi spesifik untuk pencarian.

## 3. Pecah Tugas Menjadi Fase Kecil (Incremental Steps)
- Jangan meminta model untuk membuat banyak file, komponen *frontend*, *backend*, dan struktur *database* sekaligus dalam satu giliran (satu pesan).
- Model harus memecah permintaan menjadi beberapa panggilan *tool* yang berjalan sekuensial (contoh: 1. buat skema db -> 2. jalankan sqlc -> 3. buat fungsi *service*).
- Jika ada *update* besar, model lebih baik berhenti sementara dan meminta *approval* *(request_feedback)* atau melaporkan *progress* agar *token buffer* terpotong dan batas bisa *reset* pada *turn* berikutnya.

## 4. Efisiensi Logging dan Command
- Saat menjalankan instruksi dengan `run_command`, **selalu** arahkan *output* perintah verbose (seperti log panjang atau instalasi *dependencies*) ke *file* lokal atau filter menggunakan instruksi seperti `grep` di *bash command* (jika terpaksa) atau membatasi output *characters* saat membaca *command status*. Hindari mengembalikan 1000 baris log `npm install` ke konteks AI.
- Jangan menggunakan `cat` melalui bash. Selalu gunakan *tool* `view_file` karena memiliki batasan baris untuk mencegah kelebihan muatan.

## 5. Ringkasan Singkat (Concise Reporting)
- Setiap merespons kembali ke *user*, model **wajib** memberikan respons seringkas mungkin (poin-poin utama saja). Jangan mengulang penjelasan detail arsitektur yang sama atau menceritakan kembali kode yang baru ditulis dari nol jika kode tersebut bisa dilihat lewat Artifact atau git diff.
