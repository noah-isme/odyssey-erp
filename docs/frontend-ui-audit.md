# Odyssey ERP Frontend UI Audit

Tanggal audit: 15 Juli 2026
P0 diselesaikan: 15 Juli 2026
Ruang lingkup: `web/templates`, `web/static/css`, `web/static/js`, dan referensi template dari kode Go.

## Ringkasan Eksekutif

Frontend Odyssey tidak memerlukan rewrite total. Fondasi design system sudah tersedia melalui token, app shell, dan komponen CSS modular. Namun, halaman aktif saat ini terbagi ke beberapa generasi markup yang tidak memakai kontrak komponen yang sama.

Temuan utama:

- Terdapat 98 template halaman dan semuanya memiliki render target dari kode Go.
- Seluruh nama template yang dipanggil handler tersedia di disk dan dilindungi template integrity test.
- Seluruh dialog telah memakai native `<dialog>` melalui satu kontrak CSS/JS.
- Ada dua keluarga utama markup form dan tabel: komponen canonical (`.form-input`, `.form-select`, `.table`, `.btn`) dan markup legacy/bare (`input`, `select`, `table`, `.grid`, `button.secondary`, `[role="button"]`).
- Hanya ada sembilan pemanggilan partial tingkat halaman di seluruh 101 template. Header, filter, table shell, empty state, dan action bar banyak diduplikasi.
- Design tokens sudah mendukung light/dark theme, spacing, typography, radius, shadow, form, table, badge, modal, dan navigation. Redesign sebaiknya membangun di atas fondasi ini.

Kesimpulan: lakukan redesign berbasis komponen dengan migrasi markup selektif. Jangan mengganti CSS global sekaligus sebelum template legacy dan template mati dipisahkan.

## Baseline Inventaris

| Metrik | Nilai | Catatan |
|---|---:|---|
| Template halaman | 98 | Tidak termasuk layout dan partial |
| Memakai `layouts/base.html` | 96 | Seluruh halaman internal |
| Memakai `layouts/public.html` | 2 | Landing dan login |
| Tanpa deklarasi layout | 0 | Seluruh halaman memiliki layout |
| Halaman dengan tabel | 55 | Total 62 elemen `<table>` |
| Halaman dengan form | 59 | Form adalah pola UI dominan |
| Halaman memakai `.page-container` | 36 | Belum menjadi kontrak layout universal |
| Halaman dengan utility-style classes | 45 | Misalnya `grid-cols-2`, `gap-4`, `mt-4` |
| Halaman dengan inline style/style block | 17 | Sisanya menjadi cleanup pasca-P0 |
| Halaman dengan inline script | 2 | `home.html` dan `ap_payment_form.html` |
| Halaman yang merender native `<dialog>` | 14 | Termasuk reusable delete confirmation pada 8 detail pages |
| Template tidak direferensikan | 0 | 21 orphan telah diverifikasi dan dihapus |
| Referensi template yang hilang | 0 | 18 template telah dipulihkan |

## Kelompok Pola UI

### A. Componentized ERP Pages

Karakteristik:

- Menggunakan `.page-container`, `.page-header`, `.page-content`.
- Tombol menggunakan `.btn` dengan modifier seperti `.btn--primary`.
- Form menggunakan `.form-group`, `.form-label`, `.form-input`, `.form-select`.
- Tabel menggunakan `.table`, `.table-wrap`, dan modifier tabel.
- Banyak menggunakan utility classes untuk layout lokal.

Modul dominan:

- Sales: customers, quotations, dan orders.
- Delivery: list, form, edit, dan detail order.
- Inventory aktif: adjustment, stock take, transfer, dashboard, valuation, dan stock card.
- Accounting aktif: chart of accounts, journals, bank statements, dan reconciliation.
- Finance Banking.

Penilaian: pola paling dekat dengan target redesign. Jadikan keluarga Sales sebagai baseline markup, tetapi standardisasi nama header BEM dan modal terlebih dahulu.

### B. Hybrid Master Data Pages

Karakteristik:

- Memakai `.page-container` dan tombol canonical.
- Filter memakai `.input`, `.filter-group`, dan `.filters-row`, bukan komponen form canonical.
- Setiap list page memiliki inline style kecil untuk kolom action.
- Semua delapan list page sangat mirip dan cocok diekstrak menjadi pola reusable.

Halaman:

- Branches, categories, companies, products, suppliers, taxes, units, dan warehouses.

Penilaian: visualnya relatif dekat dengan pola modern, tetapi markup form/filter perlu dinormalisasi. Terdapat 16 template detail/form yang dipanggil handler tetapi tidak ada di disk, sehingga modul ini juga memiliki risiko fungsional tertinggi.

