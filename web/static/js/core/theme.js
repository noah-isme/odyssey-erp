/**
 * Odyssey ERP - Theme Switcher
 * Light/Dark mode toggle with localStorage persistence
 */

const Theme = {
    KEY: 'odyssey.theme',
    root: document.documentElement,

    init() {
        // Apply saved theme or respect system preference
        const saved = localStorage.getItem(this.KEY);

        if (saved) {
            this.apply(saved);
        } else if (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) {
            this.apply('dark');
        }

        // Listen for system preference changes
        window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
            if (!localStorage.getItem(this.KEY)) {
                this.apply(e.matches ? 'dark' : 'light');
            }
        });

        // Setup toggle button
        this.setupToggle();
    },

    apply(theme) {
        if (theme === 'dark') {
            this.root.setAttribute('data-theme', 'dark');
        } else {
            this.root.removeAttribute('data-theme');
        }
    },

    toggle() {
        const isDark = this.root.getAttribute('data-theme') === 'dark';
        const next = isDark ? 'light' : 'dark';
        this.apply(next);
        localStorage.setItem(this.KEY, next);
        return next;
    },

    setupToggle() {
        const btn = document.getElementById('themeToggle');
        if (btn) {
            btn.addEventListener('click', () => {
                this.toggle();
            });
        }
    },

    get current() {
        return this.root.getAttribute('data-theme') === 'dark' ? 'dark' : 'light';
    }
};

export { Theme };
