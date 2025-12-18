/**
 * Form Feature - Mount + Event Delegation
 * Form validation with state-driven architecture
 * Following state-driven-ui pattern
 * 
 * Usage:
 * <form data-form="customer-edit" data-endpoint="/api/customers">
 *   <div data-field="email">
 *     <label for="email">Email</label>
 *     <input type="email" name="email" id="email">
 *     <span data-error hidden></span>
 *     <span data-validating hidden>Checking...</span>
 *   </div>
 *   <button type="submit" data-submit>
 *     Save <span data-loading hidden>...</span>
 *   </button>
 * </form>
 * 
 * JS:
 * Form.register('customer-edit', {
 *   email: {
 *     required: true,
 *     pattern: /^[^\s@]+@[^\s@]+\.[^\s@]+$/,
 *     patternMessage: 'Invalid email format',
 *     asyncValidate: async (value) => {
 *       const exists = await checkEmailExists(value);
 *       return exists ? 'Email already taken' : null;
 *     }
 *   }
 * });
 */

import { reducer, selectors, getState, setState, initForm, deleteState } from './store.js';
import { effects } from './effects.js';
import { view } from './view.js';

// Track mounted forms
const mounted = new Set();

// ========== DISPATCH ==========
function dispatch(formId, action) {
    const prevState = getState(formId);
    const nextState = reducer(prevState, action);

    if (JSON.stringify(nextState) !== JSON.stringify(prevState)) {
        setState(formId, nextState);

        // View updates based on action type
        switch (action.type) {
            case 'FORM_SET_VALUE':
            case 'FORM_SET_ERROR':
            case 'FORM_TOUCH':
                view.renderFieldError(
                    formId,
                    action.payload.field || action.payload,
                    nextState.errors[action.payload.field || action.payload],
                    nextState.touched[action.payload.field || action.payload]
                );
                view.renderSubmit(formId, nextState);
                break;

            case 'FORM_SET_ERRORS':
            case 'FORM_TOUCH_ALL':
            case 'FORM_CLEAR_ERRORS':
                view.renderAllErrors(formId, nextState.errors, nextState.touched);
                view.renderSubmit(formId, nextState);
                break;

            case 'FORM_SUBMIT_START':
            case 'FORM_SUBMIT_END':
                view.renderSubmit(formId, nextState);
                break;

            case 'FORM_ASYNC_VALIDATE_START':
                view.renderAsyncIndicator(formId, action.payload, true);
                view.renderSubmit(formId, nextState);
                break;

            case 'FORM_ASYNC_VALIDATE_END':
                view.renderAsyncIndicator(formId, action.payload, false);
                view.renderSubmit(formId, nextState);
                break;

            case 'FORM_RESET':
                // Reset all field values in DOM
                Object.keys(nextState.values).forEach(field => {
                    view.setFieldValue(formId, field, nextState.values[field]);
                });
                view.renderAllErrors(formId, nextState.errors, nextState.touched);
                view.renderSubmit(formId, nextState);
                break;
        }
    }
}

// ========== VALIDATION FLOW ==========
async function validateField(formId, field, value) {
    const state = getState(formId);

    // Sync validation
    const syncError = effects.validateField(formId, field, value, state.values);
    dispatch(formId, { type: 'FORM_SET_ERROR', payload: { field, error: syncError } });

    // If sync validation passed, try async
    if (!syncError) {
        const validators = effects.getValidators(formId);
        if (validators[field]?.asyncValidate) {
            const asyncError = await effects.validateFieldAsync(
                formId, field, value, state.values,
                () => dispatch(formId, { type: 'FORM_ASYNC_VALIDATE_START', payload: field }),
                () => dispatch(formId, { type: 'FORM_ASYNC_VALIDATE_END', payload: field })
            );
            if (asyncError) {
                dispatch(formId, { type: 'FORM_SET_ERROR', payload: { field, error: asyncError } });
            }
        }
    }
}

function validateAll(formId) {
    const state = getState(formId);
    const errors = effects.validateAllFields(formId, state.values);
    dispatch(formId, { type: 'FORM_SET_ERRORS', payload: errors });
    dispatch(formId, { type: 'FORM_TOUCH_ALL' });
    return Object.keys(errors).length === 0;
}

// ========== EVENT HANDLERS ==========
function handleInput(e) {
    const form = e.target.closest('[data-form]');
    if (!form) return;

    const input = e.target;
    const field = input.name;
    if (!field) return;

    const formId = form.dataset.form;
    let value = input.value;

    // Handle different input types
    if (input.type === 'checkbox') {
        value = input.checked;
    } else if (input.type === 'number') {
        value = input.value ? Number(input.value) : '';
    }

    dispatch(formId, { type: 'FORM_SET_VALUE', payload: { field, value } });

    // Validate on input (debounced for async)
    validateField(formId, field, value);
}

function handleBlur(e) {
    const form = e.target.closest('[data-form]');
    if (!form) return;

    const field = e.target.name;
    if (!field) return;

    const formId = form.dataset.form;
    dispatch(formId, { type: 'FORM_TOUCH', payload: field });
}

