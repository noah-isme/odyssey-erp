/**
 * Form Store - State + Reducer + Selectors
 * Form validation with state-driven architecture
 * Following state-driven-ui pattern
 * 
 * State: { values, errors, touched, submitting, dirty, valid }
 */

// ========== STATE ==========
const instances = new Map();

function createInitialState(initialValues = {}) {
    return {
        values: { ...initialValues },
        errors: {},          // { fieldName: 'error message' }
        touched: {},         // { fieldName: true }
        submitting: false,
        dirty: false,
        valid: true,
        asyncValidating: {}  // { fieldName: true } - fields currently validating
    };
}

// ========== REDUCER (pure function) ==========
function reducer(state, action) {
    switch (action.type) {
        case 'FORM_SET_VALUE': {
            const { field, value } = action.payload;
            return {
                ...state,
                values: { ...state.values, [field]: value },
                dirty: true
            };
        }

        case 'FORM_SET_VALUES': {
            return {
                ...state,
                values: { ...state.values, ...action.payload },
                dirty: true
            };
        }

        case 'FORM_SET_ERROR': {
            const { field, error } = action.payload;
            const newErrors = { ...state.errors };
            if (error) {
                newErrors[field] = error;
            } else {
                delete newErrors[field];
            }
            return {
                ...state,
                errors: newErrors,
                valid: Object.keys(newErrors).length === 0
            };
        }

        case 'FORM_SET_ERRORS': {
            return {
                ...state,
                errors: action.payload,
                valid: Object.keys(action.payload).length === 0
            };
        }

        case 'FORM_CLEAR_ERRORS': {
            return {
                ...state,
                errors: {},
                valid: true
            };
        }

        case 'FORM_TOUCH': {
            const field = action.payload;
            return {
                ...state,
                touched: { ...state.touched, [field]: true }
            };
        }

        case 'FORM_TOUCH_ALL': {
            const allTouched = {};
            Object.keys(state.values).forEach(key => {
                allTouched[key] = true;
            });
            return {
                ...state,
                touched: allTouched
            };
        }

        case 'FORM_SUBMIT_START': {
            return {
                ...state,
                submitting: true
            };
        }

        case 'FORM_SUBMIT_END': {
            return {
                ...state,
                submitting: false
            };
        }

        case 'FORM_ASYNC_VALIDATE_START': {
            const field = action.payload;
            return {
                ...state,
                asyncValidating: { ...state.asyncValidating, [field]: true }
            };
        }

        case 'FORM_ASYNC_VALIDATE_END': {
            const field = action.payload;
            const newAsync = { ...state.asyncValidating };
            delete newAsync[field];
            return {
                ...state,
                asyncValidating: newAsync
            };
        }

        case 'FORM_RESET': {
            const initialValues = action.payload || {};
            return createInitialState(initialValues);
        }

        default:
            return state;
    }
}

// ========== SELECTORS ==========
const selectors = {
    getState: (id) => instances.get(id) || createInitialState(),
    getValue: (id, field) => (instances.get(id) || {}).values?.[field],
    getValues: (id) => (instances.get(id) || {}).values || {},
    getError: (id, field) => (instances.get(id) || {}).errors?.[field],
    getErrors: (id) => (instances.get(id) || {}).errors || {},
    isTouched: (id, field) => (instances.get(id) || {}).touched?.[field] || false,
    isSubmitting: (id) => (instances.get(id) || {}).submitting || false,
    isDirty: (id) => (instances.get(id) || {}).dirty || false,
    isValid: (id) => (instances.get(id) || {}).valid !== false,
    isAsyncValidating: (id, field) => (instances.get(id) || {}).asyncValidating?.[field] || false,

    // Show error only if field is touched
    getVisibleError: (id, field) => {
        const state = instances.get(id);
        if (!state) return null;
        return state.touched[field] ? state.errors[field] : null;
    },

    // Check if form can submit
    canSubmit: (id) => {
        const state = instances.get(id);
        if (!state) return false;
        return state.valid && !state.submitting && Object.keys(state.asyncValidating).length === 0;
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

function initForm(id, initialValues = {}) {
    instances.set(id, createInitialState(initialValues));
}

export {
    reducer,
    selectors,
    getState,
    setState,
    deleteState,
    createInitialState,
    initForm
};
