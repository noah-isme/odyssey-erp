/**
 * Table Edit View - Render Layer
 * Following state-driven-ui architecture
 */

const view = {
    /**
     * Render edit state to cell
     * @param {HTMLElement} cell - Table cell element
     * @param {Object} state - Current state
     * @param {Object} editingCell - Cell info { row, column, originalValue }
     */
    renderCell(cell, state, editingCell) {
        if (!cell) return;

        const isEditing = state.editingCell &&
            state.editingCell.row === editingCell.row &&
            state.editingCell.column === editingCell.column;

        if (isEditing) {
            this.renderEditMode(cell, state);
        } else {
            this.renderViewMode(cell, state.editingCell?.originalValue || cell.textContent);
        }
    },

    /**
     * Render cell in edit mode
     * @param {HTMLElement} cell - Cell element
     * @param {Object} state - Current state
     */
    renderEditMode(cell, state) {
        const type = cell.dataset.editType || 'text';
        const inputType = type === 'number' || type === 'currency' ? 'number' : 'text';

        cell.innerHTML = `
            <div class="table-edit-wrap">
                <input type="${inputType}" 
                       class="table-edit-input${state.error ? ' error' : ''}"
                       data-table-edit-input
                       value="${this.escapeHtml(state.pendingValue)}"
                       ${state.isSubmitting ? 'disabled' : ''}>
                ${state.error ? `<span class="table-edit-error">${state.error}</span>` : ''}
                ${state.isSubmitting ? '<span class="table-edit-loading"></span>' : ''}
            </div>
        `;

        // Focus input
        const input = cell.querySelector('[data-table-edit-input]');
        if (input && !state.isSubmitting) {
            input.focus();
            input.select();
        }
    },

    /**
     * Render cell in view mode
     * @param {HTMLElement} cell - Cell element
     * @param {string} value - Display value
     */
    renderViewMode(cell, value) {
        // Preserve the original structure if cell has edit indicator
        if (cell.querySelector('.table-edit-wrap')) {
            cell.innerHTML = `<span class="table-edit-value">${this.escapeHtml(value)}</span>`;
        }
    },

    /**
     * Update cell display value after successful edit
     * @param {HTMLElement} cell - Cell element
     * @param {string} value - New value
     */
    updateValue(cell, value) {
        cell.textContent = value;
        cell.classList.add('table-edit-updated');
        setTimeout(() => cell.classList.remove('table-edit-updated'), 1000);
    },

    /**
     * Show error state on cell
     * @param {HTMLElement} cell - Cell element
     * @param {string} error - Error message
     */
    showError(cell, error) {
        cell.classList.add('table-edit-error');
        // Could add tooltip or inline error
    },

    /**
     * Escape HTML for safe display
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
