/**
 * Theme Feature - Mount + Event Delegation
 * Entry point for theme feature
 * Following state-driven-ui architecture
 */

import { reducer, selectors, getState, setState } from './store.js';
import { effects } from './effects.js';
import { view } from './view.js';

// ========== DISPATCH ==========
function dispatch(action) {
    const prevState = getState();
    const nextState = reducer(prevState, action);

    // Only update if state changed
    if (nextState.theme !== prevState.theme) {
        setState(nextState);

        // Effects (side effects) - run AFTER state change
        effects.persist(nextState.theme);

        // View (render) - update DOM
        view.render(nextState);
    }
}

// ========== INIT ==========
function init() {
    // Restore state from effects
    const saved = effects.restore();
    const initial = saved || effects.getSystemPref();

    // Set initial state (without dispatch to avoid double-persist)
    setState({ theme: initial });

    // Initial render
    view.render(getState());

    // Event Delegation (single listener at document level)
    document.addEventListener('click', handleClick);
}

// ========== EVENT HANDLER ==========
// Handler hanya dispatch action, tidak manipulasi DOM langsung
function handleClick(e) {
    const toggle = e.target.closest('[data-theme-toggle]');
    if (toggle) {
        e.preventDefault();
        dispatch({ type: 'THEME_TOGGLE' });
        return;
    }

    const setBtn = e.target.closest('[data-theme-set]');
    if (setBtn) {
        const next = setBtn.getAttribute('data-theme-set');
        if (next === 'light' || next === 'dark') {
            dispatch({ type: 'THEME_SET', payload: next });
        }
    }
}

// ========== DESTROY (cleanup) ==========
function destroy() {
    document.removeEventListener('click', handleClick);
}

// ========== PUBLIC API ==========
const Theme = {
    init,
    destroy,
    dispatch,
    selectors,
    // Legacy compatibility
    apply: (theme) => dispatch({ type: 'THEME_SET', payload: theme })
};

export { Theme };
