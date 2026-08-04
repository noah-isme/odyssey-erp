import os

dirs = ["web/templates/pages/cmms", "web/templates/pages/qms", "web/templates/pages/documents"]

for d in dirs:
    if not os.path.exists(d):
        continue
    for f in os.listdir(d):
        if not f.endswith(".html"):
            continue
        path = os.path.join(d, f)
        with open(path, "r") as file:
            content = file.read()
        
        # Check if already defined
        module = os.path.basename(d)
        define_str = f'{{{{ define "pages/{module}/{f}" }}}}'
        if define_str in content:
            continue
            
        title = f.replace(".html", "").replace("_", " ").title()
        
        prefix = f"""{define_str}
{{{{ template "layouts/base.html" . }}}}
{{{{ end }}}}

{{{{ define "title" }}}}{title}{{{{ end }}}}

"""
        
        with open(path, "w") as file:
            file.write(prefix + content)
            
print("Templates fixed!")
