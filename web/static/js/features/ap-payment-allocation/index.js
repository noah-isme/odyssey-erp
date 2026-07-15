let controller;

function init() {
    controller?.abort();
    controller = new AbortController();

    document.addEventListener('click', (event) => {
        const form = event.target.closest('[data-ap-payment-form]');
        if (!form) return;

        if (event.target.closest('[data-allocation-add]')) {
            addRow(form);
            return;
        }

        const removeButton = event.target.closest('[data-allocation-remove]');
        if (removeButton) removeRow(form, removeButton);
    }, { signal: controller.signal });
}

function addRow(form) {
    const container = form.querySelector('#allocation-rows');
    const source = container?.querySelector('.allocation-row');
    if (!container || !source) return;

    const clone = source.cloneNode(true);
    clone.querySelectorAll('select, input').forEach((control) => {
        control.value = '';
    });
    container.appendChild(clone);
}

function removeRow(form, button) {
    const rows = form.querySelectorAll('.allocation-row');
    if (rows.length <= 1) return;
    button.closest('.allocation-row')?.remove();
}

function destroy() {
    controller?.abort();
    controller = undefined;
}

export const APPaymentAllocation = { init, destroy };
