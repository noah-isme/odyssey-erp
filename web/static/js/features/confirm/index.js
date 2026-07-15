let controller;

function init() {
    controller?.abort();
    controller = new AbortController();

    document.addEventListener('submit', (event) => {
        const form = event.target.closest('form[data-confirm]');
        if (!form || window.confirm(form.dataset.confirm)) return;
        event.preventDefault();
    }, { signal: controller.signal });
}

function destroy() {
    controller?.abort();
    controller = undefined;
}

export const Confirm = { init, destroy };
