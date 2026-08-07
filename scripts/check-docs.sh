#!/usr/bin/env bash
set -euo pipefail

root_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root_dir"

status=0
doc_files=(README.md QUICK_REFERENCE.md AGENTS.md DESIGN.md)
while IFS= read -r -d '' file; do
  doc_files+=("$file")
done < <(find docs -path docs/archive -prune -o \( -name '*.md' -o -name '*.txt' \) -print0)

for file in "${doc_files[@]}"; do
  [[ -f "$file" ]] || continue
  while IFS= read -r match; do
    target=${match:2}
    target=${target#<}
    target=${target%>}
    [[ "$target" == http://* || "$target" == https://* || "$target" == mailto:* ]] && continue
    if [[ ! -e "$(dirname "$file")/$target" ]]; then
      printf 'broken local link: %s -> %s\n' "$file" "$target" >&2
      status=1
    fi
  done < <(grep -E -o '\]\([^#)]+' "$file" || true)
done

code_ref_pattern='`(cmd|internal|web|scripts|migrations|sql|deploy|tests|testing|report|jobs|docs)/[^`[:space:],;)]+'
for file in "${doc_files[@]}"; do
  [[ -f "$file" ]] || continue
  [[ "$file" == *"PLAN.md" || "$file" == *"MAP.md" || "$file" == *"SUMMARY.md" ]] && continue
  while IFS= read -r reference; do
    target=${reference#\`}
    target=${target%\`}
    target=${target%%:*}
    target=${target%/...}
    [[ -z "$target" || "$target" == *'<'* || "$target" == *'{'* || "$target" == *'*'* || "$target" == *'?'* ]] && continue
    if [[ ! -e "$target" ]]; then
      printf 'unknown repository path: %s -> %s\n' "$file" "$target" >&2
      status=1
    fi
  done < <(grep -E -o "$code_ref_pattern" "$file" || true)
done

mapfile -t make_targets < <(sed -nE 's/^([A-Za-z0-9_.-]+):.*/\1/p' Makefile | sort -u)
for file in "${doc_files[@]}"; do
  [[ -f "$file" ]] || continue
  while IFS= read -r reference; do
    target=${reference#make }
    if ! printf '%s\n' "${make_targets[@]}" | grep -q -x "$target"; then
      printf 'unknown Make target: %s -> %s\n' "$file" "$target" >&2
      status=1
    fi
  done < <(grep -E -o '\bmake [A-Za-z0-9_.-]+' "$file" | sort -u || true)
done

exit "$status"
