---
description: Panduan arsitektur HTML - fondasi, semantic structure, dan aturan markup
---

# Arsitektur HTML untuk Odyssey ERP

> **Mental Model Utama:**  
> HTML = kerangka, bukan cat  
> Tag menjelaskan MAKNA, bukan tampilan

---

## Fondasi HTML (Level Dasar Wajib)

### 1) Semantic Structure — "Tag Bukan Dekorasi"

HTML menjelaskan **makna**, bukan tampilan.

| Tag | Fungsi |
|-----|--------|
| `<header>` | Pembuka halaman/section |
| `<nav>` | Navigasi |
| `<main>` | Konten utama (1 per halaman) |
| `<section>` | Bagian tematik |
| `<article>` | Konten berdiri sendiri |
| `<aside>` | Konten pendukung |
| `<footer>` | Penutup halaman/section |

❌ **Jangan:**
```html
<div class="header">...</div>
<div class="nav">...</div>
```

✅ **Gunakan:**
```html
<header>...</header>
<nav>...</nav>
```

**Kenapa penting?**
- SEO lebih baik
- Accessibility bawaan
- Struktur mudah dipahami JS & CSS

---

### 2) Document Outline — "Hierarki yang Masuk Akal"

| Aturan | Penjelasan |
|--------|------------|
| `<h1>` satu per halaman | Judul utama |
| Heading tidak loncat level | `<h1>` → `<h2>` → `<h3>` |
| Section punya heading | Setiap `<section>` perlu judul |

❌ **Salah:**
```html
<h1>Dashboard</h1>
<h4>Sales Overview</h4>  <!-- Loncat dari h1 ke h4 -->
```

✅ **Benar:**
```html
<h1>Dashboard</h1>
<section>
  <h2>Sales Overview</h2>
</section>
```

> Struktur buruk → pembaca dan screen reader tersesat.

---

### 3) Flow vs Interactive Content — "Nesting yang Benar"

Tidak semua elemen boleh bersarang sembarangan.

❌ **Salah (konflik interaktif):**
```html
<button>
  <a href="/save">Save</a>
</button>
```

✅ **Benar:**
```html
<a href="/save" class="btn">Save</a>
<!-- atau -->
<button type="submit">Save</button>
```

**Aturan nesting:**
- `<a>` tidak boleh dalam `<button>`
- `<button>` tidak boleh dalam `<a>`
- `<form>` tidak boleh dalam `<form>`
- `<p>` tidak boleh mengandung block element

---

### 4) Forms as First-Class Citizen — "Form adalah Fitur Inti"

Form adalah fitur INTI web, bukan aksesoris.

**Aturan Form:**
```html
<!-- ✅ Form yang benar -->
<form action="/api/customers" method="POST">
  <div class="form-group">
    <label for="name">Nama Customer</label>
    <input type="text" id="name" name="name" required>
  </div>
  
  <button type="submit">Simpan</button>
</form>
```

| Element | Fungsi |
|---------|--------|
| `<label>` | HARUS terhubung ke input (`for="id"`) |
| `name` | Kontrak data (key untuk backend) |
| `method` & `action` | Arsitektur endpoint |
| `required`, `pattern` | Validasi native |

**Form yang benar:**
- Bisa submit tanpa JS
- Validasi dasar jalan
- Aksesibel

> ERP hidup dari form. HTML menang di sini.

---

### 5) Attributes are Data — "Attribute Bukan Hiasan"

| Attribute | Fungsi |
|-----------|--------|
| `name`, `value` | Data untuk submit |
| `required`, `disabled` | State |
| `aria-*` | Accessibility |
| `data-*` | Hook untuk JS |

**Pattern di Odyssey ERP:**
```html
<button 
  type="button"
  data-action="delete"
  data-id="123"
  aria-label="Hapus customer"
>
  Hapus
</button>
```

> JS yang sehat: baca dari attribute, bukan tebak DOM.

---

## Fondasi HTML (Level Lanjut)

### 6) Accessibility by Default — "HTML Sudah Aksesibel"

