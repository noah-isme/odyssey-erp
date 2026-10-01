/**
 * Odyssey ERP - POS Cashier Terminal Application
 * Follows frontend-standards and state-driven architecture.
 */

(function () {
    'use strict';

    // State
    const state = {
        catalog: [],
        categories: [],
        activeCategory: null,
        searchQuery: '',
        cart: [],
        terminal: { id: 1, code: 'POS-01', name: 'Main Cashier' },
        session: { id: 1, status: 'OPEN' },
        cashier: { id: 1, name: 'Cashier', email: '' },
        company: { id: 1, name: 'Odyssey ERP' },
        currency: 'IDR',
        taxRate: 0.11, // PPN 11%
        heldOrders: [],
        payment: {
            method: 'CASH',
            tenderedCents: 0
        },
        activeTicket: null,
        soundMuted: false
    };

    const CART_STORAGE_KEY = 'odyssey.pos.draft_cart';
    const HELD_STORAGE_KEY = 'odyssey.pos.held_orders';

    // Audio effects via Web Audio API (no external asset dependencies)
    const audio = {
        ctx: null,
        init() {
            if (!this.ctx && (window.AudioContext || window.webkitAudioContext)) {
                this.ctx = new (window.AudioContext || window.webkitAudioContext)();
            }
        },
        beep(freq = 800, duration = 0.08, type = 'sine') {
            if (state.soundMuted) return;
            try {
                this.init();
                if (!this.ctx) return;
                const osc = this.ctx.createOscillator();
                const gain = this.ctx.createGain();
                osc.type = type;
                osc.frequency.setValueAtTime(freq, this.ctx.currentTime);
                gain.gain.setValueAtTime(0.15, this.ctx.currentTime);
                gain.gain.exponentialRampToValueAtTime(0.001, this.ctx.currentTime + duration);
                osc.connect(gain);
                gain.connect(this.ctx.destination);
                osc.start();
                osc.stop(this.ctx.currentTime + duration);
            } catch (_) {}
        },
        success() {
            this.beep(880, 0.1);
            setTimeout(() => this.beep(1320, 0.15), 100);
        },
        error() {
            this.beep(300, 0.2, 'square');
        }
    };

    // Currency Formatter
    function formatMoney(cents, currency = state.currency) {
        const amount = (cents || 0) / 100;
        if (currency === 'IDR') {
            return 'Rp ' + amount.toLocaleString('id-ID', { minimumFractionDigits: 0, maximumFractionDigits: 0 });
        }
        return new Intl.NumberFormat('en-US', { style: 'currency', currency: currency || 'USD' }).format(amount);
    }

    function csrfToken() {
        return document.querySelector('meta[name="csrf-token"]')?.content || '';
    }

    // Local Storage Persistence
    function saveCartLocal() {
        try {
            localStorage.setItem(CART_STORAGE_KEY, JSON.stringify(state.cart));
        } catch (_) {}
    }

    function loadCartLocal() {
        try {
            const raw = localStorage.getItem(CART_STORAGE_KEY);
            if (raw) {
                const parsed = JSON.parse(raw);
                if (Array.isArray(parsed)) state.cart = parsed;
            }
            const rawHeld = localStorage.getItem(HELD_STORAGE_KEY);
            if (rawHeld) {
                const parsedHeld = JSON.parse(rawHeld);
                if (Array.isArray(parsedHeld)) state.heldOrders = parsedHeld;
            }
        } catch (_) {}
    }

    // Catalog & Session Loader
    async function loadCatalog() {
        try {
            const res = await fetch('/pos/catalog', { credentials: 'same-origin', headers: { Accept: 'application/json' } });
            if (res.ok) {
                const data = await res.json();
                state.catalog = data.products || [];
                state.categories = data.categories || [];
                state.terminal = data.terminal || state.terminal;
                state.session = data.session || state.session;
                state.cashier = data.cashier || state.cashier;
                state.company = data.company || state.company;
                state.currency = data.currency || 'IDR';
            }
        } catch (e) {
            console.warn('Using offline / cached catalog fallback', e);
        }

        renderHeader();
        renderCategories();
        renderProducts();
        renderCart();
    }

    // Render Header Info & Digital Clock
    function renderHeader() {
        const elTerminal = document.getElementById('posTerminalBadge');
        if (elTerminal) elTerminal.textContent = `${state.terminal.code} - ${state.terminal.name}`;

        const elCashier = document.getElementById('posCashierName');
        if (elCashier) elCashier.textContent = state.cashier.name;

        const elCompany = document.getElementById('posCompanyName');
        if (elCompany) elCompany.textContent = state.company.name;
    }

    function updateClock() {
        const clockEl = document.getElementById('posClock');
        if (!clockEl) return;
        const now = new Date();
        clockEl.textContent = now.toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
    }

    // Render Categories
    function renderCategories() {
        const container = document.getElementById('posCategoryList');
        if (!container) return;

        const allActive = state.activeCategory === null ? 'is-active' : '';
        let html = `<button class="pos-chip ${allActive}" data-cat-id="all">Semua Produk</button>`;

        state.categories.forEach(cat => {
            const active = state.activeCategory === cat.id ? 'is-active' : '';
            html += `<button class="pos-chip ${active}" data-cat-id="${cat.id}">${cat.name}</button>`;
        });

        container.innerHTML = html;
    }

    // Render Products Grid
    function renderProducts() {
        const grid = document.getElementById('posProductGrid');
        if (!grid) return;

        let filtered = state.catalog;

        if (state.activeCategory !== null) {
            filtered = filtered.filter(p => p.category_id === state.activeCategory);
        }

        if (state.searchQuery.trim()) {
            const q = state.searchQuery.toLowerCase().trim();
            filtered = filtered.filter(p =>
                (p.name && p.name.toLowerCase().includes(q)) ||
                (p.sku && p.sku.toLowerCase().includes(q)) ||
                (p.barcode && p.barcode.toLowerCase().includes(q))
            );
        }

        if (filtered.length === 0) {
            grid.innerHTML = `
                <div style="grid-column: 1 / -1; text-align: center; padding: 3rem 1rem; color: var(--pos-text-muted);">
                    <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" style="margin-bottom:0.5rem; opacity:0.5;"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
                    <p style="margin:0; font-weight: 500;">Tidak ada produk yang cocok</p>
                    <small>Coba gunakan kata kunci atau kode barcode lain</small>
                </div>
            `;
            return;
        }

        grid.innerHTML = filtered.map(p => `
            <div class="pos-card" data-product-id="${p.id}">
                <div>
                    <div class="pos-card__category">${p.category_name || 'General'}</div>
                    <h4 class="pos-card__title">${p.name}</h4>
                    <div class="pos-card__sku">${p.sku}</div>
                </div>
                <div class="pos-card__footer">
                    <span class="pos-card__price">${formatMoney(p.price_cents)}</span>
                    <span class="pos-card__badge">${p.unit || 'pcs'}</span>
                </div>
            </div>
        `).join('');
    }

    // Cart Calculation
    function calculateTotals() {
        let subtotalCents = 0;
        let discountCents = 0;

        state.cart.forEach(item => {
            const itemSubtotal = Math.round(item.quantity * item.product.price_cents);
            subtotalCents += itemSubtotal;
            discountCents += Math.round(item.discountCents || 0);
        });

        const taxableCents = Math.max(0, subtotalCents - discountCents);
        const taxCents = Math.round(taxableCents * state.taxRate);
        const totalCents = taxableCents + taxCents;

        return { subtotalCents, discountCents, taxCents, totalCents };
    }

    // Render Cart
    function renderCart() {
        const container = document.getElementById('posCartItems');
        const countBadge = document.getElementById('posCartCount');
        const payBtn = document.getElementById('posPayBtn');

        const { subtotalCents, discountCents, taxCents, totalCents } = calculateTotals();

        const totalItems = state.cart.reduce((sum, item) => sum + item.quantity, 0);
        if (countBadge) countBadge.textContent = String(totalItems);

        document.getElementById('posSubtotal').textContent = formatMoney(subtotalCents);
        document.getElementById('posTax').textContent = formatMoney(taxCents);
        document.getElementById('posDiscount').textContent = formatMoney(discountCents);
        document.getElementById('posTotal').textContent = formatMoney(totalCents);

        if (payBtn) {
            payBtn.disabled = state.cart.length === 0;
            payBtn.innerHTML = `
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="20 6 9 17 4 12"/></svg>
                <span>Bayar ${formatMoney(totalCents)} [F9]</span>
            `;
        }

        if (state.cart.length === 0) {
            container.innerHTML = `
                <div class="pos-cart-empty">
                    <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="9" cy="21" r="1"/><circle cx="20" cy="21" r="1"/><path d="M1 1h4l2.68 13.39a2 2 0 0 0 2 1.61h9.72a2 2 0 0 0 2-1.61L23 6H6"/></svg>
                    <p style="margin: 0; font-weight: 500;">Keranjang Masih Kosong</p>
                    <small>Klik produk atau pindai barcode untuk menambahkan item</small>
                </div>
            `;
            saveCartLocal();
            return;
        }

        container.innerHTML = state.cart.map((item, index) => {
            const lineTotal = item.quantity * item.product.price_cents - (item.discountCents || 0);
            return `
                <div class="pos-cart-item" data-index="${index}">
                    <div class="pos-cart-item__top">
                        <div>
                            <div class="pos-cart-item__name">${item.product.name}</div>
                            <div class="pos-cart-item__price">${formatMoney(item.product.price_cents)} / ${item.product.unit || 'pcs'}</div>
                        </div>
                        <div class="pos-cart-item__total">${formatMoney(lineTotal)}</div>
                    </div>
                    <div class="pos-cart-item__bottom">
                        <div class="pos-stepper">
                            <button class="pos-stepper-btn" data-action="dec" data-index="${index}">−</button>
                            <span class="pos-stepper-val">${item.quantity}</span>
                            <button class="pos-stepper-btn" data-action="inc" data-index="${index}">+</button>
                        </div>
                        <button class="pos-item-del" data-action="del" data-index="${index}" title="Hapus Item">
                            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
                        </button>
                    </div>
                </div>
            `;
        }).join('');

        saveCartLocal();
    }

    // Cart Operations
    function addToCart(product) {
        if (!product) return;
        const existing = state.cart.find(item => item.product.id === product.id);
        if (existing) {
            existing.quantity += 1;
        } else {
            state.cart.push({
                product,
                quantity: 1,
                discountCents: 0,
                taxCents: 0
            });
        }
        audio.beep(900, 0.06);
        renderCart();
    }

    function updateItemQty(index, delta) {
        if (!state.cart[index]) return;
        state.cart[index].quantity += delta;
        if (state.cart[index].quantity <= 0) {
            state.cart.splice(index, 1);
        }
        audio.beep(800, 0.05);
        renderCart();
    }

    function removeItem(index) {
        if (!state.cart[index]) return;
        state.cart.splice(index, 1);
        renderCart();
    }

    function clearCart() {
        if (state.cart.length === 0) return;
        if (confirm('Kosongkan keranjang belanja?')) {
            state.cart = [];
            renderCart();
        }
    }

    // Scan Barcode Logic
    function handleBarcodeScanned(code) {
        const clean = code.trim().toLowerCase();
        if (!clean) return;

        const found = state.catalog.find(p =>
            (p.barcode && p.barcode.toLowerCase() === clean) ||
            (p.sku && p.sku.toLowerCase() === clean)
        );

        if (found) {
            addToCart(found);
            const scannerInput = document.getElementById('posScannerInput');
            if (scannerInput) scannerInput.value = '';
        } else {
            audio.error();
            alert(`Produk dengan kode "${code}" tidak ditemukan!`);
        }
    }

    // Hold & Resume Orders
    function holdOrder() {
        if (state.cart.length === 0) {
            alert('Keranjang kosong, tidak ada order untuk ditunda.');
            return;
        }
        const order = {
            id: 'HOLD-' + Date.now().toString().slice(-6),
            time: new Date().toLocaleTimeString('id-ID'),
            cart: [...state.cart],
            totalCents: calculateTotals().totalCents
        };
        state.heldOrders.push(order);
        state.cart = [];
        try {
            localStorage.setItem(HELD_STORAGE_KEY, JSON.stringify(state.heldOrders));
        } catch (_) {}
        renderCart();
        audio.beep(750, 0.1);
        alert(`Order ${order.id} berhasil ditunda.`);
    }

    function openHeldOrdersModal() {
        const modal = document.getElementById('posHeldModal');
        const list = document.getElementById('posHeldList');
        if (!modal || !list) return;

        if (state.heldOrders.length === 0) {
            list.innerHTML = '<p style="text-align:center; color:var(--pos-text-muted); padding: 2rem;">Tidak ada order yang sedang ditunda.</p>';
        } else {
            list.innerHTML = state.heldOrders.map((h, i) => `
                <div style="display:flex; justify-content:space-between; align-items:center; padding:0.75rem; border:1px solid var(--pos-border); border-radius:8px; margin-bottom:0.5rem;">
                    <div>
                        <strong>${h.id}</strong> - <small>${h.time}</small>
                        <div style="color:var(--pos-text-muted); font-size:0.85rem;">${h.cart.length} item · ${formatMoney(h.totalCents)}</div>
                    </div>
                    <button class="pos-action-btn" data-resume-index="${i}" style="height:32px; padding:0 0.75rem;">Lanjutkan</button>
                </div>
            `).join('');
        }
        modal.classList.add('is-open');
    }

    function resumeHeldOrder(index) {
        if (!state.heldOrders[index]) return;
        if (state.cart.length > 0 && !confirm('Ganti keranjang aktif dengan order yang ditunda?')) return;
        state.cart = state.heldOrders[index].cart;
        state.heldOrders.splice(index, 1);
        try {
            localStorage.setItem(HELD_STORAGE_KEY, JSON.stringify(state.heldOrders));
        } catch (_) {}
        renderCart();
        closeModals();
        audio.beep(900, 0.08);
    }

    // Payment Modal & Calculations
    function openPaymentModal() {
        const { totalCents } = calculateTotals();
        if (totalCents <= 0) return;

        state.payment.method = 'CASH';
        state.payment.tenderedCents = totalCents;

        document.getElementById('posPayTotalDisplay').textContent = formatMoney(totalCents);
        const tenderInput = document.getElementById('posTenderInput');
        if (tenderInput) {
            tenderInput.value = (totalCents / 100).toString();
        }

        updateChangeDisplay();
        renderPaymentMethods();

        const modal = document.getElementById('posPaymentModal');
        if (modal) {
            modal.classList.add('is-open');
            setTimeout(() => {
                if (tenderInput) tenderInput.focus();
            }, 100);
        }
    }

    function updateChangeDisplay() {
        const { totalCents } = calculateTotals();
        const tendered = state.payment.tenderedCents;
        const change = Math.max(0, tendered - totalCents);

        const changeEl = document.getElementById('posChangeDisplay');
        if (changeEl) changeEl.textContent = formatMoney(change);

        const submitBtn = document.getElementById('posConfirmPayBtn');
        if (submitBtn) {
            if (state.payment.method === 'CASH') {
                submitBtn.disabled = tendered < totalCents;
            } else {
                submitBtn.disabled = false;
            }
        }
    }

    function renderPaymentMethods() {
        document.querySelectorAll('.pos-pm-btn').forEach(btn => {
            btn.classList.toggle('is-active', btn.dataset.method === state.payment.method);
        });

        const cashSection = document.getElementById('posCashSection');
        if (cashSection) {
            cashSection.hidden = state.payment.method !== 'CASH';
        }
    }

    // Process Transaction & Submit to Backend
    async function processTransaction() {
        const { subtotalCents, discountCents, taxCents, totalCents } = calculateTotals();
        if (totalCents <= 0 || state.cart.length === 0) return;

        const confirmBtn = document.getElementById('posConfirmPayBtn');
        if (confirmBtn) {
            confirmBtn.disabled = true;
            confirmBtn.textContent = 'Memproses...';
        }

        try {
            const ticketPayload = {
                session_id: state.session.id,
                currency: state.currency,
                subtotal_cents: subtotalCents,
                tax_cents: taxCents,
                total_cents: totalCents,
                lines: state.cart.map(item => ({
                    ProductID: item.product.id,
                    Quantity: Math.max(1, Math.round(item.quantity)),
                    UnitPriceCents: item.product.price_cents,
                    DiscountCents: Math.round(item.discountCents || 0),
                    TaxCents: Math.round(item.quantity * item.product.price_cents * state.taxRate)
                }))
            };

            const ticketRes = await fetch('/pos/tickets', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-CSRF-Token': csrfToken()
                },
                body: JSON.stringify(ticketPayload)
            });

            if (!ticketRes.ok) {
                const errText = await ticketRes.text();
                throw new Error(errText || 'Gagal membuat tiket POS');
            }

            const createdTicket = await ticketRes.json();
            const ticketID = createdTicket.ID || createdTicket.id;

            // Submit Payment
            const paymentPayload = {
                Method: state.payment.method,
                AmountCents: totalCents,
                Reference: `POS-${Date.now()}`,
                IdempotencyKey: `pay-${ticketID}-${Date.now()}`
            };

            const payRes = await fetch(`/pos/tickets/${ticketID}/payments`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-CSRF-Token': csrfToken()
                },
                body: JSON.stringify(paymentPayload)
            });

            if (!payRes.ok) {
                const errText = await payRes.text();
                throw new Error(errText || 'Gagal memproses pembayaran POS');
            }

            // Success: Build Receipt
            const receiptData = {
                ticketNumber: `POS-${new Date().getFullYear()}${String(new Date().getMonth()+1).padStart(2,'0')}-${ticketID}`,
                date: new Date().toLocaleString('id-ID'),
                cashier: state.cashier.name,
                terminal: state.terminal.name,
                company: state.company.name,
                items: [...state.cart],
                subtotal: subtotalCents,
                tax: taxCents,
                discount: discountCents,
                total: totalCents,
                tendered: state.payment.tenderedCents,
                change: Math.max(0, state.payment.tenderedCents - totalCents),
                method: state.payment.method
            };

            audio.success();
            showReceiptModal(receiptData);

            // Reset Cart
            state.cart = [];
            saveCartLocal();
            renderCart();

        } catch (err) {
            audio.error();
            alert('Kesalahan Transaksi: ' + err.message);
        } finally {
            if (confirmBtn) {
                confirmBtn.disabled = false;
                confirmBtn.textContent = 'Konfirmasi & Cetak Struk [Enter]';
            }
        }
    }

    // Receipt Modal & Thermal Printer Layout
    function showReceiptModal(data) {
        closeModals();
        state.activeTicket = data;

        const modal = document.getElementById('posReceiptModal');
        const preview = document.getElementById('posReceiptPreview');
        const printContainer = document.getElementById('posReceiptPrint');

        const lineItemsText = data.items.map(i => {
            const lTotal = (i.quantity * i.product.price_cents) / 100;
            return `${i.product.name.slice(0, 18).padEnd(18)} ${String(i.quantity).padStart(2)}x ${lTotal.toLocaleString('id-ID').padStart(9)}`;
        }).join('\n');

        const receiptText = `
========================================
             ${data.company.toUpperCase()}
           ${data.terminal}
========================================
No. Transaksi : ${data.ticketNumber}
Waktu         : ${data.date}
Kasir         : ${data.cashier}
----------------------------------------
ITEM                QTY    TOTAL (IDR)
----------------------------------------
${lineItemsText}
----------------------------------------
Subtotal          : ${formatMoney(data.subtotal)}
PPN (11%)         : ${formatMoney(data.tax)}
Diskon            : ${formatMoney(data.discount)}
TOTAL             : ${formatMoney(data.total)}
----------------------------------------
Pembayaran (${data.method}): ${formatMoney(data.tendered)}
Kembalian         : ${formatMoney(data.change)}
========================================
   Terima Kasih Atas Kunjungan Anda!
      Barang yang dibeli tidak dapat
          dikembalikan lagi.
========================================
        `.trim();

        if (preview) preview.textContent = receiptText;
        if (printContainer) printContainer.textContent = receiptText;

        if (modal) modal.classList.add('is-open');
    }

    function closeModals() {
        document.querySelectorAll('.pos-modal-backdrop').forEach(m => m.classList.remove('is-open'));
    }

    // Event Delegation & Global Handlers
    function setupEventListeners() {
        // Digital Clock
        setInterval(updateClock, 1000);
        updateClock();

        // Search bar
        const searchInput = document.getElementById('posScannerInput');
        if (searchInput) {
            searchInput.addEventListener('input', e => {
                state.searchQuery = e.target.value;
                renderProducts();
            });

            searchInput.addEventListener('keydown', e => {
                if (e.key === 'Enter') {
                    e.preventDefault();
                    handleBarcodeScanned(e.target.value);
                }
            });
        }

        // Category clicks
        document.getElementById('posCategoryList')?.addEventListener('click', e => {
            const btn = e.target.closest('[data-cat-id]');
            if (!btn) return;
            const catId = btn.dataset.catId;
            state.activeCategory = catId === 'all' ? null : Number(catId);
            renderCategories();
            renderProducts();
        });

        // Product grid clicks
        document.getElementById('posProductGrid')?.addEventListener('click', e => {
            const card = e.target.closest('[data-product-id]');
            if (!card) return;
            const id = Number(card.dataset.productId);
            const product = state.catalog.find(p => p.id === id);
            if (product) addToCart(product);
        });

        // Cart item actions
        document.getElementById('posCartItems')?.addEventListener('click', e => {
            const btn = e.target.closest('[data-action]');
            if (!btn) return;
            const action = btn.dataset.action;
            const index = Number(btn.dataset.index);

            if (action === 'inc') updateItemQty(index, 1);
            else if (action === 'dec') updateItemQty(index, -1);
            else if (action === 'del') removeItem(index);
        });

        // Pay button
        document.getElementById('posPayBtn')?.addEventListener('click', openPaymentModal);

        // Clear cart button
        document.getElementById('posClearCartBtn')?.addEventListener('click', clearCart);

        // Hold order buttons
        document.getElementById('posHoldBtn')?.addEventListener('click', holdOrder);
        document.getElementById('posResumeHeldBtn')?.addEventListener('click', openHeldOrdersModal);
        document.getElementById('posHeldList')?.addEventListener('click', e => {
            const btn = e.target.closest('[data-resume-index]');
            if (btn) resumeHeldOrder(Number(btn.dataset.resumeIndex));
        });

        // Modal Close buttons
        document.querySelectorAll('[data-modal-close]').forEach(btn => {
            btn.addEventListener('click', closeModals);
        });

        // Payment Method Selector
        document.querySelectorAll('.pos-pm-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                state.payment.method = btn.dataset.method;
                renderPaymentMethods();
                updateChangeDisplay();
            });
        });

        // Cash presets
        document.querySelectorAll('[data-preset]').forEach(btn => {
            btn.addEventListener('click', () => {
                const { totalCents } = calculateTotals();
                const val = btn.dataset.preset;
                if (val === 'exact') {
                    state.payment.tenderedCents = totalCents;
                } else {
                    state.payment.tenderedCents = Number(val) * 100;
                }
                const input = document.getElementById('posTenderInput');
                if (input) input.value = (state.payment.tenderedCents / 100).toString();
                updateChangeDisplay();
            });
        });

        // Tender input
        document.getElementById('posTenderInput')?.addEventListener('input', e => {
            const val = parseFloat(e.target.value) || 0;
            state.payment.tenderedCents = Math.round(val * 100);
            updateChangeDisplay();
        });

        // Submit Payment
        document.getElementById('posConfirmPayBtn')?.addEventListener('click', processTransaction);

        // Print receipt
        document.getElementById('posPrintBtn')?.addEventListener('click', () => {
            window.print();
        });

        // New Sale from receipt modal
        document.getElementById('posNewSaleBtn')?.addEventListener('click', () => {
            closeModals();
            document.getElementById('posScannerInput')?.focus();
        });

        // Fullscreen Toggle
        document.getElementById('posFullscreenBtn')?.addEventListener('click', () => {
            if (!document.fullscreenElement) {
                document.documentElement.requestFullscreen().catch(() => {});
            } else {
                document.exitFullscreen().catch(() => {});
            }
        });

        // Theme Toggle (integrated with Odyssey theme system)
        document.getElementById('posThemeToggleBtn')?.addEventListener('click', () => {
            const isDark = document.documentElement.getAttribute('data-theme') === 'dark';
            const next = isDark ? 'light' : 'dark';
            if (next === 'dark') {
                document.documentElement.setAttribute('data-theme', 'dark');
            } else {
                document.documentElement.removeAttribute('data-theme');
            }
            try {
                localStorage.setItem('odyssey.theme', next);
                const token = csrfToken();
                if (token) {
                    const body = new URLSearchParams();
                    body.append('theme', next);
                    body.append('csrf_token', token);
                    fetch('/settings', {
                        method: 'POST',
                        headers: {
                            'Content-Type': 'application/x-www-form-urlencoded',
                            'X-CSRF-Token': token,
                            'Accept': 'application/json'
                        },
                        body: body.toString(),
                        credentials: 'same-origin'
                    }).catch(() => {});
                }
            } catch (_) {}
        });

        // Keyboard Shortcuts
        document.addEventListener('keydown', e => {
            // Ignore if active inside an input (unless specific hotkey)
            if (e.key === 'Escape') {
                closeModals();
                return;
            }

            if (e.key === 'F2') {
                e.preventDefault();
                document.getElementById('posScannerInput')?.focus();
                return;
            }

            if (e.key === 'F4') {
                e.preventDefault();
                clearCart();
                return;
            }

            if (e.key === 'F8') {
                e.preventDefault();
                holdOrder();
                return;
            }

            if (e.key === 'F9') {
                e.preventDefault();
                const payModal = document.getElementById('posPaymentModal');
                if (payModal && payModal.classList.contains('is-open')) {
                    processTransaction();
                } else {
                    openPaymentModal();
                }
                return;
            }

            if (e.ctrlKey && e.key.toLowerCase() === 'n') {
                e.preventDefault();
                closeModals();
                document.getElementById('posScannerInput')?.focus();
            }
        });
    }

    // Initialize POS
    document.addEventListener('DOMContentLoaded', () => {
        loadCartLocal();
        setupEventListeners();
        loadCatalog();
    });

})();
