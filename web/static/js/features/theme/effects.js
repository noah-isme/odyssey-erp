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
        this.syncBackend(theme);
    },

    /**
     * Sync theme preference to backend asynchronously
     * @param {string} theme - 'light' | 'dark'
     */
    syncBackend(theme) {
        try {
            const token = document.querySelector('meta[name="csrf-token"]')?.content;
            if (!token) return;
            const body = new URLSearchParams();
            body.append('theme', theme);
            body.append('csrf_token', token);
            fetch('/settings', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/x-www-form-urlencoded',
                    'X-CSRF-Token': token,
                    'Accept': 'application/json'
                },
                body: body.toString(),
                credentials: 'same-origin'
            }).catch(() => {});
        } catch (e) {
            // Silent fail
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
