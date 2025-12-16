/**
 * Odyssey ERP - Filter Bar Component
 */

const FilterBar = {
    init() {
        document.querySelectorAll('.filter-bar').forEach(bar => {
            this.setupSearch(bar);
            this.setupChips(bar);
            this.setupDateRange(bar);
            this.setupClear(bar);
        });
    },

    setupSearch(bar) {
        const searchInput = bar.querySelector('.filter-search input');
        if (!searchInput) return;

        let timeout;
        searchInput.addEventListener('input', () => {
            clearTimeout(timeout);
            timeout = setTimeout(() => {
                const url = new URL(window.location);
                if (searchInput.value) {
                    url.searchParams.set('q', searchInput.value);
                } else {
                    url.searchParams.delete('q');
                }
                url.searchParams.delete('page');
                window.location = url;
            }, 500);
        });

        const urlParams = new URLSearchParams(window.location.search);
        if (urlParams.has('q')) {
            searchInput.value = urlParams.get('q');
        }
    },

    setupChips(bar) {
        bar.querySelectorAll('.filter-chip').forEach(chip => {
            chip.addEventListener('click', () => {
                const filter = chip.dataset.filter;
                const value = chip.dataset.value;

                const url = new URL(window.location);

                if (chip.classList.contains('active')) {
                    url.searchParams.delete(filter);
                } else {
                    url.searchParams.set(filter, value);
                }
                url.searchParams.delete('page');
                window.location = url;
            });
        });

        const urlParams = new URLSearchParams(window.location.search);
        bar.querySelectorAll('.filter-chip').forEach(chip => {
            const filter = chip.dataset.filter;
            const value = chip.dataset.value;
            if (urlParams.get(filter) === value) {
                chip.classList.add('active');
            }
        });
    },

    setupDateRange(bar) {
        const dateInputs = bar.querySelectorAll('.filter-date-range input[type="date"]');
        dateInputs.forEach(input => {
            input.addEventListener('change', () => {
                const url = new URL(window.location);
                if (input.value) {
                    url.searchParams.set(input.name, input.value);
                } else {
                    url.searchParams.delete(input.name);
                }
                url.searchParams.delete('page');
                window.location = url;
            });

            const urlParams = new URLSearchParams(window.location.search);
            if (urlParams.has(input.name)) {
                input.value = urlParams.get(input.name);
            }
        });
    },

    setupClear(bar) {
        const clearBtn = bar.querySelector('.filter-clear');
        if (!clearBtn) return;

        clearBtn.addEventListener('click', () => {
            const url = new URL(window.location);
            url.search = '';
            window.location = url;
        });
    }
};

export { FilterBar };
