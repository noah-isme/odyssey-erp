/**
 * Modal View - Render Layer
 * DOM rendering for modals/dialogs
 * Following state-driven-ui architecture
 * 
 * Uses data-state="open|closed" for explicit state
 */

const view = {
    /**
     * Render modal state to DOM
     * @param {string} id - Modal ID
     * @param {Object} state - Current state
     * @param {HTMLElement} modal - Modal element
     */
    render(id, state, modal) {
        if (!modal) return;

        // Update state attribute
        modal.setAttribute('data-state', state.isOpen ? 'open' : 'closed');
        modal.setAttribute('aria-hidden', !state.isOpen);

        if (state.isOpen) {
            modal.removeAttribute('hidden');
        } else {
            modal.setAttribute('hidden', '');
        }
    },

    /**
     * Create backdrop element if needed
     * @returns {HTMLElement} Backdrop element
     */
    ensureBackdrop() {
        let backdrop = document.querySelector('[data-modal-backdrop]');
        if (!backdrop) {
            backdrop = document.createElement('div');
            backdrop.setAttribute('data-modal-backdrop', '');
            backdrop.className = 'modal-backdrop';
            document.body.appendChild(backdrop);
        }
        return backdrop;
    },

    /**
     * Show backdrop
     */
    showBackdrop() {
        const backdrop = this.ensureBackdrop();
        backdrop.setAttribute('data-state', 'visible');
        backdrop.classList.add('visible');
    },

    /**
     * Hide backdrop
     */
    hideBackdrop() {
        const backdrop = document.querySelector('[data-modal-backdrop]');
        if (backdrop) {
            backdrop.setAttribute('data-state', 'hidden');
            backdrop.classList.remove('visible');
        }
    }
};

export { view };
