/**
 * Delivery Order Form - View
 */

export const view = {
    _cache: new Map(),

    getContainer(id) {
        if (!this._cache.has(id)) {
            this._cache.set(id, document.getElementById(id));
        }
        return this._cache.get(id);
    },

    render(id, state) {
        const container = this.getContainer(id);
        if (!container) return;

        const html = state.lines.map((line, index) => this.createLineItemHTML(line, index)).join('');
        container.innerHTML = html;
    },

    createLineItemHTML(line, index) {
        return `
        <div class="line-item p-4 bg-surface-muted border rounded-md mb-4" data-index="${index}">
            <div class="grid grid-cols-4 gap-4">
                <div class="field">
                    <label class="label">SO Line ID <span class="text-error">*</span></label>
                    <input type="number" name="so_line_id[]" class="input" required value="${line.so_line_id}" placeholder="SO Line ID">
                </div>
                <div class="field">
                    <label class="label">Product ID <span class="text-error">*</span></label>
                    <input type="number" name="product_id[]" class="input" required value="${line.product_id}" placeholder="Product ID">
                </div>
                <div class="field">
                    <label class="label">Quantity <span class="text-error">*</span></label>
                    <input type="number" name="quantity[]" class="input" step="0.01" min="0.01" required value="${line.quantity}" placeholder="Qty">
                </div>
                <div class="field">
                    <label class="label">Notes</label>
                    <div class="flex gap-2">
                        <input type="text" name="line_notes[]" class="input flex-1" value="${line.notes}" placeholder="Line notes">
                        <button type="button" class="btn btn--ghost btn--sm text-error" data-action="remove-line" aria-label="Remove item">
                            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                <polyline points="3 6 5 6 21 6" />
                                <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                            </svg>
                        </button>
                    </div>
                </div>
            </div>
        </div>
        `;
    }
};
