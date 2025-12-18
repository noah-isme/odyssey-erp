/**
 * Tabs Feature - Tab Interface Component
 * Following state-driven-ui architecture
 * 
 * Usage:
 * <div data-tabs="order-details" data-persist="true">
 *   <div class="tabs-list" role="tablist">
 *     <button data-tab="info" class="tab active">Info</button>
 *     <button data-tab="items" class="tab">Items</button>
 *     <button data-tab="history" class="tab">History</button>
 *   </div>
 *   <div class="tabs-content">
 *     <div data-tab-panel="info" class="tab-panel active">...</div>
 *     <div data-tab-panel="items" class="tab-panel">...</div>
 *     <div data-tab-panel="history" class="tab-panel">...</div>
 *   </div>
 * </div>
 */

const Tabs = {
    instances: new Map(),

    /**
     * Initialize tabs
     */
    init() {
        document.querySelectorAll('[data-tabs]').forEach(container => {
            const id = container.dataset.tabs;
            if (this.instances.has(id)) return;

            this.instances.set(id, { container, activeTab: null });

            // Restore from URL param or localStorage
            const activeTab = this.getInitialTab(container);
            if (activeTab) {
                this.activate(id, activeTab, false);
            }
        });

        // Event delegation
        document.addEventListener('click', (e) => {
            const tab = e.target.closest('[data-tab]');
            if (!tab) return;

            const container = tab.closest('[data-tabs]');
            if (!container) return;

            e.preventDefault();
            const tabId = container.dataset.tabs;
            const tabName = tab.dataset.tab;
            this.activate(tabId, tabName, true);
        });

        // Keyboard navigation
        document.addEventListener('keydown', (e) => {
            if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(e.key)) return;

            const tab = e.target.closest('[data-tab]');
            if (!tab) return;

            const container = tab.closest('[data-tabs]');
            if (!container) return;

            e.preventDefault();
            const tabs = Array.from(container.querySelectorAll('[data-tab]'));
            const currentIndex = tabs.indexOf(tab);
            let newIndex;

            switch (e.key) {
                case 'ArrowLeft':
                    newIndex = currentIndex > 0 ? currentIndex - 1 : tabs.length - 1;
                    break;
                case 'ArrowRight':
                    newIndex = currentIndex < tabs.length - 1 ? currentIndex + 1 : 0;
                    break;
                case 'Home':
                    newIndex = 0;
                    break;
                case 'End':
                    newIndex = tabs.length - 1;
                    break;
            }

            tabs[newIndex].focus();
            this.activate(container.dataset.tabs, tabs[newIndex].dataset.tab, true);
        });
    },

    /**
     * Get initial active tab
     * @param {HTMLElement} container - Tabs container
     * @returns {string|null} Tab name
     */
    getInitialTab(container) {
        const id = container.dataset.tabs;
        const persist = container.dataset.persist === 'true';
        const paramName = container.dataset.param || 'tab';

        // Check URL param first
        const urlParams = new URLSearchParams(window.location.search);
        if (urlParams.has(paramName)) {
            return urlParams.get(paramName);
        }

        // Check localStorage
        if (persist) {
            const saved = localStorage.getItem(`odyssey.tabs.${id}`);
            if (saved) return saved;
        }

        // Default to first tab or one with .active class
        const activeTab = container.querySelector('[data-tab].active');
        if (activeTab) return activeTab.dataset.tab;

        const firstTab = container.querySelector('[data-tab]');
        return firstTab?.dataset.tab || null;
    },

    /**
     * Activate a tab
     * @param {string} tabsId - Tabs container ID
     * @param {string} tabName - Tab to activate
     * @param {boolean} updateUrl - Whether to update URL
     */
    activate(tabsId, tabName, updateUrl = true) {
        const instance = this.instances.get(tabsId);
        if (!instance) return;

        const { container } = instance;
        const persist = container.dataset.persist === 'true';
        const paramName = container.dataset.param || 'tab';

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
            panel.setAttribute('hidden', !isActive ? '' : null);
        });

        // Save state
        instance.activeTab = tabName;

        if (persist) {
            localStorage.setItem(`odyssey.tabs.${tabsId}`, tabName);
        }

        // Update URL
        if (updateUrl) {
            const url = new URL(window.location);
            url.searchParams.set(paramName, tabName);
            window.history.replaceState({}, '', url);
        }

        // Emit event for any external listeners
        container.dispatchEvent(new CustomEvent('tab-change', {
            detail: { tab: tabName },
            bubbles: true
        }));
    },

    /**
     * Get active tab
     * @param {string} tabsId - Tabs container ID
     * @returns {string|null} Active tab name
     */
    getActive(tabsId) {
        return this.instances.get(tabsId)?.activeTab || null;
    }
};

export { Tabs };
