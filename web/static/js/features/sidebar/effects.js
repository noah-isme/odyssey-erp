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
    }
};

export { effects };