HTML punya aksesibilitas bawaan jika digunakan dengan benar.

| Gunakan | Bukan | Alasan |
|---------|-------|--------|
| `<button>` | `<div onclick>` | Focusable, keyboard support |
| `<a href>` | `<span onclick>` | Native navigation |
| `<label for>` | `<div>Label</div>` | Screen reader support |
| `<input type="checkbox">` | Custom div | Native form support |

> Pakai elemen semantik = 70% aksesibel tanpa usaha ekstra.

---

### 7) Progressive Enhancement — "HTML Harus Berguna Sendiri"

Urutan sehat:
1. **HTML** bisa dibaca & submit
2. **CSS** mempercantik
3. **JS** mempercepat & memperkaya

**Test sederhana:**
- Matikan JS → form masih bisa submit?
- Matikan CSS → konten masih bisa dibaca?

> Kalau JS mati dan sistem jadi nol fungsi: arsitekturnya rapuh.

---

### 8) Valid & Predictable Markup — "Browser Konsisten"

| Aturan | Contoh |
|--------|--------|
| Nesting benar | `<ul>` hanya berisi `<li>` |
| ID unik | Tidak duplikat `id` dalam halaman |
| Tag ditutup | Self-closing untuk void elements |
| Attribute dikutip | `class="..."` bukan `class=...` |

> Browser itu toleran, tapi toleransi itu hutang teknis.

---

## Template Pattern untuk Odyssey ERP

### Page Layout
```html
<!DOCTYPE html>
<html lang="id" data-theme="light">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{ .Title }} - Odyssey ERP</title>
</head>
<body>
  <div class="app-shell">
    <header class="app-header">...</header>
    <nav class="app-sidebar">...</nav>
    <main class="app-main">
      <div class="page-container">
        {{ template "content" . }}
      </div>
    </main>
  </div>
</body>
</html>
```

### Form Pattern
```html
<form 
  action="/api/{{ .Resource }}" 
  method="POST"
  data-component="form"
  data-validate="true"
>
  <div class="form-group">
    <label for="field-name">Label</label>
    <input 
      type="text" 
      id="field-name" 
      name="field_name"
      required
      aria-describedby="field-name-error"
    >
    <span id="field-name-error" class="field-error" hidden></span>
  </div>
  
  <div class="form-actions">
    <button type="submit" class="btn btn--primary">Simpan</button>
    <a href="/{{ .Resource }}" class="btn btn--secondary">Batal</a>
  </div>
</form>
```

### Table Pattern
```html
<div class="table-wrap" data-component="datatable">
  <table class="table">
    <thead>
      <tr>
        <th scope="col">Nama</th>
        <th scope="col">Email</th>
        <th scope="col">Actions</th>
      </tr>
    </thead>
    <tbody>
      {{ range .Items }}
      <tr data-id="{{ .ID }}">
        <td>{{ .Name }}</td>
        <td>{{ .Email }}</td>
        <td>
          <button type="button" data-action="edit">Edit</button>
        </td>
      </tr>
      {{ end }}
    </tbody>
  </table>
</div>
```

---

## Debugging Checklist

| Masalah | Periksa |
|---------|---------|
| Form tidak submit | `name` attribute ada? |
| Screen reader bingung | Semantic tags benar? |
| SEO buruk | Heading hierarchy? |
| JS tidak dapat element | `id` atau `data-*` benar? |
| Styling tidak apply | Nesting HTML valid? |

---

## Referensi Cepat

```
┌─────────────────────────────────────────────────────┐
│                    MENTAL MODEL                     │
├─────────────────────────────────────────────────────┤
│  HTML = Makna (struktur)                           │
│  CSS  = Tampilan (style)                           │
│  JS   = Perilaku (interaksi)                       │
├─────────────────────────────────────────────────────┤
│  Element salah?    → Semantic structure            │
│  Form tidak jalan? → name, action, method          │
│  Tidak aksesibel?  → label, button, a href         │
│  JS tidak dapat?   → id, data-*, attribute         │
└─────────────────────────────────────────────────────┘
```
