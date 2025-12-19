---
description: Aturan arsitektur UI state-driven untuk Odyssey ERP (Vanilla JS)
---

# Odyssey UI Architecture Guide

## Prinsip Inti: DOM adalah Output, Bukan Sumber Kebenaran

```
Event → Action → Reducer(update state) → Effects → Render → DOM
```

---

## 1. State Management

### Aturan State
- **Single Source of Truth** per fitur
- **State harus serializable** (tidak menyimpan DOM node, timer id, atau function)
- **Derived data dihitung**, bukan disimpan (e.g., `filteredRows` dari `rows + filters`)
- **Side-effects dipisah** di layer effects (fetch, localStorage, websocket)
- **Error adalah state**, bukan alert dadakan

### Struktur State Standar
```javascript
// Minimal state
{ theme: 'light' }

// State dengan loading/error (WAJIB untuk async)
{ data: null, loading: false, error: null }

// Multi-field state (form, combobox)
{ values: {}, errors: {}, touched: {}, submitting: false, dirty: false, valid: true }
```

---

## 2. Store Pattern

### Singleton (untuk global state)
```javascript
// features/theme/store.js
let state = { theme: 'light' };

function getState() { return state; }
function setState(newState) { state = newState; }
```

### Multi-Instance via Map (untuk reusable components)
```javascript
// features/form/store.js
const instances = new Map();

function getState(id) {
    if (!instances.has(id)) {
        instances.set(id, createInitialState());
    }
    return instances.get(id);
}

function setState(id, newState) {
    instances.set(id, newState);
}

function deleteState(id) {
    instances.delete(id);
}
```

### Kapan Pakai Apa?
| Pattern | Gunakan Untuk |
|---------|---------------|
| Singleton | Theme, User, Global notifications |
| Map | Forms, Modals, ComboBox, Menu (bisa banyak instance) |

---

## 3. Reducer (Pure Function)

### Aturan Keras
- **TIDAK ADA side effect** (fetch, localStorage, setTimeout)
- **Selalu return state baru** (spread operator)
- **Idempotency guard** untuk prevent double-action

```javascript
function reducer(state, action) {
    switch (action.type) {
        case 'FORM_SUBMIT_START':
            // ✅ Idempotency guard
            if (state.submitting) return state;
            return { ...state, submitting: true };

        case 'FORM_SET_VALUE':
            return {
                ...state,
                values: { ...state.values, [action.payload.field]: action.payload.value },
                dirty: true
            };

        default:
            return state;
    }
}
```

---

## 4. Event Handling

### Aturan
- **Satu listener per root feature** (event delegation di `document`)
- **Semua event menghasilkan action**: `dispatch({ type, payload })`
- **TIDAK ADA DOM mutation langsung di handler**

### Contoh Benar
```javascript
// ✅ Handler hanya dispatch
function handleClick(e) {
    const toggle = e.target.closest('[data-theme-toggle]');
    if (toggle) {
        dispatch({ type: 'THEME_TOGGLE' });
    }
}

document.addEventListener('click', handleClick);
```

### Contoh Salah
```javascript
// ❌ Handler langsung manipulasi DOM
function handleClick(e) {
    const toggle = e.target.closest('[data-theme-toggle]');
    if (toggle) {
        document.documentElement.setAttribute('data-theme', 'dark');
        localStorage.setItem('theme', 'dark');
    }
}
```

---

## 5. Effects (Side Effects Layer)

### Aturan
- Effects **terpisah dari reducer**
- Effects dijalankan **SETELAH state berubah**
- Effects untuk: API calls, localStorage, debounce, timers, focus management

### Struktur Effects
```javascript
// features/form/effects.js
const effects = {
    // Debounce storage
    _timers: new Map(),

    // Async validation dengan debounce
    async validateAsync(id, field, value, onStart, onEnd) {
        // Clear previous timer
        if (this._timers.has(id)) clearTimeout(this._timers.get(id));

        const timerId = setTimeout(async () => {
            onStart();
            try {
                const error = await checkServerValidation(value);
                dispatch(id, { type: 'SET_ERROR', payload: { field, error } });
            } finally {
                onEnd();
            }
        }, 300);

        this._timers.set(id, timerId);
    },

    // Cleanup (WAJIB dipanggil saat destroy)
    cleanup(id) {
        if (this._timers.has(id)) {
            clearTimeout(this._timers.get(id));
            this._timers.delete(id);
        }
    }
};
```

---

## 6. View (Render Layer)

### Aturan
- **Render idempotent**: state sama = output sama
- **Batch updates** via `requestAnimationFrame`
- **Cache node references** (jangan query ulang tiap render)
- **Gunakan `data-state` untuk visual state**

