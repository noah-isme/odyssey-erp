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

// Component modules
import { Inspector } from './components/inspector.js';
import { DataTable } from './components/datatable.js';
import { FilterBar } from './components/filterbar.js';
import { Forms } from './components/forms.js';
import { Export } from './components/export.js';
import { Charts } from './components/charts.js';
import { Progress } from './components/progress.js';

// Initialize all modules on DOMContentLoaded
document.addEventListener('DOMContentLoaded', () => {
    // Features (state-driven architecture)
    Theme.init();
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

    // Core
    Shortcuts.init();
    Loading.init();

    // Components
    Inspector.init();
    DataTable.init();
    FilterBar.init();
    Forms.init();
    Export.init();
    Charts.init();
    Progress.init();

    // Register stores with DevTools for inspection
    DevTools.register('theme', Theme);
    DevTools.register('modal', Modal);
    DevTools.register('toast', Toast);
    DevTools.register('form', Form);
    DevTools.register('combobox', ComboBox);
    DevTools.register('tabs', Tabs);
    DevTools.register('upload', Upload);
    DevTools.register('slideout', Slideout);

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

