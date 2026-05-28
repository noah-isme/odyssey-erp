/**
 * Sales Order Detail - State Driven Modals
 */

let state = {
    modals: {
        cancel: { isOpen: false }
    }
};

export function init() {
    document.addEventListener('click', handleClick);
    render();
}

function dispatch(action) {
    switch (action.type) {
        case 'OPEN_MODAL':
            state.modals[action.payload.name].isOpen = true;
            break;
        case 'CLOSE_MODAL':
            state.modals[action.payload.name].isOpen = false;
            break;
    }
    render();
}

function render() {
    Object.keys(state.modals).forEach(name => {
        const modal = document.getElementById(`${name}Modal`);
        if (!modal) return;

        const isOpen = state.modals[name].isOpen;
        modal.setAttribute('data-state', isOpen ? 'open' : 'closed');
        
        if (isOpen && !modal.open) {
            modal.showModal();
        } else if (!isOpen && modal.open) {
            modal.close();
        }
    });
}

function handleClick(e) {
    const actionBtn = e.target.closest('[data-action]');
    if (!actionBtn) return;

    const action = actionBtn.dataset.action;

    if (action === 'show-cancel-modal') {
        dispatch({ type: 'OPEN_MODAL', payload: { name: 'cancel' } });
    } else if (action === 'close-modal') {
        const modal = actionBtn.closest('dialog');
        if (modal) {
            const name = modal.id.replace('Modal', '');
            dispatch({ type: 'CLOSE_MODAL', payload: { name } });
        }
    }
}
