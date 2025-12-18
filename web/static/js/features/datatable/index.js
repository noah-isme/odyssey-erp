/**
 * DataTable Feature - Mount + Event Delegation
 * Following state-driven-ui architecture
 * 
 * Usage:
 * <div class="table-container">
 *   <div class="bulk-actions" data-state="hidden">
 *     <span class="bulk-count">0</span> selected
 *     <button data-bulk-action="delete">Delete</button>
 *     <button data-bulk-action="export">Export</button>
 *   </div>
 *   <table class="data-table" data-datatable="customers">
 *     <thead>
 *       <tr>
 *         <th><input type="checkbox" data-select-all></th>
 *         <th class="sortable" data-column="name" data-sort-dir="asc">Name</th>
 *       </tr>
 *     </thead>
 *     <tbody>
 *       <tr data-row-id="123" data-href="/customers/123">
 *         <td><input type="checkbox" data-row-select></td>
 *         <td>John Doe</td>
 *         <td>
 *           <button class="row-action-btn" data-row-menu>...</button>
 *           <div class="row-action-menu">...</div>
 *         </td>
 *       </tr>
 *     </tbody>
 *   </table>
 * </div>
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

        // Render based on action type
        switch (action.type) {
            case 'TABLE_SELECT_ROW':
            case 'TABLE_SELECT_ALL':
            case 'TABLE_CLEAR_SELECTION':
                view.renderSelection(id, nextState);
                view.renderBulkActions(id, nextState);
                break;

            case 'TABLE_OPEN_ROW_MENU':
            case 'TABLE_CLOSE_ROW_MENU':
            case 'TABLE_TOGGLE_ROW_MENU':
                view.renderRowMenu(id, nextState);
                break;

            case 'TABLE_TOGGLE_ROW_EXPAND':
            case 'TABLE_EXPAND_ALL':
            case 'TABLE_COLLAPSE_ALL':
                view.renderExpandedRows(id, nextState);
                break;

            case 'TABLE_SET_LOADING':
                view.renderLoading(id, nextState.loading);
                break;

            case 'TABLE_SET_ERROR':
            case 'TABLE_CLEAR_ERROR':
                view.renderError(id, nextState.error);
                break;

            case 'TABLE_OPEN_CONTEXT_MENU':
            case 'TABLE_CLOSE_CONTEXT_MENU':
                view.renderContextMenu(id, nextState);
                break;
        }
    }
}

// ========== EVENT HANDLERS ==========

function handleClick(e) {
    // ===== SORT HEADER =====
    const sortHeader = e.target.closest('th.sortable');
    if (sortHeader) {
        const column = sortHeader.dataset.column;
        const currentDir = sortHeader.dataset.sortDir || '';
        effects.navigateSort(column, currentDir);
        return;
    }

    // ===== SELECT ALL CHECKBOX =====
    const selectAll = e.target.closest('input[data-select-all]');
    if (selectAll) {
        const table = selectAll.closest('[data-datatable]');
        if (table) {
            const id = table.dataset.datatable;
            const rowIds = view.getAllRowIds(id);
            dispatch(id, {
                type: 'TABLE_SELECT_ALL',
                payload: { rowIds, selected: selectAll.checked }
            });
        }
        return;
    }

    // ===== ROW CHECKBOX =====
    const rowCheckbox = e.target.closest('input[data-row-select]');
    if (rowCheckbox) {
        const table = rowCheckbox.closest('[data-datatable]');
        const row = rowCheckbox.closest('tr[data-row-id]');
        if (table && row) {
            const id = table.dataset.datatable;
            dispatch(id, {
                type: 'TABLE_SELECT_ROW',
                payload: row.dataset.rowId
            });

            // Update allSelected state
            const state = getState(id);
            const allRowIds = view.getAllRowIds(id);
            const allSelected = allRowIds.length > 0 &&
                allRowIds.every(rowId => state.selectedRows.includes(rowId));

            if (allSelected !== state.allSelected) {
                setState(id, { ...state, allSelected, indeterminate: !allSelected && state.selectedRows.length > 0 });
                view.renderSelection(id, getState(id));
            }
        }
        return;
    }

    // ===== ROW ACTION MENU BUTTON =====
    const menuBtn = e.target.closest('[data-row-menu]');
    if (menuBtn) {
        e.stopPropagation();
        const table = menuBtn.closest('[data-datatable]');
        const row = menuBtn.closest('tr[data-row-id]');
        if (table && row) {
            const id = table.dataset.datatable;
            dispatch(id, {
                type: 'TABLE_TOGGLE_ROW_MENU',
                payload: row.dataset.rowId
            });
        }
        return;
    }

    // ===== ROW EXPAND BUTTON =====
    const expandBtn = e.target.closest('[data-row-expand]');
    if (expandBtn) {
        e.stopPropagation();
        const table = expandBtn.closest('[data-datatable]');
        const row = expandBtn.closest('tr[data-row-id]');
        if (table && row) {
            const id = table.dataset.datatable;
            dispatch(id, {
                type: 'TABLE_TOGGLE_ROW_EXPAND',
                payload: row.dataset.rowId
            });
        }
        return;
    }

    // ===== BULK ACTION BUTTON =====
    const bulkBtn = e.target.closest('[data-bulk-action]');
    if (bulkBtn) {
        e.preventDefault();
        const container = bulkBtn.closest('.table-container');
        const table = container?.querySelector('[data-datatable]');
        if (table) {
            const id = table.dataset.datatable;
            const action = bulkBtn.dataset.bulkAction;
            const state = getState(id);

            if (state.selectedRows.length > 0) {
                handleBulkAction(id, action, state.selectedRows);
            }
        }
        return;
    }

    // ===== ROW CLICK (NAVIGATION) =====
    const row = e.target.closest('tbody tr[data-href]');
    if (row && !e.target.closest('input, button, a, [data-row-menu]')) {
        effects.navigateRow(row.dataset.href);
        return;
    }

    // ===== CONTEXT MENU ACTION =====
    const contextAction = e.target.closest('[data-context-action]');
    if (contextAction) {
        e.preventDefault();
        const menu = contextAction.closest('[data-context-menu]');
        if (menu) {
            const tableId = menu.dataset.contextMenu;
            const rowId = menu.dataset.rowId;
            const action = contextAction.dataset.contextAction;
            handleContextAction(tableId, rowId, action);
            dispatch(tableId, { type: 'TABLE_CLOSE_CONTEXT_MENU' });
        }
        return;
    }

    // ===== CLOSE MENUS ON OUTSIDE CLICK =====
    closeAllMenus();
}

// Separate function for document-level outside click handling
function handleDocumentClick(e) {
    // Don't close if clicking inside a table (table's own handler will manage)
    if (e.target.closest('[data-datatable]')) return;
    closeAllMenus();
}

function closeAllMenus() {
    mounted.forEach(id => {
        const state = getState(id);
        if (state.activeRowMenu) {
            dispatch(id, { type: 'TABLE_CLOSE_ROW_MENU' });
        }
        if (state.contextMenu) {
            dispatch(id, { type: 'TABLE_CLOSE_CONTEXT_MENU' });
        }
    });
}

function handleKeydown(e) {
    // Escape closes row menus and context menu
    if (e.key === 'Escape') {
        mounted.forEach(id => {
            const state = getState(id);
            if (state.activeRowMenu) {
                dispatch(id, { type: 'TABLE_CLOSE_ROW_MENU' });
            }
            if (state.contextMenu) {
                dispatch(id, { type: 'TABLE_CLOSE_CONTEXT_MENU' });
            }
        });
    }
}

function handleContextMenu(e) {
    const row = e.target.closest('tbody tr[data-row-id]');
    if (!row) return;

    const table = row.closest('[data-datatable]');
    if (!table) return;

    // Check if table has context menu enabled
    if (table.dataset.contextMenu === 'false') return;

    e.preventDefault();

    const id = table.dataset.datatable;
    const rowId = row.dataset.rowId;

    dispatch(id, {
        type: 'TABLE_OPEN_CONTEXT_MENU',
        payload: { rowId, x: e.clientX, y: e.clientY }
    });
}

function handleScroll() {
    // Close context menu on scroll
    mounted.forEach(id => {
        const state = getState(id);
        if (state.contextMenu) {
            dispatch(id, { type: 'TABLE_CLOSE_CONTEXT_MENU' });
        }
    });
}

function handleContextAction(tableId, rowId, action) {
    const table = view.getTable(tableId);
    const row = table?.querySelector(`tr[data-row-id="${rowId}"]`);
    if (!table || !row) return;

    switch (action) {
        case 'view':
            if (row.dataset.href) {
                effects.navigateRow(row.dataset.href);
            }
            break;

        case 'edit':
            if (row.dataset.editHref) {
                effects.navigateRow(row.dataset.editHref);
            } else if (row.dataset.href) {
                effects.navigateRow(row.dataset.href + '/edit');
            }
            break;

        case 'duplicate':
            // Emit event for duplicate action
            table.dispatchEvent(new CustomEvent('table-action', {
                bubbles: true,
                detail: { action: 'duplicate', rowId }
            }));
            break;

        case 'delete':
            if (effects.confirmBulkAction('delete', 1)) {
                const container = table.closest('.table-container');
                const form = container?.querySelector('form[data-bulk-form]');
                effects.submitBulkAction(form, [rowId], 'delete');
            }
            break;

        default:
            // Custom action - emit event
            table.dispatchEvent(new CustomEvent('table-action', {
                bubbles: true,
                detail: { action, rowId }
            }));
    }
}

function handleBulkAction(tableId, action, selectedRows) {
    const table = view.getTable(tableId);
    if (!table) return;

    const container = table.closest('.table-container');
    const form = container?.querySelector('form[data-bulk-form]');

    switch (action) {
        case 'delete':
            if (effects.confirmBulkAction('delete', selectedRows.length)) {
                effects.submitBulkAction(form, selectedRows, 'delete');
            }
            break;

        case 'export':
            const endpoint = table.dataset.exportEndpoint || '/api/export';
            effects.exportSelected(endpoint, selectedRows, 'csv');
            break;

        case 'archive':
            if (effects.confirmBulkAction('archive', selectedRows.length)) {
                effects.submitBulkAction(form, selectedRows, 'archive');
            }
            break;

        default:
            // Custom action - submit form with action name
            effects.submitBulkAction(form, selectedRows, action);
    }
}

// ========== INIT ==========
function init() {
    document.querySelectorAll('[data-datatable]').forEach(table => {
        const id = table.dataset.datatable;
        if (mounted.has(id)) return;

        mounted.add(id);

        // Attach click handler directly to table
        table.addEventListener('click', handleClick);

        // Initialize selection state from pre-checked checkboxes
        const preSelected = Array.from(table.querySelectorAll('tbody input[data-row-select]:checked'))
            .map(cb => cb.closest('tr[data-row-id]')?.dataset.rowId)
            .filter(Boolean);

        if (preSelected.length > 0) {
            dispatch(id, {
                type: 'TABLE_SELECT_ALL',
                payload: { rowIds: preSelected, selected: true }
            });
        }
    });

    // Document-level event delegation for closing menus on outside click
    document.addEventListener('click', handleDocumentClick);
    document.addEventListener('keydown', handleKeydown);
    document.addEventListener('contextmenu', handleContextMenu);
    window.addEventListener('scroll', handleScroll, true); // Capture phase for nested scrolls
}

// ========== DESTROY ==========
function destroy() {
    document.removeEventListener('click', handleClick);
    document.removeEventListener('keydown', handleKeydown);
    document.removeEventListener('contextmenu', handleContextMenu);
    window.removeEventListener('scroll', handleScroll, true);

    mounted.forEach(id => {
        deleteState(id);
        view.clearCache(id);
    });

    mounted.clear();
}

// ========== PUBLIC API ==========
const DataTable = {
    init,
    destroy,

    /**
     * Select specific rows
     * @param {string} tableId - Table ID
     * @param {Array} rowIds - Row IDs to select
     */
    select(tableId, rowIds) {
        dispatch(tableId, {
            type: 'TABLE_SELECT_ALL',
            payload: { rowIds, selected: true }
        });
    },

    /**
     * Clear selection
     * @param {string} tableId - Table ID
     */
    clearSelection(tableId) {
        dispatch(tableId, { type: 'TABLE_CLEAR_SELECTION' });
    },

    /**
     * Get selected rows
     * @param {string} tableId - Table ID
     * @returns {Array}
     */
    getSelected(tableId) {
        return selectors.getSelectedRows(tableId);
    },

    /**
     * Expand row
     * @param {string} tableId - Table ID
     * @param {string} rowId - Row ID
     */
    expandRow(tableId, rowId) {
        const state = getState(tableId);
        if (!state.expandedRows.includes(rowId)) {
            dispatch(tableId, {
                type: 'TABLE_TOGGLE_ROW_EXPAND',
                payload: rowId
            });
        }
    },

    /**
     * Collapse row
     * @param {string} tableId - Table ID
     * @param {string} rowId - Row ID
     */
    collapseRow(tableId, rowId) {
        const state = getState(tableId);
        if (state.expandedRows.includes(rowId)) {
            dispatch(tableId, {
                type: 'TABLE_TOGGLE_ROW_EXPAND',
                payload: rowId
            });
        }
    },

    selectors
};

export { DataTable };
