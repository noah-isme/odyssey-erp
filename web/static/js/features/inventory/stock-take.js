import { api } from '../../core/api.js';
import { Toast } from '../toast/index.js';

/**
 * Inventory Stock Take Feature
 * Handles line additions and posting via AJAX.
 */
export const StockTake = {
    init() {
        document.addEventListener('submit', this.handleSubmit.bind(this));
    },

    async handleSubmit(e) {
        const form = e.target.closest('[data-feature="stock-take-line-form"]');
        if (!form) return;

        e.preventDefault();

        const submitBtn = form.querySelector('button[type="submit"]');
        if (submitBtn.disabled) return;

        const originalBtnText = submitBtn.innerHTML;
        submitBtn.disabled = true;
        submitBtn.innerHTML = '<i class="lucide-loader spin"></i> Menambah...';

        try {
            const formData = new FormData(form);
            const data = {
                product_id: parseInt(formData.get('product_id')),
                physical_qty: parseFloat(formData.get('physical_qty')),
                note: formData.get('note')
            };

            await api.post(form.action, data);
            
            Toast.success('Item berhasil ditambahkan');
            
            // Reload page to show new line (simplest for now, or we could update DOM)
            // Following "Progressive Enhancement", a full reload is a safe fallback.
            window.location.reload();

        } catch (err) {
            console.error('Stock take error:', err);
            Toast.error(err.message || 'Gagal menambahkan item');
        } finally {
            submitBtn.disabled = false;
            submitBtn.innerHTML = originalBtnText;
        }
    }
};
