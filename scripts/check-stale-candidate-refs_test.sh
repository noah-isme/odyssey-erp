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

**Current release candidate:** v0.10.0-rc.10

## Detailed reports

### v0.10.0-rc.8 — Old candidate (superseded)

The candidate commit is `20cc13a0f028e3b09573944bb9f7a1f943461253`
on the rc.8 line.

#### Nested detail

Still inside the rc.8 section.

```text
# not a heading inside a fence: rc.8
```

### v0.10.0-rc.9 — Previous candidate (superseded)

The candidate commit is `07d2ba2bbb553a5594828771361e3ab546b35b70`
on the rc.9 line.

### v0.10.0-rc.10 — Current

Lineage: the superseded rc.9 tag plus fixes.
The defect is present in `v0.10.0-rc.1` through `v0.10.0-rc.9`.
Affected builds: rc.1–rc.9.
Payslip locking is unchanged since rc.9.
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
section docs/releases/VERSION_HISTORY.md ### v0.10.0-rc.9 — Previous candidate (superseded)
line superseded|historical
line rc\.1`?( through | to |-|–)`?(v0\.10\.0-)?rc\.9
line (since|before|fixed in|resolved in|changed in) `?(v0\.10\.0-)?rc\.9
EOF

# 1. Clean fixture passes; every hit is allowlisted: file and archive rules
#    (2), the rc.8 section incl. nested heading and fenced line (5), the rc.9
#    section (3), and the 'superseded', defect-range (backtick and en-dash
#    forms) and 'since rc.9' line rules (4). The current rc.10 heading and
#    status line do not match the default pattern.
run clean 0
grep -q 'stale candidate refs: OK' "$tmp/clean.out" || fail 'clean: missing OK summary'
grep -q '14 allowlisted line(s)' "$tmp/clean.out" || fail "clean: want 14 allowlisted lines, got: $(cat "$tmp/clean.out")"

# 2. Current-status lines naming rc.8, rc.9, or the rc.9 commit outside the
#    allowlist fail with file:line output; an rc.10 line is not a hit.
cat >"$root/docs/DEPLOYMENT.md" <<'EOF'
# Deployment

The current candidate is v0.10.0-rc.8.
Line two is fine.
The current candidate is v0.10.0-rc.9.
The candidate commit is `07d2ba2`.
The current candidate is v0.10.0-rc.10.
EOF
run stale 1
grep -qx 'docs/DEPLOYMENT.md:3: The current candidate is v0.10.0-rc.8.' "$tmp/stale.err" || fail "stale: missing rc.8 file:line report: $(cat "$tmp/stale.err")"
grep -qx 'docs/DEPLOYMENT.md:5: The current candidate is v0.10.0-rc.9.' "$tmp/stale.err" || fail "stale: missing rc.9 file:line report: $(cat "$tmp/stale.err")"
grep -qx 'docs/DEPLOYMENT.md:6: The candidate commit is `07d2ba2`.' "$tmp/stale.err" || fail "stale: missing 07d2ba2 file:line report: $(cat "$tmp/stale.err")"
if grep -q 'DEPLOYMENT.md:7:' "$tmp/stale.err"; then fail 'stale: the rc.10 line must not be reported'; fi
grep -q '3 line(s) match' "$tmp/stale.err" || fail "stale: want 3 hits, got: $(cat "$tmp/stale.err")"

# 3. The 'historical' line rule is case-insensitive.
printf '# Deployment\n\nHistorical: v0.10.0-rc.8 was the previous candidate.\n' >"$root/docs/DEPLOYMENT.md"
run historical 0

# 3b. NEXT_STEPS.md at the root is scanned too: a stale current-status line
#    fails with a root-relative file:line, and fixing it passes. The earlier
#    fixtures have no NEXT_STEPS.md, which is simply skipped.
printf '# Next steps\n\n**Current candidate:** `v0.10.0-rc.9`\n' >"$root/NEXT_STEPS.md"
run next-steps-stale 1
grep -qx 'NEXT_STEPS.md:3: \*\*Current candidate:\*\* `v0.10.0-rc.9`' "$tmp/next-steps-stale.err" || fail "next-steps-stale: missing file:line report: $(cat "$tmp/next-steps-stale.err")"
printf '# Next steps\n\n**Current candidate:** `v0.10.0-rc.10`\n' >"$root/NEXT_STEPS.md"
run next-steps-fixed 0
printf '# Next steps\n\nThe rc.9 record is historical.\n' >"$root/NEXT_STEPS.md"
run next-steps-allowlisted 0
grep -q '16 allowlisted line(s)' "$tmp/next-steps-allowlisted.out" || fail "next-steps-allowlisted: want 16 allowlisted lines, got: $(cat "$tmp/next-steps-allowlisted.out")"
rm -f "$root/NEXT_STEPS.md"

# 4. A section ends at the next heading of the same or higher level, and the
#    narrow rc.9 line rules do not cover a plain current-status mention.
cat >>"$root/docs/releases/VERSION_HISTORY.md" <<'EOF'

## Follow-up work

Promote after the rc.8 candidate passes.
Promote after the rc.9 candidate passes.
EOF
run section-end 1
grep -q 'VERSION_HISTORY.md:.*Promote after the rc.8 candidate passes.' "$tmp/section-end.err" || fail 'section-end: rc.8 line after the allowlisted section must be reported'
grep -q 'VERSION_HISTORY.md:.*Promote after the rc.9 candidate passes.' "$tmp/section-end.err" || fail 'section-end: rc.9 line after the allowlisted section must be reported'

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

# 7. The committed allowlist passes against this repository's docs and
#    NEXT_STEPS.md.
got=0
bash "$checker" >"$tmp/repo.out" 2>"$tmp/repo.err" || got=$?
[[ "$got" == 0 ]] || { cat "$tmp/repo.err" >&2; fail "repository docs: exit $got"; }

echo 'check-stale-candidate-refs_test: PASS'
