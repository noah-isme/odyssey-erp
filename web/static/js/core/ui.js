/* ==========================================================================
   ODYSSEY UI COMPONENTS (Vanilla JS)
   Includes:
     1) Theme switcher (persist)
     2) Dropdown/Menu (click outside + Esc)
     3) Modal (overlay + Esc)
     4) Toast (queue)
   ========================================================================== */

(function () {
    const UI = {};
    const root = document.documentElement;

    /* ============================
       1) THEME SWITCHER
       MOVED TO: features/theme/ (modular state-driven architecture)
       - store.js: State + Reducer + Selectors
       - effects.js: localStorage persistence
       - view.js: DOM rendering
       - index.js: Init + Event delegation
       
       Theme is now initialized via main.js (ES module)
       ============================ */


    /* ============================
       2) DROPDOWN / MENU
       Pattern A (recommended):
         <button class="btn" data-menu-trigger aria-controls="menu-user" aria-expanded="false">User</button>
         <div id="menu-user" data-menu hidden role="menu">
           <button role="menuitem">Profile</button>
         </div>
  
       Notes:
         - click trigger toggles
         - click outside closes
         - Esc closes
         - focus returns to trigger on close
       ============================ */
    UI.menu = (function () {
        function isOpen(menu) {
            return !menu.hasAttribute("hidden");
        }

        function open(trigger, menu) {
            // close others
            document.querySelectorAll("[data-menu]:not([hidden])").forEach((m) => closeByMenu(m));

            menu.removeAttribute("hidden");
            trigger.setAttribute("aria-expanded", "true");

            // focus first menu item
            const first = menu.querySelector('[role="menuitem"], a, button, [tabindex="0"]');
            if (first) first.focus({ preventScroll: true });
        }

        function close(trigger, menu) {
            menu.setAttribute("hidden", "");
            trigger.setAttribute("aria-expanded", "false");
            trigger.focus({ preventScroll: true });
        }

        function closeByMenu(menu) {
            const id = menu.id;
            if (!id) { menu.setAttribute("hidden", ""); return; }
            const trigger = document.querySelector(`[data-menu-trigger][aria-controls="${CSS.escape(id)}"]`);
            if (trigger) close(trigger, menu);
            else menu.setAttribute("hidden", "");
        }

        function getMenuFromTrigger(trigger) {
            const id = trigger.getAttribute("aria-controls");
            if (!id) return null;
            return document.getElementById(id);
        }

        function init() {
            // click toggle
            document.addEventListener("click", (e) => {
                const trigger = e.target.closest("[data-menu-trigger]");
                const openMenus = document.querySelectorAll("[data-menu]:not([hidden])");

                // click outside closes any open menus
                openMenus.forEach((menu) => {
                    const t = menu.id ? document.querySelector(`[data-menu-trigger][aria-controls="${CSS.escape(menu.id)}"]`) : null;
                    const clickInside = menu.contains(e.target) || (t && t.contains(e.target));
                    if (!clickInside) closeByMenu(menu);
                });

                if (!trigger) return;

                const menu = getMenuFromTrigger(trigger);
                if (!menu) return;

                // toggle
                if (isOpen(menu)) close(trigger, menu);
                else open(trigger, menu);
            });

            // Esc closes
            document.addEventListener("keydown", (e) => {
                if (e.key !== "Escape") return;
                document.querySelectorAll("[data-menu]:not([hidden])").forEach((menu) => closeByMenu(menu));
            });
        }

        return { init, open, close };
    })();


    /* ============================
       3) MODAL / DIALOG
       Markup:
         <button data-modal-open="modal-help">Open</button>
  
         <div id="modal-help" data-modal hidden role="dialog" aria-modal="true" aria-labelledby="modalHelpTitle">
           <div data-modal-overlay></div>
           <div data-modal-panel>
             <h2 id="modalHelpTitle">Title</h2>
             <button data-modal-close>Close</button>
           </div>
         </div>
  
       Behavior:
         - open/close
         - close on overlay click
         - Esc closes
         - focus trap minimal + return focus
       ============================ */
    UI.modal = (function () {
        let lastActiveEl = null;

        function getFocusable(container) {
            return Array.from(
                container.querySelectorAll(
                    'a[href], button:not([disabled]), textarea, input, select, [tabindex]:not([tabindex="-1"])'
                )
            );
        }

        function trapFocus(modal, e) {
            if (e.key !== "Tab") return;

            const panel = modal.querySelector("[data-modal-panel]") || modal;
            const focusables = getFocusable(panel).filter((el) => !el.hasAttribute("disabled"));
            if (focusables.length === 0) return;

            const first = focusables[0];
            const last = focusables[focusables.length - 1];

            if (e.shiftKey && document.activeElement === first) {
                e.preventDefault();
                last.focus();
            } else if (!e.shiftKey && document.activeElement === last) {
                e.preventDefault();
                first.focus();
            }
        }

        function open(idOrEl) {
            const modal = typeof idOrEl === "string" ? document.getElementById(idOrEl) : idOrEl;
            if (!modal) return;

            // close other open modals (single-modal policy)
            document.querySelectorAll("[data-modal]:not([hidden])").forEach((m) => close(m));

            lastActiveEl = document.activeElement;

            modal.removeAttribute("hidden");
            modal.setAttribute("aria-hidden", "false");

            // lock scroll (simple)
            document.body.style.overflow = "hidden";

            // focus first element in panel
            const panel = modal.querySelector("[data-modal-panel]") || modal;
            const focusables = getFocusable(panel);
            (focusables[0] || panel).focus?.({ preventScroll: true });

            // attach focus trap
            modal.addEventListener("keydown", (e) => trapFocus(modal, e));
        }

        function close(idOrEl) {
            const modal = typeof idOrEl === "string" ? document.getElementById(idOrEl) : idOrEl;
            if (!modal) return;

            modal.setAttribute("hidden", "");
            modal.setAttribute("aria-hidden", "true");
            document.body.style.overflow = "";

            // restore focus
            if (lastActiveEl && lastActiveEl.focus) lastActiveEl.focus({ preventScroll: true });
            lastActiveEl = null;
        }

        function init() {
            // open
            document.addEventListener("click", (e) => {
                const openBtn = e.target.closest("[data-modal-open]");
                if (openBtn) {
                    const id = openBtn.getAttribute("data-modal-open");
                    open(id);
                    return;
                }

                // close
                if (e.target.closest("[data-modal-close]")) {
                    const modal = e.target.closest("[data-modal]");
                    if (modal) close(modal);
                    return;
                }

                // overlay click closes
                const overlay = e.target.closest("[data-modal-overlay]");
                if (overlay) {
                    const modal = e.target.closest("[data-modal]");
                    if (modal) close(modal);
                }
            });

            // Esc closes top-most modal
            document.addEventListener("keydown", (e) => {
                if (e.key !== "Escape") return;
                const openModal = document.querySelector("[data-modal]:not([hidden])");
                if (openModal) close(openModal);
            });
        }

        return { init, open, close };
    })();


    /* ============================
       4) TOAST (Queue)
       Create container automatically.
       API:
         UI.toast.show({ title, message, variant, duration })
       Variants: "neutral" | "success" | "warning" | "error" | "info"
       ============================ */
    UI.toast = (function () {
        let container = null;
        const queue = [];
        let showing = false;

        function ensureContainer() {
            if (container) return container;

            container = document.createElement("div");
            container.setAttribute("data-toast-container", "");
            container.setAttribute("aria-live", "polite");
            container.setAttribute("aria-relevant", "additions");
            document.body.appendChild(container);

            // Inline styles (minimal). You can move to CSS later.
            container.style.position = "fixed";
            container.style.right = "16px";
            container.style.bottom = "16px";
            container.style.zIndex = "9999";
            container.style.display = "flex";
            container.style.flexDirection = "column";
            container.style.gap = "10px";
            container.style.maxWidth = "360px";

            return container;
        }

        function toastColors(variant) {
            // Uses CSS vars when possible. Minimal fallback.
            const map = {
                neutral: { bg: "var(--toast-bg)", border: "var(--toast-border)", fg: "var(--text-primary)" },
                success: { bg: "var(--success-bg)", border: "rgba(31,122,77,0.22)", fg: "var(--text-primary)" },
                warning: { bg: "var(--warning-bg)", border: "rgba(178,106,0,0.22)", fg: "var(--text-primary)" },
                error: { bg: "var(--error-bg)", border: "rgba(180,35,24,0.22)", fg: "var(--text-primary)" },
                info: { bg: "var(--info-bg)", border: "rgba(37,99,235,0.22)", fg: "var(--text-primary)" },
            };
            return map[variant] || map.neutral;
        }

        function renderToast({ title, message, variant = "neutral", duration = 3200 }) {
            const wrap = document.createElement("div");
            wrap.setAttribute("role", "status");
            wrap.setAttribute("data-toast", "");
            wrap.tabIndex = -1;

            const { bg, border, fg } = toastColors(variant);
            wrap.style.background = bg;
            wrap.style.border = `1px solid ${border}`;
            wrap.style.color = fg;
            wrap.style.borderRadius = "14px";
            wrap.style.boxShadow = "var(--toast-shadow)";
            wrap.style.padding = "12px 12px";
            wrap.style.display = "grid";
            wrap.style.gridTemplateColumns = "1fr auto";
            wrap.style.gap = "12px";
            wrap.style.alignItems = "start";
            wrap.style.overflow = "hidden";

            const content = document.createElement("div");
            content.style.display = "flex";
            content.style.flexDirection = "column";
            content.style.gap = "2px";

            if (title) {
                const t = document.createElement("div");
                t.textContent = title;
                t.style.fontSize = "13px";
                t.style.fontWeight = "600";
                content.appendChild(t);
            }

            if (message) {
                const m = document.createElement("div");
                m.textContent = message;
                m.style.fontSize = "12px";
                m.style.opacity = "0.85";
                m.style.lineHeight = "1.4";
                content.appendChild(m);
            }

            const closeBtn = document.createElement("button");
            closeBtn.type = "button";
            closeBtn.setAttribute("aria-label", "Close");
            closeBtn.textContent = "✕";
            closeBtn.style.width = "28px";
            closeBtn.style.height = "28px";
            closeBtn.style.borderRadius = "10px";
            closeBtn.style.border = "1px solid rgba(15,31,51,0.12)";
            closeBtn.style.background = "rgba(255,255,255,0.45)";
            closeBtn.style.cursor = "pointer";
            closeBtn.style.lineHeight = "1";
            closeBtn.style.display = "inline-flex";
            closeBtn.style.alignItems = "center";
            closeBtn.style.justifyContent = "center";

            // dark mode button tweak
            if (root.getAttribute("data-theme") === "dark") {
                closeBtn.style.border = "1px solid rgba(231,237,244,0.14)";
                closeBtn.style.background = "rgba(231,237,244,0.06)";
                closeBtn.style.color = "rgba(231,237,244,0.88)";
            }

            wrap.appendChild(content);
            wrap.appendChild(closeBtn);

            // Entrance animation (subtle)
            wrap.style.transform = "translateY(8px)";
            wrap.style.opacity = "0";
            wrap.style.transition = "transform 180ms var(--ease-standard), opacity 180ms var(--ease-standard)";

            requestAnimationFrame(() => {
                wrap.style.transform = "translateY(0)";
                wrap.style.opacity = "1";
            });

            let timeoutId = null;

            function remove() {
                if (timeoutId) clearTimeout(timeoutId);
                wrap.style.transform = "translateY(8px)";
                wrap.style.opacity = "0";
                setTimeout(() => {
                    wrap.remove();
                    showing = false;
                    pump();
                }, 180);
            }

            closeBtn.addEventListener("click", remove);

            // auto-dismiss (errors stay longer)
            const finalDuration = variant === "error" ? Math.max(duration, 5000) : duration;
            timeoutId = setTimeout(remove, finalDuration);

            return wrap;
        }

        function pump() {
            if (showing) return;
            if (queue.length === 0) return;
            showing = true;

            const item = queue.shift();
            const el = renderToast(item);
            ensureContainer().appendChild(el);
        }

        function show(opts) {
            queue.push(opts || {});
            pump();
        }

        return { show };
    })();


    /* ============================
       Init all (except Theme - handled by main.js module)
       ============================ */
    UI.init = function () {
        // Theme is now modular: features/theme/ (initialized via main.js)
        UI.menu.init();
        UI.modal.init();
    };

    // expose globally
    window.OdysseyUI = UI;

    // auto-init
    if (document.readyState === "loading") {
        document.addEventListener("DOMContentLoaded", UI.init);
    } else {
        UI.init();
    }
})();
