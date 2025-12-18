---
description: Panduan commit perubahan kode dengan pesan yang terstruktur
---

# Workflow: Git Commit

## Aturan Commit Message

### Format
```
<type>(<scope>): <subject>

<body>

<footer>
```

### Type (WAJIB)
- `feat`: Fitur baru
- `fix`: Perbaikan bug
- `refactor`: Refaktor kode tanpa ubah fungsionalitas
- `style`: Perubahan format (spasi, koma, dll)
- `docs`: Perubahan dokumentasi
- `test`: Penambahan/perbaikan test
- `chore`: Maintenance (build, deps, dll)

### Scope (OPSIONAL tapi disarankan)
- Nama modul/fitur: `theme`, `sidebar`, `auth`, `sales`, `accounting`
- Area: `css`, `js`, `templates`, `api`

### Subject (WAJIB)
- Huruf kecil, tanpa titik di akhir
- Imperatif: "add", "fix", "change" (bukan "added", "fixed")
- Max 50 karakter

### Body (OPSIONAL)
- Jelaskan APA dan MENGAPA, bukan BAGAIMANA
- Pisahkan dari subject dengan baris kosong
- Wrap di 72 karakter

### Footer (OPSIONAL)
- Breaking changes: `BREAKING CHANGE: ...`
- Issue references: `Fixes #123`, `Closes #456`

---

## Langkah Eksekusi

### 1. Cek Status
// turbo
```bash
git status --short
```

### 2. Review Perubahan (jika perlu)
```bash
git diff --stat
```

### 3. Stage Semua Perubahan
// turbo
```bash
git add -A
```

### 4. Commit dengan Pesan Terstruktur
```bash
git commit -m "<type>(<scope>): <subject>

<body jika ada>"
```

---

## Contoh Commit Messages

### Fitur Baru
```
feat(theme): add dark mode persistence

- Store theme preference in localStorage
- Restore on page load to prevent FOUC
- Follow state-driven architecture
```

### Bug Fix
```
fix(sidebar): resolve toggle not working on mobile

Toggle button was missing event listener on touch devices.
Added touchend handler with proper delegation.

Fixes #42
```

### Refaktor
```
refactor(theme): modularize to state-driven architecture

Split theme feature into separate modules:
- store.js: State + Reducer + Selectors
- effects.js: localStorage persistence
- view.js: DOM rendering
- index.js: Init + Event delegation
```

### Perubahan CSS
```
style(tokens): standardize color variables

Replace --color-* prefix with semantic tokens.
Remove hardcoded hex values from components.
```

---

## Aturan Ketat

1. **JANGAN** commit file yang tidak terkait dengan perubahan
2. **JANGAN** gunakan pesan generik seperti "update", "fix bug", "changes"
3. **SELALU** gunakan format type(scope): subject
4. **SELALU** cek status sebelum commit
5. **JIKA** ada breaking change, WAJIB tulis di footer

---

## Quick Reference

| Command | Fungsi |
|---------|--------|
| `git status -s` | Cek file yang berubah |
| `git diff --stat` | Lihat ringkasan perubahan |
| `git add -A` | Stage semua perubahan |
| `git add <file>` | Stage file tertentu |
| `git commit -m "..."` | Commit dengan pesan |
| `git log -3 --oneline` | Lihat 3 commit terakhir |
