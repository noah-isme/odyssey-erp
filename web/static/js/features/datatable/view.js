/**
 * DataTable View - Render Layer
 * DOM rendering for table state
 * Following state-driven-ui architecture
 * 
 * Prinsip:
 * - Render hanya element yang berubah (bukan innerHTML seluruh table)
 * - Update by row id (keyed update)
 * - Cache node references
 */

const view = {
    _cache: new Map(),

    /**
     * Get table container
     * @param {string} id - Table ID
     * @returns {HTMLElement|null}
     */
    getTable(id) {
        if (this._cache.has(id)) {
            return this._cache.get(id);
        }
        const table = document.querySelector(`[data-datatable="${id}"]`);
        if (table) {
            this._cache.set(id, table);
        }
        return table;
    },

    /**
     * Render selection state
     * @param {string} id - Table ID
     * @param {Object} state - Current state
     * @param {Array} allRowIds - All row IDs in table
     */
    renderSelection(id, state) {
        const table = this.getTable(id);
        if (!table) return;

        // Update header checkbox
        const selectAll = table.querySelector('thead input[type="checkbox"][data-select-all]');
        if (selectAll) {
            selectAll.checked = state.allSelected;
            selectAll.indeterminate = state.indeterminate && !state.allSelected;
        }

        // Update row checkboxes and selected class - keyed update
        table.querySelectorAll('tbody tr[data-row-id]').forEach(row => {
            const rowId = row.dataset.rowId;
            const isSelected = state.selectedRows.includes(rowId);
            const checkbox = row.querySelector('input[type="checkbox"][data-row-select]');

            if (checkbox) {
                checkbox.checked = isSelected;
            }
            row.classList.toggle('selected', isSelected);
        });
    },

    /**
     * Render bulk actions visibility
     * @param {string} id - Table ID
     * @param {Object} state - Current state
     */
    renderBulkActions(id, state) {
        const table = this.getTable(id);
        if (!table) return;

        const container = table.closest('.table-container');
        const bulkActions = container?.querySelector('.bulk-actions');
        if (!bulkActions) return;

        const hasSelection = state.selectedRows.length > 0;
        bulkActions.classList.toggle('visible', hasSelection);
        bulkActions.setAttribute('data-state', hasSelection ? 'active' : 'hidden');

        // Update count
        const countEl = bulkActions.querySelector('.bulk-count');
        if (countEl) {
            countEl.textContent = state.selectedRows.length;
        }
    },

    /**
     * Render row action menu state
     * @param {string} id - Table ID
     * @param {Object} state - Current state
     */
    renderRowMenu(id, state) {
        const table = this.getTable(id);
        if (!table) return;

        // Close all menus first
        table.querySelectorAll('.row-action-menu').forEach(menu => {
            const row = menu.closest('tr[data-row-id]');
            const isOpen = row && state.activeRowMenu === row.dataset.rowId;
            menu.classList.toggle('open', isOpen);
            menu.setAttribute('data-state', isOpen ? 'open' : 'closed');
        });
    },

    /**
     * Render expanded rows
     * @param {string} id - Table ID
     * @param {Object} state - Current state
     */
    renderExpandedRows(id, state) {
        const table = this.getTable(id);
        if (!table) return;

        table.querySelectorAll('tr[data-row-id]').forEach(row => {
            const rowId = row.dataset.rowId;
            const isExpanded = state.expandedRows.includes(rowId);
            const expandBtn = row.querySelector('[data-row-expand]');
            const expandContent = row.nextElementSibling?.classList.contains('row-expand-content')
                ? row.nextElementSibling
                : null;

            if (expandBtn) {
                expandBtn.setAttribute('aria-expanded', isExpanded);
            }
            if (expandContent) {
                expandContent.hidden = !isExpanded;
                expandContent.classList.toggle('expanded', isExpanded);
            }
            row.classList.toggle('expanded', isExpanded);
        });
    },

    /**
     * Render loading state
     * @param {string} id - Table ID
     * @param {boolean} loading - Is loading
     */
    renderLoading(id, loading) {
        const table = this.getTable(id);
        if (!table) return;

        table.classList.toggle('loading', loading);
        table.setAttribute('data-loading', loading ? 'true' : 'false');
    },

    /**
     * Render error state
     * @param {string} id - Table ID
     * @param {string|null} error - Error message
     */
    renderError(id, error) {
        const table = this.getTable(id);
        if (!table) return;

        const errorEl = table.closest('.table-container')?.querySelector('[data-table-error]');
        if (errorEl) {
            errorEl.textContent = error || '';
            errorEl.hidden = !error;
        }
    },

    /**
     * Get all row IDs from table
     * @param {string} id - Table ID
     * @returns {Array}
     */
    getAllRowIds(id) {
        const table = this.getTable(id);
        if (!table) return [];

        return Array.from(table.querySelectorAll('tbody tr[data-row-id]'))
            .map(row => row.dataset.rowId);
    },

    /**
     * Clear cache
     * @param {string} id - Table ID
     */
    clearCache(id) {
        this._cache.delete(id);
    },

    /**
     * Clear all caches
     */
    clearAllCaches() {
        this._cache.clear();
    }
};

export { view };