### C. Semantic Legacy Pages

Karakteristik:

- Mengandalkan elemen HTML semantic dan fallback global: `.container`, `.grid`, `<nav>`, `<article>`, `<figure>`, bare `<button>`, bare `<input>`, dan bare `<select>`.
- Status sering memakai `<mark>` atau teks biasa, bukan badge canonical.
- Struktur header, action, filter, dan empty state berbeda dari pola componentized.
- Tampilan tetap mendapatkan style dari `utilities.css`, tetapi kontraknya rapuh terhadap perubahan reset/global CSS.

Modul dominan:

- Accounts Payable.
- Accounts Receivable aktif.
- Board Packs.
- Insights dan Audit Timeline.
- Permissions, Roles, dan Users.
- Sebagian Close pages.

Penilaian: perlu migrasi markup, bukan sekadar penggantian warna. Perubahan global pada bare `button`, `input`, atau `table` dapat mengubah semua halaman dalam kelompok ini sekaligus.

### D. Specialized Reporting and Operations Pages

Karakteristik:

- Memiliki kebutuhan layout khusus: KPI, chart, financial statement, close checklist, variance, eliminations, dan consolidated reports.
- Menggabungkan komponen canonical, utility classes, dan CSS page-specific.
- `finance/dashboard.html` memuat `analytics.css`; Close memuat `close.css`.
- Banyak tabel numerik membutuhkan alignment, sticky header, print, dan export behavior yang konsisten.

Modul dominan:

- Finance analytics/dashboard, cash flow, budget, consolidation, insights, dan audit.
- Close.
- Eliminations.
- Variance.
- Jobs dashboard.

Penilaian: jangan dipaksa menjadi satu layout generik. Gunakan shell, page header, filter bar, KPI card, table shell, dan status badge yang sama; pertahankan komponen chart/report khusus.

### E. Public and Entry Pages

Halaman:

- Landing.
- Login.
- Home/dashboard awal.

Karakteristik:

- Landing dan login mempunyai CSS page-specific yang besar dan identitas visual sendiri.
- `landing.css` memiliki 1.291 baris dan banyak warna hard-coded.
- `login.css` memiliki 617 baris.
- `home.html` memuat script dashboard tambahan dan memiliki beberapa inline style/event handler.

Penilaian: pisahkan roadmap public marketing dari redesign ERP internal. Tokens merek dapat dibagi, tetapi density dan layout tidak harus sama.

### F. Legacy or Orphan Templates — Resolved

Template berikut telah diverifikasi tidak mempunyai consumer langsung maupun dinamis, lalu dihapus pada P0:

- Accounting: `balance_sheet.html`, `gl.html`, `pnl.html`, `trial_balance.html`.
- AR: `aging_report.html`, `invoice_detail.html`, `invoice_form.html`, `invoices_list.html`, `payment_form.html`, `payments_list.html`.
- Delivery: `orders_by_so.html`.
- Inventory: `adjustments_list.html`, `transfers_list.html`.
- Procurement: `ap_invoice_form.html`, `ap_invoices_list.html`, `ap_payment_form.html`, `ap_payments_list.html`, `prs_list.html`.
- Reports: `bs.html`, `pl.html`, `tb.html`.

Pemeriksaan pasca-cleanup memastikan tidak ada template halaman tersisa tanpa consumer.

## Matriks Modul

| Modul | Template | Pola dominan | Risiko | Arah migrasi |
|---|---:|---|---|---|
| Accounting | 4 | Componentized | Sedang | Selaraskan report/table primitives |
| AP | 7 | Semantic legacy | Tinggi | Migrasikan list, form, detail, payment, lalu aging |
| AR | 4 | Semantic legacy | Tinggi | Migrasikan keluarga AR aktif ke komponen canonical |
| Board Packs | 3 | Semantic legacy | Sedang | Gunakan page header, filter card, table, dan status badge canonical |
| Close | 2 | Specialized | Sedang | Pertahankan `close.css`, selaraskan shell dan controls |
| Delivery | 4 | Componentized | Rendah | Dialog sudah canonical; rapikan controls lainnya |
| Eliminations | 3 | Utility/component hybrid | Sedang | Ekstrak filter, status, dan report table |
| Finance | 11 | Campuran semua pola | Tinggi | Pecah Banking, Analytics, Consolidation, dan Insights/Audit |
| Inventory | 10 | Componentized | Sedang | Jadikan stock/adjustment patterns reusable |
| Jobs | 1 | Page-specific componentized | Rendah | Pertahankan komponen khusus, selaraskan header |
| Master Data | 24 | Hybrid list + canonical form/detail | Sedang | Ekstrak list/filter/table pattern |
| Permissions | 1 | Semantic legacy | Sedang | Selaraskan admin table dan empty/error state |
| Procurement | 5 | Componentized | Sedang | Standardisasi list dan form controls |
| Reports | 0 | Legacy templates removed | Rendah | Report aktif berada di Accounting/Finance |
| Roles | 2 | Semantic list + canonical form | Sedang | Selaraskan admin table |
| Sales | 9 | Componentized | Rendah | Jadikan baseline; rapikan modal dan header naming |
| Users | 2 | Semantic list + provisioning placeholder | Sedang | Implementasikan provisioning sebelum mengaktifkan form |
| Variance | 3 | Utility/component hybrid | Sedang | Pertahankan report-specific layout, standardisasi controls |
| Public/root | 3 | Page-specific | Sedang | Roadmap terpisah dari ERP internal |

