/**
 * Sales Order Form - View
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
            <div class="grid grid-cols-3 gap-4">
                <div class="field">
                    <label for="product_id_${index}" class="label">Product <span class="text-error">*</span></label>
                    <input type="number" name="product_id" id="product_id_${index}" class="input" required min="1" value="${line.product_id}">
                </div>
                <div class="field">
                    <label for="quantity_${index}" class="label">Quantity <span class="text-error">*</span></label>
                    <input type="number" name="quantity" id="quantity_${index}" class="input" required min="0.01" step="0.01" value="${line.quantity}">
                </div>
                <div class="field">
                    <label for="uom_${index}" class="label">UOM <span class="text-error">*</span></label>
                    <input type="text" name="uom" id="uom_${index}" class="input" required value="${line.uom}">
                </div>
            </div>
            <div class="grid grid-cols-4 gap-4 mt-4">
                <div class="field">
                    <label for="unit_price_${index}" class="label">Unit Price <span class="text-error">*</span></label>
                    <input type="number" name="unit_price" id="unit_price_${index}" class="input" required min="0" step="0.01" value="${line.unit_price}">
                </div>
                <div class="field">
                    <label for="discount_percent_${index}" class="label">Discount %</label>
                    <input type="number" name="discount_percent" id="discount_percent_${index}" class="input" min="0" max="100" step="0.01" value="${line.discount_percent}">
                </div>
                <div class="field">
                    <label for="tax_percent_${index}" class="label">Tax %</label>
                    <input type="number" name="tax_percent" id="tax_percent_${index}" class="input" min="0" max="100" step="0.01" value="${line.tax_percent}">
                </div>
                <div class="flex items-end">
                    <button type="button" class="btn btn--ghost btn--sm text-error" data-action="remove-line">
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <polyline points="3 6 5 6 21 6" />
                            <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                        </svg>
                        Remove
                    </button>
                </div>
            </div>
        </div>
        `;
    }
};
