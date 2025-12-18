/**
 * Odyssey ERP - Critical Theme Restore
 * 
 * This script MUST load in <head> blocking to prevent FOUC.
 * It restores the theme from localStorage before the page renders.
 */
(function () {
    const KEY = 'odyssey.theme';
    try {
        const saved = localStorage.getItem(KEY);
        const sys = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
        if (saved === 'dark' || (!saved && sys)) {
            document.documentElement.setAttribute('data-theme', 'dark');
        } else {
            document.documentElement.removeAttribute('data-theme');
        }
    } catch (e) {
        // Silent fail - theme will default to light
    }
})();