## Template yang Dipanggil tetapi Tidak Tersedia — Resolved

P0 memulihkan seluruh template berikut:

- Master Data detail: `branch_detail.html`, `category_detail.html`, `company_detail.html`, `product_detail.html`, `supplier_detail.html`, `tax_detail.html`, `unit_detail.html`, `warehouse_detail.html`.
- Master Data form: `branch_form.html`, `category_form.html`, `company_form.html`, `product_form.html`, `supplier_form.html`, `tax_form.html`, `unit_form.html`, `warehouse_form.html`.
- RBAC: `roles/form.html` dan `users/form.html`.

Dua reusable shell baru (`partials/masterdata/form.html` dan `partials/masterdata/detail.html`) menjaga 16 halaman Master Data tetap konsisten. Template RBAC dibuat terpisah sesuai kemampuan handler. Runtime render test mencakup seluruh 18 template.

## Audit Komponen

### Layout dan Header

Empat bentuk header digunakan bersamaan:

- `.page-header-content` + `.page-actions`.
- `.page-header__content` + `.page-header__actions`.
- `<section class="page-header">` dengan child generik.
- `<header>`/`<hgroup>` tanpa class komponen.

Rekomendasi: tetapkan satu kontrak, misalnya `.page-header`, `.page-header__content`, `.page-header__actions`, `.page-title`, dan `.page-subtitle`. Sediakan modifier untuk compact/report, bukan struktur baru.

### Buttons

Canonical API sudah jelas: `.btn` dengan `--primary`, `--secondary`, `--ghost`, `--danger`, dan size modifiers. Namun halaman masih memakai bare button, `button.secondary`, `.button`, `[role="button"]`, dan satu penggunaan `btn-primary` tanpa BEM modifier.

Rekomendasi: migrasikan semua action eksplisit ke `.btn`. Sisakan bare-button styling hanya sebagai compatibility layer sementara dan tandai untuk dihapus setelah migrasi.

### Forms dan Filters

Terdapat variasi `.form-input`, `.form-select`, `.input`, `.field`, bare inputs, `.filters-card`, `.filters-form`, `.filter-grid`, `.filters-row`, dan plain `.grid`.

Masalah CSS konkret: `.form-group` didefinisikan dua kali berurutan di `forms.css`; deklarasi kedua mengubah gap dari `var(--space-2)` menjadi `var(--space-4)`.

Rekomendasi: bentuk tiga primitive: field, form grid, dan filter bar. Gunakan satu error/help contract dan jangan menggantungkan halaman bisnis pada selector bare input.

### Tables

Variasi class yang ditemukan:

- `.table` pada 30 tabel.
- `.table.table--sticky` pada 9 tabel.
- `.table.data-table` pada 8 tabel.
- `.data-table` tanpa `.table` pada 7 tabel.
- `.table.table--compact` pada 4 tabel.
- `striped`, `period-table`, `mini-table`, dan `checklist-table` masing-masing sebagai pola khusus.

Hanya tiga dari 79 tabel memiliki `<caption>`. Tidak semua tabel membutuhkan caption visual, tetapi setiap tabel report perlu accessible name melalui caption atau heading/`aria-labelledby`.

Rekomendasi: `.table` wajib sebagai base; modifier untuk sticky, compact, striped, numeric, selectable, dan responsive overflow. `data-table` sebaiknya menjadi behavior hook (`data-datatable`), bukan style base kedua.

### Cards, Status, dan Empty State

`.card` sudah tersedia, tetapi padding dan struktur bervariasi antara bare `.card`, `.card__header`, `<article>`, dan page-specific cards. Status ditampilkan melalui `.badge`, `.status-badge`, `<mark>`, atau teks biasa. Empty state canonical hanya dipakai oleh 13 halaman.

