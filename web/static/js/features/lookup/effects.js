/**
 * Lookup Effects - Side Effects Layer
 * Handles: API fetch, debounce
 * Following state-driven-ui architecture
 */

const effects = {
    // Debounce timers per instance
    _timers: new Map(),

    /**
     * Debounced search - delays fetch until user stops typing
     * @param {string} id - Lookup instance ID
     * @param {string} query - Search query
     * @param {string} endpoint - API endpoint
     * @param {Function} onSuccess - Callback with results
     * @param {Function} onError - Callback with error
     * @param {number} delay - Debounce delay in ms
     */
    debouncedSearch(id, query, endpoint, onSuccess, onError, delay = 300) {
        // Clear existing timer
        if (this._timers.has(id)) {
            clearTimeout(this._timers.get(id));
        }

        // Set new timer
        const timer = setTimeout(() => {
            this.search(query, endpoint, onSuccess, onError);
            this._timers.delete(id);
        }, delay);

        this._timers.set(id, timer);
    },

    /**
     * Fetch search results from API
     * @param {string} query - Search query
     * @param {string} endpoint - API endpoint (e.g., '/api/customers/search')
     * @param {Function} onSuccess - Callback with results array
     * @param {Function} onError - Callback with error message
     */
    async search(query, endpoint, onSuccess, onError) {
        try {
            const url = new URL(endpoint, window.location.origin);
            url.searchParams.set('q', query);
            url.searchParams.set('limit', '10');

            const response = await fetch(url, {
                method: 'GET',
                headers: {
                    'Accept': 'application/json'
                }
            });

            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }

            const data = await response.json();

            // Normalize response - expect array of { id, label, ...meta }
            const results = Array.isArray(data) ? data : (data.results || data.items || []);
            onSuccess(results);
        } catch (error) {
            onError(error.message || 'Failed to fetch results');
        }
    },

    /**
     * Cancel pending search for an instance
     * @param {string} id - Lookup instance ID
     */
    cancelSearch(id) {
        if (this._timers.has(id)) {
            clearTimeout(this._timers.get(id));
            this._timers.delete(id);
        }
    }
};

export { effects };
