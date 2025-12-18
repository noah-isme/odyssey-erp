/**
 * Export Feature - Table/Report Export
 * Handles PDF, Excel, CSV export
 * Following state-driven-ui architecture
 */

const Export = {
    /**
     * Initialize export buttons
     */
    init() {
        document.addEventListener('click', (e) => {
            const btn = e.target.closest('[data-export]');
            if (!btn) return;

            e.preventDefault();
            const format = btn.dataset.export;
            const target = btn.dataset.exportTarget || 'table';
            const endpoint = btn.dataset.exportEndpoint;

            if (endpoint) {
                this.exportFromServer(endpoint, format);
            } else {
                this.exportFromDOM(target, format);
            }
        });
    },

    /**
     * Export from server endpoint
     * @param {string} endpoint - API endpoint
     * @param {string} format - Export format (pdf, excel, csv)
     */
    async exportFromServer(endpoint, format) {
        const url = new URL(endpoint, window.location.origin);
        url.searchParams.set('format', format);

        // Preserve current filters
        const currentParams = new URLSearchParams(window.location.search);
        currentParams.forEach((value, key) => {
            if (!['format'].includes(key)) {
                url.searchParams.set(key, value);
            }
        });

        // Show loading state
        window.OdysseyLoading?.show();

        try {
            const response = await fetch(url, {
                method: 'GET',
                headers: {
                    'Accept': this.getMimeType(format)
                }
            });

            if (!response.ok) {
                throw new Error(`Export failed: ${response.status}`);
            }

            // Get filename from Content-Disposition header or generate one
            const disposition = response.headers.get('Content-Disposition');
            let filename = `export.${this.getExtension(format)}`;
            if (disposition) {
                const match = disposition.match(/filename="?([^"]+)"?/);
                if (match) filename = match[1];
            }

            // Download file
            const blob = await response.blob();
            this.downloadBlob(blob, filename);

            window.OdysseyToast?.success(`Exported to ${format.toUpperCase()}`);
        } catch (error) {
            console.error('Export error:', error);
            window.OdysseyToast?.error(error.message || 'Export failed');
        } finally {
            window.OdysseyLoading?.hide();
        }
    },

    /**
     * Export from DOM (client-side)
     * @param {string} selector - Table selector or ID
     * @param {string} format - Export format (csv only for DOM)
     */
    exportFromDOM(selector, format) {
        const table = document.querySelector(selector) || document.querySelector(`#${selector}`);
        if (!table) {
            window.OdysseyToast?.error('Table not found');
            return;
        }

        if (format === 'csv') {
            this.exportTableToCSV(table);
        } else {
            window.OdysseyToast?.warning(`${format.toUpperCase()} export requires server endpoint`);
        }
    },

    /**
     * Export table to CSV
     * @param {HTMLTableElement} table - Table element
     */
    exportTableToCSV(table) {
        const rows = [];

        // Get headers
        const headers = [];
        table.querySelectorAll('thead th').forEach(th => {
            if (!th.dataset.exportIgnore) {
                headers.push(this.escapeCSV(th.textContent.trim()));
            }
        });
        rows.push(headers.join(','));

        // Get data rows
        table.querySelectorAll('tbody tr').forEach(tr => {
            const cells = [];
            let cellIndex = 0;
            tr.querySelectorAll('td').forEach(td => {
                // Check if corresponding header is ignored
                const th = table.querySelectorAll('thead th')[cellIndex];
                if (!th?.dataset.exportIgnore) {
                    cells.push(this.escapeCSV(td.textContent.trim()));
                }
                cellIndex++;
            });
            rows.push(cells.join(','));
        });

        // Create and download CSV
        const csv = rows.join('\n');
        const blob = new Blob(['\ufeff' + csv], { type: 'text/csv;charset=utf-8' });
        const filename = `export_${new Date().toISOString().split('T')[0]}.csv`;
        this.downloadBlob(blob, filename);

        window.OdysseyToast?.success('Exported to CSV');
    },

    /**
     * Escape value for CSV
     * @param {string} value - Cell value
     * @returns {string} Escaped value
     */
    escapeCSV(value) {
        if (!value) return '';
        // Escape quotes and wrap if contains comma, quote, or newline
        if (/[,"\n\r]/.test(value)) {
            return `"${value.replace(/"/g, '""')}"`;
        }
        return value;
    },

    /**
     * Download blob as file
     * @param {Blob} blob - File blob
     * @param {string} filename - Download filename
     */
    downloadBlob(blob, filename) {
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
    },

    /**
     * Get MIME type for format
     * @param {string} format - Export format
     * @returns {string} MIME type
     */
    getMimeType(format) {
        const types = {
            pdf: 'application/pdf',
            excel: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
            xlsx: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
            csv: 'text/csv'
        };
        return types[format] || 'application/octet-stream';
    },

    /**
     * Get file extension for format
     * @param {string} format - Export format
     * @returns {string} File extension
     */
    getExtension(format) {
        const ext = {
            pdf: 'pdf',
            excel: 'xlsx',
            xlsx: 'xlsx',
            csv: 'csv'
        };
        return ext[format] || format;
    }
};

export { Export };
