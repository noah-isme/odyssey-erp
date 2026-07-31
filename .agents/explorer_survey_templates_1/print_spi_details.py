import json

with open("/home/noah/project/odyssey-erp/.agents/explorer_survey_templates_1/audit_results.json", "r") as f:
    data = json.load(f)

for dom in ["Sales", "Procurement", "Inventory"]:
    files = data.get(dom, [])
    print(f"\n==================================================")
    print(f"DOMAIN: {dom}")
    print(f"Total Files: {len(files)}")
    dirty_files = [f for f in files if len(f['issues']) > 0]
    print(f"Files with Issues ({len(dirty_files)}):")
    for df in dirty_files:
        print(f"  - {df['file']} ({len(df['issues'])} issues):")
        for iss in df['issues']:
            print(f"      L{iss['line']} [{iss['type']}]: {iss['content'][:110]}")
