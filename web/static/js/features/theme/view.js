/**
 * Theme View - Render Layer
 * Single point of DOM update for theme
 * Following state-driven-ui architecture
 */

const view = {
    // Cache root reference (don't query on every render)
    root: document.documentElement,

    /**
     * Render theme state to DOM
     * Idempotent - safe to call multiple times
     * @param {Object} state - { theme: 'light' | 'dark' }
     */
    render(state) {
        // Single point of DOM update
        if (state.theme === 'dark') {
            this.root.setAttribute('data-theme', 'dark');
        } else {
            this.root.removeAttribute('data-theme');
        }

        // Update optional labels (batch update)
        const labels = document.querySelectorAll('[data-theme-label]');
        const labelText = state.theme === 'dark' ? 'Dark' : 'Light';
        labels.forEach((el) => {
            el.textContent = labelText;
        });
    }
};

export { view };
