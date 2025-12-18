/**
 * ComboBox Store - State + Reducer + Selectors
 * Searchable select with keyboard navigation
 * Following state-driven-ui architecture
 */

// ========== STATE ==========
const instances = new Map();

function createInitialState(options = []) {
    return {
        isOpen: false,
        query: '',
        options: options,              // All options
        filteredOptions: options,       // Filtered by query
        highlightIndex: -1,
        selectedValue: null,
        selectedLabel: '',
        loading: false,
        error: null,
        // Virtualization
        scrollTop: 0,
        visibleStart: 0,
        visibleEnd: 20
    };
}

// ========== REDUCER (pure function) ==========
function reducer(state, action) {
    switch (action.type) {
        case 'COMBOBOX_OPEN':
            return {
                ...state,
                isOpen: true,
                highlightIndex: state.selectedValue
                    ? state.filteredOptions.findIndex(o => o.value === state.selectedValue)
                    : 0
            };

        case 'COMBOBOX_CLOSE':
            return {
                ...state,
                isOpen: false,
                query: '',
                filteredOptions: state.options,
                highlightIndex: -1
            };

        case 'COMBOBOX_TOGGLE':
            return state.isOpen
                ? reducer(state, { type: 'COMBOBOX_CLOSE' })
                : reducer(state, { type: 'COMBOBOX_OPEN' });

        case 'COMBOBOX_SET_QUERY': {
            const query = action.payload.toLowerCase();
            const filtered = query
                ? state.options.filter(opt =>
                    opt.label.toLowerCase().includes(query) ||
                    String(opt.value).toLowerCase().includes(query)
                )
                : state.options;
            return {
                ...state,
                query: action.payload,
                filteredOptions: filtered,
                highlightIndex: filtered.length > 0 ? 0 : -1,
                visibleStart: 0
            };
        }

        case 'COMBOBOX_SET_OPTIONS':
            return {
                ...state,
                options: action.payload,
                filteredOptions: state.query
                    ? action.payload.filter(opt =>
                        opt.label.toLowerCase().includes(state.query.toLowerCase())
                    )
                    : action.payload
            };

        case 'COMBOBOX_SELECT': {
            const option = action.payload;
            return {
                ...state,
                selectedValue: option?.value ?? null,
                selectedLabel: option?.label ?? '',
                isOpen: false,
                query: '',
                filteredOptions: state.options,
                highlightIndex: -1
            };
        }

        case 'COMBOBOX_CLEAR':
            return {
                ...state,
                selectedValue: null,
                selectedLabel: '',
                query: ''
            };

        case 'COMBOBOX_HIGHLIGHT_NEXT':
            return {
                ...state,
                highlightIndex: Math.min(state.highlightIndex + 1, state.filteredOptions.length - 1)
            };

        case 'COMBOBOX_HIGHLIGHT_PREV':
            return {
                ...state,
                highlightIndex: Math.max(state.highlightIndex - 1, 0)
            };

        case 'COMBOBOX_HIGHLIGHT_SET':
            return { ...state, highlightIndex: action.payload };

        case 'COMBOBOX_HIGHLIGHT_FIRST':
            return { ...state, highlightIndex: 0 };

        case 'COMBOBOX_HIGHLIGHT_LAST':
            return { ...state, highlightIndex: Math.max(state.filteredOptions.length - 1, 0) };

        case 'COMBOBOX_SET_LOADING':
            return { ...state, loading: action.payload };

        case 'COMBOBOX_SET_ERROR':
            return { ...state, error: action.payload, loading: false };

        case 'COMBOBOX_SET_SCROLL': {
            const { scrollTop, itemHeight, containerHeight } = action.payload;
            const visibleCount = Math.ceil(containerHeight / itemHeight);
            const buffer = 5;
            return {
                ...state,
                scrollTop,
                visibleStart: Math.max(0, Math.floor(scrollTop / itemHeight) - buffer),
                visibleEnd: Math.min(
                    state.filteredOptions.length,
                    Math.floor(scrollTop / itemHeight) + visibleCount + buffer
                )
            };
        }

        default:
            return state;
    }
}

// ========== SELECTORS ==========
const selectors = {
    getState: (id) => instances.get(id) || createInitialState(),
    isOpen: (id) => (instances.get(id) || {}).isOpen || false,
    getQuery: (id) => (instances.get(id) || {}).query || '',
    getOptions: (id) => (instances.get(id) || {}).options || [],
    getFilteredOptions: (id) => (instances.get(id) || {}).filteredOptions || [],
    getHighlightIndex: (id) => (instances.get(id) || {}).highlightIndex ?? -1,
    getSelectedValue: (id) => (instances.get(id) || {}).selectedValue,
    getSelectedLabel: (id) => (instances.get(id) || {}).selectedLabel || '',
    isLoading: (id) => (instances.get(id) || {}).loading || false,
    getError: (id) => (instances.get(id) || {}).error,

    // Virtualized options
    getVisibleOptions: (id) => {
        const state = instances.get(id);
        if (!state) return [];
        return state.filteredOptions.slice(state.visibleStart, state.visibleEnd);
    },

    getVisibleRange: (id) => {
        const state = instances.get(id);
        if (!state) return { start: 0, end: 20 };
        return { start: state.visibleStart, end: state.visibleEnd };
    }
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

function initCombobox(id, options = []) {
    instances.set(id, createInitialState(options));
}

export {
    reducer,
    selectors,
    getState,
    setState,
    deleteState,
    createInitialState,
    initCombobox
};
