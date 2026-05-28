/**
 * Delivery Order Form - Main Entry
 */
import * as Store from './store.js';
import { view } from './view.js';

const CONTAINER_ID = 'lineItemsContainer';

export function init() {
    const container = document.getElementById(CONTAINER_ID);
    if (!container) return;

    // Initialize from DOM
    const existingLines = Array.from(container.querySelectorAll('.line-item')).map(el => ({
        id: Date.now() + Math.random(),
        so_line_id: el.querySelector('[name="so_line_id[]"]')?.value || '',
        product_id: el.querySelector('[name="product_id[]"]')?.value || '',
        quantity: el.querySelector('[name="quantity[]"]')?.value || '1.00',
        notes: el.querySelector('[name="line_notes[]"]')?.value || ''
    }));

    Store.setState(CONTAINER_ID, Store.createInitialState(CONTAINER_ID, existingLines));

    document.addEventListener('click', handleClick);
    document.addEventListener('input', handleInput);

    render();
}

function dispatch(action) {
    const state = Store.getState(CONTAINER_ID);
    const newState = Store.reducer(state, action);
    Store.setState(CONTAINER_ID, newState);
    render();
}

function render() {
    const state = Store.getState(CONTAINER_ID);
    view.render(CONTAINER_ID, state);
}

function handleClick(e) {
    const actionBtn = e.target.closest('[data-action]');
    if (!actionBtn) return;

    const action = actionBtn.dataset.action;

    if (action === 'add-line') {
        dispatch({ type: 'ADD_LINE' });
    } else if (action === 'remove-line') {
        const line = actionBtn.closest('[data-index]');
        if (line) {
            const index = parseInt(line.dataset.index, 10);
            dispatch({ type: 'REMOVE_LINE', payload: { index } });
        }
    }
}

function handleInput(e) {
    const line = e.target.closest('[data-index]');
    if (!line) return;

    const index = parseInt(line.dataset.index, 10);
    const field = e.target.name;
    const value = e.target.value;

    const state = Store.getState(CONTAINER_ID);
    if (state.lines[index]) {
        // Map form names to state fields
        const fieldMap = {
            'so_line_id[]': 'so_line_id',
            'product_id[]': 'product_id',
            'quantity[]': 'quantity',
            'line_notes[]': 'notes'
        };
        const stateField = fieldMap[field] || field;
        state.lines[index][stateField] = value;
        state.dirty = true;
    }
}
