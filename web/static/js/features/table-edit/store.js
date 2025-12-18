/**
 * Table Edit Store - State + Reducer + Selectors
 * Inline editing for ERP tables
 * Following state-driven-ui architecture
 */

// ========== STATE ==========
// Each table can have one cell being edited at a time
const instances = new Map();

function createInitialState() {
    return {
        editingCell: null, // { row, column, originalValue }
        pendingValue: '',
        isDirty: false,
        isSubmitting: false,
        error: null
    };
}

// ========== REDUCER (pure function) ==========
function reducer(state, action) {
    switch (action.type) {
        case 'EDIT_START':
            return {
                ...state,
                editingCell: action.payload, // { row, column, originalValue }
                pendingValue: action.payload.originalValue,
                isDirty: false,
                error: null
            };

        case 'EDIT_CHANGE':
            return {
                ...state,
                pendingValue: action.payload,
                isDirty: action.payload !== state.editingCell?.originalValue
            };

        case 'EDIT_CANCEL':
            return {
                ...state,
                editingCell: null,
                pendingValue: '',
                isDirty: false,
                error: null
            };

        case 'EDIT_SUBMIT':
            return { ...state, isSubmitting: true, error: null };

        case 'EDIT_SUCCESS':
            return {
                ...state,
                editingCell: null,
                pendingValue: '',
                isDirty: false,
                isSubmitting: false,
                error: null
            };

        case 'EDIT_ERROR':
            return {
                ...state,
                isSubmitting: false,
                error: action.payload
            };

        default:
            return state;
    }
}

// ========== SELECTORS ==========
const selectors = {
    getState: (id) => instances.get(id) || createInitialState(),
    isEditing: (id) => !!(instances.get(id) || {}).editingCell,
    getEditingCell: (id) => (instances.get(id) || {}).editingCell,
    getPendingValue: (id) => (instances.get(id) || {}).pendingValue || '',
    isDirty: (id) => (instances.get(id) || {}).isDirty || false,
    isSubmitting: (id) => (instances.get(id) || {}).isSubmitting || false,
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
