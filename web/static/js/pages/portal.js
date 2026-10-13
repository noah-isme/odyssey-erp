// Portal app (/portal/app) behavior. Served as a static file because the
// CSP is script-src 'self': inline <script> blocks and inline on*= handlers
// are blocked. Loaded with defer, so the DOM is parsed before this runs.
'use strict';

// postJSON is the only network call site on this page. It sends the
// session CSRF token (rejected with 403 otherwise), same-origin
// credentials, and JSON headers. Omit body for GET requests. It
// resolves to the parsed JSON response (or null) and throws an Error
// carrying the server's {"error": ...} message on non-2xx responses.
async function postJSON(url, body, method) {
    const meta = document.querySelector('meta[name="csrf-token"]');
    const opts = {
        method: method || 'POST',
        credentials: 'same-origin',
        headers: {
            'Accept': 'application/json',
            'X-CSRF-Token': meta ? meta.getAttribute('content') : ''
        }
    };
    if (body !== undefined) {
        opts.headers['Content-Type'] = 'application/json';
        opts.body = JSON.stringify(body);
    }
    const res = await fetch(url, opts);
    const text = await res.text();
    let data = null;
    if (text) {
        try { data = JSON.parse(text); } catch (_) { data = null; }
    }
    if (!res.ok) {
        const msg = (data && data.error) ? data.error : (text || res.statusText);
        const err = new Error(msg);
        err.status = res.status;
        throw err;
    }
    return data;
}

// Track analytics automatically
function trackEvent(eventType, metadata = {}) {
    postJSON('/portal/analytics/event', { event_type: eventType, metadata: metadata })
        .catch(e => console.error("Analytics skipped", e));
}

function switchTab(tabId, link) {
    document.querySelectorAll('.view').forEach(v => v.classList.remove('active'));
    document.querySelectorAll('.nav-link').forEach(n => n.classList.remove('active'));

    document.getElementById('view-' + tabId).classList.add('active');
    if (link) link.classList.add('active');
    
    const titles = {
        'dashboard': 'Welcome back, Partner!',
        'profile': 'My Profile',
        'rfq': 'RFQ Negotiations',
        'chat': 'Messages',
        'hr': 'My HR'
    };
    document.getElementById('pageTitle').textContent = titles[tabId];
    trackEvent('page_view', { page: tabId });
}

// Profile Form
document.getElementById('profileForm').addEventListener('submit', async (e) => {
    e.preventDefault();
    const btn = e.target.querySelector('.btn');
    btn.textContent = 'Saving...';
    
    try {
        await postJSON('/portal/profile', {
            display_name: document.getElementById('profName').value,
            phone: document.getElementById('profPhone').value
        }, 'PUT');
        btn.textContent = 'Saved!';
        setTimeout(() => btn.textContent = 'Save Changes', 2000);
        trackEvent('profile_updated');
    } catch (err) {
        alert("Failed to update profile");
        btn.textContent = 'Save Changes';
    }
});

// Chat Form
document.getElementById('chatForm').addEventListener('submit', async (e) => {
    e.preventDefault();
    const input = document.getElementById('chatMsg');
    const msg = input.value;
    if (!msg) return;

    // Optimistic UI
    const chatList = document.getElementById('chatList');
    // textContent only: the message is user input and must never be parsed as HTML.
    chatList.appendChild(mk('div', msg, 'msg sent'));
    chatList.scrollTop = chatList.scrollHeight;
    input.value = '';

    try {
        await postJSON('/portal/chat', { context: 'general', message: msg });
        trackEvent('chat_sent');
    } catch (err) {
        console.error("Chat failed to send");
    }
});

// RFQ Negotation
async function negotiate(rfqId) {
    const price = prompt("Enter your proposed price:");
    if (!price) return;

    try {
        await postJSON('/portal/rfq/negotiate', { rfq_id: rfqId, supplier_id: 1, proposed_price: parseFloat(price) });
        alert("Proposal sent successfully!");
        trackEvent('rfq_negotiated', { rfq_id: rfqId, price: price });
    } catch (err) {
        alert("Failed to send proposal");
    }
}

// ---- Employee self-service (all network access goes through postJSON) ----
function mk(tag, text, cls) {
    const n = document.createElement(tag);
    if (text !== undefined && text !== null) n.textContent = String(text);
    if (cls) n.className = cls;
    return n;
}
function setMsg(id, text, kind) {
    const n = document.getElementById(id);
    n.textContent = text || '';
    n.className = 'hr-msg' + (kind ? ' ' + kind : '');
}
function val(id) { return document.getElementById(id).value.trim(); }

