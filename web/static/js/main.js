/**
 * Odyssey ERP - Main JavaScript Entry Point
 * Initializes all modules following state-driven architecture
 */

// Core modules
import { Loading } from './core/toast.js';
import { Shortcuts } from './core/shortcuts.js';
import { DevTools } from './core/store.js';

// Feature modules (state-driven architecture)
import { Theme } from './features/theme/index.js';
import { Sidebar, Navigation } from './features/sidebar/index.js';
import { Header } from './features/header/index.js';
import { Menu } from './features/menu/index.js';
import { Modal } from './features/modal/index.js';
import { Toast } from './features/toast/index.js';
import { Lookup } from './features/lookup/index.js';
import { DateRangePicker } from './features/datepicker/index.js';
import { TableEdit } from './features/table-edit/index.js';
import { Tabs } from './features/tabs/index.js';
import { Upload } from './features/upload/index.js';
import { Slideout } from './features/slideout/index.js';
import { Form } from './features/form/index.js';
import { ComboBox } from './features/combobox/index.js';
import * as QuotationForm from './features/quotation-form/index.js';
import * as SalesOrder from './features/sales-order/index.js';
import * as DeliveryOrder from './features/delivery-order/index.js';
import { GlobalSearch } from './features/global-search/index.js';
import { StockTake } from './features/inventory/stock-take.js';
import { Confirm } from './features/confirm/index.js';
import { APPaymentAllocation } from './features/ap-payment-allocation/index.js';

// Component modules
import { Inspector } from './components/inspector.js';
import { DataTable } from './features/datatable/index.js';
import { FilterBar } from './components/filterbar.js';
import { Forms } from './components/forms.js';
import { Export } from './components/export.js';
import { Charts } from './components/charts.js';
import { Progress } from './components/progress.js';

const FLASH_KIND_TO_TOAST_VARIANT = {
    'success': 'success',
    'error': 'error',
    'danger': 'error',
    'warning': 'warning',
    'info': 'info',
    'neutral': 'info'
};

function bridgeServerFlashToToast() {
    const flashEl = document.getElementById('server-flash');
    if (!flashEl) return;

    const message = flashEl.dataset.flashMessage;
    const kind = flashEl.dataset.flashKind;
    
    if (!message) return;

    const variant = FLASH_KIND_TO_TOAST_VARIANT[kind] || 'info';
    
    switch (variant) {
        case 'success':
            Toast.success(message);
            break;
        case 'error':
            Toast.error(message);
            break;
        case 'warning':
            Toast.warning(message);
            break;
        default:
            Toast.info(message);
    }
    
    flashEl.remove();
}

function localizeBilingualContent(language) {
    const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT, {
        acceptNode(node) {
            const parent = node.parentElement;
            if (!parent || ['SCRIPT', 'STYLE', 'TEXTAREA', 'OPTION'].includes(parent.tagName)) return NodeFilter.FILTER_REJECT;
            return node.nodeValue.includes(' / ') ? NodeFilter.FILTER_ACCEPT : NodeFilter.FILTER_REJECT;
        }
    });

    const nodes = [];
    while (walker.nextNode()) nodes.push(walker.currentNode);
    nodes.forEach((node) => {
        const parts = node.nodeValue.split(' / ');
        node.nodeValue = language === 'en' ? parts[parts.length - 1] : parts[0];
    });
}

function applyShellLanguage(language) {
    const normalized = language === 'en' ? 'en' : 'id';
    document.documentElement.lang = normalized;
    document.documentElement.dataset.uiLanguage = normalized;
    const dictionary = {
        id: { create: 'Buat Baru', profile: 'Profil', settings: 'Pengaturan', logout: 'Keluar' },
        en: { create: 'Create New', profile: 'Profile', settings: 'Settings', logout: 'Sign out' }
    };
    document.querySelectorAll('[data-i18n]').forEach((element) => {
        const value = dictionary[normalized][element.dataset.i18n];
        if (value) element.textContent = value;
    });
}

function applyInitialLanguage() {
    const language = document.documentElement.dataset.uiLanguage || 'id';
    localizeBilingualContent(language);
    applyShellLanguage(language);
    document.documentElement.dataset.uiLocalized = 'true';
}

