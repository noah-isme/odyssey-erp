/**
 * ComboBox Feature - Mount + Event Delegation
 * Searchable select with keyboard navigation and virtualization
 * Following state-driven-ui architecture
 * 
 * Usage:
 * <div data-combobox="customer-select" data-placeholder="Select customer...">
 *   <input type="hidden" name="customer_id">
 *   
 *   <button type="button" data-combobox-trigger aria-expanded="false">
 *     <span data-combobox-display>Select customer...</span>
 *     <button data-combobox-clear hidden>&times;</button>
 *     <span data-combobox-loading hidden>...</span>
 *   </button>
 *   
 *   <div data-combobox-dropdown hidden>
 *     <input type="text" data-combobox-input placeholder="Search...">
 *     <div data-combobox-listbox role="listbox"></div>
 *     <div data-combobox-error hidden></div>
 *   </div>
 * </div>
 * 
 * JS:
 * ComboBox.register('customer-select', {
 *   options: [{ value: '1', label: 'Customer A' }],
 *   // OR async loader:
 *   loadOptions: async (query) => fetchCustomers(query),
 *   onChange: (value, label) => console.log(value)
 * });
 */

import { reducer, selectors, getState, setState, initCombobox, deleteState } from './store.js';
import { effects } from './effects.js';
import { view } from './view.js';

// Track mounted comboboxes
const mounted = new Map();

// ========== DISPATCH ==========
function dispatch(id, action) {
    const container = view.getContainer(id);
    if (!container) return;

    const prevState = getState(id);
    const nextState = reducer(prevState, action);

    if (JSON.stringify(nextState) !== JSON.stringify(prevState)) {
        setState(id, nextState);

        // View updates
        switch (action.type) {
            case 'COMBOBOX_OPEN':
                view.renderOpenState(id, nextState);
                view.renderOptions(id, nextState);
                requestAnimationFrame(() => {
                    effects.focusInput(container);
                });
                break;

            case 'COMBOBOX_CLOSE':
                view.renderOpenState(id, nextState);
                effects.focusTrigger(container);
                break;

            case 'COMBOBOX_SET_QUERY':
                view.renderOptions(id, nextState);
                // Trigger async load if loader registered
                if (effects.getLoader(id)) {
                    effects.loadOptions(
                        id,
                        action.payload,
                        () => dispatch(id, { type: 'COMBOBOX_SET_LOADING', payload: true }),
                        (options) => {
                            dispatch(id, { type: 'COMBOBOX_SET_LOADING', payload: false });
                            dispatch(id, { type: 'COMBOBOX_SET_OPTIONS', payload: options });
                        },
                        (error) => dispatch(id, { type: 'COMBOBOX_SET_ERROR', payload: error })
                    );
                }
                break;

            case 'COMBOBOX_SET_OPTIONS':
                view.renderOptions(id, nextState);
                break;

            case 'COMBOBOX_SELECT':
                view.renderSelected(id, nextState);
                view.renderOpenState(id, nextState);
                effects.updateHiddenInput(container, nextState.selectedValue);

                // Call onChange callback
                const config = mounted.get(id);
                if (config?.onChange) {
                    config.onChange(nextState.selectedValue, nextState.selectedLabel);
                }

                // Emit event
                container.dispatchEvent(new CustomEvent('combobox-change', {
                    detail: { value: nextState.selectedValue, label: nextState.selectedLabel },
                    bubbles: true
                }));
                break;

            case 'COMBOBOX_CLEAR':
                view.renderSelected(id, nextState);
                effects.updateHiddenInput(container, null);
                break;

            case 'COMBOBOX_HIGHLIGHT_NEXT':
            case 'COMBOBOX_HIGHLIGHT_PREV':
            case 'COMBOBOX_HIGHLIGHT_SET':
                view.renderOptions(id, nextState);
                const listbox = container.querySelector('[data-combobox-listbox]');
                effects.scrollToHighlight(listbox, nextState.highlightIndex);
                break;

            case 'COMBOBOX_SET_LOADING':
                view.renderLoading(id, nextState.loading);
                break;

            case 'COMBOBOX_SET_ERROR':
                view.renderError(id, nextState.error);
                view.renderLoading(id, false);
                break;

            case 'COMBOBOX_SET_SCROLL':
                view.renderOptions(id, nextState);
                break;
        }
    }
}

