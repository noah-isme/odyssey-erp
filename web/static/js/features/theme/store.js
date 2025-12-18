/**
 * Theme Store - State + Reducer + Selectors
 * Following state-driven-ui architecture
 */

const KEY = 'odyssey.theme';

// ========== STATE ==========
let state = { theme: 'light' };

// ========== REDUCER (pure function) ==========
function reducer(currentState, action) {
    switch (action.type) {
        case 'THEME_SET':
            return { ...currentState, theme: action.payload };
        case 'THEME_TOGGLE':
            return { ...currentState, theme: currentState.theme === 'dark' ? 'light' : 'dark' };
        default:
            return currentState;
    }
}

// ========== SELECTORS ==========
const selectors = {
    isDark: () => state.theme === 'dark',
    current: () => state.theme,
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
