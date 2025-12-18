/**
 * Modal Effects - Side Effects Layer
 * Focus trap, scroll lock, focus restore
 * Following state-driven-ui architecture
 */

const effects = {
    // Track original body overflow
    _originalOverflow: null,
    _focusTrapHandlers: new Map(),

    /**
     * Get focusable elements in container
     * @param {HTMLElement} container - Container element
     * @returns {HTMLElement[]} Focusable elements
     */
    getFocusable(container) {
        return Array.from(container.querySelectorAll(
            'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])'
        )).filter(el => !el.hasAttribute('disabled') && el.offsetParent !== null);
    },

    /**
     * Lock body scroll
     */
    lockScroll() {
        if (this._originalOverflow === null) {
            this._originalOverflow = document.body.style.overflow;
            document.body.style.overflow = 'hidden';
        }
    },

    /**
     * Unlock body scroll
     */
    unlockScroll() {
        if (this._originalOverflow !== null) {
            document.body.style.overflow = this._originalOverflow;
            this._originalOverflow = null;
        }
    },

    /**
     * Save current focus
     * @returns {string|null} ID of focused element or null
     */
    saveFocus() {
        const active = document.activeElement;
        if (active && active !== document.body) {
            // Return element reference (not ID, since not all elements have IDs)
            return active;
        }
        return null;
    },

    /**
     * Restore focus to element
     * @param {HTMLElement|string} elOrId - Element or ID to focus
     */
    restoreFocus(elOrId) {
        let target = elOrId;
        if (typeof elOrId === 'string') {
            target = document.getElementById(elOrId);
        }
        if (target && target.focus) {
            target.focus({ preventScroll: true });
        }
    },

    /**
     * Focus first element in modal
     * @param {HTMLElement} modal - Modal element
     */
    focusFirst(modal) {
        const panel = modal.querySelector('[data-modal-panel]') || modal;
        const focusable = this.getFocusable(panel);

        if (focusable.length > 0) {
            focusable[0].focus({ preventScroll: true });
        } else {
            // Make panel focusable if nothing else is
            panel.setAttribute('tabindex', '-1');
            panel.focus({ preventScroll: true });
        }
    },

    /**
     * Setup focus trap for modal
     * @param {string} id - Modal ID
     * @param {HTMLElement} modal - Modal element
     */
    setupFocusTrap(id, modal) {
        const handler = (e) => {
            if (e.key !== 'Tab') return;

            const panel = modal.querySelector('[data-modal-panel]') || modal;
            const focusable = this.getFocusable(panel);
            if (focusable.length === 0) return;

            const first = focusable[0];
            const last = focusable[focusable.length - 1];

            if (e.shiftKey && document.activeElement === first) {
                e.preventDefault();
                last.focus();
            } else if (!e.shiftKey && document.activeElement === last) {
                e.preventDefault();
                first.focus();
            }
        };

        this._focusTrapHandlers.set(id, handler);
        modal.addEventListener('keydown', handler);
    },

    /**
     * Remove focus trap for modal
     * @param {string} id - Modal ID
     * @param {HTMLElement} modal - Modal element
     */
    removeFocusTrap(id, modal) {
        const handler = this._focusTrapHandlers.get(id);
        if (handler) {
            modal.removeEventListener('keydown', handler);
            this._focusTrapHandlers.delete(id);
        }
    }
};

export { effects };
