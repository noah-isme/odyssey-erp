/**
 * Tabs Feature - Mount + Event Delegation
 * Tab Interface Component
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
 *     <div data-tab-panel="items" class="tab-panel" hidden>...</div>
 *     <div data-tab-panel="history" class="tab-panel" hidden>...</div>
 *   </div>
 * </div>
 */

import { reducer, selectors, getState, setState, deleteState, createInitialState } from './store.js';
import { effects } from './effects.js';
import { view } from './view.js';

// Track mounted tabs
const mounted = new Set();

// ========== DISPATCH ==========
function dispatch(id, action) {
    const prevState = getState(id);
    const nextState = reducer(prevState, action);

    if (JSON.stringify(nextState) !== JSON.stringify(prevState)) {
        setState(id, nextState);

        // View updates
        if (action.type === 'TABS_ACTIVATE') {
            view.render(id, nextState.activeTab);

            // Effects
            if (nextState.persist) {
                effects.persist(id, nextState.activeTab);
            }
        }
    }
}

// ========== EVENT HANDLERS ==========
function handleClick(e) {
    const tab = e.target.closest('[data-tab]');
    if (!tab) return;

    const container = tab.closest('[data-tabs]');
    if (!container) return;

    e.preventDefault();
    const id = container.dataset.tabs;
    const tabName = tab.dataset.tab;

    dispatch(id, { type: 'TABS_ACTIVATE', payload: tabName });

    // Update URL
    const state = getState(id);
    effects.updateUrl(state.paramName, tabName);

    // Emit event
    container.dispatchEvent(new CustomEvent('tab-change', {
        detail: { tab: tabName },
        bubbles: true
    }));
}

function handleKeydown(e) {
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(e.key)) return;

    const tab = e.target.closest('[data-tab]');
    if (!tab) return;

    const container = tab.closest('[data-tabs]');
    if (!container) return;

    e.preventDefault();
    const id = container.dataset.tabs;
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

    const newTab = tabs[newIndex];
    effects.focusTab(newTab);

    dispatch(id, { type: 'TABS_ACTIVATE', payload: newTab.dataset.tab });

    // Update URL
    const state = getState(id);
    effects.updateUrl(state.paramName, newTab.dataset.tab);
}

// ========== INIT ==========
function init() {
    document.querySelectorAll('[data-tabs]').forEach(container => {
        const id = container.dataset.tabs;
        if (mounted.has(id)) return;

        mounted.add(id);

        const persist = container.dataset.persist === 'true';
        const paramName = container.dataset.param || 'tab';
        const tabs = view.getTabNames(id);

        // Initialize state
        dispatch(id, {
            type: 'TABS_INIT',
            payload: { tabs, persist, paramName }
        });

        // Get initial tab
        let activeTab = effects.getInitialTab(id, paramName, persist);
        if (!activeTab) {
            activeTab = view.getDefaultTab(id);
        }

        if (activeTab) {
            dispatch(id, { type: 'TABS_ACTIVATE', payload: activeTab });
        }
    });

    // Event delegation
    document.addEventListener('click', handleClick);
    document.addEventListener('keydown', handleKeydown);
}

// ========== DESTROY ==========
function destroy() {
    document.removeEventListener('click', handleClick);
    document.removeEventListener('keydown', handleKeydown);

    mounted.forEach(id => {
        deleteState(id);
        view.clearCache(id);
    });

    mounted.clear();
}

// ========== PUBLIC API ==========
const Tabs = {
    init,
    destroy,

    /**
     * Activate a tab programmatically
     * @param {string} tabsId - Tabs container ID
     * @param {string} tabName - Tab to activate
     * @param {boolean} updateUrl - Whether to update URL
     */
    activate(tabsId, tabName, updateUrl = true) {
        dispatch(tabsId, { type: 'TABS_ACTIVATE', payload: tabName });

        if (updateUrl) {
            const state = getState(tabsId);
            effects.updateUrl(state.paramName, tabName);
        }
    },

    /**
     * Get active tab
     * @param {string} tabsId - Tabs container ID
     * @returns {string|null} Active tab name
     */
    getActive(tabsId) {
        return selectors.getActiveTab(tabsId);
    },

    selectors
};

export { Tabs };
