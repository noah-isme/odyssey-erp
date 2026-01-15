/**
 * Odyssey ERP - Keyboard Shortcuts
 */

const Shortcuts = {
    init() {
        document.addEventListener('keydown', (e) => {
            if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') return;

            // Cmd/Ctrl + K = Search
            if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
                e.preventDefault();
                const searchInput = document.querySelector('.global-search input');
                if (searchInput) {
                    searchInput.focus();
                    searchInput.select();
                }
            }

            // g + key navigation
            if (e.key === 'g') {
                this.waitForSecondKey((key) => {
                    const routes = {
                        'h': '/',
                        'c': '/sales/customers',
                        'o': '/sales/orders',
                        'q': '/sales/quotations',
                        'p': '/procurement/pos',
                        'i': '/inventory/stock-card',
                        'd': '/delivery/orders',
                        'a': '/accounting/coa',
                        'r': '/finance/ar/invoices',
                        'b': '/finance/ap/invoices',
                        's': '/masterdata/suppliers',
                        'u': '/users',
                        'j': '/jobs',
                    };
                    if (routes[key]) window.location = routes[key];
                });
            }

            // n + key for new entity
            if (e.key === 'n') {
                this.waitForSecondKey((key) => {
                    const routes = {
                        'o': '/sales/orders/new',
                        'q': '/sales/quotations/new',
                        'c': '/sales/customers/new',
                        'p': '/procurement/pos/new',
                        's': '/masterdata/suppliers/new',
                    };
                    if (routes[key]) window.location = routes[key];
                });
            }

            // ? = Show shortcuts
            if (e.key === '?') {
                this.showHelp();
            }

            // Escape = Close modal/dropdown
            if (e.key === 'Escape') {
                const modal = document.querySelector('.shortcuts-modal');
                if (modal) modal.remove();
            }
        });
    },

    waitForSecondKey(callback, timeout = 500) {
        const handler = (e) => {
            document.removeEventListener('keydown', handler);
            callback(e.key);
        };
        document.addEventListener('keydown', handler);
        setTimeout(() => document.removeEventListener('keydown', handler), timeout);
    },

    showHelp() {
        const existing = document.querySelector('.shortcuts-modal');
        if (existing) {
            existing.remove();
            return;
        }

        const modal = document.createElement('div');
        modal.className = 'shortcuts-modal';
        modal.innerHTML = `
      <div class="shortcuts-content">
        <div class="shortcuts-header">
          <h3>Keyboard Shortcuts</h3>
          <button class="shortcuts-close">&times;</button>
        </div>
        <div class="shortcuts-body">
          <div class="shortcut-group">
            <h4>Navigation (g + key)</h4>
            <div class="shortcut"><kbd>g</kbd> <kbd>h</kbd> <span>Home</span></div>
            <div class="shortcut"><kbd>g</kbd> <kbd>c</kbd> <span>Customers</span></div>
            <div class="shortcut"><kbd>g</kbd> <kbd>o</kbd> <span>Sales Orders</span></div>
            <div class="shortcut"><kbd>g</kbd> <kbd>q</kbd> <span>Quotations</span></div>
            <div class="shortcut"><kbd>g</kbd> <kbd>p</kbd> <span>Purchase Orders</span></div>
            <div class="shortcut"><kbd>g</kbd> <kbd>d</kbd> <span>Deliveries</span></div>
            <div class="shortcut"><kbd>g</kbd> <kbd>i</kbd> <span>Inventory</span></div>
            <div class="shortcut"><kbd>g</kbd> <kbd>r</kbd> <span>AR Invoices</span></div>
            <div class="shortcut"><kbd>g</kbd> <kbd>b</kbd> <span>AP Invoices</span></div>
            <div class="shortcut"><kbd>g</kbd> <kbd>a</kbd> <span>Chart of Accounts</span></div>
          </div>
          <div class="shortcut-group">
            <h4>Create New (n + key)</h4>
            <div class="shortcut"><kbd>n</kbd> <kbd>o</kbd> <span>New Sales Order</span></div>
            <div class="shortcut"><kbd>n</kbd> <kbd>q</kbd> <span>New Quotation</span></div>
            <div class="shortcut"><kbd>n</kbd> <kbd>c</kbd> <span>New Customer</span></div>
            <div class="shortcut"><kbd>n</kbd> <kbd>p</kbd> <span>New PO</span></div>
          </div>
          <div class="shortcut-group">
            <h4>Actions</h4>
            <div class="shortcut"><kbd>⌘</kbd> <kbd>K</kbd> <span>Global Search</span></div>
            <div class="shortcut"><kbd>Esc</kbd> <span>Close modal</span></div>
            <div class="shortcut"><kbd>?</kbd> <span>Show this help</span></div>
          </div>
        </div>
      </div>
    `;

        document.body.appendChild(modal);

        modal.querySelector('.shortcuts-close').addEventListener('click', () => modal.remove());
        modal.addEventListener('click', (e) => {
            if (e.target === modal) modal.remove();
        });
    }
};

export { Shortcuts };
