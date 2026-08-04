import os

directories = [
    "cmms",
    "qms",
    "documents"
]

files = {
    "cmms": [
        "work_orders.html",
        "work_order_new.html",
        "work_order_detail.html",
        "assets.html",
        "asset_new.html",
        "asset_detail.html",
        "pm_schedules.html",
        "pm_schedule_new.html",
        "pm_schedule_detail.html",
        "spare_parts.html",
        "spare_part_new.html",
    ],
    "qms": [
        "ncrs.html",
        "ncr_new.html",
        "ncr_detail.html",
        "capas.html",
        "capa_new.html",
        "capa_detail.html",
        "audits.html",
        "audit_new.html",
        "audit_detail.html",
        "supplier_quality.html",
        "supplier_quality_new.html",
        "supplier_quality_detail.html",
    ],
    "documents": [
        "library.html",
        "document_new.html",
        "document_detail.html",
        "versions.html",
        "categories.html",
        "category_new.html",
        "classifications.html",
    ]
}

template_content = """{{ define "content" }}
<div class="px-4 sm:px-6 lg:px-8 py-8 w-full max-w-9xl mx-auto">
    <div class="sm:flex sm:justify-between sm:items-center mb-8">
        <div class="mb-4 sm:mb-0">
            <h1 class="text-2xl md:text-3xl text-slate-800 dark:text-slate-100 font-bold">{{ .Title }}</h1>
        </div>
    </div>
    
    <div class="bg-white dark:bg-slate-800 shadow-lg rounded-sm border border-slate-200 dark:border-slate-700 p-6">
        <p class="text-slate-500 dark:text-slate-400">This module component is under construction.</p>
    </div>
</div>
{{ end }}
"""

base_dir = "/home/noah/project/odyssey-erp/web/templates/pages"

for dir_name in directories:
    dir_path = os.path.join(base_dir, dir_name)
    os.makedirs(dir_path, exist_ok=True)
    
    for file_name in files[dir_name]:
        file_path = os.path.join(dir_path, file_name)
        with open(file_path, "w") as f:
            f.write(template_content)

print("Generated all missing templates successfully.")
