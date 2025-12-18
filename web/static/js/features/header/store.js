/**
 * Header Store - State + Reducer + Selectors
 * Manages dropdown states in header
 * Following state-driven-ui architecture
 */

// ========== STATE ==========
// activeDropdown: string|null - ID of currently open dropdown
let state = {
    activeDropdown: null,
    lastFocused: null // For focus restoration
};

// ========== REDUCER (pure function) ==========
function reducer(currentState, action) {
    switch (action.type) {
        case 'DROPDOWN_OPEN':
            return {
                ...currentState,
                activeDropdown: action.payload.id,
                lastFocused: action.payload.trigger || currentState.lastFocused
            };
        case 'DROPDOWN_CLOSE':
            return { ...currentState, activeDropdown: null };
        case 'DROPDOWN_TOGGLE':
            if (currentState.activeDropdown === action.payload.id) {
                return { ...currentState, activeDropdown: null };
            }
            return {
                ...currentState,
                activeDropdown: action.payload.id,
                lastFocused: action.payload.trigger || currentState.lastFocused
            };
        case 'DROPDOWN_CLOSE_ALL':
            return { ...currentState, activeDropdown: null };
        default:
            return currentState;
    }
}

// ========== SELECTORS ==========
const selectors = {
    isOpen: (id) => state.activeDropdown === id,
    hasOpen: () => state.activeDropdown !== null,
    getActive: () => state.activeDropdown,
    getLastFocused: () => state.lastFocused,
    getState: () => ({ ...state })
};

// ========== STORE API ==========
function getState() {
    return state;
}

function setState(newState) {
    state = newState;
}

export { reducer, selectors, getState, setState };
