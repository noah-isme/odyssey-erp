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
                    if (key === 'h') window.location = '/';
                    if (key === 'c') window.location = '/sales/customers';
                    if (key === 'o') window.location = '/sales/orders';
                    if (key === 'q') window.location = '/sales/quotations';
                });
            }

            // ? = Show shortcuts
            if (e.key === '?') {
                this.showHelp();
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
            <h4>Navigation</h4>
            <div class="shortcut"><kbd>g</kbd> then <kbd>h</kbd> <span>Go to Home</span></div>
            <div class="shortcut"><kbd>g</kbd> then <kbd>c</kbd> <span>Go to Customers</span></div>
            <div class="shortcut"><kbd>g</kbd> then <kbd>o</kbd> <span>Go to Orders</span></div>
            <div class="shortcut"><kbd>g</kbd> then <kbd>q</kbd> <span>Go to Quotations</span></div>
          </div>
          <div class="shortcut-group">
            <h4>Actions</h4>
            <div class="shortcut"><kbd>⌘</kbd> <kbd>K</kbd> <span>Global Search</span></div>
            <div class="shortcut"><kbd>Esc</kbd> <span>Close modal/dropdown</span></div>
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
