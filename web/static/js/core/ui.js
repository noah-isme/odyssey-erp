/* ==========================================================================
   ODYSSEY UI - Critical Theme Restore
   
   This file MUST be loaded blocking in <head> to prevent FOUC.
   All interactive UI components are now in features/ (state-driven):
     - features/menu/      → Dropdown/Menu
     - features/modal/     → Modal/Dialog
     - features/toast/     → Toast/Snackbar
   ========================================================================== */

(function () {
    'use strict';

    var KEY = 'odyssey.theme';

    try {
        var saved = localStorage.getItem(KEY);
        var prefersDark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;

        if (saved === 'dark' || (!saved && prefersDark)) {
            document.documentElement.setAttribute('data-theme', 'dark');
        }
    } catch (e) {
        // Silent fail - theme will default to light
    }
})();

