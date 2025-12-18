/**
 * DataTable Effects - Side Effects Layer
 * URL navigation, bulk actions
 * Following state-driven-ui architecture
 */

const effects = {
    /**
     * Navigate to sort URL (MPA style)
     * @param {string} column - Column to sort
     * @param {string} currentDir - Current sort direction
     */
    navigateSort(column, currentDir) {
        const newDir = currentDir === 'asc' ? 'desc' : 'asc';
        const url = new URL(window.location);
        url.searchParams.set('sort', column);
        url.searchParams.set('dir', newDir);
        window.location.href = url.href;
    },

    /**
     * Navigate to row detail page
     * @param {string} href - Row href
     */
    navigateRow(href) {
        if (href) {
            window.location.href = href;
        }
    },

    /**
     * Submit bulk action form
     * @param {HTMLFormElement} form - Bulk action form
     * @param {Array} selectedIds - Selected row IDs
     * @param {string} action - Action to perform
     */
    submitBulkAction(form, selectedIds, action) {
        if (!form || selectedIds.length === 0) return;

        // Clear existing hidden inputs
        form.querySelectorAll('input[name="ids[]"]').forEach(el => el.remove());

        // Add selected IDs
        selectedIds.forEach(id => {
            const input = document.createElement('input');
            input.type = 'hidden';
            input.name = 'ids[]';
            input.value = id;
            form.appendChild(input);
        });

        // Set action if provided
        if (action) {
            const actionInput = form.querySelector('input[name="action"]');
            if (actionInput) {
                actionInput.value = action;
            }
        }

        form.submit();
    },

    /**
     * Confirm bulk action
     * @param {string} action - Action name
     * @param {number} count - Number of selected items
     * @returns {boolean} User confirmed
     */
    confirmBulkAction(action, count) {
        return window.confirm(`Are you sure you want to ${action} ${count} item(s)?`);
    },

    /**
     * Copy selected IDs to clipboard
     * @param {Array} selectedIds - Selected row IDs
     */
    async copyToClipboard(selectedIds) {
        try {
            await navigator.clipboard.writeText(selectedIds.join(','));
            return true;
        } catch (e) {
            console.error('Failed to copy:', e);
            return false;
        }
    },

    /**
     * Export selected rows
     * @param {string} endpoint - Export endpoint
     * @param {Array} selectedIds - Selected row IDs
     * @param {string} format - Export format (csv, pdf, xlsx)
     */
    exportSelected(endpoint, selectedIds, format = 'csv') {
        const url = new URL(endpoint, window.location.origin);
        url.searchParams.set('ids', selectedIds.join(','));
        url.searchParams.set('format', format);
        window.open(url, '_blank');
    }
};

export { effects };
