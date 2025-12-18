/**
 * Progress Bar Component
 * For upload/processing progress indicators
 * Following state-driven-ui architecture
 * 
 * Usage:
 * <div class="progress" data-progress="upload" data-value="0">
 *   <div class="progress-bar" data-progress-bar></div>
 *   <span class="progress-text" data-progress-text>0%</span>
 * </div>
 */

const Progress = {
    /**
     * Initialize progress bars
     */
    init() {
        // Initial render of any existing progress bars
        document.querySelectorAll('[data-progress]').forEach(el => {
            const value = parseInt(el.dataset.value) || 0;
            this.update(el.dataset.progress, value);
        });
    },

    /**
     * Update progress bar
     * @param {string} id - Progress bar ID
     * @param {number} value - Progress value (0-100)
     * @param {string} text - Optional custom text
     */
    update(id, value, text = null) {
        const container = document.querySelector(`[data-progress="${id}"]`);
        if (!container) return;

        const clampedValue = Math.max(0, Math.min(100, value));
        container.dataset.value = clampedValue;

        // Update bar width
        const bar = container.querySelector('[data-progress-bar]');
        if (bar) {
            bar.style.width = `${clampedValue}%`;
        }

        // Update text
        const textEl = container.querySelector('[data-progress-text]');
        if (textEl) {
            textEl.textContent = text !== null ? text : `${clampedValue}%`;
        }

        // Add complete class
        container.classList.toggle('complete', clampedValue >= 100);

        // Emit event
        container.dispatchEvent(new CustomEvent('progress-update', {
            detail: { value: clampedValue },
            bubbles: true
        }));
    },

    /**
     * Create progress bar dynamically
     * @param {string} id - Progress bar ID
     * @param {Object} options - Configuration options
     * @returns {HTMLElement} Progress bar element
     */
    create(id, options = {}) {
        const {
            value = 0,
            variant = '', // 'striped', 'animated'
            size = '', // 'sm', 'lg'
            showText = true
        } = options;

        const container = document.createElement('div');
        container.className = `progress${variant ? ` progress--${variant}` : ''}${size ? ` progress--${size}` : ''}`;
        container.dataset.progress = id;
        container.dataset.value = value;

        container.innerHTML = `
            <div class="progress-bar" data-progress-bar style="width: ${value}%"></div>
            ${showText ? `<span class="progress-text" data-progress-text>${value}%</span>` : ''}
        `;

        return container;
    },

    /**
     * Reset progress bar to 0
     * @param {string} id - Progress bar ID
     */
    reset(id) {
        this.update(id, 0);
    },

    /**
     * Complete progress bar
     * @param {string} id - Progress bar ID
     */
    complete(id) {
        this.update(id, 100);
    },

    /**
     * Set indeterminate state
     * @param {string} id - Progress bar ID
     * @param {boolean} indeterminate - Is indeterminate
     */
    setIndeterminate(id, indeterminate = true) {
        const container = document.querySelector(`[data-progress="${id}"]`);
        if (!container) return;

        container.classList.toggle('indeterminate', indeterminate);

        const textEl = container.querySelector('[data-progress-text]');
        if (textEl) {
            textEl.textContent = indeterminate ? 'Processing...' : `${container.dataset.value}%`;
        }
    }
};

export { Progress };
