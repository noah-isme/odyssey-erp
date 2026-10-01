/**
 * Odyssey ERP - HR Module Client Scripts
 * Handles modal prefilling and interactions
 */

document.addEventListener('DOMContentLoaded', () => {
    // -------------------------------------------------------------------------
    // Edit Employee Modal Prefill
    // -------------------------------------------------------------------------
    document.querySelectorAll('[data-edit-employee]').forEach(btn => {
        btn.addEventListener('click', (e) => {
            const tr = e.target.closest('tr');
            if (!tr) return;

            const id = tr.dataset.id;
            const number = tr.dataset.number || '';
            const name = tr.dataset.name || '';
            const email = tr.dataset.email || '';
            const dept = tr.dataset.deptId || '';
            const pos = tr.dataset.posId || '';
            const mgr = tr.dataset.mgrId || '';
            const user = tr.dataset.userId || '';
            const hire = tr.dataset.hireDate || '';
            const status = tr.dataset.status || 'ACTIVE';

            const form = document.getElementById('edit-employee-form');
            if (form) {
                form.action = `/hr/employees/${id}/update`;
                const setVal = (name, val) => {
                    const el = form.querySelector(`[name="${name}"]`);
                    if (el) el.value = val;
                };
                setVal('employee_number', number);
                setVal('name', name);
                setVal('email', email);
                setVal('department_id', dept);
                setVal('position_id', pos);
                setVal('manager_id', mgr);
                setVal('user_id', user);
                setVal('hire_date', hire);
                setVal('status', status);
            }
        });
    });

    // -------------------------------------------------------------------------
    // Quick Attendance Log for Employee
    // -------------------------------------------------------------------------
    document.querySelectorAll('[data-record-emp-attendance]').forEach(btn => {
        btn.addEventListener('click', (e) => {
            const empId = btn.dataset.empId;
            const date = btn.dataset.date;
            const form = document.getElementById('manual-attendance-form');
            if (form) {
                if (empId) {
                    const empSelect = form.querySelector('[name="employee_id"]');
                    if (empSelect) empSelect.value = empId;
                }
                if (date) {
                    const dateInput = form.querySelector('[name="date"]');
                    if (dateInput) dateInput.value = date;
                }
            }
        });
    });
});
