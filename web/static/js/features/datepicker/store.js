/**
 * Date Range Picker Store - State + Reducer + Selectors
 * Following state-driven-ui architecture
 */

// ========== STATE ==========
const instances = new Map();

function createInitialState() {
    return {
        startDate: null,
        endDate: null,
        isOpen: false,
        activeField: null, // 'start' | 'end' | null
        presets: [] // Quick select options
    };
}

// ========== REDUCER (pure function) ==========
function reducer(state, action) {
    switch (action.type) {
        case 'DATERANGE_OPEN':
            return { ...state, isOpen: true, activeField: action.payload || 'start' };

        case 'DATERANGE_CLOSE':
            return { ...state, isOpen: false, activeField: null };

        case 'DATERANGE_TOGGLE':
            return { ...state, isOpen: !state.isOpen };

        case 'DATERANGE_SET_START':
            return { ...state, startDate: action.payload };

        case 'DATERANGE_SET_END':
            return { ...state, endDate: action.payload };

        case 'DATERANGE_SET_RANGE':
            return {
                ...state,
                startDate: action.payload.start,
                endDate: action.payload.end,
                isOpen: false
            };

        case 'DATERANGE_CLEAR':
            return { ...state, startDate: null, endDate: null };

        case 'DATERANGE_SET_ACTIVE':
            return { ...state, activeField: action.payload };

        default:
            return state;
    }
}

// ========== SELECTORS ==========
const selectors = {
    getState: (id) => instances.get(id) || createInitialState(),
    isOpen: (id) => (instances.get(id) || {}).isOpen || false,
    getStartDate: (id) => (instances.get(id) || {}).startDate,
    getEndDate: (id) => (instances.get(id) || {}).endDate,
    getRange: (id) => {
        const state = instances.get(id) || {};
        return { start: state.startDate, end: state.endDate };
    },
    isValid: (id) => {
        const state = instances.get(id) || {};
        if (!state.startDate || !state.endDate) return true;
        return new Date(state.startDate) <= new Date(state.endDate);
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

export { reducer, selectors, getState, setState, deleteState, createInitialState };
