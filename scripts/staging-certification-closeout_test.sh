#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
validator="$script_dir/staging-certification-closeout.sh"
contract="$script_dir/staging-certification-contract.json"
command -v jq >/dev/null 2>&1 || { echo 'jq is required' >&2; exit 1; }

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

candidate_sha=0123456789012345678901234567890123456789
candidate_tag=v0.10.0-rc.7

make_index() {
	local lane=$1 prefix=$2 run_id=$3 output=$4
	local ids
	ids=$(jq -c --arg lane "$lane" '[.evidence[] | select(.lane == $lane) | .evidence_id]' "$contract")
	local hash
	if [[ "$lane" == automated ]]; then hash=$(printf 'a%.0s' {1..64}); else hash=$(printf 'b%.0s' {1..64}); fi
	jq -n --arg lane "$lane" --arg prefix "$prefix" --arg run_id "$run_id" \
		--arg tag "$candidate_tag" --arg sha "$candidate_sha" --argjson ids "$ids" --arg hash "$hash" \
		'{schema_version:"v0.10-core-staging-evidence.v1",collection:{result:"PASS",prefix:$prefix},run:{id:$run_id},candidate:{tag:$tag,resolved_sha:$sha,profile:"v0.10-core",migration_ceiling:"000124"},entries:[$ids[] | {evidence_id:.,result:"PASS",collected_utc:"2026-08-26T00:00:00Z",details:"verified",artifact_uri:("s3://bucket/" + $prefix + "/" + . + ".log"),sha256:$hash,run_id:$run_id,reviewer:("reviewer-" + $lane)}],validation:{errors:[]}}' \
		>"$output"
}

auto="$tmp/automated.json"
operator="$tmp/operator.json"
make_index automated lane/automated auto-1 "$auto"
make_index operator lane/operator operator-1 "$operator"

run_validator() {
	local name=$1 auto_file=$2 operator_file=$3
	local output="$tmp/out-$name"
	"$validator" --automated "$auto_file" --operator "$operator_file" --output "$output" \
		--candidate-tag "$candidate_tag" --candidate-sha "$candidate_sha" \
		--prefix "closeout/$name" --local-only >"$tmp/$name.log" 2>&1
}

expect_reject() {
	local name=$1 auto_file=$2 operator_file=$3
	local output="$tmp/out-$name"
	if run_validator "$name" "$auto_file" "$operator_file"; then
		echo "expected closeout rejection: $name" >&2
		exit 1
	fi
	if [[ -e "$output/evidence-index.json" ]]; then
		echo "rejected closeout wrote an index: $name" >&2
		exit 1
	fi
}

run_validator valid "$auto" "$operator"
jq -e '
	.schema_version == "v0.10-core-staging-closeout.v1" and
	.collection.decision == "GO" and
	(.entries | length) == 25 and
	(.validation.errors | length) == 0 and
	(.validation.missing_ids | length) == 0 and
	(.validation.duplicate_ids | length) == 0 and
	(.validation.unknown_ids | length) == 0
' "$tmp/out-valid/evidence-index.json" >/dev/null
(cd "$tmp/out-valid" && sha256sum -c SHA256SUMS >/dev/null)

# The output is write-once, including a second local invocation with the same
# destination.
if run_validator valid "$auto" "$operator"; then
	echo 'write-once closeout unexpectedly overwrote an existing output' >&2
	exit 1
fi

missing="$tmp/missing.json"
jq 'del(.entries[0])' "$auto" >"$missing"
expect_reject missing "$missing" "$operator"

duplicate="$tmp/duplicate.json"
jq '.entries += [.entries[0]]' "$auto" >"$duplicate"
expect_reject duplicate "$duplicate" "$operator"

unknown="$tmp/unknown.json"
jq '.entries[0].evidence_id = "FAKE-001"' "$auto" >"$unknown"
expect_reject unknown "$unknown" "$operator"

mismatched="$tmp/mismatched.json"
jq '.candidate.resolved_sha = "fedcba9876543210012345678901234567890123"' "$operator" >"$mismatched"
expect_reject candidate-mismatch "$auto" "$mismatched"

bad_hash="$tmp/bad-hash.json"
jq '.entries[0].sha256 = ("A" * 64)' "$auto" >"$bad_hash"
expect_reject invalid-hash "$bad_hash" "$operator"

bad_time="$tmp/bad-time.json"
jq '.entries[0].collected_utc = "2026-02-30T00:00:00Z"' "$auto" >"$bad_time"
expect_reject invalid-timestamp "$bad_time" "$operator"

bad_uri="$tmp/bad-uri.json"
jq '.entries[0].artifact_uri = "local://mutable/artifact.log"' "$auto" >"$bad_uri"
expect_reject invalid-uri "$bad_uri" "$operator"

pending="$tmp/pending.json"
jq '.entries[0].result = "PENDING"' "$auto" >"$pending"
expect_reject pending "$pending" "$operator"

missing_reviewer="$tmp/missing-reviewer.json"
jq 'del(.entries[0].reviewer)' "$auto" >"$missing_reviewer"
expect_reject missing-reviewer "$missing_reviewer" "$operator"

unapproved_na="$tmp/unapproved-na.json"
jq '.entries[0].result = "N/A"' "$operator" >"$unapproved_na"
expect_reject unapproved-na "$auto" "$unapproved_na"

approved_na="$tmp/approved-na.json"
jq '.entries[0] |= (.result = "N/A" | .na_justification = "out of profile" | .na_approved_by = "release-owner" | .na_approved_utc = "2026-08-26T00:00:00Z")' "$operator" >"$approved_na"
run_validator approved-na "$auto" "$approved_na"
jq -e '.collection.decision == "GO" and ([.entries[] | select(.result == "N/A")] | length) == 1' "$tmp/out-approved-na/evidence-index.json" >/dev/null

echo 'staging certification closeout tests passed'
