// Manufacturing Governance UI JavaScript

let currentChallenge = null;

/**
 * Validate decision and request challenge
 */
async function validateDecision() {
    const form = document.getElementById('decisionForm');
    
    // Collect form data
    const formData = {
        record_type: form.record_type.value,
        record_id: parseInt(form.record_id.value),
        company_id: parseInt(form.company_id.value),
        actor_id: parseInt(form.actor_id.value),
        actor_role: form.actor_role.value,
        action: form.action.value,
        reason: form.reason.value,
        evidence: form.evidence.value ? JSON.parse(form.evidence.value || '{}') : {}
    };

    // Validate required fields
    const errors = validateFormData(formData);
    if (errors.length > 0) {
        showStatus('error', errors.join(', '));
        return;
    }

    try {
        showStatus('info', 'Validating decision...');
        
        // Call decision submission API
        const response = await fetch('/api/decisions', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(formData)
        });

        const result = await response.json();

        if (result.success) {
            // Display validation results
            displayValidationResults(result.validation_data);
            
            // Store challenge for later
            currentChallenge = {
                id: result.challenge_id,
                text: result.challenge_text
            };

            // Show challenge section
            document.getElementById('challengeText').textContent = result.challenge_text;
            document.getElementById('challengeSection').style.display = 'block';
            showStatus('success', result.message);
        } else {
            showStatus('error', result.error || 'Validation failed');
            if (result.validation_data) {
                displayValidationResults(result.validation_data);
            }
        }
    } catch (error) {
        showStatus('error', `Error: ${error.message}`);
    }
}

/**
 * Submit signature and complete decision
 */
async function submitSignature() {
    if (!currentChallenge) {
        showStatus('error', 'No active challenge');
        return;
    }

    const form = document.getElementById('decisionForm');
    const signature = document.getElementById('signature').value;
    const comment = document.getElementById('comment').value;
    const decision = form.action.value;

    if (!signature) {
        showStatus('error', 'Signature is required');
        return;
    }

    try {
        showStatus('info', 'Submitting signature...');

        const response = await fetch('/api/challenges/verify', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                challenge_id: currentChallenge.id,
                signature: signature,
                decision: decision,
                comment: comment
            })
        });

        const result = await response.json();

        if (result.success) {
            showStatus('success', `Decision ${decision} recorded successfully. Gate status: ${result.gate_status}`);
            
            // Reset form and hide challenge section
            setTimeout(() => {
                form.reset();
                document.getElementById('challengeSection').style.display = 'none';
                document.getElementById('validationSection').style.display = 'none';
                currentChallenge = null;
            }, 2000);
        } else {
            showStatus('error', result.error || 'Signature verification failed');
        }
    } catch (error) {
        showStatus('error', `Error: ${error.message}`);
    }
}

/**
 * Validate form data
 */
function validateFormData(data) {
    const errors = [];

    if (!data.record_type) errors.push('Record type is required');
    if (!data.record_id || data.record_id <= 0) errors.push('Valid record ID is required');
    if (!data.company_id || data.company_id <= 0) errors.push('Valid company ID is required');
    if (!data.actor_id || data.actor_id <= 0) errors.push('Valid actor ID is required');
    if (!data.actor_role) errors.push('Actor role is required');
    if (!data.action) errors.push('Action is required');
    if (!data.reason || data.reason.trim().length === 0) errors.push('Reason is required');

    return errors;
}

/**
 * Display validation results
 */
function displayValidationResults(data) {
    const container = document.getElementById('validationResults');
    container.innerHTML = '';

    if (!data || Object.keys(data).length === 0) {
        container.innerHTML = '<p>No validation data available</p>';
        return;
    }

    Object.entries(data).forEach(([key, value]) => {
        const item = document.createElement('div');
        item.className = 'validation-item';
        item.innerHTML = `
            <span class="validation-icon"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"/></svg></span>
            <div>
                <strong>${formatKey(key)}:</strong><br>
                <span>${formatValue(value)}</span>
            </div>
        `;
        container.appendChild(item);
    });

    document.getElementById('validationSection').style.display = 'block';
}

