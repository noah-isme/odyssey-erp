/**
 * Odyssey ERP - Main JavaScript Entry Point
 * Initializes all modules
 */

// Core modules
import { Sidebar, Navigation } from './core/sidebar.js';
import { Toast, Loading } from './core/toast.js';
import { Dropdown } from './core/dropdown.js';
import { Shortcuts } from './core/shortcuts.js';
import { Theme } from './core/theme.js';

// Component modules
import { Modal } from './components/modal.js';
import { Inspector } from './components/inspector.js';
import { DataTable } from './components/datatable.js';
import { FilterBar } from './components/filterbar.js';
import { Forms } from './components/forms.js';

// Initialize all modules on DOMContentLoaded
document.addEventListener('DOMContentLoaded', () => {
    // Core
    Theme.init();  // Theme first for no flash
    Sidebar.init();
    Navigation.init();
    Toast.init();
    Dropdown.init();
    Shortcuts.init();
    Loading.init();

    // Components
    Modal.init();
    Inspector.init();
    DataTable.init();
    FilterBar.init();
    Forms.init();

    // Expose globally for inline usage
    window.OdysseyToast = Toast;
    window.OdysseyLoading = Loading;
    window.OdysseyModal = Modal;
    window.OdysseyInspector = Inspector;
    window.OdysseyDataTable = DataTable;
    window.OdysseyTheme = Theme;

    console.log('🚀 Odyssey ERP initialized');
});
