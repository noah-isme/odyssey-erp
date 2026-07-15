/**
 * Native dialog lifecycle manager.
 *
 * Contract:
 * - dialog: <dialog data-dialog id="example" class="native-dialog">
 * - open:   <button data-dialog-open="example">
 * - close:  <button data-dialog-close>
 *
 * The browser owns the top layer, backdrop, focus trap, and Escape behavior.
 * This module provides delegated controls, outside-click closing, focus
 * restoration, and deterministic listener cleanup.
 */

let controller = null;
const returnFocus = new Map();

function getDialog(target) {
    if (target instanceof HTMLDialogElement) return target;
    if (typeof target !== 'string') return null;
    const dialog = document.getElementById(target);
    return dialog instanceof HTMLDialogElement && dialog.matches('[data-dialog]') ? dialog : null;
}

function open(target, trigger = document.activeElement) {
    const dialog = getDialog(target);
    if (!dialog || dialog.open) return false;

    closeAll(false);
    if (trigger instanceof HTMLElement) returnFocus.set(dialog, trigger);
    dialog.dataset.state = 'open';
    dialog.showModal();
    return true;
}

function close(target, returnValue = '') {
    const dialog = getDialog(target);
    if (!dialog || !dialog.open) return false;

    dialog.close(returnValue);
    return true;
}

function closeAll(restoreFocus = true) {
    document.querySelectorAll('dialog[data-dialog][open]').forEach(dialog => {
        if (!restoreFocus) returnFocus.delete(dialog);
        dialog.close();
    });
}

function handleDocumentClick(event) {
    const openButton = event.target.closest('[data-dialog-open]');
    if (openButton) {
        event.preventDefault();
        open(openButton.dataset.dialogOpen, openButton);
        return;
    }

    const closeButton = event.target.closest('[data-dialog-close]');
    if (closeButton) {
        event.preventDefault();
        close(closeButton.closest('dialog[data-dialog]'));
    }
}

function handleBackdropClick(event) {
    const dialog = event.currentTarget;
    if (event.target !== dialog) return;

    const bounds = dialog.getBoundingClientRect();
    const inside = event.clientX >= bounds.left && event.clientX <= bounds.right
        && event.clientY >= bounds.top && event.clientY <= bounds.bottom;
    if (!inside) close(dialog, 'backdrop');
}

function handleClose(event) {
    const dialog = event.currentTarget;
    dialog.dataset.state = 'closed';

    const target = returnFocus.get(dialog);
    returnFocus.delete(dialog);
    if (target?.isConnected) target.focus({ preventScroll: true });
}

function init() {
    destroy();
    controller = new AbortController();
    const options = { signal: controller.signal };

    document.addEventListener('click', handleDocumentClick, options);
    document.querySelectorAll('dialog[data-dialog]').forEach(dialog => {
        dialog.dataset.state = dialog.open ? 'open' : 'closed';
        dialog.addEventListener('click', handleBackdropClick, options);
        dialog.addEventListener('close', handleClose, options);
    });
}

function destroy() {
    closeAll(false);
    controller?.abort();
    controller = null;
    returnFocus.clear();
}

const Modal = {
    init,
    destroy,
    open,
    close,
    closeAll,
    isOpen: target => Boolean(getDialog(target)?.open),
    getOpenModals: () => Array.from(document.querySelectorAll('dialog[data-dialog][open]'))
};

export { Modal };
