/**
 * Slideout Effects - Side Effects Layer
 * Focus management, scroll lock, AJAX loading
 * Following state-driven-ui architecture
 */

const effects = {
    // Store last focused elements
    _lastFocused: new Map(),

    // Store focus trap handlers
    _focusTrapHandlers: new Map(),

    // Close animation timer
    _closeTimers: new Map(),

    /**
     * Save current focus
     * @param {string} id - Slideout ID
     * @param {HTMLElement} trigger - Trigger element
     */
    saveFocus(id, trigger) {
        this._lastFocused.set(id, trigger || document.activeElement);
    },

    /**
     * Restore focus
     * @param {string} id - Slideout ID
     */
    restoreFocus(id) {
        const el = this._lastFocused.get(id);
        if (el) {
            el.focus({ preventScroll: true });
            this._lastFocused.delete(id);
        }
    },

    /**
     * Focus first focusable element in panel
     * @param {HTMLElement} panel - Slideout panel
     */
    focusFirst(panel) {
        const focusable = panel.querySelector(
            'input, select, textarea, button:not([data-slideout-close]), [tabindex]:not([tabindex="-1"])'
        );
        if (focusable) {
            focusable.focus({ preventScroll: true });
        }
    },

    /**
     * Setup focus trap
     * @param {string} id - Slideout ID
     * @param {HTMLElement} panel - Panel element
     */
    setupFocusTrap(id, panel) {
        const handler = (e) => {
            if (e.key !== 'Tab') return;

            const focusableSelector = 'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])';
            const focusable = panel.querySelectorAll(focusableSelector);
            const firstFocusable = focusable[0];
            const lastFocusable = focusable[focusable.length - 1];

            if (e.shiftKey) {
                if (document.activeElement === firstFocusable) {
                    e.preventDefault();
                    lastFocusable.focus();
                }
            } else {
                if (document.activeElement === lastFocusable) {
                    e.preventDefault();
                    firstFocusable.focus();
                }
            }
        };

        panel.addEventListener('keydown', handler);
        this._focusTrapHandlers.set(id, handler);
    },

    /**
     * Remove focus trap
     * @param {string} id - Slideout ID
     * @param {HTMLElement} panel - Panel element
     */
    removeFocusTrap(id, panel) {
        const handler = this._focusTrapHandlers.get(id);
        if (handler) {
            panel.removeEventListener('keydown', handler);
            this._focusTrapHandlers.delete(id);
        }
    },

    /**
     * Lock body scroll
     */
    lockScroll() {
        document.body.style.overflow = 'hidden';
    },

    /**
     * Unlock body scroll
     */
    unlockScroll() {
        document.body.style.overflow = '';
    },

    /**
     * Schedule close animation cleanup
     * @param {string} id - Slideout ID
     * @param {Function} callback - Callback after animation
     * @param {number} delay - Animation duration
     */
    scheduleCloseCleanup(id, callback, delay = 300) {
        // Clear existing timer
        if (this._closeTimers.has(id)) {
            clearTimeout(this._closeTimers.get(id));
        }

        const timerId = setTimeout(() => {
            this._closeTimers.delete(id);
            callback();
        }, delay);

        this._closeTimers.set(id, timerId);
    },

    /**
     * Cancel close cleanup
     * @param {string} id - Slideout ID
     */
    cancelCloseCleanup(id) {
        if (this._closeTimers.has(id)) {
            clearTimeout(this._closeTimers.get(id));
            this._closeTimers.delete(id);
        }
    },

    /**
     * Load content via AJAX
     * @param {string} url - Content URL
     * @returns {Promise<string>}
     */
    async loadContent(url) {
        const response = await fetch(url);
        if (!response.ok) {
            throw new Error(`Failed to load content: ${response.status}`);
        }
        return response.text();
    },

    /**
     * Cleanup all effects for an ID
     * @param {string} id - Slideout ID
     */
    cleanup(id) {
        this._lastFocused.delete(id);
        this._focusTrapHandlers.delete(id);
        this.cancelCloseCleanup(id);
    },

    /**
     * Cleanup all
     */
    cleanupAll() {
        this._lastFocused.clear();
        this._focusTrapHandlers.clear();
        this._closeTimers.forEach(timer => clearTimeout(timer));
        this._closeTimers.clear();
        this.unlockScroll();
    }
};

export { effects };
