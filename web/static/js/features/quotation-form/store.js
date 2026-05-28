/**
 * Quotation Form - Store
 */

const instances = new Map();

export function createInitialState(id, existingLines = []) {
    return {
        id,
        lines: existingLines.length > 0 ? existingLines : [{ id: Date.now(), product_id: '', quantity: '1.00', uom: 'PCS', unit_price: '0.00', discount_percent: '0.00', tax_percent: '11.00' }],
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
                lines: [...state.lines, { id: Date.now(), product_id: '', quantity: '1.00', uom: 'PCS', unit_price: '0.00', discount_percent: '0.00', tax_percent: '11.00' }],
                dirty: true
            };

        case 'REMOVE_LINE':
            if (state.lines.length <= 1) return state;
            return {
                ...state,
                lines: state.lines.filter((_, i) => i !== action.payload.index),
                dirty: true
            };

        case 'UPDATE_LINE':
            const newLines = [...state.lines];
            newLines[action.payload.index] = {
                ...newLines[action.payload.index],
                [action.payload.field]: action.payload.value
            };
            return {
                ...state,
                lines: newLines,
                dirty: true
            };

        default:
            return state;
    }
}
