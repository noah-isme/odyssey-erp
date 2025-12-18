/**
 * Quotation Form - Line Items Management
 * Handles dynamic add/remove of line items with event delegation
 */

// State
let lineCounter = 0;

/**
 * Initialize the module
 */
export function init() {
    const container = document.getElementById('line-items-container');
    if (!container) return;

    // Count existing line items
    const existingItems = container.querySelectorAll('.line-item');
    lineCounter = existingItems.length;

    // Event delegation for the entire form
    const form = container.closest('form');
    if (form) {
        form.addEventListener('click', handleClick);
    }

    // Setup add line button
    const addButton = document.querySelector('[data-action="add-line"]');
    if (addButton) {
        addButton.addEventListener('click', addLine);
    }
}

/**
 * Handle delegated click events
 */
function handleClick(event) {
    const target = event.target.closest('[data-action]');
    if (!target) return;

    const action = target.dataset.action;

    if (action === 'remove-line') {
        removeLine(target);
    }
}

/**
 * Add a new line item
 */
function addLine() {
    const container = document.getElementById('line-items-container');
    if (!container) return;

    const newLine = document.createElement('div');
    newLine.className = 'line-item';
    newLine.setAttribute('data-index', lineCounter);

    newLine.innerHTML = createLineItemHTML(lineCounter);

    container.appendChild(newLine);
    lineCounter++;

    // Focus first input of new line
    const firstInput = newLine.querySelector('input');
    if (firstInput) {
        firstInput.focus();
    }
}

/**
 * Remove a line item
 */
function removeLine(button) {
    const container = document.getElementById('line-items-container');
    if (!container) return;

    const lineItems = container.querySelectorAll('.line-item');

    if (lineItems.length > 1) {
        const lineItem = button.closest('.line-item');
        if (lineItem) {
            lineItem.remove();
        }
    } else {
        // Show toast or alert - at least one line required
        alert('At least one line item is required.');
    }
}

/**
 * Generate HTML for a line item
 */
function createLineItemHTML(index) {
    return `
    <div class="grid">
      <div>
        <label for="product_id_${index}">Product ID <span class="text-error">*</span></label>
        <input type="number" name="product_id" id="product_id_${index}" required min="1">
      </div>
      <div>
        <label for="quantity_${index}">Quantity <span class="text-error">*</span></label>
        <input type="number" name="quantity" id="quantity_${index}" required min="0.01" step="0.01" value="1.00">
      </div>
      <div>
        <label for="uom_${index}">UOM <span class="text-error">*</span></label>
        <input type="text" name="uom" id="uom_${index}" required value="PCS">
      </div>
    </div>
    <div class="grid">
      <div>
        <label for="unit_price_${index}">Unit Price <span class="text-error">*</span></label>
        <input type="number" name="unit_price" id="unit_price_${index}" required min="0" step="0.01" value="0.00">
      </div>
      <div>
        <label for="discount_percent_${index}">Discount %</label>
        <input type="number" name="discount_percent" id="discount_percent_${index}" min="0" max="100" step="0.01" value="0.00">
      </div>
      <div>
        <label for="tax_percent_${index}">Tax %</label>
        <input type="number" name="tax_percent" id="tax_percent_${index}" min="0" max="100" step="0.01" value="11.00">
      </div>
      <div class="flex items-end">
        <button type="button" class="btn btn--ghost btn--sm" data-action="remove-line">Remove</button>
      </div>
    </div>
  `;
}
