/**
 * Lookup Feature - Mount + Event Delegation
 * Searchable dropdown component for ERP
 * Following state-driven-ui architecture
 * 
 * Usage:
 * <div class="lookup" data-lookup="customer" data-endpoint="/api/customers/search">
 *   <input type="hidden" name="customer_id" data-lookup-value>
 *   <input type="text" data-lookup-input placeholder="Search customer...">
 *   <button type="button" data-lookup-clear aria-label="Clear">&times;</button>
 *   <div class="lookup-dropdown" data-lookup-dropdown hidden></div>
 * </div>
 */

import { reducer, selectors, getState, setState, deleteState } from './store.js';
import { effects } from './effects.js';
import { view } from './view.js';

// Track mounted instances
const mounted = new Set();

// ========== DISPATCH ==========
function dispatch(id, action) {
    const prevState = getState(id);
    const nextState = reducer(prevState, action);

    if (JSON.stringify(nextState) !== JSON.stringify(prevState)) {
        setState(id, nextState);

        // Get container element
        const container = document.querySelector(`[data-lookup="${id}"]`);

        // View (render) - update DOM
        view.render(id, nextState, container);

        // Effects based on action type
        if (action.type === 'LOOKUP_SET_QUERY' && action.payload.length >= 2) {
            const endpoint = container?.dataset.endpoint;
            if (endpoint) {
                dispatch(id, { type: 'LOOKUP_SET_LOADING', payload: true });
                effects.debouncedSearch(
                    id,
                    action.payload,
                    endpoint,
                    (results) => dispatch(id, { type: 'LOOKUP_SET_RESULTS', payload: results }),
                    (error) => dispatch(id, { type: 'LOOKUP_SET_ERROR', payload: error })
                );
            }
        }

        // Clear results if query is too short
        if (action.type === 'LOOKUP_SET_QUERY' && action.payload.length < 2) {
            effects.cancelSearch(id);
            dispatch(id, { type: 'LOOKUP_SET_RESULTS', payload: [] });
        }
    }
}

// ========== EVENT HANDLERS ==========
function handleInput(e) {
    const container = e.target.closest('[data-lookup]');
    if (!container) return;

    const id = container.dataset.lookup;
    const input = e.target.closest('[data-lookup-input]');

    if (input) {
        dispatch(id, { type: 'LOOKUP_OPEN' });
        dispatch(id, { type: 'LOOKUP_SET_QUERY', payload: input.value });
    }
}

function handleClick(e) {
    // Item selection
    const item = e.target.closest('[data-lookup-item]');
    if (item) {
        const container = item.closest('[data-lookup]');
        if (!container) return;

        const id = container.dataset.lookup;
        dispatch(id, {
            type: 'LOOKUP_SELECT',
            payload: {
                id: item.dataset.id,
                label: item.dataset.label
            }
        });

        // Trigger change event on hidden input
        const hiddenInput = container.querySelector('[data-lookup-value]');
        if (hiddenInput) {
            hiddenInput.dispatchEvent(new Event('change', { bubbles: true }));
        }
        return;
    }

    // Clear button
    const clearBtn = e.target.closest('[data-lookup-clear]');
    if (clearBtn) {
        const container = clearBtn.closest('[data-lookup]');
        if (!container) return;

        e.preventDefault();
        const id = container.dataset.lookup;
        dispatch(id, { type: 'LOOKUP_CLEAR' });

        // Focus input after clear
        const input = container.querySelector('[data-lookup-input]');
        if (input) input.focus();
        return;
    }

    // Click outside - close all
    mounted.forEach(id => {
        const container = document.querySelector(`[data-lookup="${id}"]`);
        if (container && !container.contains(e.target)) {
            dispatch(id, { type: 'LOOKUP_CLOSE' });
        }
    });
}

function handleKeydown(e) {
    const container = e.target.closest('[data-lookup]');
    if (!container) return;

    const id = container.dataset.lookup;
    const state = getState(id);

    switch (e.key) {
        case 'ArrowDown':
            e.preventDefault();
            if (!state.isOpen) {
                dispatch(id, { type: 'LOOKUP_OPEN' });
            } else {
                dispatch(id, { type: 'LOOKUP_HIGHLIGHT_NEXT' });
            }
            break;

        case 'ArrowUp':
            e.preventDefault();
            dispatch(id, { type: 'LOOKUP_HIGHLIGHT_PREV' });
            break;

        case 'Enter':
            if (state.isOpen && state.highlightIndex >= 0) {
                e.preventDefault();
                const result = state.results[state.highlightIndex];
                if (result) {
                    dispatch(id, {
                        type: 'LOOKUP_SELECT',
                        payload: { id: result.id, label: result.label }
                    });
                }
            }
            break;

        case 'Escape':
            if (state.isOpen) {
                e.preventDefault();
                dispatch(id, { type: 'LOOKUP_CLOSE' });
            }
            break;

        case 'Tab':
            dispatch(id, { type: 'LOOKUP_CLOSE' });
            break;
    }
}

function handleFocus(e) {
    const container = e.target.closest('[data-lookup]');
    if (!container) return;

    const input = e.target.closest('[data-lookup-input]');
    if (input) {
        const id = container.dataset.lookup;
        dispatch(id, { type: 'LOOKUP_OPEN' });
    }
}

// ========== INIT ==========
function init() {
    // Find all lookup instances
    document.querySelectorAll('[data-lookup]').forEach(container => {
        const id = container.dataset.lookup;
        if (mounted.has(id)) return;

        mounted.add(id);

        // Initialize with pre-selected value if exists
        const hiddenInput = container.querySelector('[data-lookup-value]');
        const displayInput = container.querySelector('[data-lookup-input]');
        if (hiddenInput?.value && displayInput?.value) {
            setState(id, {
                ...getState(id),
                selectedId: hiddenInput.value,
                selectedLabel: displayInput.value,
                query: displayInput.value
            });
        }

        // Initial render
        view.render(id, getState(id), container);
    });

    // Event delegation at document level
    document.addEventListener('input', handleInput);
    document.addEventListener('click', handleClick);
    document.addEventListener('keydown', handleKeydown);
    document.addEventListener('focusin', handleFocus);
}

// ========== DESTROY ==========
function destroy() {
    document.removeEventListener('input', handleInput);
    document.removeEventListener('click', handleClick);
    document.removeEventListener('keydown', handleKeydown);
    document.removeEventListener('focusin', handleFocus);

    mounted.forEach(id => {
        effects.cancelSearch(id);
        deleteState(id);
    });
    mounted.clear();
}

// ========== PUBLIC API ==========
const Lookup = {
    init,
    destroy,
    dispatch,
    selectors,
    // Programmatic API
    open: (id) => dispatch(id, { type: 'LOOKUP_OPEN' }),
    close: (id) => dispatch(id, { type: 'LOOKUP_CLOSE' }),
    clear: (id) => dispatch(id, { type: 'LOOKUP_CLEAR' }),
    select: (id, itemId, label) => dispatch(id, { type: 'LOOKUP_SELECT', payload: { id: itemId, label } }),
    getValue: (id) => selectors.getSelectedId(id),
    getLabel: (id) => selectors.getSelectedLabel(id)
};

export { Lookup };
