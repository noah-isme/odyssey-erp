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

import { reducer, selectors, getState, setState, deleteState, createInitialState } from './store.js';
import { effects } from './effects.js';
import { view } from './view.js';

// Track mounted upload zones
const mounted = new Map();

// ========== DISPATCH ==========
function dispatch(id, action) {
    const prevState = getState(id);
    const nextState = reducer(prevState, action);

    if (JSON.stringify(nextState) !== JSON.stringify(prevState)) {
        setState(id, nextState);

        // View updates
        switch (action.type) {
            case 'UPLOAD_ADD_FILES':
            case 'UPLOAD_REMOVE_FILE':
            case 'UPLOAD_FILE_PROGRESS':
            case 'UPLOAD_FILE_DONE':
            case 'UPLOAD_FILE_ERROR':
            case 'UPLOAD_CLEAR':
                view.renderFileList(id, nextState.files);
                break;

            case 'UPLOAD_START':
            case 'UPLOAD_END':
                view.renderUploading(id, nextState.uploading);
                break;
        }
    }
}

// ========== UPLOAD LOGIC ==========
async function uploadAll(id) {
    const config = mounted.get(id);
    if (!config) return;

    const state = getState(id);
    if (state.uploading) return;

    const endpoint = config.endpoint;
    if (!endpoint) return;

    dispatch(id, { type: 'UPLOAD_START' });

    const files = state.files;
    for (let i = 0; i < files.length; i++) {
        const item = files[i];
        if (item.status === 'done') continue;

        try {
            const result = await effects.uploadFile(
                id, i, endpoint, item.file,
                (progress) => {
                    dispatch(id, {
                        type: 'UPLOAD_FILE_PROGRESS',
                        payload: { index: i, progress }
                    });
                }
            );
            dispatch(id, {
                type: 'UPLOAD_FILE_DONE',
                payload: { index: i, result }
            });
        } catch (error) {
            dispatch(id, {
                type: 'UPLOAD_FILE_ERROR',
                payload: { index: i, error: error.message }
            });
        }
    }

    dispatch(id, { type: 'UPLOAD_END' });

    // Emit event
    const container = view.getContainer(id);
    if (container) {
        container.dispatchEvent(new CustomEvent('upload-complete', {
            detail: { files: getState(id).files },
            bubbles: true
        }));
    }
}

// ========== EVENT HANDLERS ==========
function handleClick(e) {
    // Browse button
    const browseBtn = e.target.closest('[data-upload-browse]');
    if (browseBtn) {
        const container = browseBtn.closest('[data-upload]');
        const input = container?.querySelector('[data-upload-input]');
        if (input) input.click();
        return;
    }

    // Remove button
    const removeBtn = e.target.closest('[data-upload-remove]');
    if (removeBtn) {
        e.preventDefault();
        const container = removeBtn.closest('[data-upload]');
        if (container) {
            const id = container.dataset.upload;
            const index = parseInt(removeBtn.dataset.uploadRemove);
            effects.cancelUpload(id, index);
            dispatch(id, { type: 'UPLOAD_REMOVE_FILE', payload: index });
        }
    }
}

function handleDragEnter(e) {
    e.preventDefault();
    const container = e.target.closest('[data-upload]');
    if (container) {
        view.renderDragOver(container.dataset.upload, true);
    }
}

function handleDragLeave(e) {
    e.preventDefault();
    const container = e.target.closest('[data-upload]');
    if (container && !container.contains(e.relatedTarget)) {
        view.renderDragOver(container.dataset.upload, false);
    }
}

function handleDragOver(e) {
    e.preventDefault();
}

function handleDrop(e) {
    e.preventDefault();
    const container = e.target.closest('[data-upload]');
    if (!container) return;

    const id = container.dataset.upload;
    view.renderDragOver(id, false);

    const files = e.dataTransfer.files;
    if (files.length > 0) {
        handleFiles(id, files);
    }
}

function handleInputChange(e) {
    const input = e.target;
    if (!input.matches('[data-upload-input]')) return;

    const container = input.closest('[data-upload]');
    if (!container) return;

    if (input.files.length > 0) {
        handleFiles(container.dataset.upload, input.files);
        input.value = ''; // Reset for same file selection
    }
}

function handleFiles(id, fileList) {
    const config = mounted.get(id);
    if (!config) return;

    const { valid, errors } = effects.validateFiles(fileList, {
        maxSize: config.maxSize,
        accept: config.accept
    });

    // Show errors
    errors.forEach(err => effects.showError(err));

    // Add valid files to state
    if (valid.length > 0) {
        dispatch(id, { type: 'UPLOAD_ADD_FILES', payload: valid });

        // Auto-upload if enabled
        if (config.endpoint && config.autoUpload !== false) {
            uploadAll(id);
        }
    }
}

// ========== INIT ==========
function init() {
    document.querySelectorAll('[data-upload]').forEach(container => {
        const id = container.dataset.upload;
        if (mounted.has(id)) return;

        const config = {
            endpoint: container.dataset.endpoint,
            accept: container.dataset.accept || '',
            maxSize: parseInt(container.dataset.maxSize) || 10 * 1024 * 1024,
            autoUpload: container.dataset.autoUpload !== 'false'
        };

        mounted.set(id, config);

        // Set accept on input
        const input = container.querySelector('[data-upload-input]');
        if (input && config.accept) {
            input.accept = config.accept;
        }
    });

    // Event delegation
    document.addEventListener('click', handleClick);
    document.addEventListener('dragenter', handleDragEnter);
    document.addEventListener('dragleave', handleDragLeave);
    document.addEventListener('dragover', handleDragOver);
    document.addEventListener('drop', handleDrop);
    document.addEventListener('change', handleInputChange);
}

// ========== DESTROY ==========
function destroy() {
    document.removeEventListener('click', handleClick);
    document.removeEventListener('dragenter', handleDragEnter);
    document.removeEventListener('dragleave', handleDragLeave);
    document.removeEventListener('dragover', handleDragOver);
    document.removeEventListener('drop', handleDrop);
    document.removeEventListener('change', handleInputChange);

    mounted.forEach((_, id) => {
        effects.cancelAll(id);
        deleteState(id);
        view.clearCache(id);
    });

    mounted.clear();
}

// ========== PUBLIC API ==========
const Upload = {
    init,
    destroy,

    /**
     * Upload all pending files
     * @param {string} id - Upload zone ID
     */
    upload(id) {
        uploadAll(id);
    },

    /**
     * Get files from upload zone
     * @param {string} id - Upload zone ID
     * @returns {Array}
     */
    getFiles(id) {
        return selectors.getFiles(id);
    },

    /**
     * Clear files from upload zone
     * @param {string} id - Upload zone ID
     */
    clear(id) {
        effects.cancelAll(id);
        dispatch(id, { type: 'UPLOAD_CLEAR' });
    },

    /**
     * Check if uploading
     * @param {string} id - Upload zone ID
     * @returns {boolean}
     */
    isUploading(id) {
        return selectors.isUploading(id);
    },

    selectors
};

export { Upload };
