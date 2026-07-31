import os
import re
import json

TEMPLATE_DIR = "/home/noah/project/odyssey-erp/web/templates"

domains = {
    "Sales": "pages/sales",
    "Procurement": "pages/procurement",
    "Inventory": "pages/inventory",
    "Accounting & Finance": ["pages/accounting", "pages/ap", "pages/ar", "pages/finance", "pages/close", "pages/eliminations", "pages/variance", "pages/boardpacks"],
    "Auth / Login": ["pages/login.html"],
    "Master Data": "pages/masterdata",
    "Delivery": "pages/delivery",
    "Roles & Permissions": ["pages/roles", "pages/permissions", "pages/users"],
    "Other Pages": ["pages/home.html", "pages/landing.html", "pages/profile.html", "pages/settings.html", "pages/jobs"],
    "Partials": "partials",
    "Layouts": "layouts",
    "Reports": "reports"
}

def audit_file_deep(filepath):
    rel_path = os.path.relpath(filepath, TEMPLATE_DIR)
    issues = []
    with open(filepath, 'r', encoding='utf-8', errors='ignore') as f:
        lines = f.readlines()

    for idx, line in enumerate(lines, start=1):
        # 1. Inline styles
        if 'style=' in line:
            issues.append({'line': idx, 'type': 'INLINE_STYLE', 'content': line.strip()})

        # 2. Hardcoded hex colors
        hex_matches = re.findall(r'#(?:[0-9a-fA-F]{3}){1,2}\b', line)
        if hex_matches and not rel_path.startswith("reports/"):
            issues.append({'line': idx, 'type': 'HEX_COLOR', 'matches': hex_matches, 'content': line.strip()})
        elif hex_matches and rel_path.startswith("pages/"):
            issues.append({'line': idx, 'type': 'HEX_COLOR', 'matches': hex_matches, 'content': line.strip()})

        # 3. Soft shadow / non-standard radius
        shadow_matches = re.findall(r'\bshadow-(?:sm|md|lg|xl|2xl)\b', line)
        if shadow_matches:
            issues.append({'line': idx, 'type': 'SOFT_SHADOW', 'matches': shadow_matches, 'content': line.strip()})

        rounded_matches = re.findall(r'\brounded-(?:md|lg|xl|2xl|3xl)\b', line)
        if rounded_matches:
            issues.append({'line': idx, 'type': 'ROUNDED_RADIUS', 'matches': rounded_matches, 'content': line.strip()})

        # 4. Icon bubble patterns / pastel backgrounds
        if re.search(r'bg-(?:blue|indigo|purple|emerald|green|amber|red|sky|gray)-(?:50|100)', line):
            issues.append({'line': idx, 'type': 'PASTEL_BUBBLE_BG', 'content': line.strip()})

        # 5. Non-standard state indicator / badge
        if ('<span' in line or '<div' in line) and ('badge' in line or 'status' in line):
            if not re.search(r'\b(sys-badge|status-badge|badge--|badge__dot)\b', line):
                issues.append({'line': idx, 'type': 'NON_STANDARD_BADGE', 'content': line.strip()})

        # 6. Table cell numeric / code checks
        if '<td' in line:
            cell_has_numeric_or_code = re.search(r'\{\{\s*(?:formatMoney|formatNumber|formatAmount|formatCurrency|formatQty|formatQuantity|formatDate|formatDateTime|\.(?:[A-Za-z0-9_]*(?:Amount|Price|Total|Subtotal|Tax|Balance|Qty|Quantity|OnHand|Allocated|Available|Code|Number|No|SKU|Cost|Rate|Debit|Credit|Date|CreatedAt|UpdatedAt)))\b', line)
            if cell_has_numeric_or_code:
                if not re.search(r'\b(font-mono|numeric|numeric-right|sys-badge|status-badge)\b', line):
                    issues.append({'line': idx, 'type': 'TABLE_NUMERIC_MISSING_MONO', 'content': line.strip()})

    return rel_path, issues

report_data = {}
all_files = {}

for domain_name, paths in domains.items():
    if isinstance(paths, str):
        paths = [paths]
    
    domain_files = []
    for path_entry in paths:
        full_p = os.path.join(TEMPLATE_DIR, path_entry)
        if os.path.isfile(full_p):
            rel_p, iss = audit_file_deep(full_p)
            domain_files.append({'file': rel_p, 'issues': iss})
        elif os.path.isdir(full_p):
            for root, dirs, files in os.walk(full_p):
                for f in files:
                    if f.endswith('.html'):
                        fp = os.path.join(root, f)
                        rel_p, iss = audit_file_deep(fp)
                        domain_files.append({'file': rel_p, 'issues': iss})
    report_data[domain_name] = domain_files

with open("/home/noah/project/odyssey-erp/.agents/explorer_survey_templates_1/audit_results.json", "w") as out:
    json.dump(report_data, out, indent=2)

print("Saved audit results to audit_results.json")
