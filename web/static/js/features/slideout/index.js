/**
 * Slide-out Panel Feature
 * Drawer from right for quick edit/view
 * Following state-driven-ui architecture
 * 
 * Usage:
 * <button data-slideout-trigger="customer-edit">Edit</button>
 * 
 * <div class="slideout" data-slideout="customer-edit" hidden>
 *   <div class="slideout-header">
 *     <h2>Edit Customer</h2>
 *     <button data-slideout-close>&times;</button>
 *   </div>
 *   <div class="slideout-body">...</div>
 *   <div class="slideout-footer">
 *     <button data-slideout-close>Cancel</button>
 *     <button type="submit">Save</button>
 *   </div>
 * </div>
 */

const Slideout = {
    instances: new Map(),
    activeId: null,

    /**
     * Initialize slideout panels
     */
    init() {
        document.querySelectorAll('[data-slideout]').forEach(panel => {
            const id = panel.dataset.slideout;
            if (this.instances.has(id)) return;

            this.instances.set(id, {
                panel,
                isOpen: false,
                lastFocus: null
            });
        });

        // Event delegation
        document.addEventListener('click', (e) => {
            // Trigger button
            const trigger = e.target.closest('[data-slideout-trigger]');
            if (trigger) {
                e.preventDefault();
                this.open(trigger.dataset.slideoutTrigger, trigger);
                return;
            }

            // Close button
            const closeBtn = e.target.closest('[data-slideout-close]');
            if (closeBtn) {
                const panel = closeBtn.closest('[data-slideout]');
                if (panel) {
                    e.preventDefault();
                    this.close(panel.dataset.slideout);
                }
                return;
            }

            // Click on backdrop
            const backdrop = e.target.closest('.slideout-backdrop');
            if (backdrop && this.activeId) {
                this.close(this.activeId);
            }
        });

        // Escape key
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape' && this.activeId) {
                this.close(this.activeId);
            }
        });
    },

    /**
     * Open a slideout panel
     * @param {string} id - Panel ID
     * @param {HTMLElement} trigger - Trigger element for focus restore
     */
    open(id, trigger = null) {
        const instance = this.instances.get(id);
        if (!instance || instance.isOpen) return;

        instance.isOpen = true;
        instance.lastFocus = trigger || document.activeElement;
        this.activeId = id;

        // Show panel
        instance.panel.removeAttribute('hidden');
        instance.panel.classList.add('open');

        // Add backdrop if not exists
        let backdrop = document.querySelector('.slideout-backdrop');
        if (!backdrop) {
            backdrop = document.createElement('div');
            backdrop.className = 'slideout-backdrop';
            document.body.appendChild(backdrop);
        }
        backdrop.classList.add('visible');

        // Lock body scroll
        document.body.style.overflow = 'hidden';

        // Focus first focusable element
        requestAnimationFrame(() => {
            const focusable = instance.panel.querySelector(
                'input, select, textarea, button:not([data-slideout-close]), [tabindex]:not([tabindex="-1"])'
            );
            if (focusable) focusable.focus();
        });

        // Focus trap
        this.setupFocusTrap(instance.panel);

        // Emit event
        instance.panel.dispatchEvent(new CustomEvent('slideout-open', { bubbles: true }));
    },

    /**
     * Close a slideout panel
     * @param {string} id - Panel ID
     */
    close(id) {
        const instance = this.instances.get(id);
        if (!instance || !instance.isOpen) return;

        instance.isOpen = false;
        this.activeId = null;

        // Hide panel
        instance.panel.classList.remove('open');

        // Wait for animation
        setTimeout(() => {
            if (!instance.isOpen) {
                instance.panel.setAttribute('hidden', '');
            }
        }, 300);

        // Hide backdrop
        const backdrop = document.querySelector('.slideout-backdrop');
        if (backdrop) {
            backdrop.classList.remove('visible');
        }

        // Restore body scroll
        document.body.style.overflow = '';

        // Restore focus
        if (instance.lastFocus) {
            instance.lastFocus.focus();
        }

        // Emit event
        instance.panel.dispatchEvent(new CustomEvent('slideout-close', { bubbles: true }));
    },

    /**
     * Toggle a slideout panel
     * @param {string} id - Panel ID
     * @param {HTMLElement} trigger - Trigger element
     */
    toggle(id, trigger = null) {
        const instance = this.instances.get(id);
        if (!instance) return;

        if (instance.isOpen) {
            this.close(id);
        } else {
            this.open(id, trigger);
        }
    },

    /**
     * Setup focus trap within panel
     * @param {HTMLElement} panel - Panel element
     */
    setupFocusTrap(panel) {
        const focusableSelector = 'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])';

        panel.addEventListener('keydown', (e) => {
            if (e.key !== 'Tab') return;

            const focusable = panel.querySelectorAll(focusableSelector);
            const firstFocusable = focusable[0];
            const lastFocusable = focusable[focusable.length - 1];

            if (e.shiftKey) {
                if (document.activeElement === firstFocusable) {
                    e.preventDefault();
                    lastFocusable.focus();
                }
            } else {
                if (document.activeElement === lastFocusable) {
                    e.preventDefault();
                    firstFocusable.focus();
                }
            }
        });
    },

    /**
     * Load content into slideout via AJAX
     * @param {string} id - Panel ID
     * @param {string} url - Content URL
     */
    async loadContent(id, url) {
        const instance = this.instances.get(id);
        if (!instance) return;

        const body = instance.panel.querySelector('.slideout-body');
        if (!body) return;

        body.innerHTML = '<div class="slideout-loading"><div class="loading-spinner"></div></div>';

        try {
            const response = await fetch(url);
            if (!response.ok) throw new Error('Failed to load content');

            const html = await response.text();
            body.innerHTML = html;
        } catch (error) {
            body.innerHTML = `<div class="slideout-error">${error.message}</div>`;
        }
    },

    /**
     * Check if a panel is open
     * @param {string} id - Panel ID
     * @returns {boolean} Is open
     */
    isOpen(id) {
        return this.instances.get(id)?.isOpen || false;
    }
};

export { Slideout };
