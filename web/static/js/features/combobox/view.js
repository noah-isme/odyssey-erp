/**
 * ComboBox View - Render Layer
 * DOM rendering for searchable select
 * Following state-driven-ui architecture
 */

const view = {
    // Element cache
    _cache: new Map(),

    // Constants for virtualization
    ITEM_HEIGHT: 36,
    CONTAINER_HEIGHT: 250,

    /**
     * Get combobox container
     * @param {string} id - ComboBox ID
     * @returns {HTMLElement|null}
     */
    getContainer(id) {
        if (this._cache.has(id)) {
            return this._cache.get(id);
        }
        const container = document.querySelector(`[data-combobox="${id}"]`);
        if (container) {
            this._cache.set(id, container);
        }
        return container;
    },

    /**
     * Render open/closed state
     * @param {string} id - ComboBox ID
     * @param {Object} state - Current state
     */
    renderOpenState(id, state) {
        const container = this.getContainer(id);
        if (!container) return;

        const dropdown = container.querySelector('[data-combobox-dropdown]');
        const trigger = container.querySelector('[data-combobox-trigger]');

        container.setAttribute('data-state', state.isOpen ? 'open' : 'closed');

        if (dropdown) {
            dropdown.hidden = !state.isOpen;
            dropdown.setAttribute('aria-hidden', !state.isOpen);
        }

        if (trigger) {
            trigger.setAttribute('aria-expanded', state.isOpen);
        }
    },

    /**
     * Render options list (virtualized)
     * @param {string} id - ComboBox ID
     * @param {Object} state - Current state
     */
    renderOptions(id, state) {
        const container = this.getContainer(id);
        if (!container) return;

        const listbox = container.querySelector('[data-combobox-listbox]');
        if (!listbox) return;

        const { filteredOptions, visibleStart, visibleEnd, highlightIndex, selectedValue } = state;

        // Set total height for scrolling
        const totalHeight = filteredOptions.length * this.ITEM_HEIGHT;
        listbox.style.height = `${Math.min(totalHeight, this.CONTAINER_HEIGHT)}px`;

        // Get visible options
        const visibleOptions = filteredOptions.slice(visibleStart, visibleEnd);

        // Render virtualized options
        const fragment = document.createDocumentFragment();

        // Spacer for scroll position
        if (visibleStart > 0) {
            const spacer = document.createElement('div');
            spacer.style.height = `${visibleStart * this.ITEM_HEIGHT}px`;
            spacer.setAttribute('aria-hidden', 'true');
            fragment.appendChild(spacer);
        }

        visibleOptions.forEach((opt, i) => {
            const actualIndex = visibleStart + i;
            const isHighlighted = actualIndex === highlightIndex;
            const isSelected = opt.value === selectedValue;

            const el = document.createElement('div');
            el.className = `combobox-option${isHighlighted ? ' highlighted' : ''}${isSelected ? ' selected' : ''}${opt.disabled ? ' disabled' : ''}`;
            el.setAttribute('role', 'option');
            el.setAttribute('data-option-index', actualIndex);
            el.setAttribute('data-value', opt.value);
            el.setAttribute('aria-selected', isSelected);
            el.setAttribute('aria-disabled', opt.disabled || false);
            el.style.height = `${this.ITEM_HEIGHT}px`;

            // Highlight matching text
            el.innerHTML = this.highlightMatch(opt.label, state.query);

            if (isSelected) {
                el.innerHTML += ' <svg class="combobox-check" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"/></svg>';
            }

            fragment.appendChild(el);
        });

        // End spacer
        if (visibleEnd < filteredOptions.length) {
            const spacer = document.createElement('div');
            spacer.style.height = `${(filteredOptions.length - visibleEnd) * this.ITEM_HEIGHT}px`;
            spacer.setAttribute('aria-hidden', 'true');
            fragment.appendChild(spacer);
        }

        listbox.innerHTML = '';
        listbox.appendChild(fragment);

        // No results message
        if (filteredOptions.length === 0 && state.query) {
            listbox.innerHTML = '<div class="combobox-empty">No results found</div>';
        }
    },

    /**
     * Highlight matching text in label
     * @param {string} label - Option label
     * @param {string} query - Search query
     * @returns {string} HTML with highlighted text
     */
    highlightMatch(label, query) {
        if (!query) return this.escapeHtml(label);

        const escaped = this.escapeHtml(label);
        const regex = new RegExp(`(${this.escapeRegex(query)})`, 'gi');
        return escaped.replace(regex, '<mark>$1</mark>');
    },

    /**
     * Render selected value display
     * @param {string} id - ComboBox ID
     * @param {Object} state - Current state
     */
    renderSelected(id, state) {
        const container = this.getContainer(id);
        if (!container) return;

        const display = container.querySelector('[data-combobox-display]');
        const clearBtn = container.querySelector('[data-combobox-clear]');

        if (display) {
            display.textContent = state.selectedLabel || container.dataset.placeholder || 'Select...';
            display.classList.toggle('placeholder', !state.selectedLabel);
        }

        if (clearBtn) {
            clearBtn.hidden = !state.selectedValue;
        }
    },

    /**
     * Render search input
     * @param {string} id - ComboBox ID
     * @param {Object} state - Current state
     */
    renderInput(id, state) {
        const container = this.getContainer(id);
        if (!container) return;

        const input = container.querySelector('[data-combobox-input]');
        if (input && document.activeElement !== input) {
            input.value = state.query;
        }
    },

    /**
     * Render loading state
     * @param {string} id - ComboBox ID
     * @param {boolean} loading - Is loading
     */
    renderLoading(id, loading) {
        const container = this.getContainer(id);
        if (!container) return;

        container.classList.toggle('loading', loading);

        const indicator = container.querySelector('[data-combobox-loading]');
        if (indicator) {
            indicator.hidden = !loading;
        }
    },

    /**
     * Render error state
     * @param {string} id - ComboBox ID
     * @param {string|null} error - Error message
     */
    renderError(id, error) {
        const container = this.getContainer(id);
        if (!container) return;

        const errorEl = container.querySelector('[data-combobox-error]');
        if (errorEl) {
            errorEl.textContent = error || '';
            errorEl.hidden = !error;
        }
    },

    /**
     * Full render
     * @param {string} id - ComboBox ID
     * @param {Object} state - Current state
     */
    render(id, state) {
        this.renderOpenState(id, state);
        this.renderSelected(id, state);
        if (state.isOpen) {
            this.renderInput(id, state);
            this.renderOptions(id, state);
        }
        this.renderLoading(id, state.loading);
        this.renderError(id, state.error);
    },

    /**
     * Escape HTML
     * @param {string} str - String to escape
     * @returns {string}
     */
    escapeHtml(str) {
        if (!str) return '';
        const div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    },

    /**
     * Escape regex special characters
     * @param {string} str - String to escape
     * @returns {string}
     */
    escapeRegex(str) {
        return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    },

    /**
     * Clear cache
     * @param {string} id - ComboBox ID
     */
    clearCache(id) {
        this._cache.delete(id);
    }
};

export { view };
