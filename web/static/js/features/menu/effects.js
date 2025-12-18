/**
 * Menu Effects - Side Effects Layer
 * Focus management for dropdowns
 * Following state-driven-ui architecture
 */

const effects = {
    /**
     * Get focusable items in menu
     * @param {HTMLElement} menu - Menu element
     * @returns {HTMLElement[]} Focusable items
     */
    getFocusableItems(menu) {
        return Array.from(menu.querySelectorAll(
            '[role="menuitem"], a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])'
        ));
    },

    /**
     * Focus item at index
     * @param {HTMLElement} menu - Menu element
     * @param {number} index - Item index to focus
     */
    focusItemAtIndex(menu, index) {
        const items = this.getFocusableItems(menu);
        if (items[index]) {
            items[index].focus({ preventScroll: true });
        }
    },

    /**
     * Focus first item in menu
     * @param {HTMLElement} menu - Menu element
     */
    focusFirstItem(menu) {
        const items = this.getFocusableItems(menu);
        if (items[0]) items[0].focus({ preventScroll: true });
    },

    /**
     * Restore focus to trigger element
     * @param {string} triggerId - Trigger element ID or selector
     */
    restoreFocus(triggerId) {
        if (!triggerId) return;

        const trigger = document.getElementById(triggerId) ||
            document.querySelector(`[aria-controls="${triggerId}"]`);
        if (trigger) {
            trigger.focus({ preventScroll: true });
        }
    },

    /**
     * Get trigger element for a menu
     * @param {string} menuId - Menu element ID
     * @returns {HTMLElement|null} Trigger element
     */
    getTrigger(menuId) {
        return document.querySelector(`[data-menu-trigger][aria-controls="${menuId}"]`);
    }
};

export { effects };
