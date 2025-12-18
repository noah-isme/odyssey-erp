/**
 * Form Effects - Side Effects Layer
 * Async validation, debounce, API submission
 * Following state-driven-ui architecture
 */

const effects = {
    // Debounce timers for async validation
    _debounceTimers: new Map(),

    // Validation rule registry per form
    _validators: new Map(),

    /**
     * Register validators for a form
     * @param {string} formId - Form ID
     * @param {Object} validators - { fieldName: validatorFn | validatorConfig }
     */
    registerValidators(formId, validators) {
        this._validators.set(formId, validators);
    },

    /**
     * Get validators for a form
     * @param {string} formId - Form ID
     * @returns {Object} Validators
     */
    getValidators(formId) {
        return this._validators.get(formId) || {};
    },

    /**
     * Validate single field synchronously
     * @param {string} formId - Form ID
     * @param {string} field - Field name
     * @param {*} value - Field value
     * @param {Object} allValues - All form values
     * @returns {string|null} Error message or null
     */
    validateField(formId, field, value, allValues) {
        const validators = this._validators.get(formId) || {};
        const validator = validators[field];

        if (!validator) return null;

        // If validator is a function
        if (typeof validator === 'function') {
            return validator(value, allValues);
        }

        // If validator is a config object
        if (typeof validator === 'object') {
            // Required
            if (validator.required && !value && value !== 0) {
                return validator.requiredMessage || `${field} is required`;
            }

            // Min length
            if (validator.minLength && typeof value === 'string' && value.length < validator.minLength) {
                return validator.minLengthMessage || `Minimum ${validator.minLength} characters`;
            }

            // Max length
            if (validator.maxLength && typeof value === 'string' && value.length > validator.maxLength) {
                return validator.maxLengthMessage || `Maximum ${validator.maxLength} characters`;
            }

            // Pattern
            if (validator.pattern && typeof value === 'string' && !validator.pattern.test(value)) {
                return validator.patternMessage || 'Invalid format';
            }

            // Min value
            if (validator.min !== undefined && Number(value) < validator.min) {
                return validator.minMessage || `Minimum value is ${validator.min}`;
            }

            // Max value
            if (validator.max !== undefined && Number(value) > validator.max) {
                return validator.maxMessage || `Maximum value is ${validator.max}`;
            }

            // Custom sync validator
            if (validator.validate && typeof validator.validate === 'function') {
                return validator.validate(value, allValues);
            }
        }

        return null;
    },

    /**
     * Validate all fields synchronously
     * @param {string} formId - Form ID
     * @param {Object} values - All form values
     * @returns {Object} Errors object { field: message }
     */
    validateAllFields(formId, values) {
        const validators = this._validators.get(formId) || {};
        const errors = {};

        Object.keys(validators).forEach(field => {
            const error = this.validateField(formId, field, values[field], values);
            if (error) {
                errors[field] = error;
            }
        });

        return errors;
    },

    /**
     * Async validate a field with debounce
     * @param {string} formId - Form ID
     * @param {string} field - Field name
     * @param {*} value - Field value
     * @param {Object} allValues - All form values
     * @param {Function} onStart - Callback when async validation starts
     * @param {Function} onEnd - Callback when async validation ends
     * @returns {Promise<string|null>}
     */
    async validateFieldAsync(formId, field, value, allValues, onStart, onEnd) {
        const validators = this._validators.get(formId) || {};
        const validator = validators[field];

        if (!validator || !validator.asyncValidate) return null;

        // Cancel previous debounce
        const timerKey = `${formId}:${field}`;
        if (this._debounceTimers.has(timerKey)) {
            clearTimeout(this._debounceTimers.get(timerKey));
        }

        const debounceMs = validator.debounce || 300;

        return new Promise((resolve) => {
            const timerId = setTimeout(async () => {
                this._debounceTimers.delete(timerKey);

                onStart?.();

                try {
                    const error = await validator.asyncValidate(value, allValues);
                    resolve(error);
                } catch (e) {
                    resolve(e.message || 'Validation failed');
                } finally {
                    onEnd?.();
                }
            }, debounceMs);

            this._debounceTimers.set(timerKey, timerId);
        });
    },

    /**
     * Cancel all pending async validations for a form
     * @param {string} formId - Form ID
     */
    cancelAsyncValidations(formId) {
        this._debounceTimers.forEach((timerId, key) => {
            if (key.startsWith(`${formId}:`)) {
                clearTimeout(timerId);
                this._debounceTimers.delete(key);
            }
        });
    },

    /**
     * Submit form via API
     * @param {string} endpoint - API endpoint
     * @param {Object} values - Form values
     * @param {Object} options - Fetch options
     * @returns {Promise<Object>}
     */
    async submitToAPI(endpoint, values, options = {}) {
        const { method = 'POST', headers = {}, ...rest } = options;

        const response = await fetch(endpoint, {
            method,
            headers: {
                'Content-Type': 'application/json',
                ...headers
            },
            body: JSON.stringify(values),
            ...rest
        });

        if (!response.ok) {
            const errorData = await response.json().catch(() => ({}));
            throw new Error(errorData.message || `HTTP ${response.status}`);
        }

        return response.json();
    }
};

export { effects };
