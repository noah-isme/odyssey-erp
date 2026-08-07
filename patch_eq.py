import os, re

dirs = ["web/templates/pages/cmms", "web/templates/pages/qms", "web/templates/pages/documents"]

for d in dirs:
    if not os.path.exists(d): continue
    for f in os.listdir(d):
        if not f.endswith(".html"): continue
        path = os.path.join(d, f)
        with open(path, "r") as file:
            content = file.read()
        
        # We replace `eq ` with `eqStr ` when comparing Filters or other potential pointers
        new_content = re.sub(r' eq (\$\.Data\.Filter\.[A-Za-z0-9_]+) ', r' eqStr \1 ', content)
        new_content = re.sub(r' eq (\.Data\.Filter\.[A-Za-z0-9_]+) ', r' eqStr \1 ', new_content)
        new_content = re.sub(r' eq \. (\$\.Data\.Filter\.[A-Za-z0-9_]+)', r' eqStr . \1', new_content)
        new_content = re.sub(r' eq \. (\.Data\.Filter\.[A-Za-z0-9_]+)', r' eqStr . \1', new_content)

        # What if it's `eq .Data.Filter.Category "PREVENTIVE"` ?
        # It's caught by the first two regexes!
        
        if content != new_content:
            with open(path, "w") as file:
                file.write(new_content)
            print(f"Patched {f}")
print("Done patching eq.")
