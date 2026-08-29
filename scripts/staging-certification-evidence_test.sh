#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
collector="$script_dir/staging-certification-evidence.sh"
contract="$script_dir/staging-certification-contract.json"
tmp=$(mktemp -d)
trap 'rm -rf -- "$tmp"' EXIT

candidate_tag=v0.10.0-rc.7
candidate_sha=0123456789012345678901234567890123456789

make_log() {
	local output=$1
	{
		while IFS= read -r evidence_id; do
			record=$(jq -cn \
				--arg evidence_id "$evidence_id" \
				'{evidence_id:$evidence_id,result:"PASS",collected_utc:"2026-08-29T00:00:00Z",details:"verified"}')
			printf 'CERTIFICATION_EVIDENCE evidence_id=%s %s\n' "$evidence_id" "$record"
		done < <(jq -r '[.evidence[] | select(.lane == "automated") | .evidence_id] | .[]' "$contract")
	} >"$output"
}

run_local_collector() {
	local candidate=$1 evidence=$2
	env \
		CANDIDATE_TAG="$candidate_tag" \
		EXPECTED_SHA="$candidate_sha" \
		GITHUB_RUN_ID=12345 \
		GITHUB_RUN_ATTEMPT=1 \
		GITHUB_REPOSITORY=example/odyssey \
		GITHUB_WORKFLOW='Certify v0.10 Staging' \
		"$collector" \
			--candidate "$candidate" \
			--evidence "$evidence" \
			--contract "$contract" \
			--lane automated \
			--local-only
}

candidate="$tmp/candidate"
mkdir -p "$candidate"
make_log "$candidate/staging-certification.log"

clean_evidence="$tmp/evidence-clean"
run_local_collector "$candidate" "$clean_evidence"
jq -e '
	.collection.result == "PASS" and
	(.validation.errors | length) == 0 and
	(.entries | length) == 14
' "$clean_evidence/evidence-index.json" >/dev/null

failed_candidate="$tmp/candidate-metadata-failure"
mkdir -p "$failed_candidate"
cp -a "$candidate"/. "$failed_candidate"/
printf 'gh run view failed: simulated API outage\n' >"$failed_candidate/certification-run.error"

failed_evidence="$tmp/evidence-metadata-failure"
if run_local_collector "$failed_candidate" "$failed_evidence"; then
	echo 'metadata retrieval failure unexpectedly produced a passing evidence collection' >&2
	exit 1
fi
jq -e '
	.collection.result == "FAIL" and
	any(.validation.errors[]; contains("metadata retrieval failed")) and
	(.entries | length) == 14
' "$failed_evidence/evidence-index.json" >/dev/null
(cd "$failed_evidence" && sha256sum -c SHA256SUMS >/dev/null)

echo 'staging certification evidence tests passed'
