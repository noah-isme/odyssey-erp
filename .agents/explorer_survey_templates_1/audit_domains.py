import os
import re

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
            issues.append((idx, 'INLINE_STYLE', line.strip()))

        # 2. Hardcoded hex colors
        hex_matches = re.findall(r'#(?:[0-9a-fA-F]{3}){1,2}\b', line)
        if hex_matches and not rel_path.startswith("reports/"): # skip report pdfs if hex is intentional, but report in pages
            issues.append((idx, 'HEX_COLOR', f"Hex {hex_matches}: {line.strip()}"))
        elif hex_matches and rel_path.startswith("pages/"):
            issues.append((idx, 'HEX_COLOR', f"Hex {hex_matches}: {line.strip()}"))

        # 3. Soft shadow / non-standard radius
        shadow_matches = re.findall(r'\bshadow-(?:sm|md|lg|xl|2xl)\b', line)
        if shadow_matches:
            issues.append((idx, 'SOFT_SHADOW', f"{shadow_matches}: {line.strip()}"))

        rounded_matches = re.findall(r'\brounded-(?:md|lg|xl|2xl|3xl)\b', line)
        if rounded_matches:
            issues.append((idx, 'ROUNDED_RADIUS', f"{rounded_matches}: {line.strip()}"))

        # 4. Icon bubble patterns (e.g. pastel backgrounds bg-*-50, bg-*-100, bg-surface-muted with rounded-full)
        if re.search(r'bg-(?:blue|indigo|purple|emerald|green|amber|red|sky|gray)-(?:50|100)', line):
            issues.append((idx, 'PASTEL_BUBBLE_BG', line.strip()))

        # 5. Non-standard state indicator / badge
        if '<span' in line or '<div' in line:
            if re.search(r'\b(badge|status|state|tag)\b', line, re.IGNORECASE):
                if not re.search(r'\b(sys-badge|status-badge|badge--|badge__dot)\b', line):
                    # Check if it looks like a badge element
                    if 'class=' in line and ('badge' in line or 'status' in line):
                        issues.append((idx, 'NON_STANDARD_BADGE', line.strip()))

        # 6. Table cell numeric / code checks
        if '<td' in line:
            # Check if td contains currency formatting, numbers, dates, reference codes, totals, quantities
            # Look for template expressions inside <td> ... </td> or starting line
            cell_has_numeric_or_code = re.search(r'\{\{\s*(?:formatMoney|formatNumber|formatAmount|formatCurrency|formatQty|formatQuantity|formatDate|formatDateTime|\.(?:[A-Za-z0-9_]*(?:Amount|Price|Total|Subtotal|Tax|Balance|Qty|Quantity|OnHand|Allocated|Available|Code|Number|No|SKU|Cost|Rate|Debit|Credit|Date|CreatedAt|UpdatedAt)))\b', line)
            
            if cell_has_numeric_or_code:
                # check if class contains font-mono, numeric, numeric-right, sys-badge, status-badge
                if not re.search(r'\b(font-mono|numeric|numeric-right|sys-badge|status-badge)\b', line):
                    issues.append((idx, 'TABLE_NUMERIC_MISSING_MONO', line.strip()))

    return rel_path, issues

domain_reports = {}

for domain_name, paths in domains.items():
    if isinstance(paths, str):
        paths = [paths]
    
    domain_issues = {}
    for path_entry in paths:
        full_p = os.path.join(TEMPLATE_DIR, path_entry)
        if os.path.isfile(full_p):
            rel_p, iss = audit_file_deep(full_p)
            if iss:
                domain_issues[rel_p] = iss
        elif os.path.isdir(full_p):
            for root, dirs, files in os.walk(full_p):
                for f in files:
                    if f.endswith('.html'):
                        fp = os.path.join(root, f)
                        rel_p, iss = audit_file_deep(fp)
                        if iss:
                            domain_issues[rel_p] = iss
    domain_reports[domain_name] = domain_issues

for dom, file_dict in domain_reports.items():
    total_iss = sum(len(v) for v in file_dict.values())
    print(f"\n================================================================================")
    print(f"DOMAIN: {dom} ({len(file_dict)} files with issues, {total_iss} total issues)")
    print(f"================================================================================")
    for rel_p, iss in sorted(file_dict.items()):
        print(f"\n  File: {rel_p} ({len(iss)} issues)")
        for lno, cat, det in iss:
            print(f"    Line {lno:4d} [{cat}]: {det[:110]}")
