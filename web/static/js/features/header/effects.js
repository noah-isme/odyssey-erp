/**
 * Header Effects - Side Effects Layer
 * Handles: Focus management, DOM queries
 * Following state-driven-ui architecture
 */

const effects = {
    /**
     * Restore focus to last focused element
     * @param {Element|null} element
     */
    restoreFocus(element) {
        if (element && typeof element.focus === 'function') {
            element.focus({ preventScroll: true });
        }
    },

    /**
     * Focus first focusable element in dropdown
     * @param {Element} dropdown
     */
    focusFirst(dropdown) {
        if (!dropdown) return;
        const focusable = dropdown.querySelector(
            'a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])'
        );
        if (focusable) {
            focusable.focus({ preventScroll: true });
        }
    },

    /**
     * Get dropdown element by ID
     * @param {string} id
     * @returns {Element|null}
     */
    getDropdown(id) {
        return document.querySelector(`[data-dropdown="${id}"]`);
    },

    /**
     * Get trigger element for dropdown
     * @param {string} id
     * @returns {Element|null}
     */
    getTrigger(id) {
        return document.querySelector(`[data-dropdown-trigger="${id}"]`);
    }
};

export { effects };
