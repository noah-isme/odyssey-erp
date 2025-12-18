/**
 * Header View - Render Layer
 * Single point of DOM update for header dropdowns
 * Following state-driven-ui architecture
 */

const view = {
    /**
     * Render dropdown states to DOM
     * Idempotent - safe to call multiple times
     * @param {Object} state - { activeDropdown }
     */
    render(state) {
        // Close all dropdowns first (batch read)
        const allDropdowns = document.querySelectorAll('[data-dropdown]');
        const allTriggers = document.querySelectorAll('[data-dropdown-trigger]');

        // Batch write - close all
        allDropdowns.forEach(dropdown => {
            const id = dropdown.getAttribute('data-dropdown');
            const isActive = state.activeDropdown === id;

            if (isActive) {
                dropdown.classList.add('open');
                dropdown.removeAttribute('hidden');
                dropdown.setAttribute('aria-hidden', 'false');
            } else {
                dropdown.classList.remove('open');
                dropdown.setAttribute('hidden', '');
                dropdown.setAttribute('aria-hidden', 'true');
            }
        });

        // Update trigger states
        allTriggers.forEach(trigger => {
            const id = trigger.getAttribute('data-dropdown-trigger');
            const isActive = state.activeDropdown === id;

            trigger.setAttribute('aria-expanded', isActive ? 'true' : 'false');
            if (isActive) {
                trigger.classList.add('active');
            } else {
                trigger.classList.remove('active');
            }
        });
    }
};

export { view };
