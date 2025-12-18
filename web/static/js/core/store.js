/**
 * Odyssey Store Utilities
 * Base utilities for state-driven architecture
 * 
 * Features:
 * - createStore: Factory for stores with subscription
 * - DevTools logging (enable with window.__ODYSSEY_DEBUG__ = true)
 * - State snapshot for debugging
 */

// ========== DEBUG MODE ==========
const DEBUG = () => typeof window !== 'undefined' && window.__ODYSSEY_DEBUG__;

function log(feature, action, payload) {
    if (DEBUG()) {
        console.log(
            `%c[${feature}]%c ${action}`,
            'color: #6366f1; font-weight: bold',
            'color: #10b981',
            payload !== undefined ? payload : ''
        );
    }
}

function logState(feature, state) {
    if (DEBUG()) {
        console.log(
            `%c[${feature}]%c State:`,
            'color: #6366f1; font-weight: bold',
            'color: #f59e0b',
            state
        );
    }
}

// ========== STORE FACTORY ==========

/**
 * Create a store with subscription support
 * @param {string} name - Store name for logging
 * @param {Function} reducer - Reducer function
 * @param {*} initialState - Initial state
 * @returns {Object} Store API
 * 
 * @example
 * const themeStore = createStore('theme', reducer, { theme: 'light' });
 * themeStore.subscribe(state => console.log(state));
 * themeStore.dispatch({ type: 'THEME_TOGGLE' });
 */
function createStore(name, reducer, initialState) {
    let state = initialState;
    const listeners = new Set();

    function getState() {
        return state;
    }

    function dispatch(action) {
        log(name, action.type, action.payload);

        const prevState = state;
        const nextState = reducer(state, action);

        if (nextState !== prevState) {
            state = nextState;
            logState(name, state);
            notify();
        }

        return state;
    }

    function subscribe(callback) {
        listeners.add(callback);
        return () => listeners.delete(callback);
    }

    function notify() {
        listeners.forEach(fn => {
            try {
                fn(state);
            } catch (e) {
                console.error(`[${name}] Subscriber error:`, e);
            }
        });
    }

    function getSnapshot() {
        return JSON.stringify(state, null, 2);
    }

    return {
        getState,
        dispatch,
        subscribe,
        getSnapshot
    };
}

/**
 * Create a multi-instance store (Map-based)
 * @param {string} name - Store name for logging
 * @param {Function} reducer - Reducer function
 * @param {Function} createInitialState - Factory for initial state
 * @returns {Object} Store API
 * 
 * @example
 * const formStore = createMultiStore('form', reducer, () => ({ values: {} }));
 * formStore.dispatch('my-form', { type: 'SET_VALUE', payload: { field: 'name', value: 'John' } });
 */
function createMultiStore(name, reducer, createInitialState) {
    const instances = new Map();
    const listeners = new Map(); // id -> Set of callbacks

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
        listeners.delete(id);
    }

    function dispatch(id, action) {
        log(`${name}:${id}`, action.type, action.payload);

        const prevState = getState(id);
        const nextState = reducer(prevState, action);

        if (JSON.stringify(nextState) !== JSON.stringify(prevState)) {
            setState(id, nextState);
            logState(`${name}:${id}`, nextState);
            notify(id);
        }

        return nextState;
    }

    function subscribe(id, callback) {
        if (!listeners.has(id)) {
            listeners.set(id, new Set());
        }
        listeners.get(id).add(callback);
        return () => listeners.get(id)?.delete(callback);
    }

    function notify(id) {
        const subs = listeners.get(id);
        if (subs) {
            const state = getState(id);
            subs.forEach(fn => {
                try {
                    fn(state);
                } catch (e) {
                    console.error(`[${name}:${id}] Subscriber error:`, e);
                }
            });
        }
    }

    function getSnapshot(id) {
        return JSON.stringify(getState(id), null, 2);
    }

    function getAllSnapshots() {
        const all = {};
        instances.forEach((state, id) => {
            all[id] = state;
        });
        return JSON.stringify(all, null, 2);
    }

    return {
        getState,
        setState,
        deleteState,
        dispatch,
        subscribe,
        getSnapshot,
        getAllSnapshots
    };
}

// ========== DEVTOOLS ==========

/**
 * Enable debug mode
 */
function enableDebug() {
    if (typeof window !== 'undefined') {
        window.__ODYSSEY_DEBUG__ = true;
        console.log('%c🔧 Odyssey Debug Mode Enabled', 'color: #6366f1; font-weight: bold; font-size: 14px');
        console.log('Tip: All state changes will be logged to console');
    }
}

/**
 * Disable debug mode
 */
function disableDebug() {
    if (typeof window !== 'undefined') {
        window.__ODYSSEY_DEBUG__ = false;
        console.log('%c🔧 Odyssey Debug Mode Disabled', 'color: #6b7280');
    }
}

/**
 * State inspector - get all registered stores
 * Usage: OdysseyDevTools.inspect()
 */
const DevTools = {
    enable: enableDebug,
    disable: disableDebug,

    // Will be populated by features
    stores: {},

    /**
     * Register a store for inspection
     * @param {string} name - Store name
     * @param {Object} store - Store object with getState/getSnapshot
     */
    register(name, store) {
        this.stores[name] = store;
    },

    /**
     * Inspect a specific store
     * @param {string} name - Store name
     * @param {string} id - Optional instance ID for multi-stores
     */
    inspect(name, id) {
        const store = this.stores[name];
        if (!store) {
            console.log('Available stores:', Object.keys(this.stores));
            return;
        }

        if (id && store.getSnapshot) {
            console.log(store.getSnapshot(id));
        } else if (store.getAllSnapshots) {
            console.log(store.getAllSnapshots());
        } else if (store.getState) {
            console.log(JSON.stringify(store.getState(), null, 2));
        }
    },

    /**
     * List all stores
     */
    list() {
        console.log('Registered stores:', Object.keys(this.stores));
    }
};

// Expose globally
if (typeof window !== 'undefined') {
    window.OdysseyDevTools = DevTools;
}

export {
    createStore,
    createMultiStore,
    enableDebug,
    disableDebug,
    DevTools,
    log,
    logState
};
