/**
 * Theme Effects - Side Effects Layer
 * Handles: localStorage, system preferences
 * Following state-driven-ui architecture
 */

import { KEY } from './store.js';

const effects = {
    /**
     * Persist theme to localStorage
     * @param {string} theme - 'light' | 'dark'
     */
    persist(theme) {
        try {
            localStorage.setItem(KEY, theme);
        } catch (e) {
            // Silent fail - storage might be disabled
        }
    },

    /**
     * Restore theme from localStorage
     * @returns {string|null} - Saved theme or null
     */
    restore() {
        try {
            return localStorage.getItem(KEY);
        } catch (e) {
            return null;
        }
    },

    /**
     * Get system color scheme preference
     * @returns {string} - 'light' | 'dark'
     */
    getSystemPref() {
        if (!window.matchMedia) return 'light';
        return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    }
};

export { effects };
