/**
 * Slide-out Panel Feature - Mount + Event Delegation
 * Drawer from right for quick edit/view
 * Following state-driven-ui architecture
 * 
 * Usage:
 * <button data-slideout-trigger="customer-edit">Edit</button>
 * 
 * <div class="slideout" data-slideout="customer-edit" hidden>
 *   <div class="slideout-header">
 *     <h2>Edit Customer</h2>
 *     <button data-slideout-close>&times;</button>
 *   </div>
 *   <div class="slideout-body">...</div>
 *   <div class="slideout-footer">
 *     <button data-slideout-close>Cancel</button>
 *     <button type="submit">Save</button>
 *   </div>
 * </div>
 */

import {
    reducer,
    selectors,
    getState,
    setState,
    deleteState,
    setActiveId,
    getActiveIdValue
} from './store.js';
import { effects } from './effects.js';
import { view } from './view.js';

// Track mounted slideouts
const mounted = new Set();

// ========== DISPATCH ==========
function dispatch(id, action) {
    const panel = view.getPanel(id);
    if (!panel) return;

    const prevState = getState(id);
    const nextState = reducer(prevState, action);

    if (JSON.stringify(nextState) !== JSON.stringify(prevState)) {
        setState(id, nextState);

        // View updates and effects
        switch (action.type) {
            case 'SLIDEOUT_OPEN':
                setActiveId(id);
                view.renderOpen(id, true);
                effects.lockScroll();
                effects.setupFocusTrap(id, panel);
                requestAnimationFrame(() => {
                    effects.focusFirst(panel);
                });
                panel.dispatchEvent(new CustomEvent('slideout-open', { bubbles: true }));
                break;

            case 'SLIDEOUT_CLOSE':
                setActiveId(null);
                view.renderOpen(id, false);
                effects.unlockScroll();
                effects.removeFocusTrap(id, panel);
                effects.restoreFocus(id);
                effects.scheduleCloseCleanup(id, () => {
                    view.hidePanel(id);
                });
                panel.dispatchEvent(new CustomEvent('slideout-close', { bubbles: true }));
                break;

            case 'SLIDEOUT_SET_LOADING':
                view.renderLoading(id, nextState.loading);
                break;

            case 'SLIDEOUT_SET_CONTENT':
                view.renderContent(id, nextState.content);
                break;

            case 'SLIDEOUT_SET_ERROR':
                view.renderError(id, nextState.error);
                break;
        }
    }
}

// ========== EVENT HANDLERS ==========
function handleClick(e) {
    // Trigger button
    const trigger = e.target.closest('[data-slideout-trigger]');
    if (trigger) {
        e.preventDefault();
        const id = trigger.dataset.slideoutTrigger;
        effects.saveFocus(id, trigger);
        dispatch(id, { type: 'SLIDEOUT_OPEN' });
        return;
    }

    // Close button
    const closeBtn = e.target.closest('[data-slideout-close]');
    if (closeBtn) {
        const panel = closeBtn.closest('[data-slideout]');
        if (panel) {
            e.preventDefault();
            dispatch(panel.dataset.slideout, { type: 'SLIDEOUT_CLOSE' });
        }
        return;
    }

    // Backdrop click
    if (e.target.matches('.slideout-backdrop')) {
        const activeId = getActiveIdValue();
        if (activeId) {
            dispatch(activeId, { type: 'SLIDEOUT_CLOSE' });
        }
    }
}

function handleKeydown(e) {
    if (e.key === 'Escape') {
        const activeId = getActiveIdValue();
        if (activeId) {
            e.preventDefault();
            dispatch(activeId, { type: 'SLIDEOUT_CLOSE' });
        }
    }
}

// ========== INIT ==========
function init() {
    document.querySelectorAll('[data-slideout]').forEach(panel => {
        const id = panel.dataset.slideout;
        if (mounted.has(id)) return;
        mounted.add(id);
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
        effects.cleanup(id);
        deleteState(id);
        view.clearCache(id);
    });

    effects.cleanupAll();
    view.removeBackdrop();
    mounted.clear();
}

// ========== PUBLIC API ==========
const Slideout = {
    init,
    destroy,

    /**
     * Open a slideout panel
     * @param {string} id - Panel ID
     * @param {HTMLElement} trigger - Trigger element for focus restore
     */
    open(id, trigger = null) {
        if (trigger) {
            effects.saveFocus(id, trigger);
        }
        dispatch(id, { type: 'SLIDEOUT_OPEN' });
    },

    /**
     * Close a slideout panel
     * @param {string} id - Panel ID
     */
    close(id) {
        dispatch(id, { type: 'SLIDEOUT_CLOSE' });
    },

    /**
     * Toggle a slideout panel
     * @param {string} id - Panel ID
     * @param {HTMLElement} trigger - Trigger element
     */
    toggle(id, trigger = null) {
        if (selectors.isOpen(id)) {
            this.close(id);
        } else {
            this.open(id, trigger);
        }
    },

    /**
     * Load content via AJAX
     * @param {string} id - Panel ID
     * @param {string} url - Content URL
     */
    async loadContent(id, url) {
        dispatch(id, { type: 'SLIDEOUT_SET_LOADING', payload: true });

        try {
            const content = await effects.loadContent(url);
            dispatch(id, { type: 'SLIDEOUT_SET_CONTENT', payload: content });
        } catch (error) {
            dispatch(id, { type: 'SLIDEOUT_SET_ERROR', payload: error.message });
        }
    },

    /**
     * Check if a panel is open
     * @param {string} id - Panel ID
     * @returns {boolean}
     */
    isOpen(id) {
        return selectors.isOpen(id);
    },

    selectors
};

export { Slideout };
