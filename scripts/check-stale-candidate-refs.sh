#!/usr/bin/env bash
# check-stale-candidate-refs.sh - fail when docs/ or the root handoff file
# NEXT_STEPS.md still names a superseded release candidate outside an
# explicit allowlist.
#
# Usage: scripts/check-stale-candidate-refs.sh [--root DIR] [--allowlist FILE] [--pattern ERE]
#
# Defaults: --root is the repository root, --allowlist is
# scripts/check-stale-candidate-refs.allowlist, and --pattern matches the
# superseded v0.10.0-rc.8 candidate ('rc\.8|20cc13a'). Every Markdown/text
# file under <root>/docs, plus NEXT_STEPS.md at the root when it exists (it
# carries current-status text outside docs/), is scanned; each matching line
# must be covered by an allowlist rule. Allowlist paths are relative to the
# root (for example 'NEXT_STEPS.md'). Allowlist rules, one per line (blank
# lines and lines starting with '#' are ignored):
#
#   file <glob>                 whole file; glob is relative to the root and
#                               '*' also matches '/'
#   section <path> <heading>    every line from that exact Markdown heading
#                               line up to the next heading of the same or a
#                               higher level
#   line <ERE>                  any line matching the ERE (case-insensitive)
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
root_dir=$(cd -- "$script_dir/.." && pwd)
allowlist=""
pattern='rc\.8|20cc13a'

usage() {
	sed -n '2,22p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//' >&2
	exit 2
}

while (($#)); do
	case "$1" in
		--root) root_dir=${2:?--root needs a value}; shift 2 ;;
		--allowlist) allowlist=${2:?--allowlist needs a value}; shift 2 ;;
		--pattern) pattern=${2:?--pattern needs a value}; shift 2 ;;
		-h|--help) usage ;;
		*) echo "check-stale-candidate-refs: unknown argument: $1" >&2; usage ;;
	esac
done

[[ -n "$allowlist" ]] || allowlist="$root_dir/scripts/check-stale-candidate-refs.allowlist"
if [[ ! -f "$allowlist" ]]; then
	echo "check-stale-candidate-refs: allowlist not found: $allowlist" >&2
	exit 2
fi
if [[ ! -d "$root_dir/docs" ]]; then
	echo "check-stale-candidate-refs: no docs directory under $root_dir" >&2
	exit 2
fi

file_globs=()
section_paths=()
section_headings=()
line_patterns=()
lineno=0
while IFS= read -r rule || [[ -n "$rule" ]]; do
	lineno=$((lineno + 1))
	[[ -z "${rule//[[:space:]]/}" || "$rule" == \#* ]] && continue
	kind=${rule%% *}
	rest=${rule#* }
	[[ "$rest" == "$rule" ]] && rest=""
	case "$kind" in
		file)
			[[ -n "$rest" ]] || { echo "check-stale-candidate-refs: $allowlist:$lineno: file rule needs a glob" >&2; exit 2; }
			file_globs+=("$rest")
			;;
		section)
			path=${rest%% *}
			heading=${rest#* }
			if [[ -z "$path" || "$heading" == "$rest" || "$heading" != \#* ]]; then
				echo "check-stale-candidate-refs: $allowlist:$lineno: section rule needs '<path> <# heading>'" >&2
				exit 2
			fi
			section_paths+=("$path")
			section_headings+=("$heading")
			;;
		line)
			[[ -n "$rest" ]] || { echo "check-stale-candidate-refs: $allowlist:$lineno: line rule needs an ERE" >&2; exit 2; }
			line_patterns+=("$rest")
			;;
		*)
			echo "check-stale-candidate-refs: $allowlist:$lineno: unknown rule kind '$kind'" >&2
			exit 2
			;;
	esac
done <"$allowlist"

# Line patterns are joined into one alternation; section headings for the
# current file are passed newline-separated. Both go through ENVIRON so awk
# does not reinterpret backslashes.
line_re=""
for re in "${line_patterns[@]}"; do
	line_re+="${line_re:+|}($re)"
done

# Root-level files scanned in addition to docs/.
extra_files=(NEXT_STEPS.md)

list_scanned_files() {
	find "$root_dir/docs" -type f \( -name '*.md' -o -name '*.txt' \) -print0
	local extra
	for extra in "${extra_files[@]}"; do
		if [[ -f "$root_dir/$extra" ]]; then
			printf '%s\0' "$root_dir/$extra"
		fi
	done
}

hits=0
allowed=0
report=""
while IFS= read -r -d '' file; do
	rel=${file#"$root_dir"/}
	whole_file=0
	for glob in "${file_globs[@]}"; do
		# shellcheck disable=SC2053 # glob match is intended
		if [[ "$rel" == $glob ]]; then
			whole_file=1
			break
		fi
	done
	headings=""
	for i in "${!section_paths[@]}"; do
		if [[ "${section_paths[$i]}" == "$rel" ]]; then
			headings+="${section_headings[$i]}"$'\n'
		fi
	done
	result=$(STALE_PATTERN="$pattern" STALE_LINE_RE="$line_re" STALE_HEADINGS="$headings" \
		STALE_WHOLE="$whole_file" STALE_REL="$rel" awk '
		BEGIN {
			pat = ENVIRON["STALE_PATTERN"]
			line_re = tolower(ENVIRON["STALE_LINE_RE"])
			whole = ENVIRON["STALE_WHOLE"] == "1"
			rel = ENVIRON["STALE_REL"]
			n = split(ENVIRON["STALE_HEADINGS"], hs, "\n")
			for (i = 1; i <= n; i++) if (hs[i] != "") allowed_heading[hs[i]] = 1
			in_section = 0; section_level = 0; fence = 0; ok = 0; bad = 0
		}
		/^(```|~~~)/ { fence = !fence }
		!fence && /^#+[ \t]/ {
			level = match($0, /[^#]/) - 1
			if (in_section && level <= section_level) in_section = 0
			if ($0 in allowed_heading) { in_section = 1; section_level = level }
		}
		$0 ~ pat {
			if (whole || in_section || (line_re != "" && tolower($0) ~ line_re)) { ok++ }
			else { bad++; printf "%s:%d: %s\n", rel, NR, $0 }
		}
		END { printf "#counts %d %d\n", ok, bad }
	' "$file")
	counts=${result##*#counts }
	body=${result%#counts *}
	allowed=$((allowed + ${counts%% *}))
	hits=$((hits + ${counts##* }))
	report+=$body
done < <(list_scanned_files | sort -z)

if ((hits > 0)); then
	printf '%s' "$report" >&2
	printf 'stale candidate refs: %d line(s) match /%s/ outside the allowlist (%s)\n' "$hits" "$pattern" "$allowlist" >&2
	exit 1
fi
printf 'stale candidate refs: OK (pattern /%s/, %d allowlisted line(s))\n' "$pattern" "$allowed"
