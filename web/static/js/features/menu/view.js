/**
 * Menu View - Render Layer
 * DOM rendering for dropdown menus
 * Following state-driven-ui architecture
 * 
 * Uses data-state="open|closed" instead of hidden attribute
 */

const view = {
    /**
     * Render menu state to DOM
     * @param {string} id - Menu ID
     * @param {Object} state - Current state
     * @param {HTMLElement} menu - Menu element
     * @param {HTMLElement} trigger - Trigger element
     */
    render(id, state, menu, trigger) {
        if (!menu) return;

        // Update menu state attribute (not display/hidden)
        menu.setAttribute('data-state', state.isOpen ? 'open' : 'closed');
        menu.setAttribute('aria-hidden', !state.isOpen);

        // Toggle hidden for screen readers
        if (state.isOpen) {
            menu.removeAttribute('hidden');
        } else {
            menu.setAttribute('hidden', '');
        }

        // Update trigger ARIA
        if (trigger) {
            trigger.setAttribute('aria-expanded', state.isOpen);
        }

        // Update highlight on items
        if (state.isOpen) {
            this.renderHighlight(menu, state.highlightIndex);
        }
    },

    /**
     * Render highlight state on menu items
     * @param {HTMLElement} menu - Menu element
     * @param {number} highlightIndex - Index of highlighted item
     */
    renderHighlight(menu, highlightIndex) {
        const items = menu.querySelectorAll('[role="menuitem"], [data-menu-item]');
        items.forEach((item, index) => {
            const isHighlighted = index === highlightIndex;
            item.classList.toggle('highlighted', isHighlighted);
            item.setAttribute('aria-current', isHighlighted ? 'true' : 'false');
        });
    },

    /**
     * Get menu items for state initialization
     * @param {HTMLElement} menu - Menu element
     * @returns {Array} Item data
     */
    getItems(menu) {
        return Array.from(menu.querySelectorAll('[role="menuitem"], [data-menu-item]')).map((item, index) => ({
            index,
            id: item.id || null,
            text: item.textContent?.trim() || ''
        }));
    }
};

export { view };
