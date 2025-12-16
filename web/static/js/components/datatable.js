/**
 * Odyssey ERP - Data Table Component
 */

const DataTable = {
    init() {
        document.querySelectorAll('.data-table').forEach(table => {
            this.setupSorting(table);
            this.setupRowClick(table);
            this.setupBulkSelect(table);
            this.setupRowActions(table);
        });
    },

    setupSorting(table) {
        table.querySelectorAll('th.sortable').forEach(th => {
            th.addEventListener('click', () => {
                const column = th.dataset.column;
                const currentDir = th.dataset.sortDir || '';
                const newDir = currentDir === 'asc' ? 'desc' : 'asc';

                const url = new URL(window.location);
                url.searchParams.set('sort', column);
                url.searchParams.set('dir', newDir);
                window.location = url;
            });
        });
    },

    setupRowClick(table) {
        table.querySelectorAll('tbody tr[data-href]').forEach(row => {
            row.addEventListener('click', (e) => {
                if (e.target.closest('input, button, a')) return;
                window.location = row.dataset.href;
            });
        });
    },

    setupBulkSelect(table) {
        const selectAll = table.querySelector('thead input[type="checkbox"]');
        const rowCheckboxes = table.querySelectorAll('tbody input[type="checkbox"]');

        if (!selectAll || rowCheckboxes.length === 0) return;

        selectAll.addEventListener('change', () => {
            rowCheckboxes.forEach(cb => {
                cb.checked = selectAll.checked;
                cb.closest('tr').classList.toggle('selected', cb.checked);
            });
            this.updateBulkActions(table);
        });

        rowCheckboxes.forEach(cb => {
            cb.addEventListener('change', () => {
                cb.closest('tr').classList.toggle('selected', cb.checked);

                const checkedCount = table.querySelectorAll('tbody input[type="checkbox"]:checked').length;
                selectAll.checked = checkedCount === rowCheckboxes.length;
                selectAll.indeterminate = checkedCount > 0 && checkedCount < rowCheckboxes.length;

                this.updateBulkActions(table);
            });
        });
    },

    updateBulkActions(table) {
        const bulkActions = table.closest('.table-container')?.querySelector('.bulk-actions');
        if (!bulkActions) return;

        const checkedCount = table.querySelectorAll('tbody input[type="checkbox"]:checked').length;
        bulkActions.classList.toggle('visible', checkedCount > 0);

        const countEl = bulkActions.querySelector('.bulk-count');
        if (countEl) countEl.textContent = checkedCount;
    },

    setupRowActions(table) {
        table.querySelectorAll('.row-action-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                const menu = btn.nextElementSibling;
                if (menu?.classList.contains('row-action-menu')) {
                    document.querySelectorAll('.row-action-menu.open').forEach(m => {
                        if (m !== menu) m.classList.remove('open');
                    });
                    menu.classList.toggle('open');
                }
            });
        });

        document.addEventListener('click', () => {
            document.querySelectorAll('.row-action-menu.open').forEach(m => m.classList.remove('open'));
        });
    },

    getSelectedRows(table) {
        return Array.from(table.querySelectorAll('tbody input[type="checkbox"]:checked'))
            .map(cb => cb.closest('tr').dataset.id);
    }
};

export { DataTable };
