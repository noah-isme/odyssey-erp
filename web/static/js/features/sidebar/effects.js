/**
 * Sidebar Effects - Side Effects Layer
 * Handles: localStorage, window resize
 * Following state-driven-ui architecture
 */

import { KEY } from './store.js';

const effects = {
    /**
     * Persist sidebar state to localStorage
     * @param {Object} state - { collapsed, pinned }
     */
    persist(state) {
        try {
            localStorage.setItem(KEY, JSON.stringify({
                collapsed: state.collapsed,
                pinned: state.pinned
            }));
        } catch (e) {
            // Silent fail - storage might be disabled
        }
    },

    /**
     * Restore sidebar state from localStorage
     * @returns {Object|null} - Saved state or null
     */
    restore() {
        try {
            const saved = localStorage.getItem(KEY);
            return saved ? JSON.parse(saved) : null;
        } catch (e) {
            return null;
        }
    },

    /**
     * Check if viewport is mobile
     * @returns {boolean}
     */
    isMobile() {
        return window.innerWidth <= 1024;
    },

    /**
     * Lock body scroll (for mobile overlay)
     * @param {boolean} lock
     */
    lockScroll(lock) {
        document.body.style.overflow = lock ? 'hidden' : '';
    },

    /**
     * Save sidebar scroll position
     * @param {HTMLElement} element
     */
    saveScroll(element) {
        if (!element) return;
        try {
            sessionStorage.setItem('erp.sidebar.scrollTop', String(element.scrollTop));
        } catch (e) {
            // Safe fail
        }
    },

    /**
     * Restore sidebar scroll position (Robust)
     * @param {HTMLElement} element
     */
    restoreScroll(element) {
        if (!element) return;

        try {
            const saved = sessionStorage.getItem('erp.sidebar.scrollTop');
            if (saved === null) return;

            const wanted = Number(saved) || 0;

            const apply = () => {
                const max = element.scrollHeight - element.clientHeight;
                // Clamp value between 0 and max scroll
                element.scrollTop = Math.min(wanted, Math.max(0, max));
            };

            // Attempt restore in multiple ticks to handle async rendering
            requestAnimationFrame(apply);
            setTimeout(apply, 0);   // Next tick
            setTimeout(apply, 150); // After potential reflow/paint
        } catch (e) {
            // Safe fail
        }
    }
};

export { effects };
