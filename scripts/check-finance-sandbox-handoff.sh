#!/usr/bin/env bash
set -euo pipefail

root_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root_dir"

mode=prepare
case "${1:-}" in
  '')
    ;;
  --require-complete)
    mode=complete
    ;;
  --help|-h)
    cat <<'USAGE'
Usage: scripts/check-finance-sandbox-handoff.sh [--require-complete]

The default mode validates the v0.11-finance evidence-index structure and keeps
uncollected external evidence explicitly UNVERIFIED. --require-complete applies
the strict release gate and requires every registry row to be PASS or a justified
N/A with complete immutable metadata.
USAGE
    exit 0
    ;;
  *)
    printf 'finance sandbox handoff: unsupported argument %s\n' "$1" >&2
    exit 2
    ;;
esac

handoff='docs/releases/v0.11-finance-prep-handoff.md'
index='docs/releases/v0.11-finance-evidence-index.md'
status=0

fail() {
  printf 'finance sandbox handoff: %s\n' "$1" >&2
  status=1
}

trim() {
  local value=$1
  value=${value#"${value%%[![:space:]]*}"}
  value=${value%"${value##*[![:space:]]}"}
  printf '%s' "$value"
}

if [[ ! -f "$handoff" ]]; then
  fail "missing handoff document: $handoff"
fi
if [[ ! -f "$index" ]]; then
  fail "missing evidence index: $index"
fi

if [[ -f "$handoff" ]]; then
  grep -Fq '**Release profile:** `v0.11-finance`' "$handoff" ||
    fail 'handoff does not pin Release profile v0.11-finance'
  grep -Fq '**Migration ceiling:** `000129_ap_exception_resolution_events`' "$handoff" ||
    fail 'handoff does not pin migration ceiling 000129_ap_exception_resolution_events'
  grep -Eqi 'preparation candidate only.*not a release tag.*not production-certified' "$handoff" ||
    fail 'handoff does not state that the candidate is not production-certified'
  grep -Fq 'v0.11-finance-evidence-index.md' "$handoff" ||
    fail 'handoff does not link the controlled v0.11 evidence index'
fi

expected_ids=(
  REL-011 DB-011 ROUTE-011 ISO-011 SEC-011 PROV-011 PAY-011 SET-011 P2P-011 OPS-011 APR-011
)
declare -A expected=()
for id in "${expected_ids[@]}"; do
  expected["$id"]=1
done
declare -A seen=()
row_count=0

if [[ -f "$index" ]]; then
  grep -Fq '**Profile:** `v0.11-finance`' "$index" ||
    fail 'evidence index does not pin Profile v0.11-finance'
  grep -Fq '**Migration ceiling:** `000129_ap_exception_resolution_events`' "$index" ||
    fail 'evidence index does not pin migration ceiling 000129_ap_exception_resolution_events'
  grep -Fqx '| Evidence ID | Required proof | Immutable artifact URL | SHA-256 | Collected UTC | Owner | Result | Reviewer |' "$index" ||
    fail 'evidence index registry header is not stable'

  registry=$(awk '
    /^## Evidence registry$/ { in_section = 1; next }
    in_section && /^## / { exit }
    in_section { print }
  ' "$index")

  while IFS='|' read -r _ evidence_id required_proof artifact_url evidence_sha collected_utc owner result reviewer _; do
    evidence_id=$(trim "${evidence_id:-}")
    evidence_id=${evidence_id//\`/}
    [[ "$evidence_id" =~ ^[A-Z][A-Z0-9-]*-[0-9]+$ ]] || continue
    row_count=$((row_count + 1))
    if [[ -n "${seen[$evidence_id]+x}" ]]; then
      fail "duplicate evidence registry row: $evidence_id"
    fi
    seen["$evidence_id"]=1
    if [[ -z "${expected[$evidence_id]+x}" ]]; then
      fail "unexpected evidence registry row: $evidence_id"
    fi

    required_proof=$(trim "${required_proof:-}")
    artifact_url=$(trim "${artifact_url:-}")
    evidence_sha=$(trim "${evidence_sha:-}")
    collected_utc=$(trim "${collected_utc:-}")
    owner=$(trim "${owner:-}")
    result=$(trim "${result:-}")
    result=${result//\`/}
    reviewer=$(trim "${reviewer:-}")

    [[ -n "$required_proof" ]] || fail "$evidence_id has no required proof"
    case "$result" in
      PASS|N/A|UNVERIFIED|FAIL)
        ;;
      *)
        fail "$evidence_id has invalid result ${result:-missing}"
        ;;
    esac

    if [[ "$result" == UNVERIFIED || "$result" == FAIL ]]; then
      reason=${required_proof#*Reason:}
      [[ "$required_proof" == *'Reason:'* && "$reason" =~ [^[:space:]] ]] ||
        fail "$evidence_id open result must include a specific Reason:"
    fi

    if [[ "$mode" == complete && "$evidence_id" == APR-011 && "$result" != PASS ]]; then
      fail 'APR-011 approval evidence must be PASS before the strict gate can pass'
    fi

    if [[ "$result" == PASS || "$result" == N/A ]]; then
      [[ "$artifact_url" =~ ^[A-Za-z][A-Za-z0-9+.-]*://[^[:space:]]+$ ]] ||
        fail "$evidence_id result $result requires an immutable artifact URL"
      [[ "$evidence_sha" =~ ^[0-9a-f]{64}$ ]] ||
        fail "$evidence_id result $result requires a lowercase SHA-256 digest"
      [[ "$collected_utc" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]] ||
        fail "$evidence_id result $result requires an RFC3339 UTC timestamp"
      if ! date -u -d "$collected_utc" +%s >/dev/null 2>&1; then
        fail "$evidence_id collected UTC is not a valid timestamp: $collected_utc"
      fi
      [[ -n "$owner" && "$owner" != *UNVERIFIED* && "$owner" != *'<'* && "$owner" != *'>'* ]] ||
        fail "$evidence_id result $result requires an owner"
      [[ -n "$reviewer" && "$reviewer" != *UNVERIFIED* && "$reviewer" != *'<'* && "$reviewer" != *'>'* ]] ||
        fail "$evidence_id result $result requires a reviewer"
    fi
    if [[ "$result" == N/A ]]; then
      na_reason=${required_proof#*N/A:}
      [[ "$required_proof" == *'N/A:'* && "$na_reason" =~ [^[:space:]] ]] ||
        fail "$evidence_id N/A result requires an N/A: justification"
    fi
  done <<< "$registry"
fi

for id in "${expected_ids[@]}"; do
  [[ -n "${seen[$id]+x}" ]] || fail "evidence index is missing required row: $id"
done
if (( row_count != ${#expected_ids[@]} )); then
  fail "evidence index has $row_count registry rows; expected ${#expected_ids[@]}"
fi

if [[ -f "$index" ]]; then
  candidate_section=$(awk '
    /^## Candidate identity$/ { in_section = 1; next }
    in_section && /^## / { exit }
    in_section { print }
  ' "$index")
  candidate_fields=(
    'Candidate ref' 'Candidate commit' 'Bundle name' 'Bundle SHA-256'
    'GitHub artifact ID' 'GitHub artifact digest' 'Provenance verification'
    'SPDX SBOM SHA-256'
  )
  declare -A candidate_seen=()
  while IFS='|' read -r _ field value _; do
    field=$(trim "${field:-}")
    value=$(trim "${value:-}")
    case "$field" in
      'Candidate ref')
        candidate_seen["$field"]=1
        if [[ "$mode" == complete ]]; then
          [[ "$value" =~ ^[A-Za-z0-9._/@-]+$ ]] || fail 'candidate ref is incomplete or malformed'
        else
          [[ "$value" == UNVERIFIED* || "$value" =~ ^[A-Za-z0-9._/@-]+$ ]] || fail 'candidate ref is malformed'
        fi
        ;;
      'Candidate commit')
        candidate_seen["$field"]=1
        if [[ "$mode" == complete ]]; then
          [[ "$value" =~ ^[0-9a-f]{40}$ ]] || fail 'candidate commit must be a full 40-character SHA'
        else
          [[ "$value" == UNVERIFIED* || "$value" =~ ^[0-9a-f]{40}$ ]] || fail 'candidate commit is malformed'
        fi
        ;;
      'Bundle name')
        candidate_seen["$field"]=1
        if [[ "$mode" == complete ]]; then
          [[ "$value" =~ ^odyssey-v011-finance-sandbox-[0-9a-f]{40}\.tar\.gz$ ]] || fail 'bundle name is incomplete or malformed'
        else
          [[ "$value" == UNVERIFIED* || "$value" =~ ^odyssey-v011-finance-sandbox-[0-9a-f]{40}\.tar\.gz$ ]] || fail 'bundle name is malformed'
        fi
        ;;
      'Bundle SHA-256')
        candidate_seen["$field"]=1
        if [[ "$mode" == complete ]]; then
          [[ "$value" =~ ^[0-9a-f]{64}$ ]] || fail 'bundle SHA-256 is incomplete or malformed'
        else
          [[ "$value" == UNVERIFIED* || "$value" =~ ^[0-9a-f]{64}$ ]] || fail 'bundle SHA-256 is malformed'
        fi
        ;;
      'GitHub artifact ID')
        candidate_seen["$field"]=1
        if [[ "$mode" == complete ]]; then
          [[ "$value" =~ ^[0-9]+$ ]] || fail 'GitHub artifact ID is incomplete or malformed'
        else
          [[ "$value" == UNVERIFIED* || "$value" =~ ^[0-9]+$ ]] || fail 'GitHub artifact ID is malformed'
        fi
        ;;
      'GitHub artifact digest')
        candidate_seen["$field"]=1
        if [[ "$mode" == complete ]]; then
          [[ "$value" =~ ^sha256:[0-9a-f]{64}$ ]] || fail 'GitHub artifact digest is incomplete or malformed'
        else
          [[ "$value" == UNVERIFIED* || "$value" =~ ^sha256:[0-9a-f]{64}$ ]] || fail 'GitHub artifact digest is malformed'
        fi
        ;;
      'Provenance verification')
        candidate_seen["$field"]=1
        if [[ "$mode" == complete ]]; then
          [[ "$value" != *UNVERIFIED* && "$value" != *'<'* && "$value" != *'>'* ]] || fail 'provenance verification is incomplete'
        else
          [[ -n "$value" ]] || fail 'provenance verification is missing'
        fi
        ;;
      'SPDX SBOM SHA-256')
        candidate_seen["$field"]=1
        if [[ "$mode" == complete ]]; then
          [[ "$value" =~ ^[0-9a-f]{64}$ ]] || fail 'SPDX SBOM SHA-256 is incomplete or malformed'
        else
          [[ "$value" == UNVERIFIED* || "$value" =~ ^[0-9a-f]{64}$ ]] || fail 'SPDX SBOM SHA-256 is malformed'
        fi
        ;;
    esac
  done <<< "$candidate_section"
  for field in "${candidate_fields[@]}"; do
    [[ -n "${candidate_seen[$field]+x}" ]] || fail "candidate identity is missing field: $field"
  done

if [[ "$mode" == complete ]]; then
  while IFS='|' read -r _ evidence_id _ _ _ _ _ result _; do
    evidence_id=$(trim "${evidence_id:-}")
    evidence_id=${evidence_id//\`/}
    [[ "$evidence_id" =~ ^[A-Z][A-Z0-9-]*-[0-9]+$ ]] || continue
    result=$(trim "${result:-}")
    result=${result//\`/}
    if [[ "$result" != PASS && "$result" != N/A ]]; then
      fail "$evidence_id is not complete (result ${result:-missing})"
    fi
  done <<< "$registry"
fi
fi

if (( status == 0 )); then
  if [[ "$mode" == complete ]]; then
    printf 'finance sandbox handoff: strict evidence-completion gate passed\n'
  else
    printf 'finance sandbox handoff: structural preflight passed (%d rows; external evidence remains explicitly open)\n' "$row_count"
  fi
else
  if [[ "$mode" == complete ]]; then
    printf 'finance sandbox handoff: strict evidence-completion gate is blocked\n' >&2
  else
    printf 'finance sandbox handoff: structural preflight is blocked\n' >&2
  fi
fi
exit "$status"
