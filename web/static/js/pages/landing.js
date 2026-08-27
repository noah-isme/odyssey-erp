/* ==========================================================================
   ODYSSEY ERP LANDING PAGE JS
   Executive Showcase, Micro-Interactions & ROI Calculation Engine
   ========================================================================== */

document.addEventListener('DOMContentLoaded', function () {
    'use strict';

    // Check for reduced motion preference
    var prefersReducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;

    // ----------------------------------------------------------------------
    // 1. Theme Toggle Controller
    // ----------------------------------------------------------------------
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

    // ----------------------------------------------------------------------
    // 2. Sticky Nav & Scrollspy Controller
    // ----------------------------------------------------------------------
    var navElem = document.querySelector('[data-scroll-nav]');
    var navLinks = document.querySelectorAll('.landing-nav-center .nav-link');
    var sections = document.querySelectorAll('section[id]');

    function handleScrollNav() {
        if (!navElem) return;
        if (window.scrollY > 20) {
            navElem.classList.add('is-scrolled');
        } else {
            navElem.classList.remove('is-scrolled');
        }

        // Active section highlight
        var scrollPos = window.scrollY + 120;
        sections.forEach(function (sec) {
            var top = sec.offsetTop;
            var height = sec.offsetHeight;
            var id = sec.getAttribute('id');
            if (scrollPos >= top && scrollPos < top + height) {
                navLinks.forEach(function (link) {
                    if (link.getAttribute('data-nav-section') === id) {
                        link.classList.add('is-active');
                    } else {
                        link.classList.remove('is-active');
                    }
                });
            }
        });
    }

    window.addEventListener('scroll', handleScrollNav, { passive: true });
    handleScrollNav();

    // ----------------------------------------------------------------------
    // 3. Scroll-Triggered Reveals (IntersectionObserver)
    // ----------------------------------------------------------------------
    var revealElements = document.querySelectorAll('[data-reveal], [data-reveal-child]');
    if ('IntersectionObserver' in window && !prefersReducedMotion) {
        var observerOptions = {
            root: null,
            threshold: 0.12,
            rootMargin: '0px 0px -40px 0px'
        };

        var revealObserver = new IntersectionObserver(function (entries, observer) {
            entries.forEach(function (entry) {
                if (entry.isIntersecting) {
                    entry.target.classList.add('is-revealed');
                    observer.unobserve(entry.target);
                }
            });
        }, observerOptions);

        revealElements.forEach(function (el) {
            revealObserver.observe(el);
        });
    } else {
        // Fallback for older browsers or reduced motion
        revealElements.forEach(function (el) {
            el.classList.add('is-revealed');
        });
    }

    // ----------------------------------------------------------------------
    // 4. Executive Showcase Workspace Tab Switcher
    // ----------------------------------------------------------------------
    var tabBtns = document.querySelectorAll('.window-tabs .tab-btn');
    var tabPanels = document.querySelectorAll('.tab-panel');
    var promptText = document.getElementById('termPromptText');

    var tabLabels = {
        'sales': 'WORKSPACE EKSEKUTIF · PIPELINE PENJUALAN',
        'inventory': 'WORKSPACE EKSEKUTIF · KONTROL INVENTORI',
        'ledger': 'WORKSPACE EKSEKUTIF · BUKU BESAR FINANSIAL',
        'metrics': 'WORKSPACE EKSEKUTIF · KESIAPAN SISTEM'
    };

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

            if (promptText && tabLabels[targetTab]) {
                promptText.textContent = tabLabels[targetTab];
            }
        });
    });

    // ----------------------------------------------------------------------
    // 5. Dynamic ROI Calculator with Interpolated Number Counters
    // ----------------------------------------------------------------------
    var empInput = document.getElementById('empRange');
    var invInput = document.getElementById('invRange');
    var empValDisplay = document.getElementById('empVal');
    var invValDisplay = document.getElementById('invVal');

    var hoursSavedDisplay = document.getElementById('hoursSavedVal');
    var annualCostDisplay = document.getElementById('annualCostVal');
    var closingTimeDisplay = document.getElementById('closingTimeVal');
    var errorRateDisplay = document.getElementById('errorRateVal');

    var currentHours = 440;
    var currentAnnualJuta = 422;
    var animFrameHours = null;
    var animFrameAnnual = null;

    function formatNumber(num) {
        return new Intl.NumberFormat('id-ID').format(num);
    }

    function animateValue(start, end, duration, onUpdate, onComplete) {
        if (prefersReducedMotion || duration === 0) {
            onUpdate(end);
            if (onComplete) onComplete();
            return null;
        }

        var startTime = performance.now();
        function update(now) {
            var elapsed = now - startTime;
            var progress = Math.min(elapsed / duration, 1);
            // Ease out quad formula
            var easeProgress = 1 - (1 - progress) * (1 - progress);
            var val = Math.round(start + (end - start) * easeProgress);

            onUpdate(val);

            if (progress < 1) {
                requestAnimationFrame(update);
            } else if (onComplete) {
                onComplete();
            }
        }
        return requestAnimationFrame(update);
    }

    function calculateROI() {
        if (!empInput || !invInput) return;

        var employees = parseInt(empInput.value, 10) || 10;
        var invoices = parseInt(invInput.value, 10) || 100;

        if (empValDisplay) empValDisplay.textContent = employees + ' orang';
        if (invValDisplay) invValDisplay.textContent = formatNumber(invoices) + ' /bln';

        var targetHours = Math.round((employees * 14) + (invoices * 0.18));
        var annualSavingsRp = targetHours * 80000 * 12;
        var targetAnnualJuta = Math.round(annualSavingsRp / 1000000);

        var closingDaysBefore = Math.max(2, Math.round(5 + (invoices / 800)));
        var closingDaysAfter = '2 jam';

        // Animate hours count-up
        if (hoursSavedDisplay) {
            hoursSavedDisplay.classList.add('is-updating');
            if (animFrameHours) cancelAnimationFrame(animFrameHours);
            animFrameHours = animateValue(currentHours, targetHours, 300, function (val) {
                hoursSavedDisplay.textContent = formatNumber(val) + ' Jam / Bln';
            }, function () {
                hoursSavedDisplay.classList.remove('is-updating');
            });
            currentHours = targetHours;
        }

        // Animate annual cost savings count-up
        if (annualCostDisplay) {
            annualCostDisplay.classList.add('is-updating');
            if (animFrameAnnual) cancelAnimationFrame(animFrameAnnual);
            animFrameAnnual = animateValue(currentAnnualJuta, targetAnnualJuta, 300, function (val) {
                annualCostDisplay.textContent = 'Rp ' + formatNumber(val) + ' Juta / Thn';
            }, function () {
                annualCostDisplay.classList.remove('is-updating');
            });
            currentAnnualJuta = targetAnnualJuta;
        }

        if (closingTimeDisplay) closingTimeDisplay.textContent = closingDaysBefore + ' hari → ' + closingDaysAfter;
        if (errorRateDisplay) errorRateDisplay.textContent = '99.4% Reduction';
    }

    if (empInput && invInput) {
        empInput.addEventListener('input', calculateROI);
        invInput.addEventListener('input', calculateROI);
        calculateROI();
    }
});
