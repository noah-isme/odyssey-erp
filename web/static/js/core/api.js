/**
 * Odyssey API Client
 * Handles fetch with CSRF protection and error handling.
 */
export const api = {
    async fetch(url, options = {}) {
        const csrfToken = document.querySelector('meta[name="csrf-token"]')?.content;
        
        const headers = {
            'Content-Type': 'application/json',
            'X-CSRF-Token': csrfToken,
            ...options.headers
        };

        const response = await fetch(url, { ...options, headers });
        
        if (!response.ok) {
            let error;
            try {
                error = await response.json();
            } catch (e) {
                error = { message: `HTTP Error: ${response.status} ${response.statusText}` };
            }
            throw error;
        }

        // Handle empty responses
        if (response.status === 204) return null;

        return response.json();
    },

    get(url, options = {}) {
        return this.fetch(url, { ...options, method: 'GET' });
    },

    post(url, data, options = {}) {
        return this.fetch(url, {
            ...options,
            method: 'POST',
            body: JSON.stringify(data)
        });
    }
};
