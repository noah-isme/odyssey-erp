/**
 * Toast Effects - Side Effects Layer
 * Timer management for auto-dismiss
 * Following state-driven-ui architecture
 */

const effects = {
    // Active timers per toast
    _timers: new Map(),

    /**
     * Start auto-dismiss timer
     * @param {string} id - Toast ID
     * @param {number} duration - Duration in ms
     * @param {Function} onDismiss - Callback when timer fires
     */
    startTimer(id, duration, onDismiss) {
        // Clear existing timer
        this.clearTimer(id);

        if (duration <= 0) return; // No auto-dismiss if duration is 0

        const timerId = setTimeout(() => {
            this._timers.delete(id);
            onDismiss(id);
        }, duration);

        this._timers.set(id, timerId);
    },

    /**
     * Clear timer for a toast
     * @param {string} id - Toast ID
     */
    clearTimer(id) {
        const timerId = this._timers.get(id);
        if (timerId) {
            clearTimeout(timerId);
            this._timers.delete(id);
        }
    },

    /**
     * Pause timer (on hover)
     * @param {string} id - Toast ID
     */
    pauseTimer(id) {
        // For simplicity, just clear the timer
        // A more complex implementation would track remaining time
        this.clearTimer(id);
    },

    /**
     * Resume timer (on mouse leave)
     * @param {string} id - Toast ID
     * @param {number} duration - Remaining duration
     * @param {Function} onDismiss - Callback
     */
    resumeTimer(id, duration, onDismiss) {
        this.startTimer(id, duration, onDismiss);
    },

    /**
     * Clear all timers
     */
    clearAllTimers() {
        this._timers.forEach((timerId) => clearTimeout(timerId));
        this._timers.clear();
    },

    /**
     * Get variant colors for toast
     * @param {string} variant - Toast variant
     * @returns {Object} Color config
     */
    getVariantColors(variant) {
        const isDark = document.documentElement.getAttribute('data-theme') === 'dark';

        const colors = {
            neutral: { icon: '💬', bg: 'var(--toast-bg)', border: 'var(--toast-border)' },
            success: { icon: '✓', bg: 'var(--success-bg)', border: 'rgba(31,122,77,0.3)' },
            warning: { icon: '⚠', bg: 'var(--warning-bg)', border: 'rgba(178,106,0,0.3)' },
            error: { icon: '✕', bg: 'var(--error-bg)', border: 'rgba(180,35,24,0.3)' },
            info: { icon: 'ℹ', bg: 'var(--info-bg)', border: 'rgba(37,99,235,0.3)' }
        };

        return colors[variant] || colors.neutral;
    }
};

export { effects };
