/**
 * Date Range Picker Feature - Mount + Event Delegation
 * Following state-driven-ui architecture
 * 
 * Usage:
 * <div class="daterange" data-daterange="report-period" data-param-start="date_from" data-param-end="date_to">
 *   <input type="hidden" name="date_from" data-daterange-start>
 *   <input type="hidden" name="date_to" data-daterange-end>
 *   <button type="button" class="daterange-trigger" data-daterange-trigger>
 *     <span data-daterange-display>Select date range</span>
 *   </button>
 *   <div class="daterange-dropdown" data-daterange-dropdown hidden></div>
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
        const container = document.querySelector(`[data-daterange="${id}"]`);

        // View (render) - update DOM
        view.render(id, nextState, container);

        // Effects - update query params on apply
        if (action.type === 'DATERANGE_SET_RANGE') {
            const startParam = container?.dataset.paramStart || 'date_from';
            const endParam = container?.dataset.paramEnd || 'date_to';
            effects.updateQueryParams(startParam, endParam, nextState.startDate, nextState.endDate);

            // Trigger form submit or page reload if configured
            if (container?.dataset.autoSubmit === 'true') {
                const form = container.closest('form');
                if (form) form.submit();
            }
        }
    }
}

// ========== EVENT HANDLERS ==========
function handleClick(e) {
    // Trigger click
    const trigger = e.target.closest('[data-daterange-trigger]');
    if (trigger) {
        const container = trigger.closest('[data-daterange]');
        if (!container) return;

        e.preventDefault();
        const id = container.dataset.daterange;
        dispatch(id, { type: 'DATERANGE_TOGGLE' });
        return;
    }

    // Preset selection
    const preset = e.target.closest('[data-daterange-preset]');
    if (preset) {
        const container = preset.closest('[data-daterange]');
        if (!container) return;

        e.preventDefault();
        const id = container.dataset.daterange;
        dispatch(id, {
            type: 'DATERANGE_SET_RANGE',
            payload: {
                start: preset.dataset.start,
                end: preset.dataset.end
            }
        });
        return;
    }

    // Clear button
    const clearBtn = e.target.closest('[data-daterange-clear]');
    if (clearBtn) {
        const container = clearBtn.closest('[data-daterange]');
        if (!container) return;

        e.preventDefault();
        const id = container.dataset.daterange;
        dispatch(id, { type: 'DATERANGE_CLEAR' });
        return;
    }

    // Apply button
    const applyBtn = e.target.closest('[data-daterange-apply]');
    if (applyBtn) {
        const container = applyBtn.closest('[data-daterange]');
        if (!container) return;

        e.preventDefault();
        const id = container.dataset.daterange;
        const state = getState(id);
        dispatch(id, {
            type: 'DATERANGE_SET_RANGE',
            payload: { start: state.startDate, end: state.endDate }
        });
        return;
    }

    // Click outside - close all
    mounted.forEach(id => {
        const container = document.querySelector(`[data-daterange="${id}"]`);
        if (container && !container.contains(e.target)) {
            dispatch(id, { type: 'DATERANGE_CLOSE' });
        }
    });
}

function handleChange(e) {
    const input = e.target.closest('[data-daterange-input]');
    if (!input) return;

    const container = input.closest('[data-daterange]');
    if (!container) return;

    const id = container.dataset.daterange;
    const field = input.dataset.daterangeInput;

    if (field === 'start') {
        dispatch(id, { type: 'DATERANGE_SET_START', payload: input.value || null });
    } else if (field === 'end') {
        dispatch(id, { type: 'DATERANGE_SET_END', payload: input.value || null });
    }
}

function handleKeydown(e) {
    if (e.key === 'Escape') {
        mounted.forEach(id => {
            if (selectors.isOpen(id)) {
                dispatch(id, { type: 'DATERANGE_CLOSE' });
            }
        });
    }
}

// ========== INIT ==========
function init() {
    // Find all daterange instances
    document.querySelectorAll('[data-daterange]').forEach(container => {
        const id = container.dataset.daterange;
        if (mounted.has(id)) return;

        mounted.add(id);

        // Initialize with existing values from hidden inputs
        const startInput = container.querySelector('[data-daterange-start]');
        const endInput = container.querySelector('[data-daterange-end]');

        if (startInput?.value || endInput?.value) {
            setState(id, {
                ...getState(id),
                startDate: startInput?.value || null,
                endDate: endInput?.value || null
            });
        }

        // Also check URL params
        const urlParams = new URLSearchParams(window.location.search);
        const startParam = container.dataset.paramStart || 'date_from';
        const endParam = container.dataset.paramEnd || 'date_to';

        if (urlParams.has(startParam) || urlParams.has(endParam)) {
            setState(id, {
                ...getState(id),
                startDate: urlParams.get(startParam) || getState(id).startDate,
                endDate: urlParams.get(endParam) || getState(id).endDate
            });
        }

        // Initial render
        view.render(id, getState(id), container);
    });

    // Event delegation
    document.addEventListener('click', handleClick);
    document.addEventListener('change', handleChange);
    document.addEventListener('keydown', handleKeydown);
}

// ========== DESTROY ==========
function destroy() {
    document.removeEventListener('click', handleClick);
    document.removeEventListener('change', handleChange);
    document.removeEventListener('keydown', handleKeydown);

    mounted.forEach(id => deleteState(id));
    mounted.clear();
}

// ========== PUBLIC API ==========
const DateRangePicker = {
    init,
    destroy,
    dispatch,
    selectors,
    // Programmatic API
    open: (id) => dispatch(id, { type: 'DATERANGE_OPEN' }),
    close: (id) => dispatch(id, { type: 'DATERANGE_CLOSE' }),
    clear: (id) => dispatch(id, { type: 'DATERANGE_CLEAR' }),
    setRange: (id, start, end) => dispatch(id, { type: 'DATERANGE_SET_RANGE', payload: { start, end } }),
    getRange: (id) => selectors.getRange(id)
};

export { DateRangePicker };