```javascript
// features/modal/view.js
const view = {
    _cache: new Map(),

    getContainer(id) {
        if (!this._cache.has(id)) {
            this._cache.set(id, document.getElementById(id));
        }
        return this._cache.get(id);
    },

    render(id, state) {
        const el = this.getContainer(id);
        if (!el) return;

        // Gunakan data-state, bukan style.display
        el.setAttribute('data-state', state.isOpen ? 'open' : 'closed');
        el.hidden = !state.isOpen;
    },

    clearCache(id) {
        this._cache.delete(id);
    }
};
```

---

## 7. Lifecycle & Cleanup

### Lifecycle Wajib
```javascript
const Feature = {
    // Called once on app init
    init() {
        document.addEventListener('click', handleClick);
        document.addEventListener('keydown', handleKeydown);
    },

    // Called saat halaman unload atau feature tidak dipakai
    destroy() {
        // 1. Remove event listeners
        document.removeEventListener('click', handleClick);
        document.removeEventListener('keydown', handleKeydown);

        // 2. Clear timers
        effects.clearAllTimers();

        // 3. Clear state
        instances.clear();

        // 4. Clear DOM cache
        view.clearAllCaches();

        // 5. Reset side effects
        effects.unlockScroll();
    }
};
```

### Cleanup Checklist
- [ ] Event listeners removed
- [ ] setTimeout/setInterval cleared
- [ ] WebSocket/EventSource closed
- [ ] IntersectionObserver/MutationObserver disconnected
- [ ] State cleared
- [ ] DOM cache cleared
- [ ] Scroll lock released
- [ ] Focus restored

---

## 8. Error as State

### Aturan
- Error **bukan alert dadakan**
- Error **bagian dari state**
- UI render error dari state

```javascript
// State
{ loading: false, error: null, data: null }

// Reducer
case 'FETCH_ERROR':
    return { ...state, loading: false, error: action.payload };

case 'CLEAR_ERROR':
    return { ...state, error: null };

// View
function renderError(state) {
    errorEl.textContent = state.error || '';
    errorEl.hidden = !state.error;
}
```

---

## 9. Idempotency

### Masalah: Double Submit = Bencana
```javascript
// ❌ Tanpa guard - bisa submit 2x
case 'FORM_SUBMIT_START':
    return { ...state, submitting: true };

// ✅ Dengan guard
case 'FORM_SUBMIT_START':
    if (state.submitting) return state; // Block double submit
    return { ...state, submitting: true };
```

### Untuk API Calls
```javascript
// Gunakan idempotency key
async function submitForm(values) {
    const idempotencyKey = `form-${Date.now()}-${Math.random()}`;
    
    await fetch('/api/submit', {
        headers: { 'X-Idempotency-Key': idempotencyKey },
        body: JSON.stringify(values)
    });
}
```

---

## 10. Struktur Folder (4-File Pattern)

```
web/static/js/
├── core/
│   └── ui.js         ← Critical theme restore only
├── features/
│   └── [feature]/
│       ├── store.js   ← State + Reducer + Selectors
│       ├── effects.js ← Side effects (fetch, timer, focus)
│       ├── view.js    ← DOM rendering
│       └── index.js   ← Mount + Event delegation + Public API
├── components/
│   └── [simple].js   ← Simple presentational components
└── main.js           ← Entry point
```

### Kapan 4-File vs 1-File?
| Struktur | Gunakan Untuk |
|----------|---------------|
| 4-file | Complex state, async, lifecycle |
| 1-file | Simple, stateless, no async |

---



---

## 11. Checklist Implementasi

### Event Handler
- [ ] Handler hanya dispatch action
- [ ] Tidak ada DOM mutation langsung
- [ ] Event delegation di document

### Store
- [ ] Reducer pure function
- [ ] State serializable
- [ ] Selectors untuk derived data
- [ ] Idempotency guard untuk action kritikal

### Effect
- [ ] Side-effects di layer terpisah
- [ ] Timers tracked dan bisa di-cleanup
- [ ] Error handling di async operations

### Render
- [ ] Render idempotent
- [ ] Batch updates (requestAnimationFrame)
- [ ] Cache node references
- [ ] Gunakan data-state untuk visual state

### Lifecycle
- [ ] init() untuk setup
- [ ] destroy() untuk cleanup
- [ ] Event listeners removed saat destroy
- [ ] Timers cleared saat destroy
- [ ] State cleared saat destroy

### Error Handling
- [ ] Error sebagai state ({ error: null })
- [ ] Loading state ({ loading: false })
- [ ] UI render error dari state

---
```