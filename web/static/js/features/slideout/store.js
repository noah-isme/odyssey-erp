/**
 * Slideout Store - State + Reducer + Selectors
 * Following state-driven-ui architecture
 */

// ========== STATE ==========
const instances = new Map();
let activeId = null;

function createInitialState() {
    return {
        isOpen: false,
        loading: false,
        error: null,
        content: null
    };
}

// ========== REDUCER (pure function) ==========
function reducer(state, action) {
    switch (action.type) {
        case 'SLIDEOUT_OPEN':
            // Idempotency guard
            if (state.isOpen) return state;
            return {
                ...state,
                isOpen: true,
                error: null
            };

        case 'SLIDEOUT_CLOSE':
            return {
                ...state,
                isOpen: false
            };

        case 'SLIDEOUT_SET_LOADING':
            return {
                ...state,
                loading: action.payload
            };

        case 'SLIDEOUT_SET_CONTENT':
            return {
                ...state,
                content: action.payload,
                loading: false,
                error: null
            };

        case 'SLIDEOUT_SET_ERROR':
            return {
                ...state,
                error: action.payload,
                loading: false
            };

        default:
            return state;
    }
}

// ========== SELECTORS ==========
const selectors = {
    getState: (id) => instances.get(id) || createInitialState(),
    isOpen: (id) => (instances.get(id) || {}).isOpen || false,
    isLoading: (id) => (instances.get(id) || {}).loading || false,
    getError: (id) => (instances.get(id) || {}).error,
    getActiveId: () => activeId
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

function setActiveId(id) {
    activeId = id;
}

function getActiveIdValue() {
    return activeId;
}

export {
    reducer,
    selectors,
    getState,
    setState,
    deleteState,
    createInitialState,
    setActiveId,
    getActiveIdValue
};
