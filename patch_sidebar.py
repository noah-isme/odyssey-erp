import re

with open("web/templates/partials/sidebar.html", "r") as f:
    content = f.read()

mrp_html = """
            <details class="sidebar-menu" data-sidebar-menu{{ if or (isActive .CurrentPath "/mrp/dispatch") (isActive .CurrentPath "/mrp/scheduling") (isActive .CurrentPath "/mrp/exceptions") (isActive .CurrentPath "/mrp/analytics") (isActive .CurrentPath "/mrp/quality") (isActive .CurrentPath "/mrp/wip-locations/manage") }} open{{ end }}>
                <summary data-tooltip="Manufaktur"><span class="nav-item-icon">M</span><span class="nav-item-text">Manufaktur (MRP)</span><span class="sidebar-menu__arrow">⌄</span></summary>
                <div class="sidebar-menu__items">
                    <a href="/mrp/dispatch" class="nav-item{{ if isActive .CurrentPath "/mrp/dispatch" }} active{{ end }}" data-tooltip="Dispatch"><span class="nav-item-text">Dispatch Board</span></a>
                    <a href="/mrp/scheduling" class="nav-item{{ if isActive .CurrentPath "/mrp/scheduling" }} active{{ end }}" data-tooltip="Scheduling"><span class="nav-item-text">Scheduling</span></a>
                    <a href="/mrp/exceptions" class="nav-item{{ if isActive .CurrentPath "/mrp/exceptions" }} active{{ end }}" data-tooltip="Exceptions"><span class="nav-item-text">Exception Workbench</span></a>
                    <a href="/mrp/quality" class="nav-item{{ if isActive .CurrentPath "/mrp/quality" }} active{{ end }}" data-tooltip="Quality"><span class="nav-item-text">Quality</span></a>
                    <a href="/mrp/wip-locations/manage" class="nav-item{{ if isActive .CurrentPath "/mrp/wip-locations/manage" }} active{{ end }}" data-tooltip="WIP Locations"><span class="nav-item-text">WIP Locations</span></a>
                    <a href="/mrp/analytics" class="nav-item{{ if isActive .CurrentPath "/mrp/analytics" }} active{{ end }}" data-tooltip="Analytics"><span class="nav-item-text">Analytics</span></a>
                </div>
            </details>
"""

cmms_documents_html = """
            <details class="sidebar-menu" data-sidebar-menu{{ if or (isActive .CurrentPath "/cmms/assets") (isActive .CurrentPath "/cmms/work-orders") (isActive .CurrentPath "/cmms/pm-schedules") (isActive .CurrentPath "/cmms/spare-parts") }} open{{ end }}>
                <summary data-tooltip="Maintenance (CMMS)"><span class="nav-item-icon">🛠</span><span class="nav-item-text">Maintenance</span><span class="sidebar-menu__arrow">⌄</span></summary>
                <div class="sidebar-menu__items">
                    <a href="/cmms/assets" class="nav-item{{ if isActive .CurrentPath "/cmms/assets" }} active{{ end }}" data-tooltip="Assets"><span class="nav-item-text">Assets</span></a>
                    <a href="/cmms/work-orders" class="nav-item{{ if isActive .CurrentPath "/cmms/work-orders" }} active{{ end }}" data-tooltip="Work Orders"><span class="nav-item-text">Work Orders</span></a>
                    <a href="/cmms/pm-schedules" class="nav-item{{ if isActive .CurrentPath "/cmms/pm-schedules" }} active{{ end }}" data-tooltip="PM Schedules"><span class="nav-item-text">PM Schedules</span></a>
                    <a href="/cmms/spare-parts" class="nav-item{{ if isActive .CurrentPath "/cmms/spare-parts" }} active{{ end }}" data-tooltip="Spare Parts"><span class="nav-item-text">Spare Parts</span></a>
                </div>
            </details>

            <details class="sidebar-menu" data-sidebar-menu{{ if or (isActive .CurrentPath "/documents/library") (isActive .CurrentPath "/documents/categories") (isActive .CurrentPath "/documents/classifications") }} open{{ end }}>
                <summary data-tooltip="Dokumen"><span class="nav-item-icon">📄</span><span class="nav-item-text">Dokumen</span><span class="sidebar-menu__arrow">⌄</span></summary>
                <div class="sidebar-menu__items">
                    <a href="/documents/library" class="nav-item{{ if isActive .CurrentPath "/documents/library" }} active{{ end }}" data-tooltip="Library"><span class="nav-item-text">Library</span></a>
                    <a href="/documents/categories" class="nav-item{{ if isActive .CurrentPath "/documents/categories" }} active{{ end }}" data-tooltip="Kategori"><span class="nav-item-text">Kategori</span></a>
                    <a href="/documents/classifications" class="nav-item{{ if isActive .CurrentPath "/documents/classifications" }} active{{ end }}" data-tooltip="Klasifikasi"><span class="nav-item-text">Klasifikasi</span></a>
                </div>
            </details>
"""

# Insert MRP before QMS
content = content.replace('<summary data-tooltip="Quality (QMS)">', mrp_html.lstrip('\n') + '\n            <summary data-tooltip="Quality (QMS)">')

# Insert CMMS and Documents after QMS details block
qms_block_end = content.find('</details>', content.find('Quality (QMS)')) + len('</details>')
content = content[:qms_block_end] + '\n' + cmms_documents_html + content[qms_block_end:]

with open("web/templates/partials/sidebar.html", "w") as f:
    f.write(content)

print("sidebar.html patched")
