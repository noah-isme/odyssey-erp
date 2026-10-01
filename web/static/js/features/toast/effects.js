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
            neutral: { icon: '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>', bg: 'var(--toast-bg)', border: 'var(--toast-border)' },
            success: { icon: '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>', bg: 'var(--success-bg)', border: 'rgba(31,122,77,0.3)' },
            warning: { icon: '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>', bg: 'var(--warning-bg)', border: 'rgba(178,106,0,0.3)' },
            error: { icon: '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>', bg: 'var(--error-bg)', border: 'rgba(180,35,24,0.3)' },
            info: { icon: '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>', bg: 'var(--info-bg)', border: 'rgba(37,99,235,0.3)' }
        };

        return colors[variant] || colors.neutral;
    }
};

export { effects };