// renderList fills a <ul> with one row per item: a label, a status
// badge and a Cancel button for PENDING rows. Only textContent is used.
function renderList(listId, items, labelFn, cancelUrlFn, reload) {
    const ul = document.getElementById(listId);
    ul.replaceChildren();
    if (!items.length) {
        ul.appendChild(mk('li', 'Nothing submitted yet.'));
        return;
    }
    items.forEach(item => {
        const li = mk('li');
        li.appendChild(mk('span', labelFn(item)));
        const right = mk('span');
        right.appendChild(mk('span', item.status, 'hr-status ' + item.status));
        if (item.status === 'PENDING') {
            const btn = mk('button', 'Cancel', 'btn btn-sm');
            btn.type = 'button';
            btn.style.marginLeft = '0.5rem';
            btn.addEventListener('click', async () => {
                btn.disabled = true;
                try {
                    await postJSON(cancelUrlFn(item), {});
                    await reload();
                } catch (err) {
                    btn.disabled = false;
                    alert(err.message);
                }
            });
            right.appendChild(btn);
        }
        li.appendChild(right);
        ul.appendChild(li);
    });
}

let hrHolidays = {};
let hrHolidayYears = {};

async function loadHolidays(year) {
    if (hrHolidayYears[year]) return;
    hrHolidayYears[year] = true;
    try {
        const data = await postJSON('/portal/employee/holidays?year=' + encodeURIComponent(year), undefined, 'GET');
        ((data && data.holidays) || []).forEach(h => { hrHolidays[h.date] = h.name; });
    } catch (_) { hrHolidayYears[year] = false; }
}

async function updateHolidayHint() {
    const hint = document.getElementById('leave-holiday-hint');
    const start = val('leaveStart'), end = val('leaveEnd');
    hint.hidden = true;
    if (!start || !end || end < start) return;
    const years = new Set([start.slice(0, 4), end.slice(0, 4)]);
    for (const y of years) { await loadHolidays(y); }
    const hits = Object.keys(hrHolidays).filter(d => d >= start && d <= end).sort();
    if (hits.length) {
        hint.textContent = 'Public holidays in range (not counted): ' + hits.map(d => d + ' ' + hrHolidays[d]).join(', ');
        hint.hidden = false;
    }
}

async function loadLeave() {
    const data = await postJSON('/portal/employee/leave', undefined, 'GET');
    const sel = document.getElementById('leaveType');
    const prev = sel.value;
    sel.replaceChildren();
    (data.leave_types || []).forEach(t => {
        const o = mk('option', t.name);
        o.value = t.id;
        sel.appendChild(o);
    });
    if (prev) sel.value = prev;
    document.getElementById('leave-balances').textContent = (data.balances || []).length
        ? 'Balances ' + data.year + ': ' + data.balances.map(b => b.type_name + ' ' + b.available + ' available').join(' | ')
        : '';
    renderList('leave-list', data.leave || [],
        x => x.type_name + ' ' + x.start_date + ' to ' + x.end_date + ' (' + x.days + ' d'
            + (x.start_half_day ? ', start half-day' : '') + (x.end_half_day ? ', end half-day' : '') + ')',
        x => '/portal/employee/leave/' + x.id + '/cancel', loadLeave);
}

async function loadOvertime() {
    const data = await postJSON('/portal/employee/overtime', undefined, 'GET');
    renderList('overtime-list', data.overtime || [],
        x => x.worked_on + ': ' + x.minutes + ' min' + (x.reason ? ' - ' + x.reason : ''),
        x => '/portal/employee/overtime/' + x.id + '/cancel', loadOvertime);
}

async function loadExpenses() {
    const data = await postJSON('/portal/employee/expenses', undefined, 'GET');
    renderList('expense-list', data.expenses || [],
        x => x.claim_date + ': ' + x.category + ' ' + x.currency + ' ' + x.amount,
        x => '/portal/employee/expenses/' + x.id + '/cancel', loadExpenses);
}

async function loadProfile() {
    const data = await postJSON('/portal/employee/profile', undefined, 'GET');
    const p = data.profile || {};
    document.getElementById('profile-current').textContent =
        [p.name, p.address_line1, p.city].filter(Boolean).join(', ');
    // Prefill the ADDRESS change form with the employee's own current values so
    // an edit starts from them. defaultValue keeps form.reset() on the prefill,
    // and fields the user already edited are left alone.
    ['address_line1', 'address_line2', 'city', 'province', 'postal_code', 'country', 'phone'].forEach(f => {
        const el = document.getElementById('pc-' + f);
        if (!el) return;
        const next = p[f] || '';
        if (el.value === el.defaultValue) el.value = next;
        el.defaultValue = next;
    });
    const sel = document.getElementById('pc-contact_id');
    sel.replaceChildren();
    const blank = mk('option', 'New contact');
    blank.value = '';
    sel.appendChild(blank);
    (data.emergency_contacts || []).forEach(c => {
        const o = mk('option', c.name + ' (' + c.relationship + ')');
        o.value = c.id;
        sel.appendChild(o);
    });
}

