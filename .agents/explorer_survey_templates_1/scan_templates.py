import os
import re

TEMPLATE_DIR = "/home/noah/project/odyssey-erp/web/templates"

def audit_file(filepath):
    rel_path = os.path.relpath(filepath, TEMPLATE_DIR)
    issues = []
    with open(filepath, 'r', encoding='utf-8', errors='ignore') as f:
        lines = f.readlines()

    for idx, line in enumerate(lines, start=1):
        # 1. Check inline styles
        if 'style=' in line:
            issues.append((idx, 'INLINE_STYLE', line.strip()))

        # 2. Check hardcoded hex colors
        hex_matches = re.findall(r'#(?:[0-9a-fA-F]{3}){1,2}\b', line)
        if hex_matches:
            issues.append((idx, 'HEX_COLOR', f"Matches: {hex_matches} in: {line.strip()}"))

        # 3. Check soft rounded / shadow classes
        shadow_matches = re.findall(r'\bshadow-(?:sm|md|lg|xl|2xl)\b', line)
        if shadow_matches:
            issues.append((idx, 'SOFT_SHADOW', f"Matches: {shadow_matches} in: {line.strip()}"))
            
        rounded_matches = re.findall(r'\brounded-(?:sm|md|lg|xl|2xl|3xl|full)\b', line)
        if rounded_matches:
            issues.append((idx, 'ROUNDED_RADIUS', f"Matches: {rounded_matches} in: {line.strip()}"))

        # 4. Check status badge / state indicators missing .sys-badge / .status-badge
        # e.g., custom badges like <span class="badge ..."> without status-badge or sys-badge
        # or <span class="bg-green... text-green...">
        if re.search(r'class="[^"]*\bbadge\b', line) and not re.search(r'\b(sys-badge|status-badge|badge--)\b', line):
            issues.append((idx, 'NON_STANDARD_BADGE', line.strip()))

        # 5. Check numeric formatting: td containing numeric Go template vars, amount, qty, price, code, total, balance, etc. without .font-mono or .numeric
        if '<td' in line:
            # check if line or next line contains numeric template variables
            has_numeric_var = re.search(r'\{\{\s*\.(?:Total|Amount|Price|UnitPrice|Subtotal|TaxAmount|GrandTotal|Balance|Qty|Quantity|OnHand|Allocated|Available|Code|OrderNumber|DocNo|InvoiceNumber|PaymentNumber|PONumber|PRNumber|GRNNumber|JournalNumber|QuotationNumber|Number|SKU|Cost|Rate|Debit|Credit)\b', line, re.IGNORECASE)
            has_format_call = re.search(r'\{\{\s*(?:formatMoney|formatNumber|formatAmount|formatCurrency|formatQty|formatQuantity|formatDate|formatDateTime)\b', line)
            if (has_numeric_var or has_format_call) and not re.search(r'\b(font-mono|numeric|numeric-right|sys-badge|text-right)\b', line):
                issues.append((idx, 'MISSING_NUMERIC_MONO', line.strip()))

    return rel_path, issues

all_results = {}
for root, dirs, files in os.walk(TEMPLATE_DIR):
    for f in files:
        if f.endswith('.html'):
            full_path = os.path.join(root, f)
            rel_path, issues = audit_file(full_path)
            if issues:
                all_results[rel_path] = issues

print(f"Total files audited: {sum(len(files) for _, _, files in os.walk(TEMPLATE_DIR) if any(f.endswith('.html') for f in files))}")
print(f"Files with potential issues: {len(all_results)}")
print("=" * 80)
for rel_path, issues in sorted(all_results.items()):
    print(f"\n--- {rel_path} ({len(issues)} issues) ---")
    for line_no, category, detail in issues:
        print(f"  Line {line_no:4d} [{category}]: {detail[:120]}")