/**
 * Show status message
 */
function showStatus(type, message) {
    const statusEl = document.getElementById('statusMessage');
    statusEl.className = `status-message ${type}`;
    statusEl.textContent = message;
    statusEl.style.display = 'flex';

    if (type !== 'error') {
        setTimeout(() => {
            statusEl.style.display = 'none';
        }, 5000);
    }
}

/**
 * Update record type specific fields
 */
function updateRecordTypeFields() {
    const recordType = document.getElementById('recordType').value;
    
    // Update action options based on record type
    const actionSelect = document.getElementById('action');
    const actions = {
        'BOM': ['Approve'],
        'WorkOrder': ['Release'],
        'Operation': ['Complete'],
        'Hold': ['Release'],
        'NCR': ['Disposition'],
        'CAPA': ['Close']
    };

    actionSelect.innerHTML = '<option value="">-- Select Action --</option>';
    if (actions[recordType]) {
        actions[recordType].forEach(action => {
            const option = document.createElement('option');
            option.value = action;
            option.textContent = action;
            actionSelect.appendChild(option);
        });
    }
}

/**
 * Format object key for display
 */
function formatKey(key) {
    return key
        .replace(/_/g, ' ')
        .replace(/\b\w/g, l => l.toUpperCase());
}

/**
 * Format value for display
 */
function formatValue(value) {
    if (typeof value === 'object') {
        return JSON.stringify(value);
    }
    return String(value);
}

/**
 * Load and display audit log
 */
async function loadAuditLog(filters = {}) {
    try {
        const params = new URLSearchParams(filters);
        const response = await fetch(`/api/audit-log?${params}`);
        const result = await response.json();

        if (result.success) {
            displayAuditLog(result.events);
        } else {
            console.error('Failed to load audit log:', result.error);
        }
    } catch (error) {
        console.error('Error loading audit log:', error);
    }
}

/**
 * Display audit log in table format
 */
function displayAuditLog(events) {
    const table = document.querySelector('.audit-table tbody');
    if (!table) return;

    table.innerHTML = '';

    events.forEach(event => {
        const row = document.createElement('tr');
        row.innerHTML = `
            <td>${event.id}</td>
            <td>${event.entity_type}</td>
            <td>${event.entity_id}</td>
            <td>${event.action}</td>
            <td>${event.actor_id}</td>
            <td><span class="audit-badge">${formatAction(event.action)}</span></td>
            <td>${formatDate(event.created_at)}</td>
        `;
        table.appendChild(row);
    });
}

/**
 * Format action for badge display
 */
function formatAction(action) {
    const actionMap = {
        'APPROVE': 'approved',
        'REJECT': 'rejected',
        'PENDING': 'pending'
    };
    return actionMap[action] || action.toLowerCase();
}

/**
 * Format date for display
 */
function formatDate(dateStr) {
    const date = new Date(dateStr);
    return date.toLocaleString();
}

/**
 * Filter audit log
 */
function filterAuditLog() {
    const entityType = document.getElementById('filterEntityType')?.value || '';
    const action = document.getElementById('filterAction')?.value || '';
    const startDate = document.getElementById('filterStartDate')?.value || '';
    const endDate = document.getElementById('filterEndDate')?.value || '';

    const filters = {};
    if (entityType) filters.entity_type = entityType;
    if (action) filters.action = action;
    if (startDate) filters.start_date = startDate;
    if (endDate) filters.end_date = endDate;

    loadAuditLog(filters);
}

/**
 * Initialize page
 */
document.addEventListener('DOMContentLoaded', function() {
    // Initialize event listeners
    const decisionForm = document.getElementById('decisionForm');
    if (decisionForm) {
        decisionForm.addEventListener('submit', function(e) {
            e.preventDefault();
        });
    }

    // Load audit log if on audit page
    if (document.querySelector('.audit-log-container')) {
        loadAuditLog();
    }
});

/**
 * Export functions for testing
 */
window.governance = {
    validateDecision,
    submitSignature,
    loadAuditLog,
    filterAuditLog
};
