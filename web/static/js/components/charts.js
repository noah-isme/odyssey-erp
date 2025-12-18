/**
 * Charts Feature - Chart.js Integration
 * Dashboard visualizations for ERP
 * Following state-driven-ui architecture
 * 
 * Usage:
 * <canvas data-chart="sales-trend" 
 *         data-endpoint="/api/dashboard/sales"
 *         data-type="line"
 *         data-title="Sales Trend"></canvas>
 */

const Charts = {
    instances: new Map(),
    chartJS: null,

    /**
     * Initialize charts - lazy load Chart.js
     */
    async init() {
        const chartContainers = document.querySelectorAll('[data-chart]');
        if (chartContainers.length === 0) return;

        // Lazy load Chart.js from CDN
        if (!this.chartJS) {
            await this.loadChartJS();
        }

        // Initialize each chart
        chartContainers.forEach(container => {
            this.initializeChart(container);
        });
    },

    /**
     * Load Chart.js from CDN
     */
    async loadChartJS() {
        return new Promise((resolve, reject) => {
            if (window.Chart) {
                this.chartJS = window.Chart;
                resolve();
                return;
            }

            const script = document.createElement('script');
            script.src = 'https://cdn.jsdelivr.net/npm/chart.js@4.4.1/dist/chart.umd.min.js';
            script.async = true;
            script.onload = () => {
                this.chartJS = window.Chart;
                resolve();
            };
            script.onerror = () => reject(new Error('Failed to load Chart.js'));
            document.head.appendChild(script);
        });
    },

    /**
     * Initialize a single chart
     * @param {HTMLCanvasElement} canvas - Canvas element
     */
    async initializeChart(canvas) {
        const id = canvas.dataset.chart;
        const endpoint = canvas.dataset.endpoint;
        const type = canvas.dataset.type || 'line';
        const title = canvas.dataset.title || '';

        // Fetch data if endpoint provided
        let data = { labels: [], datasets: [] };
        if (endpoint) {
            try {
                const response = await fetch(endpoint);
                if (response.ok) {
                    data = await response.json();
                }
            } catch (error) {
                console.error(`Failed to fetch chart data for ${id}:`, error);
            }
        }

        // Create chart with theme-aware colors
        const chart = new this.chartJS(canvas, {
            type: type,
            data: this.prepareData(data, type),
            options: this.getDefaultOptions(type, title)
        });

        this.instances.set(id, chart);

        // Listen for theme changes
        document.addEventListener('click', (e) => {
            if (e.target.closest('[data-theme-toggle]')) {
                setTimeout(() => this.updateTheme(id), 100);
            }
        });
    },

    /**
     * Prepare chart data with consistent styling
     * @param {Object} data - Raw data from API
     * @param {string} type - Chart type
     * @returns {Object} Formatted data
     */
    prepareData(data, type) {
        const colors = this.getColors();

        // Ensure datasets have colors
        if (data.datasets) {
            data.datasets = data.datasets.map((ds, i) => ({
                ...ds,
                backgroundColor: ds.backgroundColor || colors.backgrounds[i % colors.backgrounds.length],
                borderColor: ds.borderColor || colors.borders[i % colors.borders.length],
                borderWidth: ds.borderWidth || 2,
                tension: type === 'line' ? 0.3 : undefined,
                fill: type === 'line' ? 'origin' : undefined
            }));
        }

        return data;
    },

    /**
     * Get theme-aware colors
     * @returns {Object} Color palette
     */
    getColors() {
        const isDark = document.documentElement.dataset.theme === 'dark';

        return {
            backgrounds: [
                isDark ? 'rgba(46, 196, 182, 0.2)' : 'rgba(14, 165, 233, 0.2)',
                isDark ? 'rgba(255, 107, 107, 0.2)' : 'rgba(239, 68, 68, 0.2)',
                isDark ? 'rgba(255, 230, 109, 0.2)' : 'rgba(245, 158, 11, 0.2)',
                isDark ? 'rgba(155, 89, 182, 0.2)' : 'rgba(139, 92, 246, 0.2)',
            ],
            borders: [
                isDark ? 'rgb(46, 196, 182)' : 'rgb(14, 165, 233)',
                isDark ? 'rgb(255, 107, 107)' : 'rgb(239, 68, 68)',
                isDark ? 'rgb(255, 230, 109)' : 'rgb(245, 158, 11)',
                isDark ? 'rgb(155, 89, 182)' : 'rgb(139, 92, 246)',
            ],
            grid: isDark ? 'rgba(255, 255, 255, 0.1)' : 'rgba(0, 0, 0, 0.1)',
            text: isDark ? 'rgba(255, 255, 255, 0.7)' : 'rgba(0, 0, 0, 0.7)'
        };
    },

    /**
     * Get default chart options
     * @param {string} type - Chart type
     * @param {string} title - Chart title
     * @returns {Object} Chart.js options
     */
    getDefaultOptions(type, title) {
        const colors = this.getColors();

        return {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: {
                    display: true,
                    position: 'bottom',
                    labels: {
                        color: colors.text,
                        padding: 20
                    }
                },
                title: {
                    display: !!title,
                    text: title,
                    color: colors.text,
                    font: { size: 14, weight: 600 }
                },
                tooltip: {
                    mode: 'index',
                    intersect: false,
                    backgroundColor: 'rgba(0, 0, 0, 0.8)',
                    titleColor: '#fff',
                    bodyColor: '#fff',
                    borderColor: colors.borders[0],
                    borderWidth: 1
                }
            },
            scales: type !== 'pie' && type !== 'doughnut' ? {
                x: {
                    grid: { color: colors.grid },
                    ticks: { color: colors.text }
                },
                y: {
                    grid: { color: colors.grid },
                    ticks: { color: colors.text }
                }
            } : undefined,
            interaction: {
                mode: 'nearest',
                axis: 'x',
                intersect: false
            }
        };
    },

    /**
     * Update chart theme after toggle
     * @param {string} id - Chart ID
     */
    updateTheme(id) {
        const chart = this.instances.get(id);
        if (!chart) return;

        const colors = this.getColors();

        // Update colors in datasets
        chart.data.datasets.forEach((ds, i) => {
            ds.backgroundColor = colors.backgrounds[i % colors.backgrounds.length];
            ds.borderColor = colors.borders[i % colors.borders.length];
        });

        // Update options
        if (chart.options.scales?.x) {
            chart.options.scales.x.grid.color = colors.grid;
            chart.options.scales.x.ticks.color = colors.text;
        }
        if (chart.options.scales?.y) {
            chart.options.scales.y.grid.color = colors.grid;
            chart.options.scales.y.ticks.color = colors.text;
        }

        chart.options.plugins.legend.labels.color = colors.text;
        chart.options.plugins.title.color = colors.text;

        chart.update();
    },

    /**
     * Update chart data
     * @param {string} id - Chart ID
     * @param {Object} data - New data
     */
    updateData(id, data) {
        const chart = this.instances.get(id);
        if (!chart) return;

        chart.data = this.prepareData(data, chart.config.type);
        chart.update();
    },

    /**
     * Refresh chart from endpoint
     * @param {string} id - Chart ID
     */
    async refresh(id) {
        const canvas = document.querySelector(`[data-chart="${id}"]`);
        if (!canvas) return;

        const endpoint = canvas.dataset.endpoint;
        if (!endpoint) return;

        try {
            const response = await fetch(endpoint);
            if (response.ok) {
                const data = await response.json();
                this.updateData(id, data);
            }
        } catch (error) {
            console.error(`Failed to refresh chart ${id}:`, error);
        }
    },

    /**
     * Destroy all charts
     */
    destroy() {
        this.instances.forEach((chart, id) => {
            chart.destroy();
        });
        this.instances.clear();
    }
};

export { Charts };
