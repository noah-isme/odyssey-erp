/**
 * Toast View - Render Layer
 * DOM rendering for toasts
 * Following state-driven-ui architecture
 */

import { effects } from './effects.js';

const view = {
    _container: null,

    /**
     * Ensure toast container exists
     * @returns {HTMLElement} Container element
     */
    ensureContainer() {
        if (this._container) return this._container;

        this._container = document.querySelector('[data-toast-container]');
        if (!this._container) {
            this._container = document.createElement('div');
            this._container.setAttribute('data-toast-container', '');
            this._container.setAttribute('role', 'region');
            this._container.setAttribute('aria-live', 'polite');
            this._container.setAttribute('aria-label', 'Notifications');
            this._container.className = 'toast-container';
            document.body.appendChild(this._container);
        }

        return this._container;
    },

    /**
     * Render a single toast
     * @param {Object} toast - Toast data
     * @param {Function} onDismiss - Dismiss callback
     * @returns {HTMLElement} Toast element
     */
    renderToast(toast, onDismiss) {
        const { icon, bg, border } = effects.getVariantColors(toast.variant);

        const el = document.createElement('div');
        el.id = toast.id;
        el.setAttribute('data-toast', '');
        el.setAttribute('data-state', 'entering');
        el.setAttribute('role', 'status');
        el.className = `toast toast--${toast.variant}`;
        el.tabIndex = -1;

        el.innerHTML = `
            <div class="toast-icon">${icon}</div>
            <div class="toast-content">
                ${toast.title ? `<div class="toast-title">${this.escapeHtml(toast.title)}</div>` : ''}
                ${toast.message ? `<div class="toast-message">${this.escapeHtml(toast.message)}</div>` : ''}
            </div>
            <button type="button" class="toast-close" data-toast-dismiss="${toast.id}" aria-label="Dismiss">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
                </svg>
            </button>
        `;

        // Set custom properties for theming
        el.style.setProperty('--toast-bg-color', bg);
        el.style.setProperty('--toast-border-color', border);

        // Animate in
        requestAnimationFrame(() => {
            requestAnimationFrame(() => {
                el.setAttribute('data-state', 'visible');
            });
        });

        return el;
    },

    /**
     * Add toast to container
     * @param {HTMLElement} toastEl - Toast element
     */
    addToContainer(toastEl) {
        const container = this.ensureContainer();
        container.appendChild(toastEl);
    },

    /**
     * Remove toast with animation
     * @param {string} id - Toast ID
     * @param {Function} onComplete - Callback when animation completes
     */
    removeToast(id, onComplete) {
        const el = document.getElementById(id);
        if (!el) {
            onComplete?.();
            return;
        }

        el.setAttribute('data-state', 'leaving');

        // Wait for animation
        const handleEnd = () => {
            el.removeEventListener('animationend', handleEnd);
            el.remove();
            onComplete?.();
        };

        el.addEventListener('animationend', handleEnd);

        // Fallback timeout
        setTimeout(() => {
            if (el.parentNode) {
                el.remove();
                onComplete?.();
            }
        }, 300);
    },

    /**
     * Clear all toasts
     */
    clearAll() {
        const container = this.ensureContainer();
        container.innerHTML = '';
    },

    /**
     * Escape HTML for safe rendering
     * @param {string} str - String to escape
     * @returns {string} Escaped string
     */
    escapeHtml(str) {
        if (!str) return '';
        const div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    }
};

export { view };
