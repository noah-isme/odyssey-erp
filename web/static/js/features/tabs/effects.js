/**
 * Tabs Effects - Side Effects Layer
 * localStorage, URL params
 * Following state-driven-ui architecture
 */

const effects = {
    /**
     * Get initial tab from URL or localStorage
     * @param {string} id - Tabs ID
     * @param {string} paramName - URL param name
     * @param {boolean} persist - Whether to check localStorage
     * @returns {string|null}
     */
    getInitialTab(id, paramName, persist) {
        // Check URL param first
        try {
            const urlParams = new URLSearchParams(window.location.search);
            if (urlParams.has(paramName)) {
                return urlParams.get(paramName);
            }
        } catch (e) { /* silent */ }

        // Check localStorage
        if (persist) {
            try {
                const saved = localStorage.getItem(`odyssey.tabs.${id}`);
                if (saved) return saved;
            } catch (e) { /* silent */ }
        }

        return null;
    },

    /**
     * Save tab to localStorage
     * @param {string} id - Tabs ID
     * @param {string} tabName - Tab name
     */
    persist(id, tabName) {
        try {
            localStorage.setItem(`odyssey.tabs.${id}`, tabName);
        } catch (e) { /* silent */ }
    },

    /**
     * Clear persisted tab
     * @param {string} id - Tabs ID
     */
    clearPersist(id) {
        try {
            localStorage.removeItem(`odyssey.tabs.${id}`);
        } catch (e) { /* silent */ }
    },

    /**
     * Update URL with tab param
     * @param {string} paramName - Param name
     * @param {string} tabName - Tab name
     */
    updateUrl(paramName, tabName) {
        try {
            const url = new URL(window.location);
            url.searchParams.set(paramName, tabName);
            window.history.replaceState({}, '', url);
        } catch (e) { /* silent */ }
    },

    /**
     * Focus a tab element
     * @param {HTMLElement} tabEl - Tab button element
     */
    focusTab(tabEl) {
        if (tabEl) {
            tabEl.focus({ preventScroll: true });
        }
    }
};

export { effects };
