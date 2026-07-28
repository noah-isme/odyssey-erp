/* ==========================================================================
   ODYSSEY ERP LANDING PAGE JS
   Dev-Core Interactive Split-Pane & ROI Latency Engine
   ========================================================================== */

document.addEventListener('DOMContentLoaded', function () {
    'use strict';

    // 1. Theme Toggle Controller
    var themeToggle = document.querySelector('[data-theme-toggle]');
    if (themeToggle) {
        themeToggle.addEventListener('click', function () {
            var currentTheme = document.documentElement.getAttribute('data-theme');
            var nextTheme = currentTheme === 'dark' ? 'light' : 'dark';
            if (nextTheme === 'dark') {
                document.documentElement.setAttribute('data-theme', 'dark');
            } else {
                document.documentElement.removeAttribute('data-theme');
            }
            try {
                localStorage.setItem('odyssey.theme', nextTheme);
            } catch (e) {}
        });
    }

    // 2. Interactive Hero Terminal Tab Switcher
    var tabBtns = document.querySelectorAll('.window-tabs .tab-btn');
    var tabPanels = document.querySelectorAll('.tab-panel');

    tabBtns.forEach(function (btn) {
        btn.addEventListener('click', function () {
            var targetTab = btn.getAttribute('data-tab');

            tabBtns.forEach(function (b) { b.classList.remove('is-active'); });
            tabPanels.forEach(function (p) { p.classList.remove('is-active'); });

            btn.classList.add('is-active');
            var activePanel = document.getElementById('panel-' + targetTab);
            if (activePanel) {
                activePanel.classList.add('is-active');
            }
        });
    });

    // 3. Dynamic ROI & Efficiency Calculator
    var empInput = document.getElementById('empRange');
    var invInput = document.getElementById('invRange');
    var empValDisplay = document.getElementById('empVal');
    var invValDisplay = document.getElementById('invVal');

    var hoursSavedDisplay = document.getElementById('hoursSavedVal');
    var annualCostDisplay = document.getElementById('annualCostVal');
    var closingTimeDisplay = document.getElementById('closingTimeVal');
    var errorRateDisplay = document.getElementById('errorRateVal');

    function formatNumber(num) {
        return new Intl.NumberFormat('id-ID').format(num);
    }

    function calculateROI() {
        if (!empInput || !invInput) return;

        var employees = parseInt(empInput.value, 10) || 10;
        var invoices = parseInt(invInput.value, 10) || 100;

        if (empValDisplay) empValDisplay.textContent = employees + ' orang';
        if (invValDisplay) invValDisplay.textContent = formatNumber(invoices) + ' /bln';

        // Calculation logic:
        // Estimated hours saved = (employees * 12) + (invoices * 0.15)
        var hoursSaved = Math.round((employees * 14) + (invoices * 0.18));
        // Estimated annual cost savings (Rp 80.000 / hr average finance wage)
        var annualSavingsRp = hoursSaved * 80000 * 12;

        var closingDaysBefore = Math.max(2, Math.round(5 + (invoices / 800)));
        var closingDaysAfter = '2 jam';

        if (hoursSavedDisplay) hoursSavedDisplay.textContent = formatNumber(hoursSaved) + ' Jam / Bln';
        if (annualCostDisplay) annualCostDisplay.textContent = 'Rp ' + formatNumber(Math.round(annualSavingsRp / 1000000)) + ' Juta / Thn';
        if (closingTimeDisplay) closingTimeDisplay.textContent = closingDaysBefore + ' hari → ' + closingDaysAfter;
        if (errorRateDisplay) errorRateDisplay.textContent = '99.4% Reduction';
    }

    if (empInput && invInput) {
        empInput.addEventListener('input', calculateROI);
        invInput.addEventListener('input', calculateROI);
        calculateROI();
    }
});
