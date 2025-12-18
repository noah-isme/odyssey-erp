/**
 * Date Range Picker Effects - Side Effects Layer
 * Following state-driven-ui architecture
 */

const effects = {
    /**
     * Get preset date ranges
     * @returns {Array} Preset options
     */
    getPresets() {
        const today = new Date();
        const formatDate = (d) => d.toISOString().split('T')[0];

        return [
            {
                label: 'Today',
                start: formatDate(today),
                end: formatDate(today)
            },
            {
                label: 'Yesterday',
                start: formatDate(new Date(today.setDate(today.getDate() - 1))),
                end: formatDate(new Date())
            },
            {
                label: 'Last 7 days',
                start: formatDate(new Date(Date.now() - 7 * 24 * 60 * 60 * 1000)),
                end: formatDate(new Date())
            },
            {
                label: 'Last 30 days',
                start: formatDate(new Date(Date.now() - 30 * 24 * 60 * 60 * 1000)),
                end: formatDate(new Date())
            },
            {
                label: 'This month',
                start: formatDate(new Date(new Date().getFullYear(), new Date().getMonth(), 1)),
                end: formatDate(new Date())
            },
            {
                label: 'Last month',
                start: formatDate(new Date(new Date().getFullYear(), new Date().getMonth() - 1, 1)),
                end: formatDate(new Date(new Date().getFullYear(), new Date().getMonth(), 0))
            },
            {
                label: 'This year',
                start: formatDate(new Date(new Date().getFullYear(), 0, 1)),
                end: formatDate(new Date())
            }
        ];
    },

    /**
     * Format date for display
     * @param {string} dateStr - ISO date string
     * @returns {string} Formatted date
     */
    formatDisplay(dateStr) {
        if (!dateStr) return '';
        const date = new Date(dateStr);
        return date.toLocaleDateString('id-ID', {
            day: '2-digit',
            month: 'short',
            year: 'numeric'
        });
    },

    /**
     * Format range for display
     * @param {string} start - Start date ISO string
     * @param {string} end - End date ISO string
     * @returns {string} Formatted range
     */
    formatRange(start, end) {
        if (!start && !end) return 'Select date range';
        if (start && !end) return `From ${this.formatDisplay(start)}`;
        if (!start && end) return `Until ${this.formatDisplay(end)}`;
        if (start === end) return this.formatDisplay(start);
        return `${this.formatDisplay(start)} - ${this.formatDisplay(end)}`;
    },

    /**
     * Update URL query params
     * @param {string} startParam - Start date param name
     * @param {string} endParam - End date param name
     * @param {string} start - Start date value
     * @param {string} end - End date value
     */
    updateQueryParams(startParam, endParam, start, end) {
        const url = new URL(window.location);

        if (start) {
            url.searchParams.set(startParam, start);
        } else {
            url.searchParams.delete(startParam);
        }

        if (end) {
            url.searchParams.set(endParam, end);
        } else {
            url.searchParams.delete(endParam);
        }

        // Reset pagination when filter changes
        url.searchParams.delete('page');
        url.searchParams.delete('offset');

        window.history.replaceState({}, '', url);
    }
};

export { effects };
