/**
 * Toast Store - State + Reducer + Selectors
 * Toast/Snackbar/Notification with queue
 * Following state-driven-ui architecture
 */

// ========== STATE ==========
let state = {
    queue: [],      // Pending toasts
    active: [],     // Currently visible toasts
    maxVisible: 3   // Max toasts shown at once
};

let nextId = 1;

// ========== REDUCER (pure function) ==========
function reducer(currentState, action) {
    switch (action.type) {
        case 'TOAST_ADD':
            return {
                ...currentState,
                queue: [...currentState.queue, {
                    id: action.payload.id,
                    title: action.payload.title || '',
                    message: action.payload.message || '',
                    variant: action.payload.variant || 'neutral',
                    duration: action.payload.duration ?? 3500,
                    timestamp: Date.now()
                }]
            };

        case 'TOAST_SHOW':
            return {
                ...currentState,
                queue: currentState.queue.filter(t => t.id !== action.payload),
                active: [...currentState.active, currentState.queue.find(t => t.id === action.payload)].filter(Boolean)
            };

        case 'TOAST_DISMISS':
            return {
                ...currentState,
                active: currentState.active.filter(t => t.id !== action.payload)
            };

        case 'TOAST_CLEAR_ALL':
            return {
                ...currentState,
                queue: [],
                active: []
            };

        case 'TOAST_SET_MAX':
            return {
                ...currentState,
                maxVisible: action.payload
            };

        default:
            return currentState;
    }
}

// ========== SELECTORS ==========
const selectors = {
    getState: () => state,
    getQueue: () => state.queue,
    getActive: () => state.active,
    canShowMore: () => state.active.length < state.maxVisible,
    getNextInQueue: () => state.queue[0] || null,
    getById: (id) => state.active.find(t => t.id === id) || state.queue.find(t => t.id === id)
};

// ========== STORE API ==========
function getState() {
    return state;
}

function setState(newState) {
    state = newState;
}

function generateId() {
    return `toast-${nextId++}`;
}

export { reducer, selectors, getState, setState, generateId };
