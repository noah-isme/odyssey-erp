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

**Current release candidate:** v0.10.0-rc.11

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

### v0.10.0-rc.10 — Approval fix (superseded)

The candidate commit is `074595735f82a063c3c9e0a176658ed2f04581d6`.
Deploying rc.10 stops new damage.

### v0.10.0-rc.11 — Current

Lineage: the superseded rc.9 tag plus fixes.
The defect is present in `v0.10.0-rc.1` through `v0.10.0-rc.9`.
Affected builds: rc.1–rc.9.
Payslip locking is unchanged since rc.9.
Approval finalization first shipped in `v0.10.0-rc.10`.
Every fix has a test that fails at the rc.10 tag.
EOF

# The certification record keeps the rc.10 identity, the invalidated rc.10
# run and the findings table (file references at the rc.10 tag) as sections.
cat >"$root/docs/releases/v0.10-core-staging-certification.md" <<'EOF'
# Certification record

**Candidate:** `v0.10.0-rc.11`

**Candidate lineage:** rc.10 tag plus the rc.11 fix commits; no migration.

## Invalidated certification runs

| Run | Evidence location |
|---|---|
| `37820191861` from `certification/v0.10.0-rc.10` | `s3://evidence/v0.10.0-rc.10/0745957/37820191861/1` |

## Findings

| `FIND-011` | Fails at `0745957`. |
| `FIND-013` | Routes do not exist in rc.10. |

### Finding notes

Nested under Findings: rc.10 reproduction.

## Approval and promotion decision

Approve the exact rc.11 candidate.

## Historical rc.10 candidate identity (superseded)

`v0.10.0-rc.10` resolved to `074595735f82a063c3c9e0a176658ed2f04581d6`.
Its forensic dump is under `v0.10.0-rc.10/forensics/`.
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
section docs/releases/VERSION_HISTORY.md ### v0.10.0-rc.10 — Approval fix (superseded)
section docs/releases/v0.10-core-staging-certification.md ## Invalidated certification runs
section docs/releases/v0.10-core-staging-certification.md ## Findings
section docs/releases/v0.10-core-staging-certification.md ## Historical rc.10 candidate identity (superseded)
line superseded|historical|invalidated
line rc\.1`?( through | to |-|–)`?(v0\.10\.0-)?rc\.9
line (since|before|fixed in|resolved in|changed in) `?(v0\.10\.0-)?rc\.9
line (first shipped in|up to) `?(v0\.10\.0-)?rc\.10
line rc\.10 tag plus the rc\.11
line fails? at (the )?`?(v0\.10\.0-)?rc\.10`? tag
EOF

# 1. Clean fixture passes; every hit is allowlisted. VERSION_HISTORY (17):
#    the rc.8 section incl. nested heading and fenced line (5), the rc.9
#    section (3), the rc.10 section incl. its full commit (3), and the
#    'superseded', defect-range (backtick and en-dash forms), 'since rc.9',
#    'first shipped in rc.10' and 'fails at the rc.10 tag' line rules (6) in
#    the rc.11 section, whose heading does not match. Certification record
#    (8): the lineage line rule (1), the invalidated-run table row (1), the
#    Findings rows and a line under its nested heading (3), and the historical
#    rc.10 heading and lines (3); the current rc.11 lines and the approval
#    section do not match. File and archive rules (2).
run clean 0
grep -q 'stale candidate refs: OK' "$tmp/clean.out" || fail 'clean: missing OK summary'
grep -q '27 allowlisted line(s)' "$tmp/clean.out" || fail "clean: want 27 allowlisted lines, got: $(cat "$tmp/clean.out")"
grep -qF 'rc\.10|0745957' "$tmp/clean.out" || fail "clean: default pattern must include rc.10 and 0745957: $(cat "$tmp/clean.out")"

