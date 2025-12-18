/**
 * Slideout View - Render Layer
 * DOM rendering for slideout panels
 * Following state-driven-ui architecture
 */

const view = {
    _cache: new Map(),
    _backdrop: null,

    /**
     * Get slideout panel
     * @param {string} id - Slideout ID
     * @returns {HTMLElement|null}
     */
    getPanel(id) {
        if (this._cache.has(id)) {
            return this._cache.get(id);
        }
        const panel = document.querySelector(`[data-slideout="${id}"]`);
        if (panel) {
            this._cache.set(id, panel);
        }
        return panel;
    },

    /**
     * Render open state
     * @param {string} id - Slideout ID
     * @param {boolean} isOpen - Is open
     */
    renderOpen(id, isOpen) {
        const panel = this.getPanel(id);
        if (!panel) return;

        if (isOpen) {
            panel.removeAttribute('hidden');
            panel.classList.add('open');
            panel.setAttribute('data-state', 'open');
            this.showBackdrop();
        } else {
            panel.classList.remove('open');
            panel.setAttribute('data-state', 'closed');
            this.hideBackdrop();
        }
    },

    /**
     * Hide panel after animation
     * @param {string} id - Slideout ID
     */
    hidePanel(id) {
        const panel = this.getPanel(id);
        if (panel) {
            panel.setAttribute('hidden', '');
        }
    },

    /**
     * Show backdrop
     */
    showBackdrop() {
        if (!this._backdrop) {
            this._backdrop = document.createElement('div');
            this._backdrop.className = 'slideout-backdrop';
            document.body.appendChild(this._backdrop);
        }
        this._backdrop.classList.add('visible');
    },

    /**
     * Hide backdrop
     */
    hideBackdrop() {
        if (this._backdrop) {
            this._backdrop.classList.remove('visible');
        }
    },

    /**
     * Render loading state
     * @param {string} id - Slideout ID
     * @param {boolean} loading - Is loading
     */
    renderLoading(id, loading) {
        const panel = this.getPanel(id);
        if (!panel) return;

        const body = panel.querySelector('.slideout-body');
        if (!body) return;

        if (loading) {
            body.innerHTML = '<div class="slideout-loading"><div class="loading-spinner"></div></div>';
        }
    },

    /**
     * Render content
     * @param {string} id - Slideout ID
     * @param {string} content - HTML content
     */
    renderContent(id, content) {
        const panel = this.getPanel(id);
        if (!panel) return;

        const body = panel.querySelector('.slideout-body');
        if (body && content) {
            body.innerHTML = content;
        }
    },

    /**
     * Render error
     * @param {string} id - Slideout ID
     * @param {string} error - Error message
     */
    renderError(id, error) {
        const panel = this.getPanel(id);
        if (!panel) return;

        const body = panel.querySelector('.slideout-body');
        if (body && error) {
            body.innerHTML = `<div class="slideout-error">${this.escapeHtml(error)}</div>`;
        }
    },

    /**
     * Escape HTML
     * @param {string} str - String to escape
     * @returns {string}
     */
    escapeHtml(str) {
        const div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    },

    /**
     * Clear cache
     * @param {string} id - Slideout ID
     */
    clearCache(id) {
        this._cache.delete(id);
    },

    /**
     * Clear all caches
     */
    clearAllCaches() {
        this._cache.clear();
    },

    /**
     * Remove backdrop element
     */
    removeBackdrop() {
        if (this._backdrop) {
            this._backdrop.remove();
            this._backdrop = null;
        }
    }
};

export { view };
