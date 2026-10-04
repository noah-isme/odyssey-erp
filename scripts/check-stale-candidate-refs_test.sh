#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
checker="$script_dir/check-stale-candidate-refs.sh"

tmp=$(mktemp -d)
trap 'rm -rf -- "$tmp"' EXIT

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

# run <name> <expected-exit> [args...]: runs the checker against the fixture
# root, keeping stdout/stderr in $tmp/<name>.out and .err.
run() {
	local name=$1 want=$2
	shift 2
	local got=0
	bash "$checker" --root "$root" "$@" >"$tmp/$name.out" 2>"$tmp/$name.err" || got=$?
	if [[ "$got" != "$want" ]]; then
		echo "--- stdout" >&2; cat "$tmp/$name.out" >&2
		echo "--- stderr" >&2; cat "$tmp/$name.err" >&2
		fail "$name: exit $got, want $want"
	fi
}

root="$tmp/repo"
mkdir -p "$root/docs/releases" "$root/docs/archive" "$root/scripts"

cat >"$root/docs/releases/VERSION_HISTORY.md" <<'EOF'
# History

**Current release candidate:** v0.10.0-rc.9

## Detailed reports

### v0.10.0-rc.8 — Old candidate (superseded)

The candidate commit is `20cc13a0f028e3b09573944bb9f7a1f943461253`
on the rc.8 line.

#### Nested detail

Still inside the rc.8 section.

```text
# not a heading inside a fence: rc.8
```

### v0.10.0-rc.9 — Current

Lineage: the superseded rc.8 tag plus fixes.
EOF

cat >"$root/docs/releases/v0.10.0-rc.4.md" <<'EOF'
Point-in-time note naming rc.8 at 20cc13a.
EOF
cat >"$root/docs/archive/old.md" <<'EOF'
Archived rc.8 note.
EOF
cat >"$root/docs/releases/notes.txt" <<'EOF'
Plain text, no candidate reference.
EOF
cat >"$root/docs/ignored.json" <<'EOF'
{"candidate": "rc.8"}
EOF

cat >"$root/scripts/check-stale-candidate-refs.allowlist" <<'EOF'
# comment line
file docs/releases/v0.10.0-rc.*.md
file docs/archive/*

section docs/releases/VERSION_HISTORY.md ### v0.10.0-rc.8 — Old candidate (superseded)
line superseded|historical
EOF

# 1. Clean fixture passes; every hit is allowlisted (file, archive, section
#    incl. nested heading and fenced line, and the 'superseded' line rule).
run clean 0
grep -q 'stale candidate refs: OK' "$tmp/clean.out" || fail 'clean: missing OK summary'
grep -q '8 allowlisted line(s)' "$tmp/clean.out" || fail "clean: want 8 allowlisted lines, got: $(cat "$tmp/clean.out")"

# 2. A current-status line outside the allowlist fails with file:line output.
cat >"$root/docs/DEPLOYMENT.md" <<'EOF'
# Deployment

The current candidate is v0.10.0-rc.8.
Line two is fine.
EOF
run stale 1
grep -qx 'docs/DEPLOYMENT.md:3: The current candidate is v0.10.0-rc.8.' "$tmp/stale.err" || fail "stale: missing file:line report: $(cat "$tmp/stale.err")"
grep -q '1 line(s) match' "$tmp/stale.err" || fail 'stale: missing failure summary'

# 3. The 'historical' line rule is case-insensitive.
printf '# Deployment\n\nHistorical: v0.10.0-rc.8 was the previous candidate.\n' >"$root/docs/DEPLOYMENT.md"
run historical 0

# 4. A section ends at the next heading of the same or higher level.
cat >>"$root/docs/releases/VERSION_HISTORY.md" <<'EOF'

## Follow-up work

Promote after the rc.8 candidate passes.
EOF
run section-end 1
grep -q 'VERSION_HISTORY.md:.*Promote after the rc.8 candidate passes.' "$tmp/section-end.err" || fail 'section-end: line after the allowlisted section must be reported'

# 5. --pattern scopes the check to another superseded candidate.
run pattern 0 --pattern 'rc\.99'
grep -q 'pattern /rc\\.99/' "$tmp/pattern.out" || fail 'pattern: summary must echo the pattern'

# 6. Malformed allowlist rules and a missing allowlist are usage errors (exit 2).
printf 'section docs/x.md not-a-heading\n' >"$tmp/bad-section.allowlist"
run bad-section 2 --allowlist "$tmp/bad-section.allowlist"
printf 'bogus rule\n' >"$tmp/bad-kind.allowlist"
run bad-kind 2 --allowlist "$tmp/bad-kind.allowlist"
grep -q "unknown rule kind 'bogus'" "$tmp/bad-kind.err" || fail 'bad-kind: message'
run missing-allowlist 2 --allowlist "$tmp/does-not-exist"

# 7. The committed allowlist passes against this repository's docs.
got=0
bash "$checker" >"$tmp/repo.out" 2>"$tmp/repo.err" || got=$?
[[ "$got" == 0 ]] || { cat "$tmp/repo.err" >&2; fail "repository docs: exit $got"; }

echo 'check-stale-candidate-refs_test: PASS'