async function loadProfileChanges() {
    const data = await postJSON('/portal/employee/profile-changes', undefined, 'GET');
    // Payload values (bank numbers etc.) are deliberately not rendered.
    renderList('profile-change-list', data.profile_changes || [],
        x => x.change_type.replace('_', ' ').toLowerCase() + ' change #' + x.id,
        x => '/portal/employee/profile-changes/' + x.id + '/cancel', loadProfileChanges);
}

async function loadHR() {
    const loaders = [loadLeave, loadOvertime, loadExpenses, loadProfile, loadProfileChanges];
    const results = await Promise.allSettled(loaders.map(fn => fn()));
    const failed = results.find(r => r.status === 'rejected');
    const note = document.getElementById('hr-unavailable');
    if (failed) {
        note.textContent = failed.reason && failed.reason.status === 401
            ? 'Your account is not linked to an employee record.'
            : 'Some employee self-service data could not be loaded: ' + (failed.reason && failed.reason.message);
        note.hidden = false;
    } else {
        note.hidden = true;
    }
}

function bindSubmit(formId, msgId, build, url, reload) {
    document.getElementById(formId).addEventListener('submit', async (e) => {
        e.preventDefault();
        const btn = e.target.querySelector('button[type="submit"]');
        btn.disabled = true;
        setMsg(msgId, 'Submitting...');
        try {
            await postJSON(url, build());
            e.target.reset();
            setMsg(msgId, 'Submitted for approval.', 'ok');
            await reload();
        } catch (err) {
            setMsg(msgId, err.message, 'error');
        } finally {
            btn.disabled = false;
        }
    });
}

bindSubmit('leaveForm', 'leave-msg', () => ({
    leave_type_id: parseInt(document.getElementById('leaveType').value, 10),
    start_date: val('leaveStart'),
    end_date: val('leaveEnd'),
    reason: val('leaveReason'),
    start_half_day: document.getElementById('leaveStartHalf').checked,
    end_half_day: document.getElementById('leaveEndHalf').checked
}), '/portal/employee/leave', loadLeave);

bindSubmit('overtimeForm', 'overtime-msg', () => ({
    worked_on: val('otDate'),
    minutes: parseInt(val('otMinutes'), 10),
    reason: val('otReason')
}), '/portal/employee/overtime', loadOvertime);

bindSubmit('expenseForm', 'expense-msg', () => {
    const body = {
        claim_date: val('exDate'),
        category: val('exCategory'),
        description: val('exDescription'),
        amount: val('exAmount')
    };
    const doc = val('exDocument');
    if (doc) body.document_id = parseInt(doc, 10);
    return body;
}, '/portal/employee/expenses', loadExpenses);

bindSubmit('profileChangeForm', 'profile-change-msg', () => {
    const type = document.getElementById('pcType').value;
    const payload = {};
    document.querySelectorAll('[data-pc-type="' + type + '"] [data-field]').forEach(inp => {
        if (inp.type === 'checkbox') {
            payload[inp.dataset.field] = inp.checked ? 'true' : 'false';
        } else if (inp.value.trim() !== '' || inp.dataset.field !== 'contact_id') {
            payload[inp.dataset.field] = inp.value.trim();
        }
    });
    return { change_type: type, payload: payload };
}, '/portal/employee/profile-changes', loadProfileChanges);

function showProfileChangeFields() {
    const type = document.getElementById('pcType').value;
    document.querySelectorAll('[data-pc-type]').forEach(n => { n.hidden = n.dataset.pcType !== type; });
}
document.getElementById('pcType').addEventListener('change', showProfileChangeFields);
['leaveStart', 'leaveEnd'].forEach(id => document.getElementById(id).addEventListener('change', updateHolidayHint));

// Navigation. Links carry data-tab (replaces the former inline onclick).
document.querySelectorAll('.nav-link[data-tab]').forEach(link => {
    link.addEventListener('click', () => switchTab(link.dataset.tab, link));
});

document.getElementById('nav-hr').addEventListener('click', (e) => {
    switchTab('hr', e.currentTarget);
    loadHR();
});

// RFQ "Revise Offer" buttons carry data-rfq-id (replaces inline onclick).
document.querySelectorAll('[data-rfq-id]').forEach(btn => {
    btn.addEventListener('click', () => negotiate(parseInt(btn.dataset.rfqId, 10)));
});

// Initial tracking
trackEvent('portal_login');
