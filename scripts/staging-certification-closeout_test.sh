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
fake_aws_state="$tmp/aws-state"

# Exercise the real S3/Object Lock validation path without requiring network
# credentials. The closeout invokes this exported Bash function as its AWS CLI.
fake_aws() {
	[[ "${1:-}" == s3api ]] || { echo 'fake aws only supports s3api' >&2; return 2; }
	shift
	local action=${1:-}
	shift
	local bucket='' key='' body='' metadata='' retain='' mode='' output=''
	while (($#)); do
		case "$1" in
			--bucket) bucket=$2; shift 2 ;;
			--key) key=$2; shift 2 ;;
			--body) body=$2; shift 2 ;;
			--metadata) metadata=$2; shift 2 ;;
			--object-lock-retain-until-date) retain=$2; shift 2 ;;
			--object-lock-mode) mode=$2; shift 2 ;;
			--output) output=$2; shift 2 ;;
			--region|--endpoint-url|--query) shift 2 ;;
			*) shift ;;
		esac
	done
	local state_file="$FAKE_AWS_STATE/$bucket/$key.meta"
	case "$action" in
		head-object)
			local digest='' remote_retain='' remote_mode=''
			if [[ -f "$state_file" ]]; then
				IFS='|' read -r digest remote_retain remote_mode <"$state_file"
			elif [[ "$key" == *missing* ]]; then
				echo 'An error occurred (404) when calling the HeadObject operation: Not Found' >&2
				return 1
			elif [[ "$key" == lane/automated/* ]]; then
				digest=$(printf 'a%.0s' {1..64})
				remote_retain=2099-01-01T00:00:00Z
				remote_mode=COMPLIANCE
			elif [[ "$key" == lane/operator/* ]]; then
				digest=$(printf 'b%.0s' {1..64})
				remote_retain=2099-01-01T00:00:00Z
				remote_mode=COMPLIANCE
			else
				echo 'An error occurred (404) when calling the HeadObject operation: Not Found' >&2
				return 1
			fi
			if [[ "${FAKE_AWS_SOURCE_NO_LOCK:-0}" == 1 && "$key" == lane/* ]]; then
				remote_mode=GOVERNANCE
			fi
			if [[ "${FAKE_AWS_FINAL_NO_LOCK:-0}" == 1 && "$key" == closeout/* ]]; then
				remote_mode=GOVERNANCE
			fi
			[[ "$output" == json ]] || output=json
			printf '{"ObjectLockMode":"%s","ObjectLockRetainUntilDate":"%s","Metadata":{"sha256":"%s"},"ContentLength":1}\n' \
				"$remote_mode" "$remote_retain" "$digest"
			;;
		put-object)
			if [[ "${FAKE_AWS_PUT_FAIL:-0}" == 1 ]]; then
				echo 'simulated put failure' >&2
				return 1
			fi
			mkdir -p "$(dirname "$state_file")"
			printf '%s|%s|%s\n' "${metadata#sha256=}" "$retain" "${mode:-COMPLIANCE}" >"$state_file"
			printf '{"ETag":"fake"}\n'
			;;
		*) echo "unsupported fake aws operation: $action" >&2; return 2 ;;
	esac
}
export -f fake_aws

make_index() {
	local lane=$1 prefix=$2 run_id=$3 output=$4
	local ids
	ids=$(jq -c --arg lane "$lane" '[.evidence[] | select(.lane == $lane) | .evidence_id]' "$contract")
	local hash
	if [[ "$lane" == automated ]]; then hash=$(printf 'a%.0s' {1..64}); else hash=$(printf 'b%.0s' {1..64}); fi
	jq -n --arg lane "$lane" --arg prefix "$prefix" --arg run_id "$run_id" \
		--arg tag "$candidate_tag" --arg sha "$candidate_sha" --argjson ids "$ids" --arg hash "$hash" \
		'{schema_version:"v0.10-core-staging-evidence.v1",collection:{result:"PASS",prefix:$prefix},run:{id:$run_id},candidate:{tag:$tag,resolved_sha:$sha,profile:"v0.10-core",migration_ceiling:"000124"},entries:[$ids[] | {evidence_id:.,result:"PASS",collected_utc:"2026-08-26T00:00:00Z",details:"verified",artifact_uri:("s3://bucket/" + $prefix + "/staging-certification.log"),sha256:$hash,run_id:$run_id,reviewer:("reviewer-" + $lane)}],validation:{errors:[]}}' \
		>"$output"
}

auto="$tmp/automated.json"
operator="$tmp/operator.json"
make_index automated lane/automated auto-1 "$auto"
make_index operator lane/operator operator-1 "$operator"

run_validator() {
	local name=$1 auto_file=$2 operator_file=$3
	local output="$tmp/out-$name"
	env \
		CERTIFICATION_AWS_CLI=fake_aws \
		EVIDENCE_S3_BUCKET=bucket \
		EVIDENCE_S3_ENDPOINT=http://fake-s3 \
		EVIDENCE_S3_ACCESS_KEY_ID=test-access \
		EVIDENCE_S3_SECRET_ACCESS_KEY=test-secret \
		FAKE_AWS_STATE="$fake_aws_state" \
		"$validator" --automated "$auto_file" --operator "$operator_file" --output "$output" \
		--candidate-tag "$candidate_tag" --candidate-sha "$candidate_sha" \
		--prefix "closeout/$name" >"$tmp/$name.log" 2>&1
}

run_validator_with_flags() {
	local name=$1 auto_file=$2 operator_file=$3 source_no_lock=$4 final_no_lock=$5
	local output="$tmp/out-$name"
	env \
		CERTIFICATION_AWS_CLI=fake_aws \
		EVIDENCE_S3_BUCKET=bucket \
		EVIDENCE_S3_ENDPOINT=http://fake-s3 \
		EVIDENCE_S3_ACCESS_KEY_ID=test-access \
		EVIDENCE_S3_SECRET_ACCESS_KEY=test-secret \
		FAKE_AWS_STATE="$fake_aws_state" \
		FAKE_AWS_SOURCE_NO_LOCK="$source_no_lock" \
		FAKE_AWS_FINAL_NO_LOCK="$final_no_lock" \
		"$validator" --automated "$auto_file" --operator "$operator_file" --output "$output" \
		--candidate-tag "$candidate_tag" --candidate-sha "$candidate_sha" \
		--prefix "closeout/$name" >"$tmp/$name.log" 2>&1
}

run_local_only() {
	local name=$1 auto_file=$2 operator_file=$3
	local output="$tmp/out-$name"
	env \
		CERTIFICATION_AWS_CLI=fake_aws \
		EVIDENCE_S3_BUCKET=bucket \
		EVIDENCE_S3_ENDPOINT=http://fake-s3 \
		EVIDENCE_S3_ACCESS_KEY_ID=test-access \
		EVIDENCE_S3_SECRET_ACCESS_KEY=test-secret \
		FAKE_AWS_STATE="$fake_aws_state" \
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

expect_no_go() {
	local name=$1 auto_file=$2 operator_file=$3
	local output="$tmp/out-$name"
	if run_validator "$name" "$auto_file" "$operator_file"; then
		echo "expected NO-GO closeout rejection: $name" >&2
		exit 1
	fi
	jq -e '
		.collection.decision == "NO-GO" and
		(.validation.errors | length) > 0 and
		(.entries | length) == 25
	' "$output/evidence-index.json" >/dev/null
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

# A local-only closeout retains an auditable NO-GO index but can never certify.
if run_local_only local-only "$auto" "$operator"; then
	echo 'local-only closeout unexpectedly produced GO' >&2
	exit 1
fi
jq -e '
	.collection.decision == "NO-GO" and
	any(.validation.errors[]; contains("local-only"))
' "$tmp/out-local-only/evidence-index.json" >/dev/null

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

mismatched_migration="$tmp/mismatched-migration.json"
jq '.candidate.migration_ceiling = "000124-rogue"' "$operator" >"$mismatched_migration"
expect_reject candidate-migration-mismatch "$auto" "$mismatched_migration"

missing_collection_result="$tmp/missing-collection-result.json"
jq 'del(.collection.result)' "$operator" >"$missing_collection_result"
expect_reject missing-collection-result "$auto" "$missing_collection_result"

blank_reviewer="$tmp/blank-reviewer.json"
jq '.entries[0].reviewer = "   "' "$auto" >"$blank_reviewer"
expect_reject blank-reviewer "$blank_reviewer" "$operator"

bad_hash="$tmp/bad-hash.json"
jq '.entries[0].sha256 = ("A" * 64)' "$auto" >"$bad_hash"
expect_reject invalid-hash "$bad_hash" "$operator"

bad_time="$tmp/bad-time.json"
jq '.entries[0].collected_utc = "2026-02-30T00:00:00Z"' "$auto" >"$bad_time"
expect_reject invalid-timestamp "$bad_time" "$operator"

bad_uri="$tmp/bad-uri.json"
jq '.entries[0].artifact_uri = "local://mutable/artifact.log"' "$auto" >"$bad_uri"
expect_reject invalid-uri "$bad_uri" "$operator"

unreachable_uri="$tmp/unreachable-uri.json"
jq '.entries[0].artifact_uri = "s3://bucket/lane/automated/missing.log"' "$auto" >"$unreachable_uri"
expect_no_go unreachable-uri "$unreachable_uri" "$operator"

remote_hash_mismatch="$tmp/remote-hash-mismatch.json"
jq '.entries[0].sha256 = ("c" * 64)' "$auto" >"$remote_hash_mismatch"
expect_no_go remote-hash-mismatch "$remote_hash_mismatch" "$operator"

if run_validator_with_flags source-no-lock "$auto" "$operator" 1 0; then
	echo 'source Object Lock failure unexpectedly produced GO' >&2
	exit 1
fi
jq -e '(.collection.decision == "NO-GO") and any(.validation.errors[]; contains("COMPLIANCE"))' \
	"$tmp/out-source-no-lock/evidence-index.json" >/dev/null

if run_validator_with_flags final-no-lock "$auto" "$operator" 0 1; then
	echo 'final Object Lock verification failure unexpectedly produced GO' >&2
	exit 1
fi
jq -e '(.collection.decision == "NO-GO") and any(.validation.errors[]; contains("COMPLIANCE"))' \
	"$tmp/out-final-no-lock/evidence-index.json" >/dev/null

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
expect_reject approved-na-control "$auto" "$approved_na"

approved_na_journey="$tmp/approved-na-journey.json"
jq '(.entries[] | select(.evidence_id == "J-ARAP-001")) |= (.result = "N/A" | .na_justification = "out of profile" | .na_approved_by = "release-owner" | .na_approved_utc = "2026-08-26T00:00:00Z")' "$auto" >"$approved_na_journey"
expect_reject approved-na-journey "$approved_na_journey" "$operator"

# Keep the artifact identity contract fail-closed at both verification
# boundaries: a bundle whose tag/commit labels drift from the requested
# candidate must not reach staging or certification.
grep -Fq 'grep -Fx "tag=${CANDIDATE_TAG}" "$extracted_dir/RELEASE_IDENTITY"' \
	"$script_dir/../.github/workflows/certify-v010-staging.yml"
grep -Fq 'grep -Fx "commit=${EXPECTED_SHA}" "$extracted_dir/RELEASE_IDENTITY"' \
	"$script_dir/../.github/workflows/certify-v010-staging.yml"
grep -Fq 'grep -Fx "tag=${candidate_tag}" "$release/RELEASE_IDENTITY"' \
	"$script_dir/../.github/workflows/deploy-native.yml"
grep -Fq 'grep -Fx "commit=${candidate_sha}" "$release/RELEASE_IDENTITY"' \
	"$script_dir/../.github/workflows/deploy-native.yml"
grep -Fq 'trap cleanup_supervised_on_exit EXIT' \
	"$script_dir/../.github/workflows/deploy-native.yml"
grep -Fq 'if [[ "$exit_code" -ne 0 && "$runtime_mode" == supervised-process ]]' \
	"$script_dir/../.github/workflows/deploy-native.yml"
grep -Fq 'if ! sudo -n systemctl --no-pager --full status odyssey-staging.service >/dev/null 2>&1; then' \
	"$script_dir/../.github/workflows/deploy-native.yml"
grep -Fq 'if ! sudo -n systemctl --no-pager --full status odyssey-staging-worker.service >/dev/null 2>&1; then' \
	"$script_dir/../.github/workflows/deploy-native.yml"
awk '
	/^      - name: Initialize certification evidence workspace$/ { in_step=1; next }
	in_step && /^      - name: / { exit found ? 0 : 1 }
	in_step && /^          set -euo pipefail$/ { found=1 }
	END { exit found ? 0 : 1 }
' "$script_dir/../.github/workflows/certify-v010-staging.yml"
grep -Fq 'before_run_ids=$(gh run list --workflow deploy-native.yml --branch "$deployment_ref" \' \
	"$script_dir/../.github/workflows/certify-v010-staging.yml"
if grep -Fq 'done < <(gh run list --workflow deploy-native.yml --branch "$deployment_ref"' \
	"$script_dir/../.github/workflows/certify-v010-staging.yml"; then
	echo 'certification deployment baseline must not hide gh run list failures' >&2
	exit 1
fi

echo 'staging certification closeout tests passed'
