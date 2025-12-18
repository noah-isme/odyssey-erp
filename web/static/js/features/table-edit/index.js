/**
 * Table Edit Feature - Mount + Event Delegation
 * Inline editing for ERP tables
 * Following state-driven-ui architecture
 * 
 * Usage:
 * <table data-table-edit="customers" data-endpoint="/api/customers/update">
 *   <tr data-row-id="123">
 *     <td data-editable data-column="name" data-edit-type="text">John Doe</td>
 *     <td data-editable data-column="credit_limit" data-edit-type="currency">1,000.00</td>
 *   </tr>
 * </table>
 */

import { reducer, selectors, getState, setState, deleteState } from './store.js';
import { effects } from './effects.js';
import { view } from './view.js';

// Track mounted tables
const mounted = new Set();

// ========== DISPATCH ==========
function dispatch(id, action) {
    const prevState = getState(id);
    const nextState = reducer(prevState, action);

    if (JSON.stringify(nextState) !== JSON.stringify(prevState)) {
        setState(id, nextState);

        // Get table element
        const table = document.querySelector(`[data-table-edit="${id}"]`);
        if (!table) return;

        // Handle effects based on action
        if (action.type === 'EDIT_SUBMIT') {
            const endpoint = table.dataset.endpoint;
            const { row, column } = nextState.editingCell;
            const value = effects.parseValue(nextState.pendingValue, getCellType(table, row, column));

            effects.save(
                endpoint,
                row,
                column,
                value,
                (data) => {
                    dispatch(id, { type: 'EDIT_SUCCESS' });

                    // Update cell display
                    const cell = table.querySelector(`tr[data-row-id="${row}"] td[data-column="${column}"]`);
                    if (cell) {
                        const type = cell.dataset.editType || 'text';
                        cell.textContent = effects.formatValue(value, type);
                        view.updateValue(cell, effects.formatValue(value, type));
                    }
                },
                (error) => dispatch(id, { type: 'EDIT_ERROR', payload: error })
            );
        }

        // Render current editing cell
        if (nextState.editingCell) {
            const cell = table.querySelector(
                `tr[data-row-id="${nextState.editingCell.row}"] td[data-column="${nextState.editingCell.column}"]`
            );
            if (cell) {
                view.renderCell(cell, nextState, nextState.editingCell);
            }
        }

        // If cancelled or success, restore previous cell
        if ((action.type === 'EDIT_CANCEL' || action.type === 'EDIT_SUCCESS') && prevState.editingCell) {
            const cell = table.querySelector(
                `tr[data-row-id="${prevState.editingCell.row}"] td[data-column="${prevState.editingCell.column}"]`
            );
            if (cell && action.type === 'EDIT_CANCEL') {
                cell.textContent = prevState.editingCell.originalValue;
            }
        }
    }
}

/**
 * Get cell type from table/cell attributes
 */
function getCellType(table, rowId, column) {
    const cell = table.querySelector(`tr[data-row-id="${rowId}"] td[data-column="${column}"]`);
    return cell?.dataset.editType || 'text';
}

// ========== EVENT HANDLERS ==========
function handleDblClick(e) {
    const cell = e.target.closest('td[data-editable]');
    if (!cell) return;

    const table = cell.closest('[data-table-edit]');
    if (!table) return;

    const row = cell.closest('tr');
    if (!row || !row.dataset.rowId) return;

    e.preventDefault();

    const id = table.dataset.tableEdit;
    const column = cell.dataset.column;
    const originalValue = cell.textContent.trim();

    dispatch(id, {
        type: 'EDIT_START',
        payload: {
            row: row.dataset.rowId,
            column: column,
            originalValue: originalValue
        }
    });
}

function handleInput(e) {
    const input = e.target.closest('[data-table-edit-input]');
    if (!input) return;

    const table = input.closest('[data-table-edit]');
    if (!table) return;

    const id = table.dataset.tableEdit;
    dispatch(id, { type: 'EDIT_CHANGE', payload: input.value });
}

function handleKeydown(e) {
    const input = e.target.closest('[data-table-edit-input]');
    if (!input) return;

    const table = input.closest('[data-table-edit]');
    if (!table) return;

    const id = table.dataset.tableEdit;

    switch (e.key) {
        case 'Enter':
            e.preventDefault();
            if (selectors.isDirty(id)) {
                dispatch(id, { type: 'EDIT_SUBMIT' });
            } else {
                dispatch(id, { type: 'EDIT_CANCEL' });
            }
            break;

        case 'Escape':
            e.preventDefault();
            dispatch(id, { type: 'EDIT_CANCEL' });
            break;

        case 'Tab':
            // Could implement tab-to-next-cell navigation
            break;
    }
}

function handleBlur(e) {
    const input = e.target.closest('[data-table-edit-input]');
    if (!input) return;

    // Small delay to allow click on submit button
    setTimeout(() => {
        const table = input.closest('[data-table-edit]');
        if (!table) return;

        const id = table.dataset.tableEdit;
        const state = getState(id);

        if (state.editingCell && !state.isSubmitting) {
            if (state.isDirty) {
                dispatch(id, { type: 'EDIT_SUBMIT' });
            } else {
                dispatch(id, { type: 'EDIT_CANCEL' });
            }
        }
    }, 150);
}

// ========== INIT ==========
function init() {
    // Find all editable tables
    document.querySelectorAll('[data-table-edit]').forEach(table => {
        const id = table.dataset.tableEdit;
        if (mounted.has(id)) return;

        mounted.add(id);

        // Add visual indicator for editable cells
        table.querySelectorAll('td[data-editable]').forEach(cell => {
            cell.classList.add('editable');
            cell.title = 'Double-click to edit';
        });
    });

    // Event delegation
    document.addEventListener('dblclick', handleDblClick);
    document.addEventListener('input', handleInput);
    document.addEventListener('keydown', handleKeydown);
    document.addEventListener('focusout', handleBlur);
}

// ========== DESTROY ==========
function destroy() {
    document.removeEventListener('dblclick', handleDblClick);
    document.removeEventListener('input', handleInput);
    document.removeEventListener('keydown', handleKeydown);
    document.removeEventListener('focusout', handleBlur);

    mounted.forEach(id => deleteState(id));
    mounted.clear();
}

// ========== PUBLIC API ==========
const TableEdit = {
    init,
    destroy,
    dispatch,
    selectors,
    // Programmatic API
    startEdit: (tableId, rowId, column) => {
        const table = document.querySelector(`[data-table-edit="${tableId}"]`);
        const cell = table?.querySelector(`tr[data-row-id="${rowId}"] td[data-column="${column}"]`);
        if (cell) {
            dispatch(tableId, {
                type: 'EDIT_START',
                payload: {
                    row: rowId,
                    column: column,
                    originalValue: cell.textContent.trim()
                }
            });
        }
    },
    cancelEdit: (tableId) => dispatch(tableId, { type: 'EDIT_CANCEL' }),
    submitEdit: (tableId) => dispatch(tableId, { type: 'EDIT_SUBMIT' })
};

export { TableEdit };