Rekomendasi: satu card primitive, satu status mapping per domain state, dan satu partial empty state yang menerima icon/title/description/action.

### Modal dan Dialog — Resolved

Seluruh dialog sekarang menggunakan kontrak tunggal:

1. `<dialog class="native-dialog" data-dialog>` sebagai primitive.
2. `[data-dialog-open]` dan `[data-dialog-close]` sebagai behavior hooks.
3. `features/modal/index.js` sebagai satu-satunya lifecycle manager.
4. Native top layer, focus trap, Escape, dan `::backdrop` dari browser; module menangani outside click, focus restore, dan cleanup listener melalui `AbortController`.

CSS dan JavaScript modal lama serta tiga page-specific modal managers telah dihapus.

### JavaScript Behavior

`main.js` menginisialisasi seluruh feature module pada setiap halaman. Ini aman bila setiap `init()` no-op saat target tidak ada, tetapi menambah coupling dan biaya regresi. Terdapat inline event handler pada delapan halaman dan inline script pada dua halaman.

Rekomendasi: pertahankan event delegation global untuk behavior umum, pindahkan inline handlers ke `data-action`, lalu lakukan mount feature berdasarkan marker halaman atau komponen.

### Reuse dan Partials

Selain layout, sidebar, header, dan flash global, hanya sembilan pemanggilan partial ditemukan pada template halaman. Reuse terbesar terbatas pada Finance dan Close.

Kandidat partial/primitive prioritas:

- Page header.
- Filter bar/form grid.
- Table shell dan pagination.
- Status badge.
- Empty state.
- Form actions.
- Native dialog shell.
- KPI/summary card.

## Responsive dan Accessibility

Risiko responsive:

- Sebanyak 23 halaman memakai fixed `grid-cols-2/3/4`; utility tersebut tidak mempunyai breakpoint otomatis.
- Table wrapper belum konsisten di semua tabel.
- App shell responsif, tetapi isi halaman legacy dapat tetap overflow pada layar kecil.
- Native dialog dan form grids menggunakan beberapa kontrak ukuran berbeda.

Risiko accessibility:

- Modal memiliki lifecycle dan focus behavior yang tidak seragam.
- Inline `onclick` menghambat konsistensi keyboard/event handling.
- Icon-only buttons perlu audit `aria-label`; saat ini hanya 11 button yang secara eksplisit memiliki label tersebut.
- Report tables perlu accessible naming dan alignment numerik yang konsisten.
- Focus ring global di-reset, sehingga semua komponen interaktif harus memastikan `:focus-visible` sendiri.

## Prioritas Redesign

### P0 — Stabilkan Kontrak

1. ~~Putuskan dan perbaiki 18 template yang hilang.~~ Selesai.
2. ~~Konfirmasi lalu hapus/arsipkan 21 template orphan.~~ Selesai.
3. ~~Satukan modal/dialog agar bug visual tidak menyebar.~~ Selesai.
4. Tetapkan kontrak canonical untuk page header, button, field, table, badge, dan empty state.

### P1 — Rapikan Fondasi

1. Hilangkan duplikasi selector di `utilities.css` dan `forms.css`.
2. Tambahkan responsive grid primitives dan table overflow contract.
3. Pisahkan style compatibility legacy dari komponen canonical.
4. Buat visual fixture/component gallery untuk light, dark, desktop, dan mobile.

### P2 — Migrasi Halaman Aktif

Urutan yang disarankan:

1. Sales sebagai reference implementation.
2. Delivery, Inventory, Accounting, dan Banking.
3. Master Data setelah template detail/form dipulihkan.
4. AP dan AR.
5. Board Packs, Insights, Audit, Close, Eliminations, dan Variance.
6. Public pages sebagai stream desain terpisah.

### P3 — Quality Gates

1. Screenshot regression untuk viewport desktop dan mobile.
2. Keyboard/focus test untuk modal, menu, form, dan table action.
3. Audit contrast pada light/dark theme.
4. Template-route integrity test agar handler tidak dapat merujuk template yang hilang.
5. Larangan baru untuk inline style, inline script, dan inline event handler.

## Definition of Done untuk Fase Audit

- Semua 98 template aktif telah masuk inventaris modul dan kelompok pola.
- Kandidat orphan telah dihapus dan seluruh referensi template tersedia.
- Komponen utama dan variasi kontraknya telah dicatat.
- Risiko responsive, accessibility, dan JavaScript telah dicatat.
- Urutan migrasi tersedia tanpa mengharuskan rewrite seluruh halaman sekaligus.

Audit ini bersifat struktural/static. Validasi visual runtime per route dan screenshot regression sebaiknya menjadi langkah berikutnya setelah daftar template aktif dikonfirmasi.
