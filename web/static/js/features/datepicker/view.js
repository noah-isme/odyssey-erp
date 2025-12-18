/**
 * Date Range Picker View - Render Layer
 * Following state-driven-ui architecture
 */

import { effects } from './effects.js';

const view = {
    /**
     * Render date range picker state to DOM
     * @param {string} id - Instance ID
     * @param {Object} state - Current state
     * @param {HTMLElement} container - Container element
     */
    render(id, state, container) {
        if (!container) return;

        const trigger = container.querySelector('[data-daterange-trigger]');
        const dropdown = container.querySelector('[data-daterange-dropdown]');
        const startInput = container.querySelector('[data-daterange-start]');
        const endInput = container.querySelector('[data-daterange-end]');
        const display = container.querySelector('[data-daterange-display]');

        // Update hidden inputs
        if (startInput) startInput.value = state.startDate || '';
        if (endInput) endInput.value = state.endDate || '';

        // Update display text
        if (display) {
            display.textContent = effects.formatRange(state.startDate, state.endDate);
        }

        // Render dropdown
        if (dropdown) {
            this.renderDropdown(dropdown, state);
        }

        // ARIA
        if (trigger) {
            trigger.setAttribute('aria-expanded', state.isOpen);
        }
    },

    /**
     * Render dropdown content
     * @param {HTMLElement} dropdown - Dropdown element
     * @param {Object} state - Current state
     */
    renderDropdown(dropdown, state) {
        if (state.isOpen) {
            dropdown.classList.add('open');
            dropdown.removeAttribute('hidden');
        } else {
            dropdown.classList.remove('open');
            dropdown.setAttribute('hidden', '');
            return;
        }

        const presets = effects.getPresets();

        dropdown.innerHTML = `
            <div class="daterange-content">
                <div class="daterange-presets">
                    <div class="daterange-presets-title">Quick Select</div>
                    ${presets.map(preset => `
                        <button type="button" 
                                class="daterange-preset${state.startDate === preset.start && state.endDate === preset.end ? ' active' : ''}"
                                data-daterange-preset
                                data-start="${preset.start}"
                                data-end="${preset.end}">
                            ${preset.label}
                        </button>
                    `).join('')}
                </div>
                <div class="daterange-inputs">
                    <div class="daterange-input-group">
                        <label for="daterange-start-${dropdown.id}">From</label>
                        <input type="date" 
                               id="daterange-start-${dropdown.id}"
                               data-daterange-input="start"
                               value="${state.startDate || ''}"
                               class="${state.activeField === 'start' ? 'active' : ''}">
                    </div>
                    <div class="daterange-input-group">
                        <label for="daterange-end-${dropdown.id}">To</label>
                        <input type="date" 
                               id="daterange-end-${dropdown.id}"
                               data-daterange-input="end"
                               value="${state.endDate || ''}"
                               min="${state.startDate || ''}"
                               class="${state.activeField === 'end' ? 'active' : ''}">
                    </div>
                </div>
                <div class="daterange-actions">
                    <button type="button" class="btn btn--ghost btn--sm" data-daterange-clear>Clear</button>
                    <button type="button" class="btn btn--primary btn--sm" data-daterange-apply>Apply</button>
                </div>
            </div>
        `;
    }
};

export { view };
