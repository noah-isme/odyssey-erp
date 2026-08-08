import os
import glob

def fix():
    for filepath in glob.glob("internal/connectors/providers/*/*.go"):
        with open(filepath, "r") as f:
            content = f.read()

        lines = content.split('\n')
        out_lines = []
        for line in lines:
            if 'signature := headers["X-Provider-Signature"]' in line:
                line = line.replace('signature := headers["X-Provider-Signature"]', '_ = headers["X-Provider-Signature"]; signature := headers["X-Provider-Signature"]; _ = signature')
            if 'providerEventID := headers["X-Provider-Event-Id"]' in line:
                line = line.replace('providerEventID := headers["X-Provider-Event-Id"]', '_ = headers["X-Provider-Event-Id"]; providerEventID := headers["X-Provider-Event-Id"]; _ = providerEventID')
            out_lines.append(line)

        with open(filepath, "w") as f:
            f.write('\n'.join(out_lines))
            print(f"Fixed {filepath}")

fix()
