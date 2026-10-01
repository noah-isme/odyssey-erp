#!/usr/bin/env bash
set -euo pipefail

# Merge automated and operator certification lanes into a write-once closeout.
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
contract_file=${CERTIFICATION_CONTRACT_FILE:-"$script_dir/staging-certification-contract.json"}
automated_index=''
operator_index=''
output_dir=''
candidate_tag=${CERTIFICATION_CANDIDATE_TAG:-}
candidate_sha=${CERTIFICATION_CANDIDATE_SHA:-}
closeout_prefix=${EVIDENCE_CLOSEOUT_PREFIX:-}
automated_reviewer=${CERTIFICATION_AUTOMATED_REVIEWER:-}
operator_reviewer=${CERTIFICATION_OPERATOR_REVIEWER:-}
bucket=${EVIDENCE_S3_BUCKET:-}
endpoint=${EVIDENCE_S3_ENDPOINT:-}
region=${EVIDENCE_S3_REGION:-us-east-1}
aws_bin=${CERTIFICATION_AWS_CLI:-aws}
local_only=false

usage() {
	cat <<'EOF'
Usage: staging-certification-closeout.sh --automated FILE --operator FILE --output DIR [options]

Validate and merge automated and operator evidence indexes for v0.10-core.
The output directory is write-once and contains evidence-index.json plus
SHA256SUMS. Existing output directory contents and S3 objects are never
overwritten.

Required:
  --automated FILE       Automated-lane evidence-index.json
  --operator FILE        Operator-lane evidence-index.json
  --output DIR           Empty output directory for the final closeout

Options:
  --contract FILE        Canonical contract (default: scripts/staging-certification-contract.json)
  --candidate-tag TAG   Expected immutable candidate tag
  --candidate-sha SHA   Expected 40-hex candidate commit
  --prefix PREFIX       Immutable S3 object prefix for the final index
  --automated-reviewer NAME
                         Reviewer to apply when automated entries have no reviewer field
  --operator-reviewer NAME
                         Reviewer to apply when operator entries have no reviewer field
  --bucket NAME          Upload the final index to this S3 bucket
  --endpoint-url URL    S3-compatible endpoint
  --region REGION        S3 region (default: EVIDENCE_S3_REGION or us-east-1)
  --local-only           Build a local NO-GO index without uploading to S3
  --help                 Show this help

Each source index must carry a candidate tag/commit, v0.10-core profile,
000124 migration ceiling, and collection prefix. Every entry must use an
s3:// URI beneath that source prefix, a lowercase 64-hex SHA-256, an RFC3339
UTC timestamp, non-empty details, and a reviewer. N/A additionally requires
justification, approver, and approval UTC timestamp.
EOF
}

