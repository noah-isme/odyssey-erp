/**
 * Lookup Store - State + Reducer + Selectors
 * Searchable dropdown component for ERP
 * Following state-driven-ui architecture
 */

// ========== STATE ==========
// Each lookup instance has its own state keyed by ID
const instances = new Map();

function createInitialState() {
    return {
        query: '',
        results: [],
        selectedId: null,
        selectedLabel: '',
        isOpen: false,
        isLoading: false,
        highlightIndex: -1,
        error: null
    };
}

// ========== REDUCER (pure function) ==========
function reducer(state, action) {
    switch (action.type) {
        case 'LOOKUP_OPEN':
            return { ...state, isOpen: true, highlightIndex: -1 };

        case 'LOOKUP_CLOSE':
            return { ...state, isOpen: false, highlightIndex: -1 };

        case 'LOOKUP_TOGGLE':
            return { ...state, isOpen: !state.isOpen, highlightIndex: -1 };

        case 'LOOKUP_SET_QUERY':
            return { ...state, query: action.payload, highlightIndex: -1 };

        case 'LOOKUP_SET_LOADING':
            return { ...state, isLoading: action.payload };

        case 'LOOKUP_SET_RESULTS':
            return { ...state, results: action.payload, isLoading: false, error: null };

        case 'LOOKUP_SET_ERROR':
            return { ...state, error: action.payload, isLoading: false, results: [] };

        case 'LOOKUP_SELECT':
            return {
                ...state,
                selectedId: action.payload.id,
                selectedLabel: action.payload.label,
                query: action.payload.label,
                isOpen: false,
                results: [],
                highlightIndex: -1
            };

        case 'LOOKUP_CLEAR':
            return {
                ...state,
                selectedId: null,
                selectedLabel: '',
                query: '',
                results: [],
                isOpen: false
            };

        case 'LOOKUP_HIGHLIGHT_NEXT':
            return {
                ...state,
                highlightIndex: Math.min(state.highlightIndex + 1, state.results.length - 1)
            };

        case 'LOOKUP_HIGHLIGHT_PREV':
            return {
                ...state,
                highlightIndex: Math.max(state.highlightIndex - 1, 0)
            };

        case 'LOOKUP_HIGHLIGHT_SET':
            return { ...state, highlightIndex: action.payload };

        default:
            return state;
    }
}

// ========== SELECTORS ==========
const selectors = {
    getState: (id) => instances.get(id) || createInitialState(),
    isOpen: (id) => (instances.get(id) || {}).isOpen || false,
    getQuery: (id) => (instances.get(id) || {}).query || '',
    getResults: (id) => (instances.get(id) || {}).results || [],
    getSelectedId: (id) => (instances.get(id) || {}).selectedId,
    getSelectedLabel: (id) => (instances.get(id) || {}).selectedLabel || '',
    isLoading: (id) => (instances.get(id) || {}).isLoading || false,
    getHighlightIndex: (id) => (instances.get(id) || {}).highlightIndex ?? -1
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