async function handleSubmit(e) {
    const form = e.target.closest('[data-form]');
    if (!form || e.target.tagName !== 'FORM') return;

    e.preventDefault();

    const formId = form.dataset.form;

    // Validate all fields
    const isValid = validateAll(formId);
    if (!isValid) return;

    // Check if can submit
    if (!selectors.canSubmit(formId)) return;

    dispatch(formId, { type: 'FORM_SUBMIT_START' });

    try {
        const state = getState(formId);
        const endpoint = form.dataset.endpoint;

        if (endpoint) {
            // API submission
            const result = await effects.submitToAPI(endpoint, state.values, {
                method: form.dataset.method || 'POST'
            });

            // Emit success event
            form.dispatchEvent(new CustomEvent('form-success', {
                detail: { values: state.values, result },
                bubbles: true
            }));
        } else {
            // Just emit event for manual handling
            form.dispatchEvent(new CustomEvent('form-submit', {
                detail: { values: state.values },
                bubbles: true
            }));
        }

        dispatch(formId, { type: 'FORM_SUBMIT_END' });

    } catch (error) {
        dispatch(formId, { type: 'FORM_SUBMIT_END' });

        // Emit error event
        form.dispatchEvent(new CustomEvent('form-error', {
            detail: { error: error.message },
            bubbles: true
        }));

        // Show toast if available
        if (window.OdysseyToast) {
            window.OdysseyToast.error(error.message, 'Submission Failed');
        }
    }
}

function handleReset(e) {
    const form = e.target.closest('[data-form]');
    if (!form || e.target.tagName !== 'FORM') return;

    e.preventDefault();

    const formId = form.dataset.form;
    const initialValues = JSON.parse(form.dataset.initialValues || '{}');
    dispatch(formId, { type: 'FORM_RESET', payload: initialValues });
}

// ========== INIT ==========
function init() {
    // Event delegation at document level
    document.addEventListener('input', handleInput);
    document.addEventListener('blur', handleBlur, true); // Capture phase for blur
    document.addEventListener('submit', handleSubmit);
    document.addEventListener('reset', handleReset);
}

// ========== DESTROY ==========
function destroy() {
    document.removeEventListener('input', handleInput);
    document.removeEventListener('blur', handleBlur, true);
    document.removeEventListener('submit', handleSubmit);
    document.removeEventListener('reset', handleReset);
    mounted.clear();
}

// ========== PUBLIC API ==========
const Form = {
    init,
    destroy,

    /**
     * Register a form with validators
     * @param {string} formId - Form ID
     * @param {Object} validators - Validators config
     * @param {Object} initialValues - Initial form values
     */
    register(formId, validators = {}, initialValues = {}) {
        if (mounted.has(formId)) return;

        mounted.add(formId);
        initForm(formId, initialValues);
        effects.registerValidators(formId, validators);

        // Store initial values on form element
        const form = view.getForm(formId);
        if (form) {
            form.dataset.initialValues = JSON.stringify(initialValues);
        }

        // Initial render
        const state = getState(formId);
        view.renderSubmit(formId, state);
    },

    /**
     * Unregister a form
     * @param {string} formId - Form ID
     */
    unregister(formId) {
        mounted.delete(formId);
        effects.cancelAsyncValidations(formId);
        deleteState(formId);
        view.clearCache(formId);
    },

    /**
     * Set form value programmatically
     * @param {string} formId - Form ID
     * @param {string} field - Field name
     * @param {*} value - Value
     */
    setValue(formId, field, value) {
        dispatch(formId, { type: 'FORM_SET_VALUE', payload: { field, value } });
        view.setFieldValue(formId, field, value);
    },

    /**
     * Set multiple values
     * @param {string} formId - Form ID
     * @param {Object} values - Values object
     */
    setValues(formId, values) {
        dispatch(formId, { type: 'FORM_SET_VALUES', payload: values });
        Object.keys(values).forEach(field => {
            view.setFieldValue(formId, field, values[field]);
        });
    },

    /**
     * Set error programmatically
     * @param {string} formId - Form ID
     * @param {string} field - Field name
     * @param {string} error - Error message
     */
    setError(formId, field, error) {
        dispatch(formId, { type: 'FORM_SET_ERROR', payload: { field, error } });
        dispatch(formId, { type: 'FORM_TOUCH', payload: field });
    },

    /**
     * Reset form
     * @param {string} formId - Form ID
     * @param {Object} initialValues - Optional new initial values
     */
    reset(formId, initialValues) {
        dispatch(formId, { type: 'FORM_RESET', payload: initialValues });
    },

    /**
     * Validate form
     * @param {string} formId - Form ID
     * @returns {boolean} Is valid
     */
    validate(formId) {
        return validateAll(formId);
    },

    /**
     * Get form values
     * @param {string} formId - Form ID
     * @returns {Object} Form values
     */
    getValues(formId) {
        return selectors.getValues(formId);
    },

    /**
     * Check if form is dirty
     * @param {string} formId - Form ID
     * @returns {boolean} Is dirty
     */
    isDirty(formId) {
        return selectors.isDirty(formId);
    },

    /**
     * Check if form is valid
     * @param {string} formId - Form ID
     * @returns {boolean} Is valid
     */
    isValid(formId) {
        return selectors.isValid(formId);
    },

    selectors
};

export { Form };
