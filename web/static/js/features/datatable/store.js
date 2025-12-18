/**
 * DataTable Store - State + Reducer + Selectors
 * Following state-driven-ui architecture
 * 
 * State table = satu objek
 * Derived data dihitung, bukan disimpan
 */

// ========== STATE ==========
const instances = new Map();

function createInitialState() {
    return {
        // Selection state
        selectedRows: [],       // Array of row IDs
        allSelected: false,
        indeterminate: false,

        // Row actions menu (click trigger)
        activeRowMenu: null,    // Row ID with open menu

        // Context menu (right-click)
        contextMenu: null,      // { rowId, x, y } or null

        // Expanded rows (for row expand feature)
        expandedRows: [],       // Array of row IDs

        // Loading state for async operations
        loading: false,
        error: null
    };
}

// ========== REDUCER (pure function) ==========
function reducer(state, action) {
    switch (action.type) {
        // ========== SELECTION ==========
        case 'TABLE_SELECT_ROW': {
            const rowId = action.payload;
            const isSelected = state.selectedRows.includes(rowId);
            const newSelected = isSelected
                ? state.selectedRows.filter(id => id !== rowId)
                : [...state.selectedRows, rowId];

            return {
                ...state,
                selectedRows: newSelected,
                allSelected: false, // Will be recalculated by selector
                indeterminate: newSelected.length > 0
            };
        }

        case 'TABLE_SELECT_ALL': {
            const { rowIds, selected } = action.payload;
            return {
                ...state,
                selectedRows: selected ? [...rowIds] : [],
                allSelected: selected,
                indeterminate: false
            };
        }

        case 'TABLE_CLEAR_SELECTION': {
            return {
                ...state,
                selectedRows: [],
                allSelected: false,
                indeterminate: false
            };
        }

        // ========== ROW MENU ==========
        case 'TABLE_OPEN_ROW_MENU': {
            return {
                ...state,
                activeRowMenu: action.payload
            };
        }

        case 'TABLE_CLOSE_ROW_MENU': {
            return {
                ...state,
                activeRowMenu: null
            };
        }

        case 'TABLE_TOGGLE_ROW_MENU': {
            const rowId = action.payload;
            return {
                ...state,
                activeRowMenu: state.activeRowMenu === rowId ? null : rowId,
                contextMenu: null // Close context menu when opening row menu
            };
        }

        // ========== CONTEXT MENU (RIGHT-CLICK) ==========
        case 'TABLE_OPEN_CONTEXT_MENU': {
            const { rowId, x, y } = action.payload;
            return {
                ...state,
                contextMenu: { rowId, x, y },
                activeRowMenu: null // Close row menu when opening context menu
            };
        }

        case 'TABLE_CLOSE_CONTEXT_MENU': {
            return {
                ...state,
                contextMenu: null
            };
        }

        // ========== ROW EXPAND ==========
        case 'TABLE_TOGGLE_ROW_EXPAND': {
            const rowId = action.payload;
            const isExpanded = state.expandedRows.includes(rowId);
            return {
                ...state,
                expandedRows: isExpanded
                    ? state.expandedRows.filter(id => id !== rowId)
                    : [...state.expandedRows, rowId]
            };
        }

        case 'TABLE_EXPAND_ALL': {
            return {
                ...state,
                expandedRows: [...action.payload]
            };
        }

        case 'TABLE_COLLAPSE_ALL': {
            return {
                ...state,
                expandedRows: []
            };
        }

        // ========== ASYNC STATE ==========
        case 'TABLE_SET_LOADING': {
            return {
                ...state,
                loading: action.payload,
                error: action.payload ? null : state.error
            };
        }

        case 'TABLE_SET_ERROR': {
            return {
                ...state,
                loading: false,
                error: action.payload
            };
        }

        case 'TABLE_CLEAR_ERROR': {
            return {
                ...state,
                error: null
            };
        }

        default:
            return state;
    }
}

// ========== SELECTORS ==========
const selectors = {
    getState: (id) => instances.get(id) || createInitialState(),

    // Selection
    getSelectedRows: (id) => (instances.get(id) || {}).selectedRows || [],
    getSelectedCount: (id) => ((instances.get(id) || {}).selectedRows || []).length,
    isRowSelected: (id, rowId) => ((instances.get(id) || {}).selectedRows || []).includes(rowId),
    isAllSelected: (id) => (instances.get(id) || {}).allSelected || false,
    isIndeterminate: (id) => (instances.get(id) || {}).indeterminate || false,
    hasSelection: (id) => ((instances.get(id) || {}).selectedRows || []).length > 0,

    // Row menu
    getActiveRowMenu: (id) => (instances.get(id) || {}).activeRowMenu,
    isRowMenuOpen: (id, rowId) => (instances.get(id) || {}).activeRowMenu === rowId,

    // Context menu
    getContextMenu: (id) => (instances.get(id) || {}).contextMenu,
    isContextMenuOpen: (id) => !!(instances.get(id) || {}).contextMenu,
    getContextMenuRowId: (id) => ((instances.get(id) || {}).contextMenu || {}).rowId,

    // Row expand
    getExpandedRows: (id) => (instances.get(id) || {}).expandedRows || [],
    isRowExpanded: (id, rowId) => ((instances.get(id) || {}).expandedRows || []).includes(rowId),

    // Async
    isLoading: (id) => (instances.get(id) || {}).loading || false,
    getError: (id) => (instances.get(id) || {}).error
};

// ========== STORE API ==========
function getState(id) {
    if (!instances.has(id)) {
        instances.set(id, createInitialState());
    }
    return instances.get(id);
}

function setState(id, newState) {
    instances.set(id, newState);
}

function deleteState(id) {
    instances.delete(id);
}

export { reducer, selectors, getState, setState, deleteState, createInitialState };
