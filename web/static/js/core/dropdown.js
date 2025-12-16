/**
 * Odyssey ERP - Dropdown Module
 */

const Dropdown = {
    activeDropdown: null,

    init() {
        this.setupCreateDropdown();
        this.setupUserDropdown();

        document.addEventListener('click', (e) => {
            if (this.activeDropdown && !this.activeDropdown.contains(e.target)) {
                this.closeAll();
            }
        });

        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape') this.closeAll();
        });
    },

    setupCreateDropdown() {
        const createBtn = document.querySelector('.header-btn');
        if (!createBtn) return;

        const dropdown = document.createElement('div');
        dropdown.className = 'dropdown-menu';
        dropdown.innerHTML = `
      <a href="/sales/quotations/new" class="dropdown-item">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
        New Quotation
      </a>
      <a href="/sales/orders/new" class="dropdown-item">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 2 3 6v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V6l-3-4z"/><line x1="3" y1="6" x2="21" y2="6"/></svg>
        New Sales Order
      </a>
      <a href="/procurement/pos/new" class="dropdown-item">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/></svg>
        New Purchase Order
      </a>
      <a href="/sales/customers/new" class="dropdown-item">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><line x1="19" y1="8" x2="19" y2="14"/><line x1="22" y1="11" x2="16" y2="11"/></svg>
        New Customer
      </a>
    `;

        createBtn.style.position = 'relative';
        createBtn.appendChild(dropdown);

        createBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            this.toggle(dropdown, createBtn);
        });
    },

    setupUserDropdown() {
        const userMenu = document.querySelector('.user-menu');
        if (!userMenu) return;

        const dropdown = document.createElement('div');
        dropdown.className = 'dropdown-menu dropdown-menu-right';
        dropdown.innerHTML = `
      <div class="dropdown-header">
        <strong>Admin User</strong>
        <span class="text-muted">admin@odyssey.local</span>
      </div>
      <div class="dropdown-divider"></div>
      <a href="/profile" class="dropdown-item">Profile</a>
      <a href="/settings" class="dropdown-item">Settings</a>
      <div class="dropdown-divider"></div>
      <a href="/auth/logout" class="dropdown-item text-error">Logout</a>
    `;

        userMenu.style.position = 'relative';
        userMenu.appendChild(dropdown);

        userMenu.addEventListener('click', (e) => {
            e.stopPropagation();
            this.toggle(dropdown, userMenu);
        });
    },

    toggle(dropdown, trigger) {
        const isOpen = dropdown.classList.contains('open');
        this.closeAll();

        if (!isOpen) {
            dropdown.classList.add('open');
            trigger.classList.add('active');
            this.activeDropdown = trigger;
        }
    },

    closeAll() {
        document.querySelectorAll('.dropdown-menu.open').forEach(menu => {
            menu.classList.remove('open');
            menu.parentElement.classList.remove('active');
        });
        this.activeDropdown = null;
    }
};

export { Dropdown };