// ========== EVENT HANDLERS ==========
function handleClick(e) {
    // Click outside closes
    mounted.forEach((_, id) => {
        if (selectors.isOpen(id)) {
            const container = view.getContainer(id);
            if (container && !container.contains(e.target)) {
                dispatch(id, { type: 'COMBOBOX_CLOSE' });
            }
        }
    });

    // Trigger click
    const trigger = e.target.closest('[data-combobox-trigger]');
    if (trigger) {
        e.preventDefault();
        const container = trigger.closest('[data-combobox]');
        if (container) {
            dispatch(container.dataset.combobox, { type: 'COMBOBOX_TOGGLE' });
        }
        return;
    }

    // Clear button
    const clearBtn = e.target.closest('[data-combobox-clear]');
    if (clearBtn) {
        e.preventDefault();
        e.stopPropagation();
        const container = clearBtn.closest('[data-combobox]');
        if (container) {
            dispatch(container.dataset.combobox, { type: 'COMBOBOX_CLEAR' });
        }
        return;
    }

    // Option click
    const option = e.target.closest('[data-option-index]');
    if (option && !option.hasAttribute('aria-disabled')) {
        const container = option.closest('[data-combobox]');
        if (container) {
            const id = container.dataset.combobox;
            const state = getState(id);
            const index = parseInt(option.dataset.optionIndex);
            const selectedOption = state.filteredOptions[index];

            if (selectedOption && !selectedOption.disabled) {
                dispatch(id, { type: 'COMBOBOX_SELECT', payload: selectedOption });
            }
        }
    }
}

function handleInput(e) {
    const input = e.target.closest('[data-combobox-input]');
    if (!input) return;

    const container = input.closest('[data-combobox]');
    if (container) {
        dispatch(container.dataset.combobox, {
            type: 'COMBOBOX_SET_QUERY',
            payload: input.value
        });
    }
}

function handleKeydown(e) {
    // Find active combobox
    const container = e.target.closest('[data-combobox]');
    if (!container) return;

    const id = container.dataset.combobox;
    const state = getState(id);

    // Handle trigger keyboard
    if (e.target.matches('[data-combobox-trigger]')) {
        if (['ArrowDown', 'ArrowUp', 'Enter', ' '].includes(e.key)) {
            e.preventDefault();
            dispatch(id, { type: 'COMBOBOX_OPEN' });
            return;
        }
    }

    // Handle dropdown keyboard
    if (!state.isOpen) return;

    switch (e.key) {
        case 'Escape':
            e.preventDefault();
            dispatch(id, { type: 'COMBOBOX_CLOSE' });
            break;

        case 'ArrowDown':
            e.preventDefault();
            dispatch(id, { type: 'COMBOBOX_HIGHLIGHT_NEXT' });
            break;

        case 'ArrowUp':
            e.preventDefault();
            dispatch(id, { type: 'COMBOBOX_HIGHLIGHT_PREV' });
            break;

        case 'Home':
            e.preventDefault();
            dispatch(id, { type: 'COMBOBOX_HIGHLIGHT_FIRST' });
            break;

        case 'End':
            e.preventDefault();
            dispatch(id, { type: 'COMBOBOX_HIGHLIGHT_LAST' });
            break;

        case 'Enter':
            e.preventDefault();
            if (state.highlightIndex >= 0) {
                const selectedOption = state.filteredOptions[state.highlightIndex];
                if (selectedOption && !selectedOption.disabled) {
                    dispatch(id, { type: 'COMBOBOX_SELECT', payload: selectedOption });
                }
            }
            break;

        case 'Tab':
            // Close on tab
            dispatch(id, { type: 'COMBOBOX_CLOSE' });
            break;
    }
}

