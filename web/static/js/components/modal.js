/**
 * Odyssey ERP - Modal Component
 */

const Modal = {
    activeModals: [],

    init() {
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape' && this.activeModals.length > 0) {
                this.close(this.activeModals[this.activeModals.length - 1]);
            }
        });

        document.querySelectorAll('[data-modal-trigger]').forEach(trigger => {
            trigger.addEventListener('click', () => {
                const modalId = trigger.dataset.modalTrigger;
                const modal = document.getElementById(modalId);
                if (modal) this.open(modal);
            });
        });
    },

    open(modal) {
        const overlay = modal.closest('.modal-overlay') || modal;
        overlay.classList.add('open');
        document.body.style.overflow = 'hidden';
        this.activeModals.push(modal);

        const focusable = modal.querySelector('button, input, select, textarea, [tabindex]:not([tabindex="-1"])');
        if (focusable) focusable.focus();

        overlay.addEventListener('click', (e) => {
            if (e.target === overlay) this.close(modal);
        });

        modal.querySelectorAll('[data-modal-close]').forEach(btn => {
            btn.addEventListener('click', () => this.close(modal));
        });
    },

    close(modal) {
        const overlay = modal.closest('.modal-overlay') || modal;
        overlay.classList.remove('open');

        const index = this.activeModals.indexOf(modal);
        if (index > -1) this.activeModals.splice(index, 1);

        if (this.activeModals.length === 0) {
            document.body.style.overflow = '';
        }
    },

    confirm(options) {
        const { title, message, confirmText = 'Confirm', cancelText = 'Cancel', destructive = false, onConfirm, onCancel } = options;

        const overlay = document.createElement('div');
        overlay.className = 'modal-overlay open';
        overlay.innerHTML = `
      <div class="modal ${destructive ? 'destructive' : ''} modal-sm">
        <div class="modal-header">
          <h3 class="modal-title">${title}</h3>
          <button class="modal-close" data-modal-close aria-label="Close">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
            </svg>
          </button>
        </div>
        <div class="modal-body"><p>${message}</p></div>
        <div class="modal-footer">
          <button class="btn btn-secondary" data-modal-close>${cancelText}</button>
          <button class="btn ${destructive ? 'btn-danger' : 'btn-primary'}" data-confirm>${confirmText}</button>
        </div>
      </div>
    `;

        document.body.appendChild(overlay);
        document.body.style.overflow = 'hidden';

        const modal = overlay.querySelector('.modal');

        const cleanup = () => {
            overlay.classList.remove('open');
            setTimeout(() => overlay.remove(), 200);
            document.body.style.overflow = '';
        };

        overlay.addEventListener('click', (e) => {
            if (e.target === overlay) { cleanup(); if (onCancel) onCancel(); }
        });

        modal.querySelectorAll('[data-modal-close]').forEach(btn => {
            btn.addEventListener('click', () => { cleanup(); if (onCancel) onCancel(); });
        });

        modal.querySelector('[data-confirm]').addEventListener('click', () => {
            cleanup();
            if (onConfirm) onConfirm();
        });

        setTimeout(() => modal.querySelector('[data-confirm]').focus(), 100);
    }
};

export { Modal };
