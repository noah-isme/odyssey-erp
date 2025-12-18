/**
 * Tabs Store - State + Reducer + Selectors
 * Following state-driven-ui architecture
 */

// ========== STATE ==========
const instances = new Map();

function createInitialState() {
    return {
        activeTab: null,
        tabs: [],      // Available tab names
        persist: false,
        paramName: 'tab'
    };
}

// ========== REDUCER (pure function) ==========
function reducer(state, action) {
    switch (action.type) {
        case 'TABS_INIT':
            return {
                ...state,
                tabs: action.payload.tabs || [],
                persist: action.payload.persist || false,
                paramName: action.payload.paramName || 'tab'
            };

        case 'TABS_ACTIVATE':
            return {
                ...state,
                activeTab: action.payload
            };

        default:
            return state;
    }
}

// ========== SELECTORS ==========
const selectors = {
    getState: (id) => instances.get(id) || createInitialState(),
    getActiveTab: (id) => (instances.get(id) || {}).activeTab,
    getTabs: (id) => (instances.get(id) || {}).tabs || [],
    isPersist: (id) => (instances.get(id) || {}).persist || false,
    getParamName: (id) => (instances.get(id) || {}).paramName || 'tab'
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
