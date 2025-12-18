/**
 * Tabs View - Render Layer
 * DOM rendering for tabs
 * Following state-driven-ui architecture
 */

const view = {
    _cache: new Map(),

    /**
     * Get tabs container
     * @param {string} id - Tabs ID
     * @returns {HTMLElement|null}
     */
    getContainer(id) {
        if (this._cache.has(id)) {
            return this._cache.get(id);
        }
        const container = document.querySelector(`[data-tabs="${id}"]`);
        if (container) {
            this._cache.set(id, container);
        }
        return container;
    },

    /**
     * Render active tab state
     * @param {string} id - Tabs ID
     * @param {string} tabName - Active tab name
     */
    render(id, tabName) {
        const container = this.getContainer(id);
        if (!container) return;

        // Update tab buttons
        container.querySelectorAll('[data-tab]').forEach(tab => {
            const isActive = tab.dataset.tab === tabName;
            tab.classList.toggle('active', isActive);
            tab.setAttribute('aria-selected', isActive);
            tab.setAttribute('tabindex', isActive ? '0' : '-1');
        });

        // Update panels
        container.querySelectorAll('[data-tab-panel]').forEach(panel => {
            const isActive = panel.dataset.tabPanel === tabName;
            panel.classList.toggle('active', isActive);
            if (isActive) {
                panel.removeAttribute('hidden');
            } else {
                panel.setAttribute('hidden', '');
            }
        });
    },

    /**
     * Get all tab names from container
     * @param {string} id - Tabs ID
     * @returns {Array} Tab names
     */
    getTabNames(id) {
        const container = this.getContainer(id);
        if (!container) return [];

        return Array.from(container.querySelectorAll('[data-tab]'))
            .map(tab => tab.dataset.tab);
    },

    /**
     * Get tab at index
     * @param {string} id - Tabs ID
     * @param {number} index - Index
     * @returns {HTMLElement|null}
     */
    getTabAtIndex(id, index) {
        const container = this.getContainer(id);
        if (!container) return null;

        const tabs = container.querySelectorAll('[data-tab]');
        return tabs[index] || null;
    },

    /**
     * Get initial active tab from DOM
     * @param {string} id - Tabs ID
     * @returns {string|null}
     */
    getDefaultTab(id) {
        const container = this.getContainer(id);
        if (!container) return null;

        const activeTab = container.querySelector('[data-tab].active');
        if (activeTab) return activeTab.dataset.tab;

        const firstTab = container.querySelector('[data-tab]');
        return firstTab?.dataset.tab || null;
    },

    /**
     * Clear cache
     * @param {string} id - Tabs ID
     */
    clearCache(id) {
        this._cache.delete(id);
    },

    /**
     * Clear all caches
     */
    clearAllCaches() {
        this._cache.clear();
    }
};

export { view };
