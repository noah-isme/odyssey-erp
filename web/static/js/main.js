/**
 * Odyssey ERP - Main JavaScript Entry Point
 * Initializes all modules
 */

// Core modules
import { Sidebar, Navigation } from './core/sidebar.js';
import { Toast, Loading } from './core/toast.js';
import { Dropdown } from './core/dropdown.js';
import { Shortcuts } from './core/shortcuts.js';

// Feature modules (state-driven architecture)
import { Theme } from './features/theme/index.js';

// Component modules
import { Modal } from './components/modal.js';
import { Inspector } from './components/inspector.js';
import { DataTable } from './components/datatable.js';
import { FilterBar } from './components/filterbar.js';
import { Forms } from './components/forms.js';

// Initialize all modules on DOMContentLoaded
document.addEventListener('DOMContentLoaded', () => {
    // Features (state-driven)
    Theme.init();

    // Core
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
    window.OdysseyTheme = Theme; // Expose theme for external access

    console.log('🚀 Odyssey ERP initialized');
});
