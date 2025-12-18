---
description: Aturan arsitektur UI state-driven untuk Odyssey ERP (Vanilla JS)
---

# Odyssey UI Architecture Guide

## Prinsip Inti: DOM adalah Output, Bukan Sumber Kebenaran

```
Event → Action → Reducer(update state) → Render → DOM
```

---

## 1. State Management

### Aturan State
- **Single Source of Truth** per fitur
- **State harus serializable** (tidak menyimpan DOM node, timer id, atau function)
- **Derived data dihitung**, bukan disimpan (e.g., `filteredRows` dari `rows + filters`)
- **Side-effects dipisah** di layer effects (fetch, localStorage, websocket)

### Struktur Store
```javascript
// features/theme/store.js
const ThemeStore = {
    state: { theme: 'light' },
    
    reducer(state, action) {
        switch (action.type) {
            case 'THEME_SET':
                return { ...state, theme: action.payload };
            default:
                return state;
        }
    },
    
    // Selectors (derived data)
    selectors: {
        isDark: (state) => state.theme === 'dark'
    }
};
```

---

## 2. Event Handling

### Aturan
- **Satu listener per root feature** (event delegation di `document`)
- **Semua event menghasilkan action**: `dispatch({ type, payload })`
- **TIDAK ADA DOM mutation langsung di handler**

### Contoh Benar
```javascript
// ✅ Handler hanya dispatch
document.addEventListener('click', (e) => {
    const toggle = e.target.closest('[data-theme-toggle]');
    if (toggle) {
        dispatch({ type: 'THEME_TOGGLE' });
    }
});
```

### Contoh Salah
```javascript
// ❌ Handler langsung manipulasi DOM
document.addEventListener('click', (e) => {
    const toggle = e.target.closest('[data-theme-toggle]');
    if (toggle) {
        document.documentElement.setAttribute('data-theme', 'dark');
        localStorage.setItem('theme', 'dark');
        document.querySelector('.icon-sun').style.display = 'none';
    }
});
```

---

## 3. Effects (Side Effects)

### Aturan
- Effects terpisah dari reducer (reducer pure function)
- Effects dijalankan SETELAH state berubah
- Effects untuk: API calls, localStorage, debounce, websocket

### Struktur
```javascript
// features/theme/effects.js
const ThemeEffects = {
    persist(state) {
        try {
            localStorage.setItem('odyssey.theme', state.theme);
        } catch (e) {
            console.error('Theme persist failed:', e);
        }
    },
    
    restore() {
        try {
            return localStorage.getItem('odyssey.theme');
        } catch (e) {
            return null;
        }
    }
};
```

---

## 4. Rendering

### Aturan
- **Render minimal, batch, dan idempotent**
- **Cache referensi node penting** (jangan query ulang tiap render)
- **Gunakan kelas/atribut untuk state visual** (toggle `[data-state="open"]`)
- **Update parsial** (hanya bagian yang berubah)

### Contoh Render Function
```javascript
// features/theme/view.js
const ThemeView = {
    root: document.documentElement,
    
    render(state) {
        // Satu pintu update UI
        if (state.theme === 'dark') {
            this.root.setAttribute('data-theme', 'dark');
        } else {
            this.root.removeAttribute('data-theme');
        }
    }
};
```

---

## 5. Komponen Contract

### Lifecycle
```javascript
const MyComponent = {
    mount(root, props, ctx) { /* ... */ },
    update(nextProps, ctx) { /* ... */ },
    destroy() { /* cleanup listener/observer */ }
};
```

### Aturan
- Komponen hanya menyentuh `root` miliknya
- Gunakan `data-*` attributes sebagai contract (bukan selector rapuh)
- Pisahkan: **Dumb component** (presentational) vs **Smart component** (container)

---

## 6. Performa DOM

### Aturan Anti-Reflow
1. **Batch DOM updates**: Buat fragment dulu, lalu `replaceChildren()` sekali
2. **Pisahkan read/write phase**: Jangan bolak-balik `getBoundingClientRect()` lalu `style.left`
3. **Gunakan kelas untuk state visual**: Toggle class, bukan set 20 inline styles
4. **Schedule render**: Gabung render via `requestAnimationFrame`

### Untuk Tabel Besar
- Virtualization (render hanya baris terlihat)
- Minimal: Keyed update (update row by id, bukan re-render seluruh tbody)

---

## 7. Struktur Folder

```
web/static/js/
├── core/
│   ├── store.js      ← Base store utilities
│   └── ui.js         ← Global UI (theme, modal, toast)
├── features/
│   └── orders/
│       ├── store.js   ← State + reducer + selectors
│       ├── effects.js ← Fetch/save/cache
│       ├── view.js    ← Render functions
│       └── index.js   ← Mount + event delegation
└── main.js           ← Entry point
```

---

## 8. Checklist Implementasi

### Event Handler
- [ ] Handler hanya dispatch action
- [ ] Tidak ada DOM mutation langsung
- [ ] Event delegation di root

### Store
- [ ] Reducer pure function
- [ ] State serializable
- [ ] Selectors untuk derived data

### Effect
- [ ] Side-effects di layer terpisah
- [ ] Dijalankan setelah state berubah

### Render
- [ ] Render idempotent
- [ ] Batch updates
- [ ] Cache node references

---

## Mental Model

```
Bayangkan UI itu papan skor:
- State = angka di server
- Render = ngeprint papan skor
- Event = tombol operator

Operator tidak ngecat papan manual per digit.
Operator bilang "tambah skor tim A", sistem update dan cetak ulang bagian yang perlu.
```
