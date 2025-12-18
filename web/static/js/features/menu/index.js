/**
 * Menu Feature - Mount + Event Delegation
 * Dropdown/Menu component with full keyboard support
 * Following state-driven-ui architecture
 * 
 * Lifecycle: mount → open → close → destroy
 * 
 * Usage:
 * <button data-menu-trigger aria-controls="menu-user" aria-expanded="false">
 *   User Menu
 * </button>
 * <div id="menu-user" data-menu data-state="closed" role="menu" hidden>
 *   <button role="menuitem">Profile</button>
 *   <button role="menuitem">Settings</button>
 *   <button role="menuitem">Logout</button>
 * </div>
 */

import { reducer, selectors, getState, setState, getAllOpenMenus } from './store.js';
import { effects } from './effects.js';
import { view } from './view.js';

// Track mounted menus
const mounted = new Set();

// ========== DISPATCH ==========
function dispatch(id, action) {
    const menu = document.getElementById(id);
    if (!menu) return;

    const trigger = effects.getTrigger(id);
    const prevState = getState(id);
    const nextState = reducer(prevState, action);

    if (JSON.stringify(nextState) !== JSON.stringify(prevState)) {
        setState(id, nextState);

        // View (render) - update DOM
        view.render(id, nextState, menu, trigger);

        // Effects based on action type
        if (action.type === 'MENU_OPEN') {
            // Focus first item on open
            requestAnimationFrame(() => {
                effects.focusFirstItem(menu);
            });
        }

        if (action.type === 'MENU_CLOSE') {
            // Restore focus to trigger on close
            if (trigger) {
                effects.restoreFocus(trigger.id || id);
            }
        }

        if (action.type === 'MENU_HIGHLIGHT_NEXT' || action.type === 'MENU_HIGHLIGHT_PREV') {
            effects.focusItemAtIndex(menu, nextState.highlightIndex);
        }
    }
}

// ========== EVENT HANDLERS ==========
function handleClick(e) {
    const trigger = e.target.closest('[data-menu-trigger]');

    // Close all menus if clicking outside
    const openMenus = getAllOpenMenus();
    openMenus.forEach(menuId => {
        const menu = document.getElementById(menuId);
        const menuTrigger = effects.getTrigger(menuId);
        const clickedInside = menu?.contains(e.target) || menuTrigger?.contains(e.target);

        if (!clickedInside) {
            dispatch(menuId, { type: 'MENU_CLOSE' });
        }
    });

    // Toggle menu on trigger click
    if (trigger) {
        e.preventDefault();
        const menuId = trigger.getAttribute('aria-controls');
        if (!menuId) return;

        // Close other menus first
        openMenus.forEach(id => {
            if (id !== menuId) {
                dispatch(id, { type: 'MENU_CLOSE' });
            }
        });

        dispatch(menuId, { type: 'MENU_TOGGLE', payload: { triggerId: trigger.id || menuId } });
        return;
    }

    // Handle menu item click
    const menuItem = e.target.closest('[role="menuitem"], [data-menu-item]');
    if (menuItem) {
        const menu = menuItem.closest('[data-menu]');
        if (menu) {
            dispatch(menu.id, { type: 'MENU_CLOSE' });
        }
    }
}

function handleKeydown(e) {
    // Check if we're in an open menu
    const openMenus = getAllOpenMenus();
    if (openMenus.length === 0) return;

    const currentMenu = e.target.closest('[data-menu]');
    const menuId = currentMenu?.id;

    // If focus is on trigger, handle trigger keyboard
    const trigger = e.target.closest('[data-menu-trigger]');
    if (trigger) {
        const triggeredMenuId = trigger.getAttribute('aria-controls');
        if (['ArrowDown', 'ArrowUp', 'Enter', ' '].includes(e.key)) {
            e.preventDefault();
            dispatch(triggeredMenuId, { type: 'MENU_OPEN', payload: { triggerId: trigger.id } });
            return;
        }
    }

    if (!menuId || !openMenus.includes(menuId)) return;

    switch (e.key) {
        case 'Escape':
            e.preventDefault();
            dispatch(menuId, { type: 'MENU_CLOSE' });
            break;

        case 'ArrowDown':
            e.preventDefault();
            dispatch(menuId, { type: 'MENU_HIGHLIGHT_NEXT' });
            break;

        case 'ArrowUp':
            e.preventDefault();
            dispatch(menuId, { type: 'MENU_HIGHLIGHT_PREV' });
            break;

        case 'Home':
            e.preventDefault();
            dispatch(menuId, { type: 'MENU_HIGHLIGHT_FIRST' });
            effects.focusItemAtIndex(document.getElementById(menuId), 0);
            break;

        case 'End':
            e.preventDefault();
            dispatch(menuId, { type: 'MENU_HIGHLIGHT_LAST' });
            const state = getState(menuId);
            effects.focusItemAtIndex(document.getElementById(menuId), state.items.length - 1);
            break;

        case 'Enter':
        case ' ':
            // Let the click happen naturally on the focused item
            const activeItem = document.activeElement;
            if (activeItem?.closest('[data-menu]')?.id === menuId) {
                dispatch(menuId, { type: 'MENU_CLOSE' });
            }
            break;

        case 'Tab':
            // Close menu on Tab
            dispatch(menuId, { type: 'MENU_CLOSE' });
            break;
    }
}

// Handle Escape globally for all menus
function handleGlobalKeydown(e) {
    if (e.key === 'Escape') {
        const openMenus = getAllOpenMenus();
        openMenus.forEach(menuId => {
            dispatch(menuId, { type: 'MENU_CLOSE' });
        });
    }
}

// ========== INIT ==========
function init() {
    // Find all menus
    document.querySelectorAll('[data-menu]').forEach(menu => {
        const id = menu.id;
        if (!id || mounted.has(id)) return;

        mounted.add(id);

        // Initialize items in state
        const items = view.getItems(menu);
        const state = getState(id);
        setState(id, { ...state, items });

        // Set initial state attribute
        menu.setAttribute('data-state', 'closed');

        // Initial render
        view.render(id, getState(id), menu, effects.getTrigger(id));
    });

    // Event delegation at document level
    document.addEventListener('click', handleClick);
    document.addEventListener('keydown', handleKeydown);
    document.addEventListener('keydown', handleGlobalKeydown);
}

// ========== DESTROY ==========
function destroy() {
    document.removeEventListener('click', handleClick);
    document.removeEventListener('keydown', handleKeydown);
    document.removeEventListener('keydown', handleGlobalKeydown);
    mounted.clear();
}

// ========== PUBLIC API ==========
const Menu = {
    init,
    destroy,
    dispatch,
    selectors,
    // Programmatic API
    open: (id) => dispatch(id, { type: 'MENU_OPEN' }),
    close: (id) => dispatch(id, { type: 'MENU_CLOSE' }),
    toggle: (id) => dispatch(id, { type: 'MENU_TOGGLE' }),
    closeAll: () => getAllOpenMenus().forEach(id => dispatch(id, { type: 'MENU_CLOSE' })),
    isOpen: (id) => selectors.isOpen(id)
};

export { Menu };
