---
description: Aturan untuk membuat UI/UX enterprise yang konsisten di Odyssey ERP
---

# Enterprise UI/UX Guidelines

Dokumen ini mendefinisikan standar untuk komponen UI utama guna memastikan pengalaman pengguna yang konsisten, profesional, dan berkelas enterprise.

## 0. Component Strategy: Reuse vs Custom
Sebelum membangun UI baru, agen HARUS mengikuti hirarki keputusan berikut untuk menjaga konsistensi:

1.  **Cari & Gunakan yang Sudah Ada (DRY Principle)**:
    - SELALU periksa `web/static/css/components/` untuk melihat apakah sudah ada class CSS yang menangani komponen tersebut (misal: `.btn`, `.card`, `.table`).
    - SELALU periksa `web/templates/partials/` untuk melihat apakah ada fragmen HTML reusable yang bisa di-include.
    - Gunakan komponen JS global dari `OdysseyUI` (di `js/core/ui.js`) untuk fungsi standar seperti Toast, Modal, dan Theme.
2.  **Modifikasi via Variant (BEM Modifier)**:
    - Jika komponen dasar sudah ada tapi butuh sedikit penyesuaian, gunakan modifier BEM (misal: `.btn--ghost`, `.card--floating`). 
    - JANGAN membuat class baru jika hanya butuh perubahan minor (warna/padding); gunakan utility class (`u-mt-2`, `.numeric`) jika tersedia.
3.  **Buat Komponen Kustom Khusus**:
    - Buat komponen baru HANYA jika fungsionalitasnya sangat unik dan tidak bisa diakomodasi oleh komponen yang ada.
    - Ikuti pola penamaan BEM-lite: `.new-component`, `.new-component__element`, `.new-component--variant`.
    - Pastikan komponen kustom tetap menggunakan design tokens (warna, spacing, shadow) agar tetap serasi dengan UI lainnya.
    - Dokumentasikan komponen kustom baru jika dirasa akan berguna di halaman lain.

---


## 1. Typography & Paragraphs
Gunakan hierarki tipografi yang jelas untuk memandu mata pengguna.

- **Paragraph (`<p>`)**: 
    - Gunakan class `.text-body` untuk teks standar.
    - **Max-width**: Untuk paragraf panjang, batasi lebar maksimal (sekitar `65-75ch`) agar mudah dibaca.
    - **Line-height**: Gunakan `1.5` hingga `1.6` untuk keterbacaan optimal.
    - **Warna**: Gunakan `var(--text-secondary)` untuk paragraf umum agar tidak terlalu kontras dengan `var(--text-primary)` (digunakan untuk heading).
- **Heading**:
    - `h1`: Page Title (`.page-title`). Satu per halaman.
    - `h2`: Section Header. Gunakan untuk memisahkan grup informasi besar.
    - `h3`: Sub-section Header atau Card Title.

## 2. Lists (`<ul>`, `<ol>`, `<dl>`)
Hindari list default browser yang terlihat mentah.

- **Data List (`.data-list`)**: Digunakan untuk menampilkan pasangan label-value (sering digunakan di Detail Page).
    ```html
    <dl class="data-list">
        <div class="data-list__item">
            <dt>Label</dt>
            <dd>Value</dd>
        </div>
    </dl>
    ```
- **Standard List (`.list`)**: Gunakan spacing antar item (`var(--space-2)`).
- **Interactive List**: Gunakan hover state dan indicator jika item dapat diklik.

## 3. Table & Datatable
Tabel adalah jantung dari aplikasi ERP. Harus fungsional dan bersih.

- **Alignment**:
    - **Teks**: Rata kiri (default).
    - **Angka/Mata Uang**: SELALU rata kanan (`.text-right`) dan gunakan font monospaced/tabular-nums (`.numeric`).
    - **Status/Badge**: Rata tengah atau kiri tergantung konteks.
    - **Actions**: SELALU di kolom terakhir, rata kanan.
- **States**:
    - **Hover**: Baris harus memiliki hover state (`var(--table-row-hover)`).
    - **Empty State**: Tampilkan ilustrasi/icon dan pesan yang jelas jika tidak ada data.
- **Header**:
    - Gunakan sticky header jika tabel sangat panjang.
    - Gunakan indikator sorting yang jelas (↑/↓).

## 4. Form Design
Form harus intuitif dan mengurangi beban kognitif.

- **Layout**:
    - Gunakan grid (`.filters-grid` atau `.form-grid`) untuk merapikan field.
    - Grupkan field yang berhubungan menggunakan `<fieldset>` atau pembatas visual.
- **Field Consistency**:
    - **Label**: Selalu di atas input. Gunakan `.form-label`.
    - **Helper Text**: Gunakan `.text-xs.text-muted` di bawah input untuk panduan tambahan.
    - **Sizing**: Gunakan tinggi input yang konsisten (`var(--input-h)`).
- **Validation State**:
    - Gunakan border warna (`var(--danger)`) dan pesan error di bawah field.
    - Berikan feedback saat input sedang validasi (loading spinner jika perlu).
- **Actions**:
    - Tombol utama (Submit/Save) harus paling menonjol.
    - Tombol pembatalan (Cancel/Back) harus menggunakan style secondary/ghost.

## 5. Enterprise Aesthetics
- **Spacing**: Gunakan sistem grid yang konsisten (8px base). Jangan gunakan nilai ad-hoc.
- **Shadows**: Gunakan shadow yang halus (`var(--shadow-sm)`, `var(--shadow-md)`) untuk memberikan kedalaman, bukan garis hitam tebal.
- **Borders**: Gunakan warna border yang sangat halus (`var(--border-subtle)`) agar tidak membuat UI terlihat "boxY".
- **States**: Setiap elemen interaktif HARUS memiliki state: Default, Hover, Active, Focus, dan Disabled.
