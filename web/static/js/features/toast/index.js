/**
 * Toast Feature - Mount + Event Delegation
 * Toast/Snackbar with queue + auto-dismiss
 * Following state-driven-ui architecture
 * 
 * API:
 * Toast.show({ title, message, variant, duration })
 * Toast.dismiss(id)
 * Toast.clearAll()
 * 
 * Variants: 'neutral' | 'success' | 'warning' | 'error' | 'info'
 */

import { reducer, selectors, getState, setState, generateId } from './store.js';
import { effects } from './effects.js';
import { view } from './view.js';

// ========== DISPATCH ==========
function dispatch(action) {
    const prevState = getState();
    const nextState = reducer(prevState, action);

    if (JSON.stringify(nextState) !== JSON.stringify(prevState)) {
        setState(nextState);

        // Process queue after state update
        if (action.type === 'TOAST_ADD' || action.type === 'TOAST_DISMISS') {
            processQueue();
        }
    }
}

// ========== QUEUE PROCESSOR ==========
function processQueue() {
    const state = getState();

    // Show toasts from queue if we have room
    while (state.active.length < state.maxVisible && state.queue.length > 0) {
        const nextToast = state.queue[0];
        if (!nextToast) break;

        dispatch({ type: 'TOAST_SHOW', payload: nextToast.id });

        // Render the toast
        const updatedState = getState();
        const toast = updatedState.active.find(t => t.id === nextToast.id);
        if (toast) {
            const el = view.renderToast(toast, handleDismiss);
            view.addToContainer(el);

            // Start auto-dismiss timer
            const duration = toast.variant === 'error'
                ? Math.max(toast.duration, 5000)
                : toast.duration;

            effects.startTimer(toast.id, duration, handleDismiss);
        }
    }
}

// ========== HANDLERS ==========
function handleDismiss(id) {
    effects.clearTimer(id);

    view.removeToast(id, () => {
        dispatch({ type: 'TOAST_DISMISS', payload: id });
    });
}

function handleClick(e) {
    const dismissBtn = e.target.closest('[data-toast-dismiss]');
    if (dismissBtn) {
        e.preventDefault();
        const id = dismissBtn.getAttribute('data-toast-dismiss');
        handleDismiss(id);
    }
}

// ========== INIT ==========
function init() {
    // Event delegation for dismiss buttons
    document.addEventListener('click', handleClick);

    // Ensure container exists
    view.ensureContainer();
}

// ========== DESTROY ==========
function destroy() {
    document.removeEventListener('click', handleClick);
    effects.clearAllTimers();
    view.clearAll();
    dispatch({ type: 'TOAST_CLEAR_ALL' });
}

// ========== PUBLIC API ==========
const Toast = {
    init,
    destroy,

    /**
     * Show a toast notification
     * @param {Object} options - Toast options
     * @param {string} options.title - Toast title
     * @param {string} options.message - Toast message
     * @param {string} options.variant - 'neutral' | 'success' | 'warning' | 'error' | 'info'
     * @param {number} options.duration - Auto-dismiss duration in ms (0 = no auto-dismiss)
     * @returns {string} Toast ID
     */
    show(options = {}) {
        const id = generateId();
        dispatch({
            type: 'TOAST_ADD',
            payload: { id, ...options }
        });
        return id;
    },

    /**
     * Convenience methods
     */
    success(message, title) {
        return this.show({ message, title, variant: 'success' });
    },

    error(message, title) {
        return this.show({ message, title, variant: 'error' });
    },

    warning(message, title) {
        return this.show({ message, title, variant: 'warning' });
    },

    info(message, title) {
        return this.show({ message, title, variant: 'info' });
    },

    /**
     * Dismiss a specific toast
     * @param {string} id - Toast ID
     */
    dismiss(id) {
        handleDismiss(id);
    },

    /**
     * Clear all toasts
     */
    clearAll() {
        effects.clearAllTimers();
        view.clearAll();
        dispatch({ type: 'TOAST_CLEAR_ALL' });
    },

    selectors
};

export { Toast };
