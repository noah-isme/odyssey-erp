/**
 * Delivery Order Form - Store
 */

const instances = new Map();

export function createInitialState(id, existingLines = []) {
    return {
        id,
        lines: existingLines.length > 0 ? existingLines : [{ id: Date.now(), so_line_id: '', product_id: '', quantity: '1.00', notes: '' }],
        dirty: false
    };
}

export function getState(id) {
    return instances.get(id);
}

export function setState(id, newState) {
    instances.set(id, newState);
}

export function reducer(state, action) {
    switch (action.type) {
        case 'ADD_LINE':
            return {
                ...state,
                lines: [...state.lines, { id: Date.now(), so_line_id: '', product_id: '', quantity: '1.00', notes: '' }],
                dirty: true
            };
        case 'REMOVE_LINE':
            if (state.lines.length <= 1) return state;
            return {
                ...state,
                lines: state.lines.filter((_, i) => i !== action.payload.index),
                dirty: true
            };
        default:
            return state;
    }
}
