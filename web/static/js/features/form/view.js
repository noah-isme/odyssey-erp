/**
 * Form View - Render Layer
 * DOM rendering for form validation states
 * Following state-driven-ui architecture
 */

const view = {
    // Cached form elements
    _formCache: new Map(),

    /**
     * Get form element and cache it
     * @param {string} formId - Form ID
     * @returns {HTMLFormElement|null}
     */
    getForm(formId) {
        if (this._formCache.has(formId)) {
            return this._formCache.get(formId);
        }
        const form = document.querySelector(`[data-form="${formId}"]`);
        if (form) {
            this._formCache.set(formId, form);
        }
        return form;
    },

    /**
     * Get field wrapper element
     * @param {HTMLFormElement} form - Form element
     * @param {string} field - Field name
     * @returns {HTMLElement|null}
     */
    getFieldWrapper(form, field) {
        return form?.querySelector(`[data-field="${field}"]`);
    },

    /**
     * Get input element
     * @param {HTMLFormElement} form - Form element
     * @param {string} field - Field name
     * @returns {HTMLInputElement|null}
     */
    getInput(form, field) {
        return form?.querySelector(`[name="${field}"]`);
    },

    /**
     * Get error element
     * @param {HTMLElement} wrapper - Field wrapper
     * @returns {HTMLElement|null}
     */
    getErrorElement(wrapper) {
        return wrapper?.querySelector('[data-error]');
    },

    /**
     * Render field error state
     * @param {string} formId - Form ID
     * @param {string} field - Field name
     * @param {string|null} error - Error message
     * @param {boolean} touched - Is field touched
     */
    renderFieldError(formId, field, error, touched) {
        const form = this.getForm(formId);
        if (!form) return;

        const wrapper = this.getFieldWrapper(form, field);
        const input = this.getInput(form, field);
        const errorEl = this.getErrorElement(wrapper);

        const showError = touched && error;

        // Update wrapper state
        if (wrapper) {
            wrapper.setAttribute('data-state', showError ? 'error' : 'valid');
            wrapper.classList.toggle('has-error', Boolean(showError));
        }

        // Update input state
        if (input) {
            input.setAttribute('aria-invalid', Boolean(showError));
            if (showError) {
                input.setAttribute('aria-describedby', `${field}-error`);
            } else {
                input.removeAttribute('aria-describedby');
            }
        }

        // Update error message
        if (errorEl) {
            errorEl.textContent = showError ? error : '';
            errorEl.hidden = !showError;
        }
    },

    /**
     * Render all field errors
     * @param {string} formId - Form ID
     * @param {Object} errors - Errors object
     * @param {Object} touched - Touched object
     */
    renderAllErrors(formId, errors, touched) {
        const form = this.getForm(formId);
        if (!form) return;

        // Get all fields
        form.querySelectorAll('[data-field]').forEach(wrapper => {
            const field = wrapper.dataset.field;
            this.renderFieldError(formId, field, errors[field], touched[field]);
        });
    },

    /**
     * Render submit button state
     * @param {string} formId - Form ID
     * @param {Object} state - Form state
     */
    renderSubmit(formId, state) {
        const form = this.getForm(formId);
        if (!form) return;

        const submit = form.querySelector('[type="submit"], [data-submit]');
        if (!submit) return;

        const canSubmit = state.valid && !state.submitting &&
            Object.keys(state.asyncValidating).length === 0;

        submit.disabled = !canSubmit;
        submit.setAttribute('data-state', state.submitting ? 'submitting' : 'idle');

        // Toggle loading indicator
        const loadingIndicator = submit.querySelector('[data-loading]');
        if (loadingIndicator) {
            loadingIndicator.hidden = !state.submitting;
        }
    },

    /**
     * Render async validation indicator
     * @param {string} formId - Form ID
     * @param {string} field - Field name
     * @param {boolean} validating - Is validating
     */
    renderAsyncIndicator(formId, field, validating) {
        const form = this.getForm(formId);
        if (!form) return;

        const wrapper = this.getFieldWrapper(form, field);
        if (!wrapper) return;

        wrapper.classList.toggle('validating', validating);

        const indicator = wrapper.querySelector('[data-validating]');
        if (indicator) {
            indicator.hidden = !validating;
        }
    },

    /**
     * Set field value in DOM
     * @param {string} formId - Form ID
     * @param {string} field - Field name
     * @param {*} value - Value to set
     */
    setFieldValue(formId, field, value) {
        const form = this.getForm(formId);
        if (!form) return;

        const input = this.getInput(form, field);
        if (!input) return;

        if (input.type === 'checkbox') {
            input.checked = Boolean(value);
        } else if (input.type === 'radio') {
            const radio = form.querySelector(`[name="${field}"][value="${value}"]`);
            if (radio) radio.checked = true;
        } else {
            input.value = value ?? '';
        }
    },

    /**
     * Clear form cache
     * @param {string} formId - Form ID
     */
    clearCache(formId) {
        this._formCache.delete(formId);
    }
};

export { view };
