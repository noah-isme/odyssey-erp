(function() {
    function formatCurrency(amount) {
        if (amount >= 1000000000) return 'Rp ' + (amount / 1000000000).toFixed(1) + ' M';
        if (amount >= 1000000) return 'Rp ' + (amount / 1000000).toFixed(0) + ' jt';
        if (amount >= 1000) return 'Rp ' + (amount / 1000).toFixed(0) + ' rb';
        return 'Rp ' + amount.toFixed(0);
    }

    fetch('/api/dashboard/kpis')
        .then(r => r.json())
        .then(data => {
            document.getElementById('kpi-open-so').textContent = data.open_sales_orders || 0;
            const overdueEl = document.getElementById('kpi-overdue-so');
            if (data.overdue_so > 0) {
                overdueEl.textContent = data.overdue_so + ' overdue';
            }
            document.getElementById('kpi-pending-do').textContent = data.pending_deliveries || 0;
            document.getElementById('kpi-ap-due').textContent = formatCurrency(data.ap_due_this_week || 0);
            document.getElementById('kpi-low-stock').textContent = data.low_stock_items || 0;
        })
        .catch(err => console.error('Failed to load KPIs:', err));

    const activityContainer = document.getElementById('recent-activity');
    if (activityContainer) {
        fetch('/api/dashboard/activity')
            .then(r => r.json())
            .then(activities => {
                if (!activities || activities.length === 0) return;
                activityContainer.innerHTML = activities.map(a => `
                    <li class="pending-item">
                        <div class="pending-icon">
                            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                <circle cx="12" cy="12" r="10"/>
                                <polyline points="12 6 12 12 16 14"/>
                            </svg>
                        </div>
                        <div class="pending-content">
                            <span class="pending-text">${a.actor} ${a.action} ${a.entity}</span>
                            <small style="color: var(--text-muted)">${new Date(a.at).toLocaleString()}</small>
                        </div>
                    </li>
                `).join('');
            })
            .catch(err => console.error('Failed to load activity:', err));
    }
})();
