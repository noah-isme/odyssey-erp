/**
 * Odyssey ERP - Form Enhancements
 */

const Forms = {
    init() {
        document.querySelectorAll('form').forEach(form => {
            this.setupValidation(form);
            this.setupDirtyState(form);
            this.setupSubmitGuard(form);
        });

        this.setupCurrencyInputs();
        this.setupCharCount();
    },

    setupValidation(form) {
        form.querySelectorAll('[required], [pattern], [minlength], [maxlength]').forEach(input => {
            input.addEventListener('blur', () => this.validateField(input));
            input.addEventListener('input', () => {
                if (input.classList.contains('error')) {
                    this.validateField(input);
                }
            });
        });
    },

    validateField(input) {
        const errorEl = input.parentElement.querySelector('.form-error');
        let error = '';

        if (input.required && !input.value.trim()) {
            error = 'This field is required';
        } else if (input.pattern && !new RegExp(input.pattern).test(input.value)) {
            error = input.dataset.patternError || 'Invalid format';
        } else if (input.minLength && input.value.length < input.minLength) {
            error = `Minimum ${input.minLength} characters`;
        } else if (input.maxLength && input.value.length > input.maxLength) {
            error = `Maximum ${input.maxLength} characters`;
        }

        input.classList.toggle('error', !!error);

        if (errorEl) {
            errorEl.textContent = error;
            errorEl.style.display = error ? 'block' : 'none';
        }

        return !error;
    },

    setupDirtyState(form) {
        if (!form.dataset.warnUnsaved) return;

        let isDirty = false;

        form.addEventListener('change', () => { isDirty = true; });
        form.addEventListener('submit', () => { isDirty = false; });

        window.addEventListener('beforeunload', (e) => {
            if (isDirty) {
                e.preventDefault();
                e.returnValue = '';
            }
        });
    },

    setupSubmitGuard(form) {
        form.addEventListener('submit', (e) => {
            const submitBtn = form.querySelector('[type="submit"]');
            if (submitBtn && submitBtn.classList.contains('loading')) {
                e.preventDefault();
                return;
            }

            if (submitBtn) {
                submitBtn.classList.add('loading');
                submitBtn.disabled = true;
            }
        });
    },

    setupCurrencyInputs() {
        document.querySelectorAll('input[data-format="currency"]').forEach(input => {
            input.addEventListener('blur', () => {
                const value = parseFloat(input.value.replace(/[^0-9.-]/g, ''));
                if (!isNaN(value)) {
                    input.value = value.toLocaleString('id-ID', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
                }
            });

            input.addEventListener('focus', () => {
                const value = parseFloat(input.value.replace(/[^0-9.-]/g, ''));
                if (!isNaN(value)) input.value = value;
            });
        });
    },

    setupCharCount() {
        document.querySelectorAll('textarea[maxlength]').forEach(textarea => {
            const max = parseInt(textarea.maxLength);
            const counter = document.createElement('div');
            counter.className = 'char-count';
            counter.textContent = `0 / ${max}`;
            textarea.parentElement.appendChild(counter);

            textarea.addEventListener('input', () => {
                counter.textContent = `${textarea.value.length} / ${max}`;
                counter.classList.toggle('warning', textarea.value.length > max * 0.9);
            });
        });
    }
};

export { Forms };