function handleScroll(e) {
    const listbox = e.target.closest('[data-combobox-listbox]');
    if (!listbox) return;

    const container = listbox.closest('[data-combobox]');
    if (!container) return;

    dispatch(container.dataset.combobox, {
        type: 'COMBOBOX_SET_SCROLL',
        payload: {
            scrollTop: listbox.scrollTop,
            itemHeight: view.ITEM_HEIGHT,
            containerHeight: view.CONTAINER_HEIGHT
        }
    });
}

function handleGlobalKeydown(e) {
    if (e.key === 'Escape') {
        mounted.forEach((_, id) => {
            if (selectors.isOpen(id)) {
                dispatch(id, { type: 'COMBOBOX_CLOSE' });
            }
        });
    }
}

// ========== INIT ==========
function init() {
    document.addEventListener('click', handleClick);
    document.addEventListener('input', handleInput);
    document.addEventListener('keydown', handleKeydown);
    document.addEventListener('keydown', handleGlobalKeydown);
    document.addEventListener('scroll', handleScroll, true);
}

// ========== DESTROY ==========
function destroy() {
    document.removeEventListener('click', handleClick);
    document.removeEventListener('input', handleInput);
    document.removeEventListener('keydown', handleKeydown);
    document.removeEventListener('keydown', handleGlobalKeydown);
    document.removeEventListener('scroll', handleScroll, true);
    mounted.clear();
}

// ========== PUBLIC API ==========
const ComboBox = {
    init,
    destroy,

    /**
     * Register a combobox
     * @param {string} id - ComboBox ID
     * @param {Object} config - Configuration
     * @param {Array} config.options - Static options
     * @param {Function} config.loadOptions - Async option loader
     * @param {Function} config.onChange - Change callback
     * @param {*} config.value - Initial selected value
     */
    register(id, config = {}) {
        if (mounted.has(id)) return;

        mounted.set(id, config);

        // Initialize state
        const options = config.options || [];
        initCombobox(id, options);

        // Register async loader
        if (config.loadOptions) {
            effects.registerLoader(id, config.loadOptions);
        }

        // Parse options from HTML if not provided
        const container = view.getContainer(id);
        if (container && options.length === 0 && !config.loadOptions) {
            const htmlOptions = effects.parseOptionsFromHTML(container);
            if (htmlOptions.length > 0) {
                dispatch(id, { type: 'COMBOBOX_SET_OPTIONS', payload: htmlOptions });
            }
        }

        // Set initial value
        if (config.value !== undefined) {
            const state = getState(id);
            const option = state.options.find(o => o.value === config.value);
            if (option) {
                dispatch(id, { type: 'COMBOBOX_SELECT', payload: option });
            }
        }

        // Initial render
        view.render(id, getState(id));
    },

    /**
     * Unregister a combobox
     * @param {string} id - ComboBox ID
     */
    unregister(id) {
        mounted.delete(id);
        effects.cancelSearch(id);
        deleteState(id);
        view.clearCache(id);
    },

    /**
     * Set options
     * @param {string} id - ComboBox ID
     * @param {Array} options - Options array
     */
    setOptions(id, options) {
        dispatch(id, { type: 'COMBOBOX_SET_OPTIONS', payload: options });
    },

    /**
     * Set selected value
     * @param {string} id - ComboBox ID
     * @param {*} value - Value to select
     */
    setValue(id, value) {
        const state = getState(id);
        const option = state.options.find(o => o.value === value);
        if (option) {
            dispatch(id, { type: 'COMBOBOX_SELECT', payload: option });
        }
    },

    /**
     * Get selected value
     * @param {string} id - ComboBox ID
     * @returns {*}
     */
    getValue(id) {
        return selectors.getSelectedValue(id);
    },

    /**
     * Clear selection
     * @param {string} id - ComboBox ID
     */
    clear(id) {
        dispatch(id, { type: 'COMBOBOX_CLEAR' });
    },

    /**
     * Open dropdown
     * @param {string} id - ComboBox ID
     */
    open(id) {
        dispatch(id, { type: 'COMBOBOX_OPEN' });
    },

    /**
     * Close dropdown
     * @param {string} id - ComboBox ID
     */
    close(id) {
        dispatch(id, { type: 'COMBOBOX_CLOSE' });
    },

    selectors
};

export { ComboBox };