while (($#)); do
	case "$1" in
		--automated|--automated-index) [[ $# -ge 2 ]] || { echo "missing value for $1" >&2; exit 2; }; automated_index=$2; shift 2 ;;
		--operator|--operator-index) [[ $# -ge 2 ]] || { echo "missing value for $1" >&2; exit 2; }; operator_index=$2; shift 2 ;;
		--output|--output-dir) [[ $# -ge 2 ]] || { echo "missing value for $1" >&2; exit 2; }; output_dir=$2; shift 2 ;;
		--contract) [[ $# -ge 2 ]] || { echo "missing value for --contract" >&2; exit 2; }; contract_file=$2; shift 2 ;;
		--candidate-tag) [[ $# -ge 2 ]] || { echo "missing value for --candidate-tag" >&2; exit 2; }; candidate_tag=$2; shift 2 ;;
		--candidate-sha) [[ $# -ge 2 ]] || { echo "missing value for --candidate-sha" >&2; exit 2; }; candidate_sha=$2; shift 2 ;;
		--prefix|--final-prefix) [[ $# -ge 2 ]] || { echo "missing value for $1" >&2; exit 2; }; closeout_prefix=$2; shift 2 ;;
		--automated-reviewer) [[ $# -ge 2 ]] || { echo "missing value for --automated-reviewer" >&2; exit 2; }; automated_reviewer=$2; shift 2 ;;
		--operator-reviewer) [[ $# -ge 2 ]] || { echo "missing value for --operator-reviewer" >&2; exit 2; }; operator_reviewer=$2; shift 2 ;;
		--bucket) [[ $# -ge 2 ]] || { echo "missing value for --bucket" >&2; exit 2; }; bucket=$2; shift 2 ;;
		--endpoint-url) [[ $# -ge 2 ]] || { echo "missing value for --endpoint-url" >&2; exit 2; }; endpoint=$2; shift 2 ;;
		--region) [[ $# -ge 2 ]] || { echo "missing value for --region" >&2; exit 2; }; region=$2; shift 2 ;;
		--local-only) local_only=true; shift ;;
		-h|--help) usage; exit 0 ;;
		*) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
	esac
done

[[ -n "$automated_index" && -f "$automated_index" ]] || { echo "--automated must name an existing file" >&2; exit 2; }
[[ -n "$operator_index" && -f "$operator_index" ]] || { echo "--operator must name an existing file" >&2; exit 2; }
[[ -n "$output_dir" ]] || { echo "--output is required" >&2; exit 2; }
[[ -f "$contract_file" ]] || { echo "missing certification contract: $contract_file" >&2; exit 2; }
command -v jq >/dev/null 2>&1 || { echo "jq is required" >&2; exit 1; }
command -v sha256sum >/dev/null 2>&1 || { echo "sha256sum is required" >&2; exit 1; }

if ! jq -e '
	. as $contract |
	type == "object" and
	.schema_version == "v0.10-core-certification-contract.v1" and
	.profile == "v0.10-core" and
	.migration_ceiling == "000124" and
	(.evidence | type == "array" and length == 25) and
	([.evidence[].evidence_id] | length == (unique | length)) and
	(all(.evidence[]; (.evidence_id | type == "string") and (.lane == "automated" or .lane == "operator"))) and
	(.control_aliases | type == "array" and ([.[].control_id] | sort == ["ENV-003", "ENV-004"])) and
	(all($contract.control_aliases[]; all(.evidence_ids[]; . as $id | any($contract.evidence[]; .evidence_id == $id))))
' "$contract_file" >/dev/null 2>&1; then
	echo "invalid certification contract: $contract_file" >&2
	exit 2
fi

declare -a all_ids=()
mapfile -t all_ids < <(jq -r '.evidence[].evidence_id' "$contract_file")
declare -A canonical_ids=()
declare -A expected_lane=()
for id in "${all_ids[@]}"; do
	canonical_ids["$id"]=1
done
while IFS=$'\t' read -r id id_lane; do
	expected_lane["$id"]=$id_lane
done < <(jq -r '.evidence[] | [.evidence_id, .lane] | @tsv' "$contract_file")

declare -a validation_errors=()
declare -a missing_ids=()
declare -a duplicate_ids=()
declare -a unknown_ids=()
hard_failure=0
source_failure=0

add_error() {
	validation_errors+=("$1")
	hard_failure=1
}

is_placeholder() {
	local value=$1
	value=${value#"${value%%[![:space:]]*}"}
	value=${value%"${value##*[![:space:]]}"}
	[[ -n "$value" && "$value" != PENDING && "$value" != UNKNOWN && "$value" != unknown && "$value" != *'<'* && "$value" != *'>'* ]]
}

is_utc_timestamp() {
	local timestamp=$1
	[[ "$timestamp" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,9})?Z$ ]] || return 1
	local normalized expected
	normalized=$(date -u -d "$timestamp" '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null) || return 1
	if [[ "$timestamp" == *.*Z ]]; then
		expected="${timestamp%%.*}Z"
	else
		expected="$timestamp"
	fi
	[[ "$normalized" == "$expected" ]]
}

is_prefix() {
	local value=$1
	[[ -n "$value" && "$value" != /* && "$value" != */ && "$value" != *'..'* && "$value" != *[[:space:]]* && "$value" != *$'\n'* ]]
}

validate_uri() {
	local uri=$1 source_prefix=$2
	[[ "$uri" == s3://* ]] || return 1
	[[ "$uri" != *'?'* && "$uri" != *'#'* && "$uri" != *$'\n'* && "$uri" != *[[:space:]]* ]] || return 1
	local rest=${uri#s3://}
	local uri_bucket=${rest%%/*}
	local key=${rest#*/}
	[[ -n "$uri_bucket" && "$key" != "$rest" ]] || return 1
	[[ "$uri_bucket" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] || return 1
	[[ -n "$key" && "$key" == "$source_prefix/"* ]] || return 1
	[[ "$key" != *'//'* ]] || return 1
	[[ "$key" != */./* && "$key" != */../* && "$key" != ../* && "$key" != */.. && "$key" != */. ]] || return 1
	[[ "$key" != */ ]] || return 1
}

is_mandatory_evidence() {
	# The v0.10-core registry currently contains only in-profile release,
	# environment, migration, operational, journey, isolation, and approval
	# evidence. None of those rows may be waived with N/A. If an explicitly
	# out-of-profile row is added later, it must be excluded here deliberately
	# and documented in the contract/runbook.
	case "$1" in
		REL-00[1-4]|ENV-00[1-2]|DB-00[1-4]|OPS-00[1-5]|ISO-00[1-4]|APR-001|\
		J-ARAP-001|J-SALES-001|J-INV-001|J-DOC-001|J-CMMS-001)
			return 0
		;;
		*) return 1 ;;
	esac
}

append_gate_error() {
	# Gate errors are kept separate from structural validation failures so a
	# local NO-GO index can still be written when cloud verification is absent.
	validation_errors+=("$1")
}

is_lock_retained_until() {
	local retained=$1 minimum=$2
	[[ -n "$retained" && "$retained" != None && "$retained" != null ]] || return 1
	local retained_epoch minimum_epoch
	retained_epoch=$(date -u -d "$retained" '+%s' 2>/dev/null) || return 1
	minimum_epoch=$(date -u -d "$minimum" '+%s' 2>/dev/null) || return 1
	((retained_epoch >= minimum_epoch))
}

verify_locked_object() {
	local label=$1 response=$2 expected_sha=$3 minimum_retention=$4
	local mode retained remote_sha content_length
	if ! jq -e 'type == "object"' <<<"$response" >/dev/null 2>&1; then
		append_gate_error "$label returned invalid S3 metadata"
		return 1
	fi
	mode=$(jq -r '.ObjectLockMode // ""' <<<"$response")
	retained=$(jq -r '.ObjectLockRetainUntilDate // ""' <<<"$response")
	remote_sha=$(jq -r '.Metadata.sha256 // .metadata.sha256 // ""' <<<"$response")
	content_length=$(jq -r '.ContentLength // 0' <<<"$response")
	[[ "$mode" == COMPLIANCE ]] || append_gate_error "$label is not protected by COMPLIANCE Object Lock (mode: ${mode:-missing})"
	is_lock_retained_until "$retained" "$minimum_retention" || append_gate_error "$label Object Lock retention is missing or expired"
	[[ "$remote_sha" == "$expected_sha" ]] || append_gate_error "$label SHA-256 metadata does not match the recorded digest"
	[[ "$content_length" =~ ^[0-9]+$ && "$content_length" -gt 0 ]] || append_gate_error "$label is empty or has invalid ContentLength"
	[[ "$mode" == COMPLIANCE && "$remote_sha" == "$expected_sha" && "$content_length" =~ ^[0-9]+$ && "$content_length" -gt 0 ]] || return 1
	is_lock_retained_until "$retained" "$minimum_retention"
}

verify_source_artifacts() {
	local minimum_retention
	minimum_retention=$(date -u -d '+7 years' '+%Y-%m-%dT%H:%M:%SZ')
	declare -A checked_artifacts=()
	for record in "${artifact_records[@]}"; do
		local id uri sha rest uri_bucket key response
		IFS=$'\t' read -r id uri sha <<<"$record"
		local artifact_cache_key="$uri|$sha"
		if [[ -n "${checked_artifacts[$artifact_cache_key]+set}" ]]; then
			continue
		fi
		checked_artifacts["$artifact_cache_key"]=1
		rest=${uri#s3://}
		uri_bucket=${rest%%/*}
		key=${rest#*/}
		if [[ "$uri_bucket" != "$bucket" ]]; then
			append_gate_error "$id artifact URI bucket $uri_bucket does not match immutable evidence bucket $bucket"
			continue
		fi
		if ! response=$("$aws_bin" s3api head-object --bucket "$uri_bucket" "${aws_args[@]}" --key "$key" --output json 2>&1); then
			append_gate_error "$id artifact URI is not reachable: $uri"
			continue
		fi
		verify_locked_object "$id artifact $uri" "$response" "$sha" "$minimum_retention" || true
	done
}

load_source_metadata() {
	local file=$1
	source_tag=$(jq -r 'if (.candidate|type) == "object" then (.candidate.tag // .candidate.candidate_tag // "") else (.candidate_tag // "") end' "$file" 2>/dev/null || true)
	source_sha=$(jq -r 'if (.candidate|type) == "object" then (.candidate.resolved_sha // .candidate.sha // .candidate.candidate_sha // "") else (.candidate_sha // "") end' "$file" 2>/dev/null || true)
	source_profile=$(jq -r 'if (.candidate|type) == "object" then (.candidate.profile // "") else (.profile // "") end' "$file" 2>/dev/null || true)
	source_migration=$(jq -r 'if (.candidate|type) == "object" then (.candidate.migration_ceiling // "") else (.migration_ceiling // "") end' "$file" 2>/dev/null || true)
	source_prefix=$(jq -r '.collection.prefix // .prefix // .run.prefix // ""' "$file" 2>/dev/null || true)
	source_result=$(jq -r '.collection.result // .result // ""' "$file" 2>/dev/null || true)
	source_run_id=$(jq -r '.run.id // .run_id // ""' "$file" 2>/dev/null || true)
	if [[ "$source_migration" =~ ^([0-9]{6}) ]]; then
		source_migration_number=${BASH_REMATCH[1]}
	else
		source_migration_number=''
	fi
}

validate_source_shape() {
	local file=$1 lane=$2
	if ! jq -e 'type == "object" and (.entries | type == "array")' "$file" >/dev/null 2>&1; then
		add_error "$lane index must be a JSON object with an entries array: $file"
		return 1
	fi
	load_source_metadata "$file"
	if ! is_placeholder "$source_tag"; then add_error "$lane index has no candidate tag"; fi
	if [[ ! "$source_sha" =~ ^[0-9a-f]{40}$ ]]; then add_error "$lane index has an invalid resolved candidate SHA"; fi
	if [[ "$source_profile" != "v0.10-core" ]]; then add_error "$lane index profile is ${source_profile:-missing}, want v0.10-core"; fi
	if [[ "$source_migration_number" != 000124 ]]; then add_error "$lane index migration ceiling is ${source_migration:-missing}, want 000124"; fi
	if ! is_prefix "$source_prefix"; then add_error "$lane index has an invalid collection prefix"; fi
	if [[ -z "$source_run_id" || "$source_run_id" == unknown || "$source_run_id" == UNKNOWN ]]; then
		add_error "$lane index has no collection run ID"
	fi
	case "$source_result" in
		PASS) ;;
		FAIL|NO-GO|NO_GO) source_failure=1; validation_errors+=("$lane source index result is $source_result") ;;
		*) add_error "$lane index has invalid collection result: ${source_result:-missing}" ;;
	esac
	if (( $(jq -r '(.validation.errors // []) | length' "$file" 2>/dev/null || echo 0) > 0 )); then
		source_failure=1
		while IFS= read -r source_error; do
			[[ -n "$source_error" ]] && validation_errors+=("$lane source validation: $source_error")
		done < <(jq -r '.validation.errors[]? // empty' "$file" 2>/dev/null || true)
	fi
	return 0
}

declare -A seen_ids=()
declare -a final_entries=()
declare -a artifact_records=()
process_source() {
	local file=$1 lane=$2 default_reviewer=$3
	if ! validate_source_shape "$file" "$lane"; then
		return 0
	fi
	local source_name
	source_name=$(basename "$file")
	local entry
	while IFS= read -r entry; do
		[[ -n "$entry" ]] || continue
		local id result collected details uri sha run_id reviewer
		if ! jq -e '(.evidence_id | type) == "string" and (.evidence_id | length) > 0' <<<"$entry" >/dev/null 2>&1; then
			unknown_ids+=("<missing>")
			add_error "$lane index contains unknown or missing evidence ID"
			continue
		fi
		id=$(jq -r '.evidence_id' <<<"$entry")
		if [[ "${canonical_ids[$id]+set}" != set ]]; then
			unknown_ids+=("$id")
			add_error "$lane index contains unknown evidence ID: $id"
			continue
		fi
		if [[ -n "${seen_ids[$id]+set}" ]]; then
			duplicate_ids+=("$id")
			add_error "duplicate certification evidence ID: $id"
			continue
		fi
		seen_ids["$id"]=$lane
		if [[ "${expected_lane[$id]}" != "$lane" ]]; then
			add_error "$lane index contains $id, assigned to ${expected_lane[$id]} lane"
		fi

		result=$(jq -r '.result // ""' <<<"$entry" 2>/dev/null || true)
		collected=$(jq -r '.collected_utc // ""' <<<"$entry" 2>/dev/null || true)
		details=$(jq -r '.details // ""' <<<"$entry" 2>/dev/null || true)
		uri=$(jq -r '.artifact_uri // ""' <<<"$entry" 2>/dev/null || true)
		sha=$(jq -r '.sha256 // ""' <<<"$entry" 2>/dev/null || true)
		run_id=$(jq -r '.run_id // ""' <<<"$entry" 2>/dev/null || true)
		reviewer=$(jq -r '.reviewer // ""' <<<"$entry" 2>/dev/null || true)
		jq -e '(.result | type) == "string" and (.details | type) == "string" and (.collected_utc | type) == "string" and (.artifact_uri | type) == "string" and (.sha256 | type) == "string"' <<<"$entry" >/dev/null 2>&1 || add_error "$id is missing a required evidence field"
		[[ "$result" == PASS || "$result" == FAIL || "$result" == N/A ]] || add_error "$id has invalid result: ${result:-missing}"
		[[ -n "$details" && "$details" != PENDING ]] || add_error "$id has missing or pending details"
		is_utc_timestamp "$collected" || add_error "$id has invalid UTC timestamp: ${collected:-missing}"
		[[ "$sha" =~ ^[0-9a-f]{64}$ ]] || add_error "$id has invalid lowercase SHA-256"
		if validate_uri "$uri" "$source_prefix"; then
			artifact_records+=("$id"$'\t'"$uri"$'\t'"$sha")
		else
			add_error "$id has a mutable, non-S3, or out-of-prefix artifact URI: ${uri:-missing}"
		fi
		[[ -n "$run_id" && "$run_id" != PENDING && "$run_id" != unknown && "$run_id" != UNKNOWN ]] || add_error "$id has no run ID"
		if [[ -z "$reviewer" ]]; then reviewer=$default_reviewer; fi
		if ! is_placeholder "$reviewer"; then add_error "$id has no reviewer"; fi

		if [[ "$result" == N/A ]]; then
			if is_mandatory_evidence "$id"; then
				add_error "$id is mandatory for v0.10-core and cannot be N/A"
			fi
			local na_justification na_approved_by na_approved_utc
			na_justification=$(jq -r '.na_justification // .justification // .na_approval.justification // .approval.justification // ""' <<<"$entry" 2>/dev/null || true)
			na_approved_by=$(jq -r '.na_approved_by // .na_approval.approved_by // .approval.approved_by // ""' <<<"$entry" 2>/dev/null || true)
			na_approved_utc=$(jq -r '.na_approved_utc // .na_approval.approved_utc // .approval.approved_utc // ""' <<<"$entry" 2>/dev/null || true)
			[[ -n "$na_justification" && "$na_justification" != PENDING ]] || add_error "$id is N/A without justification"
			is_placeholder "$na_approved_by" || add_error "$id is N/A without an approver"
			is_utc_timestamp "$na_approved_utc" || add_error "$id is N/A without a valid approval UTC timestamp"
		fi
		local normalized
		normalized=$(jq -c --arg lane "$lane" --arg source "$source_name" --arg reviewer "$reviewer" \
			'. + {lane:$lane, source_index:$source, reviewer:$reviewer}' <<<"$entry")
		final_entries+=("$normalized")
		[[ "$result" == FAIL ]] && source_failure=1
	done < <(jq -c '.entries[]' "$file" 2>/dev/null || true)
	return 0
}

process_source "$automated_index" automated "$automated_reviewer"
automated_tag=${source_tag:-}
automated_sha=${source_sha:-}
automated_profile=${source_profile:-}
automated_migration=${source_migration:-}
automated_prefix=${source_prefix:-}
automated_run_id=${source_run_id:-}
process_source "$operator_index" operator "$operator_reviewer"
operator_tag=${source_tag:-}
operator_sha=${source_sha:-}
operator_profile=${source_profile:-}
operator_migration=${source_migration:-}
operator_prefix=${source_prefix:-}
operator_run_id=${source_run_id:-}

if [[ -z "$candidate_tag" ]]; then candidate_tag=$automated_tag; fi
if [[ -z "$candidate_sha" ]]; then candidate_sha=$automated_sha; fi
is_placeholder "$candidate_tag" || add_error "expected candidate tag is missing"
[[ "$candidate_sha" =~ ^[0-9a-f]{40}$ ]] || add_error "expected candidate SHA is not a 40-hex lowercase commit"
for source_name in automated operator; do
	case "$source_name" in
		automated) source_tag_check=$automated_tag; source_sha_check=$automated_sha; source_profile_check=$automated_profile; source_migration_check=$automated_migration ;;
		operator) source_tag_check=$operator_tag; source_sha_check=$operator_sha; source_profile_check=$operator_profile; source_migration_check=$operator_migration ;;
	esac
	[[ "$source_tag_check" == "$candidate_tag" ]] || add_error "$source_name candidate tag does not match $candidate_tag"
	[[ "$source_sha_check" == "$candidate_sha" ]] || add_error "$source_name candidate SHA does not match $candidate_sha"
	[[ "$source_profile_check" == "v0.10-core" ]] || add_error "$source_name profile does not match v0.10-core"
	[[ "$source_migration_check" == 000124 ]] || add_error "$source_name migration ceiling does not match 000124"
done

for id in "${all_ids[@]}"; do
	if [[ -z "${seen_ids[$id]+set}" ]]; then
		missing_ids+=("$id")
		add_error "missing certification evidence ID: $id"
	fi
done

if ((${#unknown_ids[@]} > 0)); then
	mapfile -t unknown_ids < <(printf '%s\n' "${unknown_ids[@]}" | sort -u)
fi
if ((${#duplicate_ids[@]} > 0)); then
	mapfile -t duplicate_ids < <(printf '%s\n' "${duplicate_ids[@]}" | sort -u)
fi

if ((hard_failure)); then
	printf 'staging certification closeout rejected:\n' >&2
	printf '  - %s\n' "${validation_errors[@]}" >&2
	exit 1
fi

if [[ -z "$closeout_prefix" ]]; then
	closeout_prefix="staging-certification-closeout/${candidate_sha}/$(date -u '+%Y%m%dT%H%M%SZ')-$$"
fi
if ! is_prefix "$closeout_prefix"; then
	echo "invalid final evidence prefix: $closeout_prefix" >&2
	exit 2
fi

if [[ -e "$output_dir" ]]; then
	if [[ ! -d "$output_dir" || -n "$(find "$output_dir" -mindepth 1 -print -quit)" ]]; then
		echo "output directory must be absent or empty (write-once): $output_dir" >&2
		exit 2
	fi
else
	mkdir -p "$output_dir"
fi

entries_json='[]'
if ((${#final_entries[@]} > 0)); then
	entries_json=$(printf '%s\n' "${final_entries[@]}" | jq -sc 'sort_by(.evidence_id)')
fi
source_records=$(jq -cn \
	--arg lane automated --arg file "$(basename "$automated_index")" --arg prefix "$automated_prefix" --arg run_id "$automated_run_id" \
	'{lane:$lane,index_file:$file,prefix:$prefix,run_id:$run_id}')
source_records=$(jq -cn --argjson first "$source_records" \
	--arg lane operator --arg file "$(basename "$operator_index")" --arg prefix "$operator_prefix" --arg run_id "$operator_run_id" \
	'[$first, {lane:$lane,index_file:$file,prefix:$prefix,run_id:$run_id}]')

missing_json='[]'
duplicate_json='[]'
unknown_json='[]'
if ((${#missing_ids[@]} > 0)); then missing_json=$(printf '%s\n' "${missing_ids[@]}" | jq -Rsc 'split("\n") | map(select(length > 0))'); fi
if ((${#duplicate_ids[@]} > 0)); then duplicate_json=$(printf '%s\n' "${duplicate_ids[@]}" | jq -Rsc 'split("\n") | map(select(length > 0))'); fi
if ((${#unknown_ids[@]} > 0)); then unknown_json=$(printf '%s\n' "${unknown_ids[@]}" | jq -Rsc 'split("\n") | map(select(length > 0))'); fi
control_aliases=$(jq -c '.control_aliases' "$contract_file")
generated_utc=$(date -u '+%Y-%m-%dT%H:%M:%SZ')

aws_args=(--region "$region")
[[ -n "$endpoint" ]] && aws_args+=(--endpoint-url "$endpoint")

calculate_decision() {
	decision='GO'
	collection_result='PASS'
	if ((source_failure)) || ((${#validation_errors[@]} > 0)); then
		decision='NO-GO'
		collection_result='FAIL'
	fi
}

render_index_files() {
	local index_path=$1 sums_path=$2 render_decision=$3 render_result=$4
	local errors_json='[]'
	if ((${#validation_errors[@]} > 0)); then
		errors_json=$(printf '%s\n' "${validation_errors[@]}" | jq -Rsc 'split("\n") | map(select(length > 0))')
	fi
	jq -n \
		--arg generated_utc "$generated_utc" --arg result "$render_result" --arg decision "$render_decision" \
		--arg prefix "$closeout_prefix" --arg profile "v0.10-core" --arg migration "000124" \
		--arg candidate_tag "$candidate_tag" --arg candidate_sha "$candidate_sha" \
		--argjson aliases "$control_aliases" --argjson sources "$source_records" \
		--argjson entries "$entries_json" --argjson errors "$errors_json" \
		--argjson missing "$missing_json" --argjson duplicates "$duplicate_json" --argjson unknown "$unknown_json" \
		'{schema_version:"v0.10-core-staging-closeout.v1",contract:{schema_version:"v0.10-core-certification-contract.v1",profile:$profile,migration_ceiling:$migration,control_aliases:$aliases},collection:{generated_utc:$generated_utc,result:$result,decision:$decision,prefix:$prefix},candidate:{tag:$candidate_tag,resolved_sha:$candidate_sha,profile:$profile,migration_ceiling:$migration},sources:$sources,entries:$entries,validation:{errors:$errors,missing_ids:$missing,duplicate_ids:$duplicates,unknown_ids:$unknown}}' \
		>"$index_path"
	printf '%s  evidence-index.json\n' "$(sha256sum "$index_path" | awk '{print $1}')" >"$sums_path"
}

index_tmp=''
sums_tmp=''
cleanup_staging() {
	[[ -z "$index_tmp" ]] || rm -f -- "$index_tmp"
	[[ -z "$sums_tmp" ]] || rm -f -- "$sums_tmp"
}
trap cleanup_staging EXIT

upload_config_ok=true
if [[ "$local_only" == true ]]; then
	append_gate_error 'local-only closeout cannot produce GO without immutable S3 upload and Object Lock verification'
	upload_config_ok=false
else
	export AWS_ACCESS_KEY_ID=${AWS_ACCESS_KEY_ID:-${EVIDENCE_S3_ACCESS_KEY_ID:-}}
	export AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY:-${EVIDENCE_S3_SECRET_ACCESS_KEY:-}}
	missing_config=()
	[[ -n "$bucket" ]] || missing_config+=("EVIDENCE_S3_BUCKET")
	[[ -n "$endpoint" ]] || missing_config+=("EVIDENCE_S3_ENDPOINT")
	[[ -n "$AWS_ACCESS_KEY_ID" ]] || missing_config+=("EVIDENCE_S3_ACCESS_KEY_ID")
	[[ -n "$AWS_SECRET_ACCESS_KEY" ]] || missing_config+=("EVIDENCE_S3_SECRET_ACCESS_KEY")
	command -v "$aws_bin" >/dev/null 2>&1 || missing_config+=("aws CLI ($aws_bin)")
	if [[ -n "$bucket" && ! "$bucket" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]]; then
		missing_config+=("valid S3 bucket name")
	fi
	if ((${#missing_config[@]} > 0)); then
		append_gate_error "immutable S3 closeout preflight failed: ${missing_config[*]}"
		upload_config_ok=false
	fi
fi

# A closeout cannot certify a source row merely because its URI is syntactically
# plausible. Verify that every recorded object exists in the configured bucket,
# carries the uploader's digest metadata, and remains under COMPLIANCE retention.
if [[ "$upload_config_ok" == true ]]; then
	verify_source_artifacts
fi

calculate_decision
if [[ "$upload_config_ok" != true ]]; then
	render_index_files "$output_dir/evidence-index.json" "$output_dir/SHA256SUMS" "$decision" "$collection_result"
	index_sha=$(sha256sum "$output_dir/evidence-index.json" | awk '{print $1}')
	echo "local staging certification closeout created: $output_dir/evidence-index.json"
	echo "closeout decision: $decision"
	exit 1
fi

# Keep the candidate index out of its final write-once location until both
# closeout objects have been uploaded and their Object Lock metadata verified.
index_tmp="$output_dir/.evidence-index.$$.tmp"
sums_tmp="$output_dir/.SHA256SUMS.$$.tmp"
render_index_files "$index_tmp" "$sums_tmp" "$decision" "$collection_result"

base_args=(--bucket "$bucket" "${aws_args[@]}")
retain_until=$(date -u -d '+7 years' '+%Y-%m-%dT%H:%M:%SZ')
upload_failed=0
for rel in evidence-index.json SHA256SUMS; do
	key="$closeout_prefix/$rel"
	if head_output=$("$aws_bin" s3api head-object "${base_args[@]}" --key "$key" --output json 2>&1); then
		append_gate_error "refusing to overwrite existing immutable object: s3://$bucket/$key"
		upload_failed=1
	elif ! grep -Eqi '404|not[[:space:]-]?found|nosuchkey|no[[:space:]-]?such[[:space:]]key' <<<"$head_output"; then
		append_gate_error "unable to determine whether immutable object exists: s3://$bucket/$key"
		upload_failed=1
	fi
done

if ((upload_failed == 0)); then
	for rel in evidence-index.json SHA256SUMS; do
		file="$output_dir/.$rel.$$.tmp"
		if [[ "$rel" == evidence-index.json ]]; then file=$index_tmp; else file=$sums_tmp; fi
		key="$closeout_prefix/$rel"
		digest=$(sha256sum "$file" | awk '{print $1}')
		if ! put_output=$("$aws_bin" s3api put-object "${base_args[@]}" --key "$key" --body "$file" \
			--metadata "sha256=$digest" --object-lock-mode COMPLIANCE \
			--object-lock-retain-until-date "$retain_until" 2>&1); then
			append_gate_error "immutable closeout upload failed for s3://$bucket/$key"
			upload_failed=1
			break
		fi
	done
fi

if ((upload_failed == 0)); then
	for rel in evidence-index.json SHA256SUMS; do
		file="$index_tmp"
		[[ "$rel" == SHA256SUMS ]] && file=$sums_tmp
		key="$closeout_prefix/$rel"
		digest=$(sha256sum "$file" | awk '{print $1}')
		if ! head_output=$("$aws_bin" s3api head-object "${base_args[@]}" --key "$key" --output json 2>&1); then
			append_gate_error "unable to verify uploaded immutable object: s3://$bucket/$key"
			upload_failed=1
			continue
		fi
		if ! verify_locked_object "uploaded closeout s3://$bucket/$key" "$head_output" "$digest" "$retain_until"; then
			upload_failed=1
		fi
	done
fi

if ((upload_failed != 0)); then
	rm -f -- "$index_tmp" "$sums_tmp"
	calculate_decision
	render_index_files "$output_dir/evidence-index.json" "$output_dir/SHA256SUMS" "$decision" "$collection_result"
	index_sha=$(sha256sum "$output_dir/evidence-index.json" | awk '{print $1}')
	echo "local staging certification closeout created: $output_dir/evidence-index.json"
	echo "closeout decision: $decision"
	exit 1
fi

mv "$index_tmp" "$output_dir/evidence-index.json"
mv "$sums_tmp" "$output_dir/SHA256SUMS"
index_sha=$(sha256sum "$output_dir/evidence-index.json" | awk '{print $1}')
echo "local staging certification closeout created: $output_dir/evidence-index.json"
echo "uploaded immutable staging certification closeout: s3://$bucket/$closeout_prefix (index sha256=$index_sha)"
echo "closeout decision: $decision"

if [[ "$decision" != GO ]]; then
	exit 1
fi
