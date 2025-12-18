/**
 * ComboBox Effects - Side Effects Layer
 * Async option loading, focus management
 * Following state-driven-ui architecture
 */

const effects = {
    // Debounce timers for search
    _searchTimers: new Map(),

    // Option loaders registry
    _loaders: new Map(),

    /**
     * Register async option loader
     * @param {string} id - ComboBox ID
     * @param {Function} loader - Async function that returns options
     */
    registerLoader(id, loader) {
        this._loaders.set(id, loader);
    },

    /**
     * Get registered loader
     * @param {string} id - ComboBox ID
     * @returns {Function|null}
     */
    getLoader(id) {
        return this._loaders.get(id);
    },

    /**
     * Load options with debounce
     * @param {string} id - ComboBox ID
     * @param {string} query - Search query
     * @param {Function} onStart - Loading start callback
     * @param {Function} onSuccess - Success callback with options
     * @param {Function} onError - Error callback
     * @param {number} debounceMs - Debounce delay
     */
    loadOptions(id, query, onStart, onSuccess, onError, debounceMs = 300) {
        const loader = this._loaders.get(id);
        if (!loader) return;

        // Cancel previous
        if (this._searchTimers.has(id)) {
            clearTimeout(this._searchTimers.get(id));
        }

        const timerId = setTimeout(async () => {
            this._searchTimers.delete(id);
            onStart?.();

            try {
                const options = await loader(query);
                onSuccess?.(options);
            } catch (e) {
                onError?.(e.message || 'Failed to load options');
            }
        }, debounceMs);

        this._searchTimers.set(id, timerId);
    },

    /**
     * Cancel pending search
     * @param {string} id - ComboBox ID
     */
    cancelSearch(id) {
        if (this._searchTimers.has(id)) {
            clearTimeout(this._searchTimers.get(id));
            this._searchTimers.delete(id);
        }
    },

    /**
     * Focus the search input
     * @param {HTMLElement} container - ComboBox container
     */
    focusInput(container) {
        const input = container?.querySelector('[data-combobox-input]');
        if (input) {
            input.focus({ preventScroll: true });
        }
    },

    /**
     * Focus the trigger button
     * @param {HTMLElement} container - ComboBox container
     */
    focusTrigger(container) {
        const trigger = container?.querySelector('[data-combobox-trigger]');
        if (trigger) {
            trigger.focus({ preventScroll: true });
        }
    },

    /**
     * Scroll highlighted option into view
     * @param {HTMLElement} listbox - Listbox element
     * @param {number} index - Highlighted index
     */
    scrollToHighlight(listbox, index) {
        if (!listbox || index < 0) return;

        const option = listbox.querySelector(`[data-option-index="${index}"]`);
        if (option) {
            option.scrollIntoView({ block: 'nearest' });
        }
    },

    /**
     * Parse options from HTML
     * @param {HTMLElement} container - ComboBox container
     * @returns {Array} Options array
     */
    parseOptionsFromHTML(container) {
        const options = [];
        container.querySelectorAll('[data-option]').forEach(el => {
            options.push({
                value: el.dataset.value,
                label: el.textContent?.trim() || el.dataset.value,
                disabled: el.hasAttribute('disabled'),
                group: el.dataset.group
            });
        });
        return options;
    },

    /**
     * Get hidden input for form submission
     * @param {HTMLElement} container - ComboBox container
     * @returns {HTMLInputElement|null}
     */
    getHiddenInput(container) {
        return container?.querySelector('input[type="hidden"]');
    },

    /**
     * Update hidden input value
     * @param {HTMLElement} container - ComboBox container
     * @param {*} value - Selected value
     */
    updateHiddenInput(container, value) {
        const input = this.getHiddenInput(container);
        if (input) {
            input.value = value ?? '';
            // Trigger change event for form validation
            input.dispatchEvent(new Event('change', { bubbles: true }));
        }
    }
};

export { effects };
