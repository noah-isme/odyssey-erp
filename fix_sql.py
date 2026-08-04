import re

with open("migrations/000081_manufacturing_governance.up.sql", "r") as f:
    lines = f.readlines()

new_lines = []
indexes_to_create = []

table_name = ""
table_name_re = re.compile(r"CREATE\s+TABLE\s+(\w+)\s*\(")

for i, line in enumerate(lines):
    m = table_name_re.search(line)
    if m:
        table_name = m.group(1)
        
    if "INDEX idx_" in line and not line.strip().startswith("CREATE"):
        # Extract the index name and columns
        idx_match = re.search(r"INDEX\s+(idx_\w+)\s*\((.*?)\)", line)
        if idx_match:
            idx_name = idx_match.group(1)
            columns = idx_match.group(2)
            indexes_to_create.append(f"CREATE INDEX {idx_name} ON {table_name}({columns});\n")
            
        # If it has a trailing comma on the previous line, we might need to remove it?
        # Actually it's easier to just replace this line with a comment and fix trailing commas later
        pass # Skip adding this line to new_lines
    elif line.strip() == ");" or line.strip() == ")":
        # Check if the previous line ended with a comma
        if len(new_lines) > 0:
            new_lines[-1] = new_lines[-1].rstrip().rstrip(",") + "\n"
        new_lines.append(line)
        
        # Add collected indexes
        if indexes_to_create:
            new_lines.extend(indexes_to_create)
            indexes_to_create = []
    else:
        new_lines.append(line)

with open("migrations/000081_manufacturing_governance.up.sql", "w") as f:
    f.writelines(new_lines)
