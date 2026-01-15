const GlobalSearch = {
    searchTimeout: null,
    
    init() {
        const input = document.getElementById('global-search-input');
        const results = document.getElementById('global-search-results');
        if (!input || !results) return;

        input.addEventListener('input', (e) => {
            clearTimeout(this.searchTimeout);
            const query = e.target.value.trim();
            
            if (query.length < 2) {
                results.hidden = true;
                return;
            }

            this.searchTimeout = setTimeout(() => this.search(query), 200);
        });

        input.addEventListener('focus', () => {
            if (input.value.length >= 2) {
                results.hidden = false;
            }
        });

        document.addEventListener('click', (e) => {
            if (!e.target.closest('#global-search')) {
                results.hidden = true;
            }
        });

        input.addEventListener('keydown', (e) => {
            if (e.key === 'Escape') {
                results.hidden = true;
                input.blur();
            }
            if (e.key === 'Enter') {
                const first = results.querySelector('.search-result-item');
                if (first) {
                    window.location = first.dataset.url;
                }
            }
        });
    },

    async search(query) {
        const results = document.getElementById('global-search-results');
        try {
            const response = await fetch(`/api/search?q=${encodeURIComponent(query)}`);
            const data = await response.json();
            
            if (data.length === 0) {
                results.innerHTML = '<div class="search-no-results">No results found</div>';
            } else {
                results.innerHTML = data.map(item => `
                    <a href="${item.url}" class="search-result-item" data-url="${item.url}">
                        <span class="search-result-type">${this.formatType(item.type)}</span>
                        <span class="search-result-title">${item.title}</span>
                        <span class="search-result-subtitle">${item.subtitle}</span>
                    </a>
                `).join('');
            }
            results.hidden = false;
        } catch (err) {
            console.error('Search failed:', err);
            results.innerHTML = '<div class="search-no-results">Search failed</div>';
            results.hidden = false;
        }
    },

    formatType(type) {
        const types = {
            'customer': 'Customer',
            'sales_order': 'SO',
            'quotation': 'Quote',
            'purchase_order': 'PO',
            'product': 'Product',
            'supplier': 'Supplier'
        };
        return types[type] || type;
    }
};

document.addEventListener('DOMContentLoaded', () => GlobalSearch.init());

export { GlobalSearch };
