/**
 * Sidebar Feature - Mount + Event Delegation
 * Entry point for sidebar feature
 * Following state-driven-ui architecture
 */

import { reducer, selectors, getState, setState } from './store.js';
import { effects } from './effects.js';
import { view } from './view.js';

// ========== DISPATCH ==========
function dispatch(action) {
    const prevState = getState();
    const nextState = reducer(prevState, action);

    // Only update if state changed
    if (JSON.stringify(nextState) !== JSON.stringify(prevState)) {
        setState(nextState);

        // Effects (side effects) - run AFTER state change
        // Only persist collapsed/pinned (not mobileOpen)
        if (nextState.collapsed !== prevState.collapsed || nextState.pinned !== prevState.pinned) {
            effects.persist(nextState);
        }

        // Lock scroll for mobile
        if (nextState.mobileOpen !== prevState.mobileOpen) {
            effects.lockScroll(nextState.mobileOpen);
        }

        // View (render) - update DOM
        view.render(nextState);
    }
}

// ========== EVENT HANDLERS ==========
// Named function for cleanup
function handleClick(e) {
    // Sidebar toggle button
    const toggleBtn = e.target.closest('[data-sidebar-toggle]');
    if (toggleBtn) {
        e.preventDefault();
        if (effects.isMobile()) {
            dispatch({ type: 'SIDEBAR_MOBILE_TOGGLE' });
        } else {
            dispatch({ type: 'SIDEBAR_TOGGLE' });
        }
        return;
    }

    // Sidebar pin button
    const pinBtn = e.target.closest('[data-sidebar-pin]');
    if (pinBtn) {
        e.preventDefault();
        dispatch({ type: 'SIDEBAR_TOGGLE_PIN' });
        return;
    }

    // Mobile overlay click - close sidebar
    const overlay = e.target.closest('#sidebarOverlay');
    if (overlay && selectors.isMobileOpen()) {
        dispatch({ type: 'SIDEBAR_MOBILE_CLOSE' });
        return;
    }

    // Nav item click on mobile - close sidebar
    const navItem = e.target.closest('.nav-item');
    if (navItem && effects.isMobile() && selectors.isMobileOpen()) {
        dispatch({ type: 'SIDEBAR_MOBILE_CLOSE' });
    }
}

function handleKeydown(e) {
    // Esc closes mobile sidebar
    if (e.key === 'Escape' && selectors.isMobileOpen()) {
        dispatch({ type: 'SIDEBAR_MOBILE_CLOSE' });
    }
}

function handleResize() {
    // Close mobile sidebar when resizing to desktop
    if (!effects.isMobile() && selectors.isMobileOpen()) {
        dispatch({ type: 'SIDEBAR_MOBILE_CLOSE' });
    }
}

// ========== INIT ==========
function init() {
    // Cache DOM elements
    view.cacheElements();

    // Restore state from effects
    const saved = effects.restore();
    if (saved) {
        setState({
            collapsed: saved.collapsed || false,
            pinned: saved.pinned || false,
            mobileOpen: false // Never restore mobile open state
        });
    }

    // Initial render
    view.render(getState());

    // Event Delegation (single listener at document level)
    document.addEventListener('click', handleClick);
    document.addEventListener('keydown', handleKeydown);
    window.addEventListener('resize', handleResize);
}

// ========== DESTROY (cleanup) ==========
function destroy() {
    document.removeEventListener('click', handleClick);
    document.removeEventListener('keydown', handleKeydown);
    window.removeEventListener('resize', handleResize);
}

// ========== NAVIGATION (highlight active) ==========
const Navigation = {
    init() {
        this.highlightActive();
    },

    highlightActive() {
        const currentPath = window.location.pathname;
        document.querySelectorAll('.nav-item').forEach(item => {
            const href = item.getAttribute('href');
            item.classList.remove('active');
            if (href === currentPath || (href !== '/' && currentPath.startsWith(href))) {
                item.classList.add('active');
            }
        });
    }
};

// ========== PUBLIC API ==========
const Sidebar = {
    init,
    destroy,
    dispatch,
    selectors
};

export { Sidebar, Navigation };
