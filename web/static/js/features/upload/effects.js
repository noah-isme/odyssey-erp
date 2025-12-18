/**
 * Upload Effects - Side Effects Layer
 * XHR upload, file validation
 * Following state-driven-ui architecture
 */

const effects = {
    // Active XHR requests
    _requests: new Map(),

    /**
     * Validate files
     * @param {FileList} fileList - Files to validate
     * @param {Object} config - Validation config
     * @returns {Object} { valid: File[], errors: string[] }
     */
    validateFiles(fileList, config) {
        const { maxSize = 10 * 1024 * 1024, accept = '' } = config;
        const acceptTypes = accept.split(',').map(s => s.trim().toLowerCase()).filter(Boolean);

        const valid = [];
        const errors = [];

        Array.from(fileList).forEach(file => {
            // Check size
            if (file.size > maxSize) {
                errors.push(`${file.name}: File too large (max ${this.formatSize(maxSize)})`);
                return;
            }

            // Check type
            if (acceptTypes.length > 0) {
                const ext = '.' + file.name.split('.').pop().toLowerCase();
                const mimeMatch = acceptTypes.some(a => file.type.includes(a.replace('.', '')));
                const extMatch = acceptTypes.includes(ext);

                if (!mimeMatch && !extMatch) {
                    errors.push(`${file.name}: File type not allowed`);
                    return;
                }
            }

            valid.push(file);
        });

        return { valid, errors };
    },

    /**
     * Upload single file via XHR
     * @param {string} id - Upload ID
     * @param {number} index - File index
     * @param {string} endpoint - Upload endpoint
     * @param {File} file - File to upload
     * @param {Function} onProgress - Progress callback
     * @returns {Promise}
     */
    uploadFile(id, index, endpoint, file, onProgress) {
        return new Promise((resolve, reject) => {
            const xhr = new XMLHttpRequest();
            const formData = new FormData();
            formData.append('file', file);

            // Store for cancellation
            const key = `${id}:${index}`;
            this._requests.set(key, xhr);

            xhr.upload.addEventListener('progress', (e) => {
                if (e.lengthComputable) {
                    onProgress(Math.round((e.loaded / e.total) * 100));
                }
            });

            xhr.addEventListener('load', () => {
                this._requests.delete(key);
                if (xhr.status >= 200 && xhr.status < 300) {
                    try {
                        resolve(JSON.parse(xhr.responseText));
                    } catch (e) {
                        resolve({});
                    }
                } else {
                    reject(new Error(xhr.statusText || 'Upload failed'));
                }
            });

            xhr.addEventListener('error', () => {
                this._requests.delete(key);
                reject(new Error('Network error'));
            });

            xhr.addEventListener('abort', () => {
                this._requests.delete(key);
                reject(new Error('Upload cancelled'));
            });

            xhr.open('POST', endpoint);
            xhr.send(formData);
        });
    },

    /**
     * Cancel upload
     * @param {string} id - Upload ID
     * @param {number} index - File index
     */
    cancelUpload(id, index) {
        const key = `${id}:${index}`;
        const xhr = this._requests.get(key);
        if (xhr) {
            xhr.abort();
            this._requests.delete(key);
        }
    },

    /**
     * Cancel all uploads for instance
     * @param {string} id - Upload ID
     */
    cancelAll(id) {
        this._requests.forEach((xhr, key) => {
            if (key.startsWith(`${id}:`)) {
                xhr.abort();
                this._requests.delete(key);
            }
        });
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
     * Show error toast
     * @param {string} message - Error message
     */
    showError(message) {
        if (window.OdysseyToast) {
            window.OdysseyToast.warning(message);
        }
    }
};

export { effects };
