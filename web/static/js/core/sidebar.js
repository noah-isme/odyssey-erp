/**
 * Odyssey ERP - Sidebar Module
 */

const Sidebar = {
    sidebar: null,
    overlay: null,
    toggleBtn: null,

    init() {
        this.sidebar = document.getElementById('sidebar');
        this.overlay = document.getElementById('sidebarOverlay');
        this.toggleBtn = document.querySelector('.sidebar-toggle');

        if (!this.sidebar) return;

        if (this.toggleBtn) {
            this.toggleBtn.addEventListener('click', () => this.toggle());
        }

        if (this.overlay) {
            this.overlay.addEventListener('click', () => this.close());
        }

        document.querySelectorAll('.nav-item').forEach(item => {
            item.addEventListener('click', () => {
                if (this.isMobile()) this.close();
            });
        });

        window.addEventListener('resize', () => this.handleResize());
        this.restoreState();
    },

    toggle() {
        if (this.isMobile()) {
            this.sidebar.classList.toggle('open');
            this.overlay.classList.toggle('open');
            document.body.style.overflow = this.sidebar.classList.contains('open') ? 'hidden' : '';
        } else {
            document.body.classList.toggle('sidebar-collapsed');
            localStorage.setItem('sidebar-collapsed', document.body.classList.contains('sidebar-collapsed'));
        }
    },

    close() {
        this.sidebar.classList.remove('open');
        this.overlay.classList.remove('open');
        document.body.style.overflow = '';
    },

    isMobile() {
        return window.innerWidth <= 1024;
    },

    handleResize() {
        if (!this.isMobile()) this.close();
    },

    restoreState() {
        const collapsed = localStorage.getItem('sidebar-collapsed') === 'true';
        if (collapsed && !this.isMobile()) {
            document.body.classList.add('sidebar-collapsed');
        }
    }
};

const Navigation = {
    init() {
        this.highlightActive();
    },

    highlightActive() {
        const currentPath = window.location.pathname;
        document.querySelectorAll('.nav-item').forEach(item => {
            const href = item.getAttribute('href');
            item.classList.remove('active');
            if (href === currentPath || (href !== '/' && currentPath.startsWith(href))) {
                item.classList.add('active');
            }
        });
    }
};

export { Sidebar, Navigation };
