/**
 * Modal Store - State + Reducer + Selectors
 * Modal/Dialog/Drawer component with proper state management
 * Following state-driven-ui architecture
 */

// ========== STATE ==========
const instances = new Map();

// Stack for nested modals
let modalStack = [];

function createInitialState() {
    return {
        isOpen: false,
        lastFocusedEl: null, // ID of element to restore focus to
        triggerEl: null // ID of trigger element
    };
}

// ========== REDUCER (pure function) ==========
function reducer(state, action) {
    switch (action.type) {
        case 'MODAL_OPEN':
            return {
                ...state,
                isOpen: true,
                lastFocusedEl: action.payload?.lastFocusedEl || null,
                triggerEl: action.payload?.triggerEl || null
            };

        case 'MODAL_CLOSE':
            return {
                ...state,
                isOpen: false
            };

        default:
            return state;
    }
}

// ========== SELECTORS ==========
const selectors = {
    getState: (id) => instances.get(id) || createInitialState(),
    isOpen: (id) => (instances.get(id) || {}).isOpen || false,
    getLastFocused: (id) => (instances.get(id) || {}).lastFocusedEl,
    getModalStack: () => [...modalStack],
    getTopModal: () => modalStack[modalStack.length - 1] || null
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

function pushToStack(id) {
    if (!modalStack.includes(id)) {
        modalStack.push(id);
    }
}

function removeFromStack(id) {
    modalStack = modalStack.filter(m => m !== id);
}

function clearStack() {
    modalStack = [];
}

export {
    reducer,
    selectors,
    getState,
    setState,
    deleteState,
    createInitialState,
    pushToStack,
    removeFromStack,
    clearStack
};
