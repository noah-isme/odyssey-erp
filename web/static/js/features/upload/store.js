/**
 * Upload Store - State + Reducer + Selectors
 * Following state-driven-ui architecture
 */

// ========== STATE ==========
const instances = new Map();

function createInitialState() {
    return {
        files: [],          // Array of { file, status, progress, error }
        uploading: false,
        error: null
    };
}

// ========== REDUCER (pure function) ==========
function reducer(state, action) {
    switch (action.type) {
        case 'UPLOAD_ADD_FILES': {
            return {
                ...state,
                files: [
                    ...state.files,
                    ...action.payload.map(file => ({
                        file,
                        name: file.name,
                        size: file.size,
                        status: 'pending',
                        progress: 0,
                        error: null
                    }))
                ]
            };
        }

        case 'UPLOAD_REMOVE_FILE': {
            const index = action.payload;
            return {
                ...state,
                files: state.files.filter((_, i) => i !== index)
            };
        }

        case 'UPLOAD_CLEAR': {
            return {
                ...state,
                files: [],
                error: null
            };
        }

        case 'UPLOAD_START': {
            // Idempotency guard
            if (state.uploading) return state;
            return {
                ...state,
                uploading: true
            };
        }

        case 'UPLOAD_END': {
            return {
                ...state,
                uploading: false
            };
        }

        case 'UPLOAD_FILE_PROGRESS': {
            const { index, progress } = action.payload;
            const files = [...state.files];
            if (files[index]) {
                files[index] = { ...files[index], progress, status: 'uploading' };
            }
            return { ...state, files };
        }

        case 'UPLOAD_FILE_DONE': {
            const { index, result } = action.payload;
            const files = [...state.files];
            if (files[index]) {
                files[index] = { ...files[index], status: 'done', progress: 100, result };
            }
            return { ...state, files };
        }

        case 'UPLOAD_FILE_ERROR': {
            const { index, error } = action.payload;
            const files = [...state.files];
            if (files[index]) {
                files[index] = { ...files[index], status: 'error', error };
            }
            return { ...state, files };
        }

        case 'UPLOAD_SET_ERROR': {
            return { ...state, error: action.payload };
        }

        default:
            return state;
    }
}

// ========== SELECTORS ==========
const selectors = {
    getState: (id) => instances.get(id) || createInitialState(),
    getFiles: (id) => (instances.get(id) || {}).files || [],
    isUploading: (id) => (instances.get(id) || {}).uploading || false,
    getError: (id) => (instances.get(id) || {}).error,
    hasPendingFiles: (id) => {
        const files = (instances.get(id) || {}).files || [];
        return files.some(f => f.status === 'pending');
    },
    getProgress: (id) => {
        const files = (instances.get(id) || {}).files || [];
        if (files.length === 0) return 0;
        const total = files.reduce((acc, f) => acc + f.progress, 0);
        return Math.round(total / files.length);
    }
};

// ========== STORE API ==========
function getState(id) {
    if (!instances.has(id)) {
        instances.set(id, createInitialState());
    }
    return instances.get(id);
}

function setState(id, newState) {
    instances.set(id, newState);
}

function deleteState(id) {
    instances.delete(id);
}

export { reducer, selectors, getState, setState, deleteState, createInitialState };