async function applyWorkspacePreferences() {
    try {
        const response = await fetch('/api/me', { credentials: 'same-origin', headers: { Accept: 'application/json' } });
        if (!response.ok) return;

        const workspace = await response.json();
        const user = workspace.user;
        const name = user.name || user.email || 'Pengguna';
        const initial = Array.from(name.trim())[0] || 'U';
        document.querySelectorAll('[data-current-user-name]').forEach((element) => { element.textContent = name; });
        document.querySelectorAll('[data-current-user-email]').forEach((element) => { element.textContent = user.email || '—'; });
        document.querySelectorAll('[data-current-user-avatar]').forEach((element) => { element.textContent = initial.toUpperCase(); });
        document.querySelectorAll('[data-notification-control]').forEach((element) => { element.hidden = !user.notifications; });

        const companySelect = document.querySelector('[data-active-company]');
        if (companySelect) {
            companySelect.replaceChildren();
            (workspace.companies || []).forEach((company) => {
                const option = new Option(company.name, String(company.id), false, company.id === workspace.activeCompanyID);
                companySelect.add(option);
            });
            companySelect.disabled = companySelect.options.length === 0;
            companySelect.addEventListener('change', () => companySelect.form?.requestSubmit(), { once: true });
        }

        const language = user.language === 'en' ? 'en' : 'id';
        const renderedLanguage = document.documentElement.dataset.uiLanguage || 'id';
        localStorage.setItem('odyssey.language', language);
        if (language !== renderedLanguage) {
            window.location.reload();
            return;
        }
        applyShellLanguage(language);

        const preference = user.theme || 'system';
        const theme = preference === 'system'
            ? (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
            : preference;
        Theme.apply(theme);
    } catch (_) {
        // The shell remains usable with its safe default values.
    }
}

function persistLanguagePreferenceOnSubmit() {
    const form = document.querySelector('form[action="/settings"]');
    const language = form?.querySelector('[name="language"]');
    form?.addEventListener('submit', () => {
        if (language?.value === 'id' || language?.value === 'en') {
            localStorage.setItem('odyssey.language', language.value);
        }
    });
}

function csrfToken() {
    return document.querySelector('meta[name="csrf-token"]')?.content || '';
}

function escapeNotificationText(value) {
    const span = document.createElement('span');
    span.textContent = value || '';
    return span.innerHTML;
}

function safeNotificationURL(value) {
    return typeof value === 'string' && value.startsWith('/') && !value.startsWith('//') ? value : '#';
}

async function refreshNotifications() {
    const badge = document.querySelector('[data-notification-count]');
    const list = document.querySelector('[data-notification-list]');
    if (!badge || !list) return;
    const [countResponse, listResponse] = await Promise.all([
        fetch('/api/notifications/unread-count', { credentials: 'same-origin', headers: { Accept: 'application/json' } }),
        fetch('/api/notifications?limit=10', { credentials: 'same-origin', headers: { Accept: 'application/json' } })
    ]);
    if (!countResponse.ok || !listResponse.ok) return;
    const count = (await countResponse.json()).count || 0;
    const notifications = (await listResponse.json()).notifications || [];
    badge.textContent = count > 99 ? '99+' : String(count);
    badge.hidden = count === 0;
    list.innerHTML = notifications.length === 0
        ? '<p class="notification-empty">No recent notifications.</p>'
        : notifications.map((item) => `<a class="notification-item${item.readAt ? '' : ' notification-item--unread'}" href="${safeNotificationURL(item.url)}" data-notification-id="${item.id}"><strong>${escapeNotificationText(item.title)}</strong><span>${escapeNotificationText(item.body)}</span></a>`).join('');
}

function initNotifications() {
    const trigger = document.querySelector('[data-notification-trigger]');
    const dropdown = document.querySelector('[data-notification-dropdown]');
    if (!trigger || !dropdown) return;
    trigger.addEventListener('click', async () => {
        dropdown.hidden = !dropdown.hidden;
        trigger.setAttribute('aria-expanded', String(!dropdown.hidden));
        if (!dropdown.hidden) await refreshNotifications();
    });
    dropdown.addEventListener('click', async (event) => {
        const item = event.target.closest('[data-notification-id]');
        if (item) await fetch(`/api/notifications/${item.dataset.notificationId}/read`, { method: 'POST', credentials: 'same-origin', headers: { 'X-CSRF-Token': csrfToken() } });
        if (event.target.closest('[data-notification-read-all]')) {
            event.preventDefault();
            await fetch('/api/notifications/read-all', { method: 'POST', credentials: 'same-origin', headers: { 'X-CSRF-Token': csrfToken() } });
            await refreshNotifications();
        }
    });
    refreshNotifications().catch(() => {});
}

// Initialize all modules on DOMContentLoaded
document.addEventListener('DOMContentLoaded', () => {
    // Features (state-driven architecture)
    Theme.init();
    applyInitialLanguage();
    applyWorkspacePreferences();
    persistLanguagePreferenceOnSubmit();
	initNotifications();
    Sidebar.init();
    Navigation.init();
    Header.init();
    Menu.init();
    Modal.init();
    Toast.init();
    Lookup.init();
    DateRangePicker.init();
    TableEdit.init();
    Tabs.init();
    Upload.init();
    Slideout.init();
    Form.init();
    ComboBox.init();
    DataTable.init();
    QuotationForm.init();
    SalesOrder.init();
    DeliveryOrder.init();
    StockTake.init();
    Confirm.init();
    APPaymentAllocation.init();

    // Core
    Shortcuts.init();
    Loading.init();

    // Components
    Inspector.init();
    FilterBar.init();
    Forms.init();
    Export.init();
    Charts.init();
    Progress.init();
    
    bridgeServerFlashToToast();

    // Register stores with DevTools for inspection
    DevTools.register('theme', Theme);
    DevTools.register('modal', Modal);
    DevTools.register('toast', Toast);
    DevTools.register('form', Form);
    DevTools.register('combobox', ComboBox);
    DevTools.register('tabs', Tabs);
    DevTools.register('upload', Upload);
    DevTools.register('slideout', Slideout);
    DevTools.register('datatable', DataTable);

    // Expose globally for inline usage
    window.OdysseyToast = Toast;
    window.OdysseyLoading = Loading;
    window.OdysseyModal = Modal;
    window.OdysseyMenu = Menu;
    window.OdysseyInspector = Inspector;
    window.OdysseyDataTable = DataTable;
    window.OdysseyTheme = Theme;
    window.OdysseySidebar = Sidebar;
    window.OdysseyHeader = Header;
    window.OdysseyLookup = Lookup;
    window.OdysseyDateRangePicker = DateRangePicker;
    window.OdysseyTableEdit = TableEdit;
    window.OdysseyExport = Export;
    window.OdysseyCharts = Charts;
    window.OdysseyTabs = Tabs;
    window.OdysseyUpload = Upload;
    window.OdysseySlideout = Slideout;
    window.OdysseyProgress = Progress;
    window.OdysseyForm = Form;
    window.OdysseyComboBox = ComboBox;

    console.log('🚀 Odyssey ERP initialized');
    console.log('💡 Tip: Run OdysseyDevTools.enable() for debug mode');
});
