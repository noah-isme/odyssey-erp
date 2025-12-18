/**
 * Upload View - Render Layer
 * DOM rendering for upload zone
 * Following state-driven-ui architecture
 */

const view = {
    _cache: new Map(),

    /**
     * Get upload zone container
     * @param {string} id - Upload ID
     * @returns {HTMLElement|null}
     */
    getContainer(id) {
        if (this._cache.has(id)) {
            return this._cache.get(id);
        }
        const container = document.querySelector(`[data-upload="${id}"]`);
        if (container) {
            this._cache.set(id, container);
        }
        return container;
    },

    /**
     * Render file list
     * @param {string} id - Upload ID
     * @param {Array} files - Files array from state
     */
    renderFileList(id, files) {
        const container = this.getContainer(id);
        if (!container) return;

        const list = container.querySelector('[data-upload-list]');
        if (!list) return;

        if (files.length === 0) {
            list.innerHTML = '';
            return;
        }

        list.innerHTML = files.map((item, index) => `
            <div class="upload-item${item.status === 'uploading' ? ' uploading' : ''}${item.status === 'done' ? ' done' : ''}${item.status === 'error' ? ' error' : ''}" 
                 data-file-index="${index}" data-state="${item.status}">
                <div class="upload-item-icon">
                    ${this.getFileIcon(item.name)}
                </div>
                <div class="upload-item-info">
                    <div class="upload-item-name">${this.escapeHtml(item.name)}</div>
                    <div class="upload-item-meta">
                        ${this.formatSize(item.size)}
                        ${item.error ? ` - <span class="upload-error">${item.error}</span>` : ''}
                    </div>
                    ${item.status === 'uploading' && item.progress < 100 ? `
                        <div class="upload-item-progress">
                            <div class="upload-item-progress-bar" style="width: ${item.progress}%"></div>
                        </div>
                    ` : ''}
                </div>
                <button class="upload-item-remove" data-upload-remove="${index}" aria-label="Remove">
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
                    </svg>
                </button>
            </div>
        `).join('');
    },

    /**
     * Render drag-over state
     * @param {string} id - Upload ID
     * @param {boolean} isDragOver - Is dragging over
     */
    renderDragOver(id, isDragOver) {
        const container = this.getContainer(id);
        if (container) {
            container.classList.toggle('drag-over', isDragOver);
        }
    },

    /**
     * Render uploading state
     * @param {string} id - Upload ID
     * @param {boolean} isUploading - Is uploading
     */
    renderUploading(id, isUploading) {
        const container = this.getContainer(id);
        if (container) {
            container.setAttribute('data-state', isUploading ? 'uploading' : 'idle');
        }
    },

    /**
     * Get file icon based on extension
     * @param {string} filename - File name
     * @returns {string} SVG icon
     */
    getFileIcon(filename) {
        const ext = filename.split('.').pop().toLowerCase();
        const icons = {
            pdf: '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="var(--error-600)" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><text x="8" y="18" font-size="6" fill="var(--error-600)">PDF</text></svg>',
            jpg: '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="var(--brand)" stroke-width="2"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"/><circle cx="8.5" cy="8.5" r="1.5"/><polyline points="21 15 16 10 5 21"/></svg>',
            jpeg: '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="var(--brand)" stroke-width="2"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"/><circle cx="8.5" cy="8.5" r="1.5"/><polyline points="21 15 16 10 5 21"/></svg>',
            png: '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="var(--brand)" stroke-width="2"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"/><circle cx="8.5" cy="8.5" r="1.5"/><polyline points="21 15 16 10 5 21"/></svg>',
            default: '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/><polyline points="13 2 13 9 20 9"/></svg>'
        };
        return icons[ext] || icons.default;
    },

    /**
     * Format file size
     * @param {number} bytes - Size in bytes
     * @returns {string}
     */
    formatSize(bytes) {
        if (bytes < 1024) return bytes + ' B';
        if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
        return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
    },

    /**
     * Escape HTML
     * @param {string} str - String to escape
     * @returns {string}
     */
    escapeHtml(str) {
        const div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    },

    /**
     * Clear cache
     * @param {string} id - Upload ID
     */
    clearCache(id) {
        this._cache.delete(id);
    },

    /**
     * Clear all caches
     */
    clearAllCaches() {
        this._cache.clear();
    }
};

export { view };
