/**
 * Odyssey ERP - Main JavaScript Entry Point
 * Initializes all modules following state-driven architecture
 */

// Core modules (legacy - to be migrated)
import { Toast, Loading } from './core/toast.js';
import { Shortcuts } from './core/shortcuts.js';

// Feature modules (state-driven architecture)
import { Theme } from './features/theme/index.js';
import { Sidebar, Navigation } from './features/sidebar/index.js';
import { Header } from './features/header/index.js';

// Component modules
import { Modal } from './components/modal.js';
import { Inspector } from './components/inspector.js';
import { DataTable } from './components/datatable.js';
import { FilterBar } from './components/filterbar.js';
import { Forms } from './components/forms.js';

// Initialize all modules on DOMContentLoaded
document.addEventListener('DOMContentLoaded', () => {
    // Features (state-driven architecture)
    Theme.init();
    Sidebar.init();
    Navigation.init();
    Header.init();

    // Core (legacy)
    Toast.init();
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
    window.OdysseySidebar = Sidebar;
    window.OdysseyHeader = Header;

    console.log('🚀 Odyssey ERP initialized');
});
