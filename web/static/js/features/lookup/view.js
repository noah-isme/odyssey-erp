/**
 * Lookup View - Render Layer
 * DOM rendering for searchable dropdown
 * Following state-driven-ui architecture
 */

const view = {
    /**
     * Render lookup state to DOM
     * @param {string} id - Lookup instance ID
     * @param {Object} state - Current state
     * @param {HTMLElement} container - Lookup container element
     */
    render(id, state, container) {
        if (!container) return;

        const input = container.querySelector('[data-lookup-input]');
        const dropdown = container.querySelector('[data-lookup-dropdown]');
        const hiddenInput = container.querySelector('[data-lookup-value]');
        const clearBtn = container.querySelector('[data-lookup-clear]');

        // Update input value
        if (input && document.activeElement !== input) {
            input.value = state.query;
        }

        // Update hidden input (for form submission)
        if (hiddenInput) {
            hiddenInput.value = state.selectedId || '';
        }

        // Toggle clear button visibility
        if (clearBtn) {
            clearBtn.style.display = state.selectedId ? 'flex' : 'none';
        }

        // Render dropdown
        if (dropdown) {
            this.renderDropdown(dropdown, state);
        }

        // Update ARIA
        if (input) {
            input.setAttribute('aria-expanded', state.isOpen);
        }
        if (dropdown) {
            dropdown.setAttribute('aria-hidden', !state.isOpen);
        }
    },

    /**
     * Render dropdown content
     * @param {HTMLElement} dropdown - Dropdown element
     * @param {Object} state - Current state
     */
    renderDropdown(dropdown, state) {
        // Toggle visibility
        if (state.isOpen) {
            dropdown.classList.add('open');
            dropdown.removeAttribute('hidden');
        } else {
            dropdown.classList.remove('open');
            dropdown.setAttribute('hidden', '');
            return;
        }

        // Build content
        let html = '';

        if (state.isLoading) {
            html = '<div class="lookup-loading">Searching...</div>';
        } else if (state.error) {
            html = `<div class="lookup-error">${state.error}</div>`;
        } else if (state.results.length === 0 && state.query.length > 0) {
            html = '<div class="lookup-empty">No results found</div>';
        } else if (state.results.length === 0) {
            html = '<div class="lookup-hint">Type to search...</div>';
        } else {
            html = state.results.map((item, index) => {
                const isHighlighted = index === state.highlightIndex;
                const isSelected = item.id === state.selectedId;
                return `
                    <div class="lookup-item${isHighlighted ? ' highlighted' : ''}${isSelected ? ' selected' : ''}"
                         data-lookup-item
                         data-id="${item.id}"
                         data-label="${this.escapeHtml(item.label)}"
                         role="option"
                         aria-selected="${isSelected}">
                        <span class="lookup-item-label">${this.escapeHtml(item.label)}</span>
                        ${item.meta ? `<span class="lookup-item-meta">${this.escapeHtml(item.meta)}</span>` : ''}
                    </div>
                `;
            }).join('');
        }

        dropdown.innerHTML = html;
    },

    /**
     * Escape HTML to prevent XSS
     * @param {string} str - String to escape
     * @returns {string} Escaped string
     */
    escapeHtml(str) {
        if (!str) return '';
        const div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    }
};

export { view };
