/**
 * Odyssey ERP - Inspector Component
 */

const Inspector = {
    current: null,

    init() {
        document.querySelectorAll('[data-inspector-trigger]').forEach(trigger => {
            trigger.addEventListener('click', (e) => {
                e.preventDefault();
                const inspectorId = trigger.dataset.inspectorTrigger;
                const dataUrl = trigger.dataset.inspectorUrl;
                const inspector = document.getElementById(inspectorId);
                if (inspector) this.open(inspector, dataUrl);
            });
        });
    },

    open(inspector, dataUrl) {
        if (this.current && this.current !== inspector) {
            this.close(this.current, false);
        }

        inspector.classList.add('open');

        let overlay = document.querySelector('.inspector-overlay');
        if (!overlay) {
            overlay = document.createElement('div');
            overlay.className = 'inspector-overlay';
            document.body.appendChild(overlay);
        }
        overlay.classList.add('open');

        this.current = inspector;

        if (dataUrl) this.loadContent(inspector, dataUrl);

        overlay.addEventListener('click', () => this.close(inspector));

        inspector.querySelectorAll('[data-inspector-close]').forEach(btn => {
            btn.addEventListener('click', () => this.close(inspector));
        });

        const escHandler = (e) => {
            if (e.key === 'Escape') {
                this.close(inspector);
                document.removeEventListener('keydown', escHandler);
            }
        };
        document.addEventListener('keydown', escHandler);
    },

    close(inspector, removeOverlay = true) {
        inspector.classList.remove('open');

        if (removeOverlay) {
            const overlay = document.querySelector('.inspector-overlay');
            if (overlay) overlay.classList.remove('open');
        }

        if (this.current === inspector) this.current = null;
    },

    async loadContent(inspector, url) {
        const body = inspector.querySelector('.inspector-body');
        if (!body) return;

        body.innerHTML = `
      <div class="skeleton skeleton-text lg" style="width: 60%;"></div>
      <div class="skeleton skeleton-text" style="width: 100%;"></div>
      <div class="skeleton skeleton-text" style="width: 80%;"></div>
    `;

        try {
            const response = await fetch(url);
            const html = await response.text();
            body.innerHTML = html;
        } catch (error) {
            body.innerHTML = `<div class="empty-state"><p class="empty-state-description">Failed to load content</p></div>`;
        }
    }
};

export { Inspector };
