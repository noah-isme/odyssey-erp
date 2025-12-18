---
description: Panduan arsitektur CSS - fondasi, mental model, dan aturan debugging
---

# Arsitektur CSS untuk Odyssey ERP

> **Mental Model Utama:**  
> JS = alur eksekusi  
> CSS = konflik yang harus dikelola

---

## Fondasi CSS (Level Dasar Wajib)

### 1) Cascade — "Siapa yang Menang?"

CSS dibaca berdasarkan urutan prioritas:

1. `!important` (hindari)
2. Specificity (kekuatan selector)
3. Urutan deklarasi (yang terakhir menang)

**Kalau warna/style tidak berubah → Periksa cascade.**

---

### 2) Specificity — "Seberapa Kuat Selector?"

Urutan kekuatan (lemah → kuat):

```
element < class < attribute < id < inline < !important
```

❌ **Jangan:**
```css
button { color: red; }
```

✅ **Gunakan:**
```css
.btn.primary { color: red; }
```

> **Aturan:** Jika specificity tidak dikendalikan, stylesheet menjadi arena gladiator.

---

### 3) Box Model — "Bagaimana Ukuran Dihitung?"

Default CSS:
```
width = content + padding + border
```

**Solusi standar (sudah diterapkan di reset.css):**
```css
*, *::before, *::after {
  box-sizing: border-box;
}
```

**Gejala tanpa pemahaman box model:**
- Layout meleset
- Scroll misterius muncul
- Padding "terasa salah"

---

### 4) Layout System — "Cara Elemen Disusun"

| Kebutuhan | Gunakan |
|-----------|---------|
| Satu baris/kolom | Flexbox |
| Layout halaman/grid | CSS Grid |
| Flow normal | Block/Inline default |

**Aturan cepat:**
- 1 dimensi → `display: flex`
- 2 dimensi → `display: grid`

> **Warning:** Kalau layout pakai margin untuk segalanya, itu bukan layout — itu negosiasi paksa.

---

### 5) Positioning — "Hubungan dengan Dunia Sekitar"

| Value | Perilaku |
|-------|----------|
| `static` | Default, ikut flow |
| `relative` | Anchor untuk absolute child |
| `absolute` | Keluar dari flow, relatif ke parent positioned |
| `fixed` | Nempel ke viewport |
| `sticky` | Hibrida (scroll-aware) |

**Bug paling umum:**  
`position: absolute` tanpa parent yang `position: relative`.

---

## Fondasi CSS (Level Lanjut)

### 6) Inheritance — "Properti yang Diwariskan"

| Diwariskan | Tidak Diwariskan |
|------------|------------------|
| `color`, `font-*`, `line-height` | `margin`, `padding`, `border` |
| `text-align`, `visibility` | `width`, `height`, `display` |

**Kalau font tiba-tiba aneh → Cek parent element.**

---

### 7) Stacking Context (z-index) — "Kenapa z-index 999 Tetap Kalah?"

`z-index` hanya bekerja dalam **stacking context yang sama**.

**Yang membuat stacking context baru:**
- `position` + `z-index`
- `transform`
- `opacity < 1`
- `filter`
- `isolation: isolate`

**Solusi tooltip/modal ketimbun:**
```css
.modal-container {
  isolation: isolate;
  z-index: var(--z-modal); /* dari design tokens */
}
```

---

### 8) Responsive Design — "CSS di Dunia yang Berubah Ukuran"

**Prinsip:**
1. Mobile first
2. Gunakan `min-width` > `max-width`
3. Jangan desain desktop lalu mengecilkan

**Pattern:**
```css
/* Base: mobile */
.container { padding: var(--space-sm); }

/* Tablet ke atas */
@media (min-width: 768px) {
  .container { padding: var(--space-md); }
}

/* Desktop */
@media (min-width: 1024px) {
  .container { padding: var(--space-lg); }
}
```

> Media query bukan fitur tambahan — itu realita.

---

## Debugging Checklist

| Masalah | Periksa |
|---------|---------|
| Layout rusak | Box model, flow |
| Style tidak diterapkan | Cascade, specificity |
| Elemen hilang/ketutup | Positioning, stacking context |
| Font aneh | Inheritance |
| Responsive pecah | Media query order |

---

## Aturan CSS di Odyssey ERP

### Struktur File

```
web/static/css/
├── base/
│   ├── reset.css      # CSS reset + box-sizing
│   └── typography.css # Font system
├── tokens/
│   └── design-tokens.css # CSS custom properties
├── components/
│   └── [component].css   # Component-specific styles
├── layouts/
│   └── [layout].css      # Page layouts
└── main.css              # Entry point, @import semua
```

### Naming Convention (BEM-lite)

```css
/* Block */
.card { }

/* Element */
.card__header { }
.card__body { }

/* Modifier */
.card--highlighted { }
.btn--primary { }
```

### Custom Properties (CSS Variables)

Selalu gunakan design tokens:

```css
/* ✅ Benar */
.element {
  color: var(--color-text-primary);
  padding: var(--space-md);
  border-radius: var(--radius-sm);
}

/* ❌ Jangan */
.element {
  color: #333;
  padding: 16px;
  border-radius: 4px;
}
```

### Z-Index Scale

Gunakan tokens, bukan angka random:

```css
:root {
  --z-dropdown: 100;
  --z-sticky: 200;
  --z-modal-backdrop: 300;
  --z-modal: 400;
  --z-tooltip: 500;
  --z-toast: 600;
}
```

---

## Referensi Cepat

```
┌─────────────────────────────────────────────────────┐
│                    MENTAL MODEL                     │
├─────────────────────────────────────────────────────┤
│  JS  = Alur eksekusi (langkah demi langkah)        │
│  CSS = Konflik (siapa yang menang?)                │
├─────────────────────────────────────────────────────┤
│  Layout rusak?     → Box model / Flow              │
│  Style diabaikan?  → Cascade / Specificity         │
│  Elemen hilang?    → Positioning / Stacking        │
└─────────────────────────────────────────────────────┘
```
