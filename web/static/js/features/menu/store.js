/**
 * Menu Store - State + Reducer + Selectors
 * Dropdown/Menu component with proper state management
 * Following state-driven-ui architecture
 */

// ========== STATE ==========
const instances = new Map();

function createInitialState() {
    return {
        isOpen: false,
        highlightIndex: -1,
        items: [],
        triggerId: null
    };
}

// ========== REDUCER (pure function) ==========
function reducer(state, action) {
    switch (action.type) {
        case 'MENU_OPEN':
            return {
                ...state,
                isOpen: true,
                highlightIndex: 0,
                triggerId: action.payload?.triggerId || null
            };

        case 'MENU_CLOSE':
            return {
                ...state,
                isOpen: false,
                highlightIndex: -1
            };

        case 'MENU_TOGGLE':
            return state.isOpen
                ? reducer(state, { type: 'MENU_CLOSE' })
                : reducer(state, { type: 'MENU_OPEN', payload: action.payload });

        case 'MENU_HIGHLIGHT_NEXT':
            return {
                ...state,
                highlightIndex: Math.min(state.highlightIndex + 1, state.items.length - 1)
            };

        case 'MENU_HIGHLIGHT_PREV':
            return {
                ...state,
                highlightIndex: Math.max(state.highlightIndex - 1, 0)
            };

        case 'MENU_HIGHLIGHT_SET':
            return { ...state, highlightIndex: action.payload };

        case 'MENU_SET_ITEMS':
            return { ...state, items: action.payload };

        case 'MENU_HIGHLIGHT_FIRST':
            return { ...state, highlightIndex: 0 };

        case 'MENU_HIGHLIGHT_LAST':
            return { ...state, highlightIndex: Math.max(state.items.length - 1, 0) };

        default:
            return state;
    }
}

// ========== SELECTORS ==========
const selectors = {
    getState: (id) => instances.get(id) || createInitialState(),
    isOpen: (id) => (instances.get(id) || {}).isOpen || false,
    getHighlightIndex: (id) => (instances.get(id) || {}).highlightIndex ?? -1,
    getItems: (id) => (instances.get(id) || {}).items || [],
    getTriggerId: (id) => (instances.get(id) || {}).triggerId
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

function getAllOpenMenus() {
    const openMenus = [];
    instances.forEach((state, id) => {
        if (state.isOpen) openMenus.push(id);
    });
    return openMenus;
}

export { reducer, selectors, getState, setState, deleteState, createInitialState, getAllOpenMenus };
