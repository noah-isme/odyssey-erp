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
  --local-only           Do not upload to S3 even when a bucket is configured
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

load_source_metadata() {
	local file=$1
	source_tag=$(jq -r 'if (.candidate|type) == "object" then (.candidate.tag // .candidate.candidate_tag // "") else (.candidate_tag // "") end' "$file" 2>/dev/null || true)
	source_sha=$(jq -r 'if (.candidate|type) == "object" then (.candidate.resolved_sha // .candidate.sha // .candidate.candidate_sha // "") else (.candidate_sha // "") end' "$file" 2>/dev/null || true)
	source_profile=$(jq -r 'if (.candidate|type) == "object" then (.candidate.profile // "") else (.profile // "") end' "$file" 2>/dev/null || true)
	source_migration=$(jq -r 'if (.candidate|type) == "object" then (.candidate.migration_ceiling // "") else (.migration_ceiling // "") end' "$file" 2>/dev/null || true)
	source_prefix=$(jq -r '.collection.prefix // .prefix // .run.prefix // ""' "$file" 2>/dev/null || true)
	source_result=$(jq -r '.collection.result // .result // "PASS"' "$file" 2>/dev/null || true)
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
		validate_uri "$uri" "$source_prefix" || add_error "$id has a mutable, non-S3, or out-of-prefix artifact URI: ${uri:-missing}"
		[[ -n "$run_id" && "$run_id" != PENDING && "$run_id" != unknown && "$run_id" != UNKNOWN ]] || add_error "$id has no run ID"
		if [[ -z "$reviewer" ]]; then reviewer=$default_reviewer; fi
		if ! is_placeholder "$reviewer"; then add_error "$id has no reviewer"; fi

		if [[ "$result" == N/A ]]; then
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
	[[ "$source_migration_check" =~ ^000124($|[^0-9]) ]] || add_error "$source_name migration ceiling does not match 000124"
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

decision='GO'
collection_result='PASS'
if ((source_failure)) || ((${#validation_errors[@]} > 0)); then
	decision='NO-GO'
	collection_result='FAIL'
fi
errors_json='[]'
if ((${#validation_errors[@]} > 0)); then
	errors_json=$(printf '%s\n' "${validation_errors[@]}" | jq -Rsc 'split("\n") | map(select(length > 0))')
fi
missing_json='[]'
duplicate_json='[]'
unknown_json='[]'
if ((${#missing_ids[@]} > 0)); then missing_json=$(printf '%s\n' "${missing_ids[@]}" | jq -Rsc 'split("\n") | map(select(length > 0))'); fi
if ((${#duplicate_ids[@]} > 0)); then duplicate_json=$(printf '%s\n' "${duplicate_ids[@]}" | jq -Rsc 'split("\n") | map(select(length > 0))'); fi
if ((${#unknown_ids[@]} > 0)); then unknown_json=$(printf '%s\n' "${unknown_ids[@]}" | jq -Rsc 'split("\n") | map(select(length > 0))'); fi
control_aliases=$(jq -c '.control_aliases' "$contract_file")
generated_utc=$(date -u '+%Y-%m-%dT%H:%M:%SZ')

index_tmp="$output_dir/.evidence-index.$$.tmp"
jq -n \
	--arg generated_utc "$generated_utc" --arg result "$collection_result" --arg decision "$decision" \
	--arg prefix "$closeout_prefix" --arg profile "v0.10-core" --arg migration "000124" \
	--arg candidate_tag "$candidate_tag" --arg candidate_sha "$candidate_sha" \
	--argjson aliases "$control_aliases" --argjson sources "$source_records" \
	--argjson entries "$entries_json" --argjson errors "$errors_json" \
	--argjson missing "$missing_json" --argjson duplicates "$duplicate_json" --argjson unknown "$unknown_json" \
	'{schema_version:"v0.10-core-staging-closeout.v1",contract:{schema_version:"v0.10-core-certification-contract.v1",profile:$profile,migration_ceiling:$migration,control_aliases:$aliases},collection:{generated_utc:$generated_utc,result:$result,decision:$decision,prefix:$prefix},candidate:{tag:$candidate_tag,resolved_sha:$candidate_sha,profile:$profile,migration_ceiling:$migration},sources:$sources,entries:$entries,validation:{errors:$errors,missing_ids:$missing,duplicate_ids:$duplicates,unknown_ids:$unknown}}' \
	>"$index_tmp"
mv "$index_tmp" "$output_dir/evidence-index.json"
(cd "$output_dir" && sha256sum evidence-index.json) >"$output_dir/SHA256SUMS"

index_sha=$(sha256sum "$output_dir/evidence-index.json" | awk '{print $1}')
echo "local staging certification closeout created: $output_dir/evidence-index.json"
echo "closeout decision: $decision"

if [[ "$local_only" == false && -n "$bucket" ]]; then
	export AWS_ACCESS_KEY_ID=${AWS_ACCESS_KEY_ID:-${EVIDENCE_S3_ACCESS_KEY_ID:-}}
	export AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY:-${EVIDENCE_S3_SECRET_ACCESS_KEY:-}}
	missing_config=()
	[[ -n "$endpoint" ]] || missing_config+=("EVIDENCE_S3_ENDPOINT")
	[[ -n "$AWS_ACCESS_KEY_ID" ]] || missing_config+=("EVIDENCE_S3_ACCESS_KEY_ID")
	[[ -n "$AWS_SECRET_ACCESS_KEY" ]] || missing_config+=("EVIDENCE_S3_SECRET_ACCESS_KEY")
	command -v aws >/dev/null 2>&1 || missing_config+=("aws CLI")
	if ((${#missing_config[@]} > 0)); then
		printf 'final index retained locally; immutable upload preflight failed: %s\n' "${missing_config[*]}" >&2
		exit 1
	fi
	aws_args=(--region "$region")
	[[ -n "$endpoint" ]] && aws_args+=(--endpoint-url "$endpoint")
	base_args=(--bucket "$bucket" "${aws_args[@]}")
	retain_until=$(date -u -d '+7 years' '+%Y-%m-%dT%H:%M:%SZ')
	for file in "$output_dir/evidence-index.json" "$output_dir/SHA256SUMS"; do
		rel=${file#"$output_dir"/}
		key="$closeout_prefix/$rel"
		if head_output=$(aws s3api head-object "${base_args[@]}" --key "$key" 2>&1); then
			echo "refusing to overwrite existing object: s3://$bucket/$key" >&2
			exit 1
		fi
		if ! grep -Eqi '404|not[[:space:]-]?found|nosuchkey|no[[:space:]-]?such[[:space:]]key' <<<"$head_output"; then
			echo "unable to determine whether object exists: s3://$bucket/$key" >&2
			echo "$head_output" >&2
			exit 1
		fi
	done
	for file in "$output_dir/evidence-index.json" "$output_dir/SHA256SUMS"; do
		rel=${file#"$output_dir"/}
		key="$closeout_prefix/$rel"
		digest=$(sha256sum "$file" | awk '{print $1}')
		aws s3api put-object "${base_args[@]}" --key "$key" --body "$file" \
			--metadata "sha256=$digest" --object-lock-mode COMPLIANCE \
			--object-lock-retain-until-date "$retain_until" >/dev/null
	done
	echo "uploaded immutable staging certification closeout: s3://$bucket/$closeout_prefix (index sha256=$index_sha)"
fi

if [[ "$decision" != GO ]]; then
	exit 1
fi
