/**
 * Header Feature - Mount + Event Delegation
 * Entry point for header dropdowns
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
    if (nextState.activeDropdown !== prevState.activeDropdown) {
        setState(nextState);

        // View (render) - update DOM
        view.render(nextState);

        // Effects - focus management
        if (nextState.activeDropdown) {
            // Opening: focus first item in dropdown
            const dropdown = effects.getDropdown(nextState.activeDropdown);
            effects.focusFirst(dropdown);
        } else if (prevState.lastFocused) {
            // Closing: restore focus to trigger
            effects.restoreFocus(prevState.lastFocused);
        }
    }
}

// ========== EVENT HANDLERS ==========
function handleClick(e) {
    // Dropdown trigger click
    const trigger = e.target.closest('[data-dropdown-trigger]');
    if (trigger) {
        e.preventDefault();
        e.stopPropagation();
        const id = trigger.getAttribute('data-dropdown-trigger');
        dispatch({ type: 'DROPDOWN_TOGGLE', payload: { id, trigger } });
        return;
    }

    // Click inside dropdown - don't close
    const insideDropdown = e.target.closest('[data-dropdown]');
    if (insideDropdown) {
        return;
    }

    // Click outside - close all
    if (selectors.hasOpen()) {
        dispatch({ type: 'DROPDOWN_CLOSE_ALL' });
    }
}

function handleKeydown(e) {
    // Esc closes dropdown
    if (e.key === 'Escape' && selectors.hasOpen()) {
        dispatch({ type: 'DROPDOWN_CLOSE_ALL' });
        return;
    }

    // Tab trap within dropdown (optional enhancement)
    // Arrow key navigation (optional enhancement)
}

// ========== INIT ==========
function init() {
    // Initial render (all closed)
    view.render(getState());

    // Event Delegation (single listener at document level)
    document.addEventListener('click', handleClick);
    document.addEventListener('keydown', handleKeydown);
}

// ========== DESTROY (cleanup) ==========
function destroy() {
    document.removeEventListener('click', handleClick);
    document.removeEventListener('keydown', handleKeydown);
}

// ========== PUBLIC API ==========
const Header = {
    init,
    destroy,
    dispatch,
    selectors,
    // Convenience methods
    open: (id) => dispatch({ type: 'DROPDOWN_OPEN', payload: { id } }),
    close: () => dispatch({ type: 'DROPDOWN_CLOSE_ALL' }),
    toggle: (id) => dispatch({ type: 'DROPDOWN_TOGGLE', payload: { id } })
};

export { Header };
