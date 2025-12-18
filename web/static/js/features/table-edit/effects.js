/**
 * Table Edit Effects - Side Effects Layer
 * API calls for inline editing
 * Following state-driven-ui architecture
 */

const effects = {
    /**
     * Save cell value to server
     * @param {string} endpoint - API endpoint
     * @param {string} rowId - Row ID
     * @param {string} column - Column name
     * @param {string} value - New value
     * @param {Function} onSuccess - Success callback
     * @param {Function} onError - Error callback
     */
    async save(endpoint, rowId, column, value, onSuccess, onError) {
        try {
            const response = await fetch(endpoint, {
                method: 'PATCH',
                headers: {
                    'Content-Type': 'application/json',
                    'Accept': 'application/json'
                },
                body: JSON.stringify({
                    id: rowId,
                    field: column,
                    value: value
                })
            });

            if (!response.ok) {
                const data = await response.json().catch(() => ({}));
                throw new Error(data.error || `HTTP error ${response.status}`);
            }

            const data = await response.json();
            onSuccess(data);
        } catch (error) {
            onError(error.message || 'Failed to save');
        }
    },

    /**
     * Format value based on column type
     * @param {string} value - Raw value
     * @param {string} type - Column type (text, number, currency, date)
     * @returns {string} Formatted value
     */
    formatValue(value, type) {
        switch (type) {
            case 'currency':
                const num = parseFloat(value);
                return isNaN(num) ? value : num.toLocaleString('id-ID', {
                    minimumFractionDigits: 2,
                    maximumFractionDigits: 2
                });
            case 'number':
                return parseFloat(value).toLocaleString('id-ID');
            case 'date':
                return new Date(value).toLocaleDateString('id-ID');
            default:
                return value;
        }
    },

    /**
     * Parse input value based on type
     * @param {string} value - Input value
     * @param {string} type - Column type
     * @returns {any} Parsed value
     */
    parseValue(value, type) {
        switch (type) {
            case 'currency':
            case 'number':
                return parseFloat(value.replace(/[^0-9.-]/g, ''));
            default:
                return value;
        }
    }
};

export { effects };
