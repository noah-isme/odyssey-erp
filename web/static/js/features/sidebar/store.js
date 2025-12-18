/**
 * Sidebar Store - State + Reducer + Selectors
 * Following state-driven-ui architecture
 */

const KEY = 'odyssey.sidebar';

// ========== STATE ==========
let state = {
    collapsed: false,
    pinned: false,
    mobileOpen: false
};

// ========== REDUCER (pure function) ==========
function reducer(currentState, action) {
    switch (action.type) {
        case 'SIDEBAR_TOGGLE':
            return { ...currentState, collapsed: !currentState.collapsed };
        case 'SIDEBAR_COLLAPSE':
            return { ...currentState, collapsed: true };
        case 'SIDEBAR_EXPAND':
            return { ...currentState, collapsed: false };
        case 'SIDEBAR_PIN':
            return { ...currentState, pinned: true };
        case 'SIDEBAR_UNPIN':
            return { ...currentState, pinned: false };
        case 'SIDEBAR_TOGGLE_PIN':
            return { ...currentState, pinned: !currentState.pinned };
        case 'SIDEBAR_MOBILE_OPEN':
            return { ...currentState, mobileOpen: true };
        case 'SIDEBAR_MOBILE_CLOSE':
            return { ...currentState, mobileOpen: false };
        case 'SIDEBAR_MOBILE_TOGGLE':
            return { ...currentState, mobileOpen: !currentState.mobileOpen };
        case 'SIDEBAR_SET':
            return { ...currentState, ...action.payload };
        default:
            return currentState;
    }
}

// ========== SELECTORS ==========
const selectors = {
    isCollapsed: () => state.collapsed,
    isPinned: () => state.pinned,
    isMobileOpen: () => state.mobileOpen,
    getState: () => ({ ...state })
};

// ========== STORE API ==========
function getState() {
    return state;
}

function setState(newState) {
    state = newState;
}

export { KEY, reducer, selectors, getState, setState };
