/**
 * File Upload Feature - Drag & Drop Upload
 * Following state-driven-ui architecture
 * 
 * Usage:
 * <div class="upload-zone" data-upload="invoice-docs" 
 *      data-endpoint="/api/uploads"
 *      data-accept=".pdf,.jpg,.png"
 *      data-max-size="5242880">
 *   <div class="upload-zone-content">
 *     <svg>...</svg>
 *     <p>Drag files here or <button data-upload-browse>browse</button></p>
 *   </div>
 *   <input type="file" data-upload-input hidden multiple>
 *   <div class="upload-list" data-upload-list></div>
 * </div>
 */

const Upload = {
    instances: new Map(),

    /**
     * Initialize upload zones
     */
    init() {
        document.querySelectorAll('[data-upload]').forEach(zone => {
            const id = zone.dataset.upload;
            if (this.instances.has(id)) return;

            this.instances.set(id, {
                zone,
                files: [],
                uploading: false
            });

            this.setupDragDrop(zone);
            this.setupInput(zone);
        });

        // Event delegation for browse button
        document.addEventListener('click', (e) => {
            const browseBtn = e.target.closest('[data-upload-browse]');
            if (browseBtn) {
                const zone = browseBtn.closest('[data-upload]');
                const input = zone?.querySelector('[data-upload-input]');
                if (input) input.click();
            }
        });
    },

    /**
     * Setup drag & drop events
     * @param {HTMLElement} zone - Upload zone element
     */
    setupDragDrop(zone) {
        ['dragenter', 'dragover', 'dragleave', 'drop'].forEach(eventName => {
            zone.addEventListener(eventName, (e) => {
                e.preventDefault();
                e.stopPropagation();
            });
        });

        ['dragenter', 'dragover'].forEach(eventName => {
            zone.addEventListener(eventName, () => {
                zone.classList.add('drag-over');
            });
        });

        ['dragleave', 'drop'].forEach(eventName => {
            zone.addEventListener(eventName, () => {
                zone.classList.remove('drag-over');
            });
        });

        zone.addEventListener('drop', (e) => {
            const files = e.dataTransfer.files;
            if (files.length > 0) {
                this.handleFiles(zone.dataset.upload, files);
            }
        });
    },

    /**
     * Setup file input
     * @param {HTMLElement} zone - Upload zone element
     */
    setupInput(zone) {
        const input = zone.querySelector('[data-upload-input]');
        if (!input) return;

        // Set accept attribute
        if (zone.dataset.accept) {
            input.accept = zone.dataset.accept;
        }

        input.addEventListener('change', () => {
            if (input.files.length > 0) {
                this.handleFiles(zone.dataset.upload, input.files);
                input.value = ''; // Reset for same file selection
            }
        });
    },

    /**
     * Handle selected files
     * @param {string} id - Upload zone ID
     * @param {FileList} fileList - Selected files
     */
    handleFiles(id, fileList) {
        const instance = this.instances.get(id);
        if (!instance) return;

        const zone = instance.zone;
        const maxSize = parseInt(zone.dataset.maxSize) || 10 * 1024 * 1024; // 10MB default
        const accept = (zone.dataset.accept || '').split(',').map(s => s.trim().toLowerCase());

        const validFiles = [];
        const errors = [];

        Array.from(fileList).forEach(file => {
            // Check size
            if (file.size > maxSize) {
                errors.push(`${file.name}: File too large (max ${this.formatSize(maxSize)})`);
                return;
            }

            // Check type
            if (accept.length > 0 && accept[0] !== '') {
                const ext = '.' + file.name.split('.').pop().toLowerCase();
                const mimeMatch = accept.some(a => file.type.includes(a.replace('.', '')));
                const extMatch = accept.includes(ext);

                if (!mimeMatch && !extMatch) {
                    errors.push(`${file.name}: File type not allowed`);
                    return;
                }
            }

            validFiles.push(file);
        });

        // Show errors
        errors.forEach(err => window.OdysseyToast?.warning(err));

        // Add valid files to queue
        if (validFiles.length > 0) {
            instance.files.push(...validFiles);
            this.renderFileList(id);

            // Auto-upload if endpoint provided
            if (zone.dataset.endpoint && zone.dataset.autoUpload !== 'false') {
                this.uploadAll(id);
            }
        }
    },

    /**
     * Render file list
     * @param {string} id - Upload zone ID
     */
    renderFileList(id) {
        const instance = this.instances.get(id);
        if (!instance) return;

        const list = instance.zone.querySelector('[data-upload-list]');
        if (!list) return;

        list.innerHTML = instance.files.map((file, index) => `
            <div class="upload-item${file.status === 'uploading' ? ' uploading' : ''}${file.status === 'done' ? ' done' : ''}${file.status === 'error' ? ' error' : ''}" data-file-index="${index}">
                <div class="upload-item-icon">
                    ${this.getFileIcon(file.name)}
                </div>
                <div class="upload-item-info">
                    <div class="upload-item-name">${this.escapeHtml(file.name)}</div>
                    <div class="upload-item-meta">${this.formatSize(file.size)}${file.error ? ` - ${file.error}` : ''}</div>
                    ${file.progress !== undefined && file.progress < 100 ? `
                        <div class="upload-item-progress">
                            <div class="upload-item-progress-bar" style="width: ${file.progress}%"></div>
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

        // Remove button handlers
        list.querySelectorAll('[data-upload-remove]').forEach(btn => {
            btn.addEventListener('click', () => {
                const index = parseInt(btn.dataset.uploadRemove);
                instance.files.splice(index, 1);
                this.renderFileList(id);
            });
        });
    },

    /**
     * Upload all files
     * @param {string} id - Upload zone ID
     */
    async uploadAll(id) {
        const instance = this.instances.get(id);
        if (!instance || instance.uploading) return;

        const endpoint = instance.zone.dataset.endpoint;
        if (!endpoint) return;

        instance.uploading = true;

        for (let i = 0; i < instance.files.length; i++) {
            const file = instance.files[i];
            if (file.status === 'done') continue;

            file.status = 'uploading';
            file.progress = 0;
            this.renderFileList(id);

            try {
                await this.uploadFile(endpoint, file, (progress) => {
                    file.progress = progress;
                    this.renderFileList(id);
                });

                file.status = 'done';
                file.progress = 100;
            } catch (error) {
                file.status = 'error';
                file.error = error.message;
            }

            this.renderFileList(id);
        }

        instance.uploading = false;

        // Emit event
        instance.zone.dispatchEvent(new CustomEvent('upload-complete', {
            detail: { files: instance.files },
            bubbles: true
        }));
    },

    /**
     * Upload single file
     * @param {string} endpoint - Upload endpoint
     * @param {File} file - File to upload
     * @param {Function} onProgress - Progress callback
     * @returns {Promise} Upload promise
     */
    uploadFile(endpoint, file, onProgress) {
        return new Promise((resolve, reject) => {
            const xhr = new XMLHttpRequest();
            const formData = new FormData();
            formData.append('file', file);

            xhr.upload.addEventListener('progress', (e) => {
                if (e.lengthComputable) {
                    onProgress(Math.round((e.loaded / e.total) * 100));
                }
            });

            xhr.addEventListener('load', () => {
                if (xhr.status >= 200 && xhr.status < 300) {
                    resolve(JSON.parse(xhr.responseText));
                } else {
                    reject(new Error(xhr.statusText || 'Upload failed'));
                }
            });

            xhr.addEventListener('error', () => reject(new Error('Network error')));

            xhr.open('POST', endpoint);
            xhr.send(formData);
        });
    },

    /**
     * Format file size
     * @param {number} bytes - Size in bytes
     * @returns {string} Formatted size
     */
    formatSize(bytes) {
        if (bytes < 1024) return bytes + ' B';
        if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
        return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
    },

    /**
     * Get file icon based on extension
     * @param {string} filename - File name
     * @returns {string} SVG icon
     */
    getFileIcon(filename) {
        const ext = filename.split('.').pop().toLowerCase();
        const icons = {
            pdf: '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>',
            default: '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/><polyline points="13 2 13 9 20 9"/></svg>'
        };
        return icons[ext] || icons.default;
    },

    /**
     * Escape HTML
     * @param {string} str - String to escape
     * @returns {string} Escaped string
     */
    escapeHtml(str) {
        const div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    },

    /**
     * Get files from an upload zone
     * @param {string} id - Upload zone ID
     * @returns {Array} Files array
     */
    getFiles(id) {
        return this.instances.get(id)?.files || [];
    },

    /**
     * Clear files from an upload zone
     * @param {string} id - Upload zone ID
     */
    clear(id) {
        const instance = this.instances.get(id);
        if (instance) {
            instance.files = [];
            this.renderFileList(id);
        }
    }
};

export { Upload };
