# Arsitektur JavaScript untuk Odyssey ERP

> **Mental Model Utama:**  
> JS = Perilaku, bukan struktur.  
> Gunakan State-Driven UI untuk komponen kompleks.

---

## 1. Organisasi Kode & Modul

### Struktur Folder
```
web/static/js/
├── core/         # Utilitas kritikal (api, toast, store)
├── features/     # Modul fitur (state-driven: store, view, effects)
├── components/   # Komponen UI stateless
└── main.js       # Entry point & inisialisasi
```

### Aturan Modul
- Gunakan **ES Modules** (`import`/`export`).
- Setiap fitur harus memiliki fungsi `init()` yang dipanggil di `main.js`.
- Gunakan **PascalCase** untuk Class/Instance Utama (e.g., `Theme`, `Modal`).
- Gunakan **camelCase** untuk fungsi dan variabel.

---

## 2. Komunikasi API & Keamanan

### CSRF Protection
Semua permintaan non-GET (`POST`, `PUT`, `DELETE`) **WAJIB** menyertakan token CSRF di header.

**Pattern Wrapper API:**
```javascript
// web/static/js/core/api.js
export const api = {
    async fetch(url, options = {}) {
        const csrfToken = document.querySelector('meta[name="csrf-token"]')?.content;
        
        const headers = {
            'Content-Type': 'application/json',
            'X-CSRF-Token': csrfToken,
            ...options.headers
        };

        const response = await fetch(url, { ...options, headers });
        
        if (!response.ok) {
            const error = await response.json().catch(() => ({ message: 'Terjadi kesalahan server' }));
            throw error;
        }

        return response.json();
    }
};
```

---

## 3. Passing Data dari Server ke JS

❌ **Jangan gunakan inline script:**
```html
<script>
  const data = {{ .Data }}; // ❌ Bahaya XSS & Sulit di-debug
</script>
```

✅ **Gunakan data attributes:**
```html
<div 
  id="my-feature" 
  data-props='{{ .Data | json }}'
></div>
```

Di JS:
```javascript
const el = document.getElementById('my-feature');
const props = JSON.parse(el.dataset.props);
```

---

## 4. DOM Manipulation (Vanilla JS)

| Aturan | Penjelasan |
|--------|------------|
| **Event Delegation** | Pasang listener di root atau document, bukan tiap elemen. |
| **Data Hook** | Gunakan `data-action` atau `data-id` sebagai selector, bukan class CSS. |
| **Visual State** | Gunakan `setAttribute('data-state', '...')`, bukan manipulasi inline style. |

---

## 5. Progressive Enhancement

1. **HTML First**: Form harus tetap bisa submit tanpa JS.
2. **JS Enhancement**: Gunakan JS untuk mempercepat (e.g., fetch submit) atau memperkaya (e.g., charts).
3. **Graceful Degradation**: Jika JS gagal dimuat, fungsi inti ERP (Save/Edit/List) tidak boleh mati total.

---

## 6. Debugging & Logging

- Gunakan `console.log` hanya di development.
- Manfaatkan `DevTools.register()` (lihat `main.js`) agar state fitur bisa diinspeksi dari konsol browser.

---

## Checklist Implementasi JS

- [ ] Menggunakan ES Modules.
- [ ] Inisialisasi via `main.js`.
- [ ] Penanganan CSRF untuk request mutasi.
- [ ] Tidak ada inline script di template.
- [ ] Pembersihan (cleanup) event listener/timer jika diperlukan.
