---
description: Panduan menerapkan Midnight Ledger design system di Odyssey ERP
---

# Odyssey UI Design System Guide

## Prinsip Utama

### 1. MODULAR - Tidak Buat File Besar
- **Pisahkan komponen** ke file terpisah, bukan satu file besar
- Struktur folder yang benar:
  ```
  css/
  ├── core/           ← Foundation (tokens, utilities)
  ├── components/     ← Modular component files
  ├── layout/         ← App shell (header, sidebar)
  └── pages/          ← Page-specific styles
  ```

### 2. NO REDUNDANCY - Cek Dulu Sebelum Buat
- **SELALU cek folder yang ada** sebelum membuat file baru
- Jangan buat folder baru jika sudah ada folder dengan fungsi sama
- Contoh kesalahan: membuat `ui/` padahal sudah ada `core/`

### 3. TOKEN-BASED - Gunakan Design Tokens
- Semua komponen HARUS menggunakan tokens dari `core/tokens.css`
- **JANGAN** hardcode warna, spacing, atau sizing
- Contoh benar:
  ```css
  .card { background: var(--card-bg); }
  ```
- Contoh salah:
  ```css
  .card { background: #ffffff; }
  ```

## Struktur CSS

### core/ (Foundation - 2 files)
| File | Isi |
|------|-----|
| `tokens.css` | Design tokens + dark mode |
| `utilities.css` | Reset + utilities + `.numeric` |

### components/ (Modular - 7 files)
| File | Isi |
|------|-----|
| `buttons.css` | `.btn`, `.btn--primary`, dll |
| `forms.css` | `.field`, `.input`, `.label` |
| `cards.css` | `.card`, `.card--floating` |
| `tables.css` | `.table-wrap`, `.table` |
| `misc.css` | `.badge`, `.nav`, `.dropdown` |
| `modals.css` | `.modal`, `.inspector` |
| `feedback.css` | `.toast`, `.alert`, `.skeleton` |

### pages/ (Page-specific)
Contoh: `landing.css`, `login.css`, `dashboard.css`

## Token Naming Convention

### Strict Rules (JANGAN DILANGGAR)
- **Critical State Restoration**: Logic untuk tema (theme) atau state visual kritikal HARUS ada di inline script `<head>` untuk mencegah FOUC.
- **Single Source of Truth**: `ui.js` adalah satu-satunya controller untuk UI state global. Jangan buat logic duplikat di module lain.
- **Event Delegation**: Gunakan delegation di `document` untuk handler global (modal, dropdown, shortcuts), jangan attach ke elemen spesifik yang mungkin belum ada.

### Semantic Tokens (gunakan ini)
- `--bg-app`, `--bg-surface`, `--bg-surface-muted`
- `--text-primary`, `--text-secondary`, `--text-muted`
- `--border-subtle`, `--border-strong`
- `--brand`, `--brand-hover`

### Component Tokens
- `--btn-h`, `--btn-radius`, `--btn-primary-bg`
- `--input-h`, `--input-border`, `--input-border-focus`
- `--card-bg`, `--card-border`, `--card-radius`
- `--table-bg`, `--table-border`, `--table-head-bg`

## CSS Class Naming (BEM-lite)

```
.component           → Base
.component--variant  → Variant (modifier)
.component__element  → Child element
.component.is-state  → State (is-active, is-loading)
```

Contoh:
```css
.card                → Base card
.card--floating      → Floating variant
.card__header        → Card header element
.card.is-clickable   → Clickable state
```

## JavaScript Architecture
- **`js/core/ui.js`**: Core UI logic (Vanilla JS) - Theme, Mobile Menu, Toast (Base). Must be included in head/body.
- **`js/main.js`**: Application Logic (Modules) - Sidebar, Component Init, Complex Interactions.
- **Conflict Avoidance**: Theme logic is handled EXCLUSIVELY by `ui.js`. Do not import `theme.js` in modules.

### UI patterns (ui.js)
```js
OdysseyUI.theme.apply('dark');
OdysseyUI.modal.open('modal-id');
OdysseyUI.toast.show({ title: 'Saved', variant: 'success' });
```

## Checklist Sebelum Membuat File Baru

1. [ ] Cek apakah folder sudah ada
2. [ ] Cek apakah file serupa sudah ada
3. [ ] Pastikan menggunakan tokens, bukan hardcode
4. [ ] Ikuti naming convention yang sudah ada
5. [ ] Update `main.css` jika perlu import baru
