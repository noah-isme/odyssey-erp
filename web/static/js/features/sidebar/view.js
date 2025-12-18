/**
 * Sidebar View - Render Layer
 * Single point of DOM update for sidebar
 * Following state-driven-ui architecture
 */

const view = {
    // Cache DOM references (don't query on every render)
    body: document.body,
    sidebar: null,
    overlay: null,

    /**
     * Initialize cached references
     */
    cacheElements() {
        this.sidebar = document.getElementById('sidebar');
        this.overlay = document.getElementById('sidebarOverlay');
    },

    /**
     * Render sidebar state to DOM
     * Idempotent - safe to call multiple times
     * @param {Object} state - { collapsed, pinned, mobileOpen }
     */
    render(state) {
        if (!this.sidebar) this.cacheElements();
        if (!this.sidebar) return;

        // Desktop: collapsed state via body class
        if (state.collapsed) {
            this.body.classList.add('sidebar-collapsed');
        } else {
            this.body.classList.remove('sidebar-collapsed');
        }

        // Desktop: pinned state via body class
        if (state.pinned) {
            this.body.classList.add('sidebar-pinned');
        } else {
            this.body.classList.remove('sidebar-pinned');
        }

        // Mobile: open state via sidebar class
        if (state.mobileOpen) {
            this.sidebar.classList.add('open');
            if (this.overlay) this.overlay.classList.add('open');
        } else {
            this.sidebar.classList.remove('open');
            if (this.overlay) this.overlay.classList.remove('open');
        }

        // Update ARIA attributes
        this.sidebar.setAttribute('aria-expanded', !state.collapsed);
    }
};

export { view };
