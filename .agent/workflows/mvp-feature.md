---
description: Panduan MVP Feature Development untuk menghindari over-engineering dan looping
---

# MVP Feature Development Guide

## Prinsip Anti-Looping

### 1. SCOPE KECIL - Satu Fitur, Satu Focus
- **Definisikan scope SEBELUM mulai coding**
- Jangan expand scope di tengah jalan
- Jika menemukan masalah lain → CATAT, jangan langsung fix

### 2. VERIFY EARLY - Test Setelah Setiap Perubahan
- **Buat perubahan kecil → verify → lanjut**
- Jangan stack 10 perubahan tanpa testing
- Jika error, rollback ke state terakhir yang working

### 3. TIME-BOX - Batasi Waktu per Task
- **Max 3 attempts** untuk fix satu bug
- Jika tidak bisa fix setelah 3x → tanya user
- Jangan looping tanpa progress visible

## Workflow MVP

```
1. DEFINE  → Apa yang harus jadi? (max 3 bullet points)
2. CHECK   → File apa yang perlu diubah?
3. EDIT    → Buat perubahan MINIMAL
4. VERIFY  → Test apakah working
5. COMMIT  → Jika OK, commit segera
```

## Scope Control

### ✅ Scope yang BENAR
```
Task: "Tambah tombol Export CSV di halaman Customers"

Scope:
- [ ] Tambah button di template
- [ ] Buat handler export
- [ ] Test download working
```

### ❌ Scope yang SALAH (Over-engineering)
```
Task: "Tambah tombol Export CSV di halaman Customers"

JANGAN expand ke:
- Refactor seluruh template system
- Buat generic export framework
- Redesign UI halaman
- Fix unrelated CSS bugs
```

## Checklist Sebelum Mulai

1. [ ] **Scope jelas?** - Max 3 deliverables
2. [ ] **File target?** - Sudah tahu file mana yang diubah
3. [ ] **Test plan?** - Bagaimana verify hasilnya
4. [ ] **Rollback plan?** - Bisa revert jika gagal

## Anti-Looping Rules

### Rule 1: Satu Bug = Max 3 Attempts
```
Attempt 1: Fix dengan solusi pertama
Attempt 2: Coba pendekatan berbeda
Attempt 3: Debug lebih dalam
→ Jika masih gagal → STOP, tanya user
```

### Rule 2: Jangan Fix Cascade Issues
```
Menemukan bug A saat fix bug B?
→ CATAT bug A di catatan
→ SELESAIKAN bug B dulu
→ Tanya user apakah perlu fix bug A
```

### Rule 3: Verify Sebelum Lanjut
```
Selesai edit file X
→ Build/refresh untuk verify
→ Jika OK, lanjut ke file Y
→ Jika ERROR, fix dulu sebelum lanjut
```

### Rule 4: Commit Incremental
```
Fitur besar? Pecah jadi commits kecil:
1. commit: "feat: add export button to template"
2. commit: "feat: implement export handler"
3. commit: "feat: add CSV generation logic"
```

## Template Task Breakdown

```markdown
## Task: [Nama Fitur]

### Deliverables (max 3)
1. [ ] ...
2. [ ] ...
3. [ ] ...

### Files to Change
- `path/to/file1.go`
- `path/to/template.html`

### Test Plan
- [ ] Verify X works
- [ ] Check Y displays correctly

### Out of Scope (JANGAN sentuh)
- Unrelated CSS fixes
- Refactoring existing code
- New features not requested
```

## Kapan STOP dan Tanya User

1. **Bug tidak bisa difix** setelah 3 attempts
2. **Scope creep** - menemukan issue lain yang perlu fix
3. **Keputusan desain** - ada beberapa pilihan approach
4. **Breaking change** - perubahan akan affect fitur lain
5. **Tidak yakin** - requirements tidak jelas

## Common Looping Patterns (HINDARI)

### ❌ Pattern: CSS Whack-a-Mole
```
Fix header color → sidebar broken
Fix sidebar → button broken
Fix button → header broken lagi
→ STOP! Step back, cek root cause
```

### ❌ Pattern: Endless Refactoring
```
Lihat code "jelek" → refactor
Refactor lagi → jelek di tempat lain
→ STOP! Fokus ke task original
```

### ❌ Pattern: Dependency Rabbit Hole
```
Update package A → requires B
B requires C → C requires D
→ STOP! Tanya user dulu sebelum upgrade chain
```

## Quick Reference

| Situasi | Action |
|---------|--------|
| Bug simple | Fix langsung, verify, commit |
| Bug kompleks | Max 3 attempts, lalu tanya user |
| Scope creep | Catat issue, tanya user |
| Tidak yakin | Tanya user sebelum implementasi |
| Banyak file berubah | Commit incremental |
| Test gagal | Rollback, coba approach lain |
