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

---

## Workflow MVP

```
1. DEFINE  → Apa yang harus jadi? (max 3 bullet points)
2. CHECK   → File apa yang perlu diubah?
3. EDIT    → Buat perubahan MINIMAL
4. VERIFY  → Test apakah working
5. COMMIT  → Jika OK, commit segera
```

---

## Scope Control

### ✅ Scope yang BENAR
```
Task: "Tambah ComboBox untuk select customer"

Scope:
- [ ] Buat feature files (store/effects/view/index)
- [ ] Buat CSS component
- [ ] Test keyboard navigation works
```

### ❌ Scope yang SALAH (Over-engineering)
```
Task: "Tambah ComboBox untuk select customer"

JANGAN expand ke:
- Refactor seluruh form system
- Buat generic component framework
- Redesign UI halaman
- Fix unrelated CSS bugs
```

---

## Checklist Sebelum Mulai

### Backend Feature
1. [ ] **Scope jelas?** - Max 3 deliverables
2. [ ] **Files target?** - domain.go, service.go, handler.go
3. [ ] **Test plan?** - Unit test atau manual test
4. [ ] **Rollback plan?** - Bisa revert jika gagal

### Frontend Feature
1. [ ] **Scope jelas?** - Max 3 deliverables
2. [ ] **Pattern?** - 4-file (complex) atau 1-file (simple)
3. [ ] **Files target?**
   - [ ] `features/<name>/store.js`
   - [ ] `features/<name>/effects.js`
   - [ ] `features/<name>/view.js`
   - [ ] `features/<name>/index.js`
   - [ ] `components/<name>.css`
   - [ ] `main.js` (import)
   - [ ] `main.css` (import)
4. [ ] **State shape?** - Definisikan state sebelum coding
5. [ ] **Action types?** - List semua actions
6. [ ] **Test plan?** - Console test atau browser test

---

## Frontend Feature Checklist (State-Driven)

### Step 1: Define State Shape
```javascript
// SEBELUM coding, tulis state shape
{
    isOpen: false,
    data: null,
    loading: false,
    error: null
}
```

### Step 2: Define Actions
```javascript
// List semua actions
'FEATURE_OPEN'
'FEATURE_CLOSE'
'FEATURE_SET_DATA'
'FEATURE_SET_LOADING'
'FEATURE_SET_ERROR'
```

### Step 3: Create Files in Order
1. `store.js` → State + Reducer + Selectors
2. `effects.js` → Side effects
3. `view.js` → DOM rendering
4. `index.js` → Event delegation + Public API
5. Update `main.js` → Import + init
6. Create CSS → `components/<name>.css`
7. Update `main.css` → Import CSS

### Step 4: Verify Each Layer
```
[ ] State changes correctly (console.log di reducer)
[ ] Effects run after state change
[ ] View updates DOM correctly
[ ] Events dispatch actions (not mutate DOM)
[ ] Global API works (window.OdysseyFeature)
```

---

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
1. commit: "feat(form): add store with state and reducer"
2. commit: "feat(form): add effects for async validation"
3. commit: "feat(form): add view for error rendering"
4. commit: "feat(form): add index with event delegation"
```

---

## Template Task Breakdown

### Backend Feature
```markdown
## Task: [Nama Fitur]

### Deliverables (max 3)
1. [ ] Create domain.go with entities
2. [ ] Create service.go with business logic
3. [ ] Create handler.go with routes

### Files to Change
- `internal/<domain>/domain.go`
- `internal/<domain>/service.go`
- `internal/<domain>/handler.go`
- `internal/app/router.go`

### Test Plan
- [ ] Unit test service methods
- [ ] Manual test via browser

### Out of Scope (JANGAN sentuh)
- Unrelated domains
- Frontend changes
- Database migrations not required
```

### Frontend Feature
```markdown
## Task: [Nama Fitur]

### State Shape
```javascript
{ isOpen: false, data: null, loading: false, error: null }
```

### Actions
- OPEN, CLOSE, SET_DATA, SET_LOADING, SET_ERROR

### Deliverables (max 3)
1. [ ] Create 4-file feature structure
2. [ ] Create CSS component
3. [ ] Expose global API

### Files to Create/Change
- `features/<name>/store.js` [NEW]
- `features/<name>/effects.js` [NEW]
- `features/<name>/view.js` [NEW]
- `features/<name>/index.js` [NEW]
- `components/<name>.css` [NEW]
- `main.js` [MODIFY: add import]
- `main.css` [MODIFY: add import]

### Test Plan
- [ ] Console: OdysseyFeature.open('test')
- [ ] Keyboard: Arrow keys work
- [ ] State: Check with selectors

### Out of Scope (JANGAN sentuh)
- Other features
- Backend changes
- Unrelated CSS fixes
```

---

## Kapan STOP dan Tanya User

1. **Bug tidak bisa difix** setelah 3 attempts
2. **Scope creep** - menemukan issue lain yang perlu fix
3. **Keputusan desain** - ada beberapa pilihan approach
4. **Breaking change** - perubahan akan affect fitur lain
5. **Tidak yakin** - requirements tidak jelas
6. **State shape unclear** - tidak tahu structure yang tepat

---

## Common Looping Patterns (HINDARI)

### ❌ Pattern: State Mutation Error
```
State tidak update → tambah direct DOM change
DOM change → mismatch dengan state
→ STOP! Cek reducer apakah return new object
```

### ❌ Pattern: Event Handler Chaos
```
Click tidak work → tambah listener lagi
Multiple listeners → fire multiple times
→ STOP! Cek event delegation pattern
```

### ❌ Pattern: CSS Whack-a-Mole
```
Fix header color → sidebar broken
Fix sidebar → button broken
→ STOP! Step back, cek CSS specificity
```

### ❌ Pattern: Import Error Loop
```
Module not found → change import path
Still not found → change again
→ STOP! Verify file exists, check main.js
```

---

## Quick Reference

| Situasi | Action |
|---------|--------|
| Bug simple | Fix langsung, verify, commit |
| Bug kompleks | Max 3 attempts, lalu tanya user |
| Scope creep | Catat issue, tanya user |
| Tidak yakin | Tanya user sebelum implementasi |
| Banyak file berubah | Commit incremental |
| Test gagal | Rollback, coba approach lain |
| State tidak update | Cek reducer return new object |
| Event tidak fire | Cek event delegation di document |