# 1b. A section rule ends at the next heading of the same level: an rc.10
#    line in the certification record's current approval section fails.
cert="$root/docs/releases/v0.10-core-staging-certification.md"
cp "$cert" "$tmp/cert.md"
sed -i 's/^Approve the exact rc.11 candidate.$/Approve the exact rc.10 candidate./' "$cert"
run cert-section-end 1
grep -q 'v0.10-core-staging-certification.md:.*Approve the exact rc.10 candidate.' "$tmp/cert-section-end.err" || fail "cert-section-end: rc.10 line outside the sections must be reported: $(cat "$tmp/cert-section-end.err")"
grep -q '1 line(s) match' "$tmp/cert-section-end.err" || fail "cert-section-end: want 1 hit, got: $(cat "$tmp/cert-section-end.err")"
cp "$tmp/cert.md" "$cert"

# 2. Current-status lines naming rc.8, rc.9, rc.10, or the rc.9 or rc.10
#    commit outside the allowlist fail with file:line output; an rc.11 line is
#    not a hit.
cat >"$root/docs/DEPLOYMENT.md" <<'EOF'
# Deployment

The current candidate is v0.10.0-rc.8.
Line two is fine.
The current candidate is v0.10.0-rc.9.
The candidate commit is `07d2ba2`.
The current candidate is v0.10.0-rc.11.
The current candidate is v0.10.0-rc.10.
The candidate commit is `0745957`.
EOF
run stale 1
grep -qx 'docs/DEPLOYMENT.md:3: The current candidate is v0.10.0-rc.8.' "$tmp/stale.err" || fail "stale: missing rc.8 file:line report: $(cat "$tmp/stale.err")"
grep -qx 'docs/DEPLOYMENT.md:5: The current candidate is v0.10.0-rc.9.' "$tmp/stale.err" || fail "stale: missing rc.9 file:line report: $(cat "$tmp/stale.err")"
grep -qx 'docs/DEPLOYMENT.md:6: The candidate commit is `07d2ba2`.' "$tmp/stale.err" || fail "stale: missing 07d2ba2 file:line report: $(cat "$tmp/stale.err")"
grep -qx 'docs/DEPLOYMENT.md:8: The current candidate is v0.10.0-rc.10.' "$tmp/stale.err" || fail "stale: missing rc.10 file:line report: $(cat "$tmp/stale.err")"
grep -qx 'docs/DEPLOYMENT.md:9: The candidate commit is `0745957`.' "$tmp/stale.err" || fail "stale: missing 0745957 file:line report: $(cat "$tmp/stale.err")"
if grep -q 'DEPLOYMENT.md:7:' "$tmp/stale.err"; then fail 'stale: the rc.11 line must not be reported'; fi
grep -q '5 line(s) match' "$tmp/stale.err" || fail "stale: want 5 hits, got: $(cat "$tmp/stale.err")"

# 3. The 'historical' line rule is case-insensitive.
printf '# Deployment\n\nHistorical: v0.10.0-rc.8 was the previous candidate.\n' >"$root/docs/DEPLOYMENT.md"
run historical 0

# 3b. NEXT_STEPS.md at the root is scanned too: a stale current-status line
#    fails with a root-relative file:line, and fixing it passes. The earlier
#    fixtures have no NEXT_STEPS.md, which is simply skipped.
printf '# Next steps\n\n**Current candidate:** `v0.10.0-rc.10`\n' >"$root/NEXT_STEPS.md"
run next-steps-stale 1
grep -qx 'NEXT_STEPS.md:3: \*\*Current candidate:\*\* `v0.10.0-rc.10`' "$tmp/next-steps-stale.err" || fail "next-steps-stale: missing file:line report: $(cat "$tmp/next-steps-stale.err")"
printf '# Next steps\n\n**Current candidate:** `v0.10.0-rc.11`\n' >"$root/NEXT_STEPS.md"
run next-steps-fixed 0
printf '# Next steps\n\nThe rc.10 record is historical.\n' >"$root/NEXT_STEPS.md"
run next-steps-allowlisted 0
grep -q '29 allowlisted line(s)' "$tmp/next-steps-allowlisted.out" || fail "next-steps-allowlisted: want 29 allowlisted lines, got: $(cat "$tmp/next-steps-allowlisted.out")"
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
