/**
 * Modal Feature - Mount + Event Delegation
 * Modal/Dialog/Drawer with proper lifecycle
 * Following state-driven-ui architecture
 * 
 * Lifecycle: mount → open → close → destroy
 * 
 * Usage:
 * <button data-modal-open="modal-confirm">Open Modal</button>
 * 
 * <div id="modal-confirm" data-modal data-state="closed" role="dialog" aria-modal="true" hidden>
 *   <div data-modal-overlay></div>
 *   <div data-modal-panel>
 *     <h2>Confirm</h2>
 *     <button data-modal-close>Cancel</button>
 *   </div>
 * </div>
 */

import {
    reducer,
    selectors,
    getState,
    setState,
    pushToStack,
    removeFromStack,
    clearStack
} from './store.js';
import { effects } from './effects.js';
import { view } from './view.js';

// Track mounted modals
const mounted = new Set();

// Store last focused elements per modal (not serializable, so kept here)
const lastFocusedElements = new Map();

// ========== DISPATCH ==========
function dispatch(id, action) {
    const modal = document.getElementById(id);
    if (!modal) return;

    const prevState = getState(id);
    const nextState = reducer(prevState, action);

    if (JSON.stringify(nextState) !== JSON.stringify(prevState)) {
        setState(id, nextState);

        // View (render) - update DOM
        view.render(id, nextState, modal);

        // Effects based on action type
        if (action.type === 'MODAL_OPEN') {
            // Save focus
            lastFocusedElements.set(id, effects.saveFocus());

            // Add to stack
            pushToStack(id);

            // Lock scroll
            effects.lockScroll();

            // Show backdrop
            view.showBackdrop();

            // Setup focus trap
            effects.setupFocusTrap(id, modal);

            // Focus first element
            requestAnimationFrame(() => {
                effects.focusFirst(modal);
            });
        }

        if (action.type === 'MODAL_CLOSE') {
            // Remove focus trap
            effects.removeFocusTrap(id, modal);

            // Remove from stack
            removeFromStack(id);

            // If no more modals, unlock scroll and hide backdrop
            if (selectors.getModalStack().length === 0) {
                effects.unlockScroll();
                view.hideBackdrop();
            }

            // Restore focus
            const lastFocused = lastFocusedElements.get(id);
            if (lastFocused) {
                effects.restoreFocus(lastFocused);
                lastFocusedElements.delete(id);
            }
        }
    }
}

// ========== EVENT HANDLERS ==========
function handleClick(e) {
    // Open button
    const openBtn = e.target.closest('[data-modal-open]');
    if (openBtn) {
        e.preventDefault();
        const modalId = openBtn.getAttribute('data-modal-open');
        dispatch(modalId, {
            type: 'MODAL_OPEN',
            payload: { triggerEl: openBtn.id }
        });
        return;
    }

    // Close button
    const closeBtn = e.target.closest('[data-modal-close]');
    if (closeBtn) {
        const modal = closeBtn.closest('[data-modal]');
        if (modal) {
            e.preventDefault();
            dispatch(modal.id, { type: 'MODAL_CLOSE' });
        }
        return;
    }

    // Overlay click
    const overlay = e.target.closest('[data-modal-overlay]');
    if (overlay) {
        const modal = overlay.closest('[data-modal]');
        if (modal) {
            dispatch(modal.id, { type: 'MODAL_CLOSE' });
        }
        return;
    }

    // Backdrop click (for auto-generated backdrop)
    if (e.target.matches('[data-modal-backdrop]')) {
        const topModal = selectors.getTopModal();
        if (topModal) {
            dispatch(topModal, { type: 'MODAL_CLOSE' });
        }
    }
}

function handleKeydown(e) {
    if (e.key !== 'Escape') return;

    // Close top modal on Escape
    const topModal = selectors.getTopModal();
    if (topModal) {
        e.preventDefault();
        dispatch(topModal, { type: 'MODAL_CLOSE' });
    }
}

// ========== INIT ==========
function init() {
    // Find all modals
    document.querySelectorAll('[data-modal]').forEach(modal => {
        const id = modal.id;
        if (!id || mounted.has(id)) return;

        mounted.add(id);

        // Set initial state attribute
        modal.setAttribute('data-state', 'closed');

        // Initial render
        view.render(id, getState(id), modal);
    });

    // Event delegation at document level
    document.addEventListener('click', handleClick);
    document.addEventListener('keydown', handleKeydown);
}

// ========== DESTROY ==========
function destroy() {
    document.removeEventListener('click', handleClick);
    document.removeEventListener('keydown', handleKeydown);

    // Cleanup
    mounted.forEach(id => {
        const modal = document.getElementById(id);
        if (modal) {
            effects.removeFocusTrap(id, modal);
        }
    });

    effects.unlockScroll();
    view.hideBackdrop();
    lastFocusedElements.clear();
    clearStack();
    mounted.clear();
}

// ========== PUBLIC API ==========
const Modal = {
    init,
    destroy,
    dispatch,
    selectors,
    // Programmatic API
    open: (id) => dispatch(id, { type: 'MODAL_OPEN' }),
    close: (id) => dispatch(id, { type: 'MODAL_CLOSE' }),
    closeAll: () => {
        selectors.getModalStack().forEach(id => {
            dispatch(id, { type: 'MODAL_CLOSE' });
        });
    },
    isOpen: (id) => selectors.isOpen(id),
    getOpenModals: () => selectors.getModalStack()
};

export { Modal };
