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
     * Render context menu
     * @param {string} id - Table ID
     * @param {Object} state - Current state
     */
    renderContextMenu(id, state) {
        const table = this.getTable(id);
        if (!table) return;

        const container = table.closest('.table-container');
        let menu = container?.querySelector('.context-menu[data-context-menu]');

        if (!state.contextMenu) {
            // Close menu
            if (menu) {
                menu.classList.remove('visible');
                menu.setAttribute('data-state', 'closed');
                menu.hidden = true;
            }
            return;
        }

        // Create menu if not exists
        if (!menu) {
            menu = this.createContextMenu(id, container);
        }

        // Position menu
        const { x, y, rowId } = state.contextMenu;
        const positioned = this.positionContextMenu(menu, x, y);

        menu.dataset.rowId = rowId;
        menu.style.left = `${positioned.x}px`;
        menu.style.top = `${positioned.y}px`;
        menu.hidden = false;
        menu.classList.add('visible');
        menu.setAttribute('data-state', 'open');

        // Focus first item for accessibility
        requestAnimationFrame(() => {
            const firstItem = menu.querySelector('button, [role="menuitem"]');
            if (firstItem) firstItem.focus();
        });
    },

    /**
     * Create context menu element
     * @param {string} id - Table ID
     * @param {HTMLElement} container - Table container
     * @returns {HTMLElement}
     */
    createContextMenu(id, container) {
        const menu = document.createElement('div');
        menu.className = 'context-menu';
        menu.setAttribute('data-context-menu', id);
        menu.setAttribute('role', 'menu');
        menu.setAttribute('aria-label', 'Row actions');
        menu.hidden = true;

        // Default menu items - can be customized via data attributes
        menu.innerHTML = `
            <button type="button" role="menuitem" data-context-action="view">View</button>
            <button type="button" role="menuitem" data-context-action="edit">Edit</button>
            <hr role="separator">
            <button type="button" role="menuitem" data-context-action="duplicate">Duplicate</button>
            <button type="button" role="menuitem" data-context-action="delete" class="danger">Delete</button>
        `;

        container.appendChild(menu);
        return menu;
    },

    /**
     * Position context menu within viewport
     * @param {HTMLElement} menu - Menu element
     * @param {number} x - Mouse X
     * @param {number} y - Mouse Y
     * @returns {{ x: number, y: number }}
     */
    positionContextMenu(menu, x, y) {
        // Temporarily show to measure
        menu.style.visibility = 'hidden';
        menu.hidden = false;

        const rect = menu.getBoundingClientRect();
        const viewportWidth = window.innerWidth;
        const viewportHeight = window.innerHeight;

        // Adjust if overflowing right
        let finalX = x;
        if (x + rect.width > viewportWidth - 10) {
            finalX = x - rect.width;
        }

        // Adjust if overflowing bottom
        let finalY = y;
        if (y + rect.height > viewportHeight - 10) {
            finalY = y - rect.height;
        }

        // Ensure not negative
        finalX = Math.max(10, finalX);
        finalY = Math.max(10, finalY);

        menu.style.visibility = '';
        menu.hidden = true;

        return { x: finalX, y: finalY };
    },

    /**
     * Get context menu element
     * @param {string} id - Table ID
     * @returns {HTMLElement|null}
     */
    getContextMenu(id) {
        const table = this.getTable(id);
        const container = table?.closest('.table-container');
        return container?.querySelector('.context-menu[data-context-menu]');
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
