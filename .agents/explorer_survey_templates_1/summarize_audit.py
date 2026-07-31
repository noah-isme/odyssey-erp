import json

with open("/home/noah/project/odyssey-erp/.agents/explorer_survey_templates_1/audit_results.json", "r") as f:
    data = json.load(f)

for domain, files in data.items():
    print(f"\n==================================================")
    print(f"DOMAIN: {domain}")
    print(f"Total Files in Domain: {len(files)}")
    clean_files = [f['file'] for f in files if len(f['issues']) == 0]
    dirty_files = [f for f in files if len(f['issues']) > 0]
    print(f"Clean Files ({len(clean_files)}): {clean_files}")
    print(f"Files with Issues ({len(dirty_files)}):")
    for df in dirty_files:
        print(f"  - {df['file']} ({len(df['issues'])} issues):")
        for iss in df['issues']:
            print(f"      L{iss['line']} [{iss['type']}]: {iss['content'][:100]}")
