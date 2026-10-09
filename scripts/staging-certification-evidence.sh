#!/usr/bin/env bash
set -euo pipefail

usage() {
	cat <<'EOF'
Usage: staging-certification-evidence.sh --candidate DIR --evidence DIR [options]

Copy a staging certification candidate into an evidence bundle, redact secrets from
text files, parse certification records, create a SHA-256 manifest and JSON index,
and store the bundle in S3 Object Lock. Local evidence is assembled before cloud
configuration is checked so early workflow failures remain auditable.

Required:
  --candidate DIR       Candidate artifacts/logs to collect
  --evidence DIR        Local output directory for the collected evidence

Options:
  --bucket NAME         S3 bucket (default: EVIDENCE_S3_BUCKET)
  --prefix PREFIX       Object prefix (default: EVIDENCE_S3_PREFIX or timestamp)
  --endpoint-url URL    S3-compatible endpoint (default: EVIDENCE_S3_ENDPOINT)
  --region REGION       S3 region (default: EVIDENCE_S3_REGION or us-east-1)
  --contract FILE       Canonical evidence contract (default: scripts/staging-certification-contract.json)
  --lane NAME           Evidence lane: automated, operator, or all (default: automated)
  --expected-evidence-ids CSV
                        Override the contract lane IDs (legacy compatibility)
  --local-only          Build the local bundle without uploading to S3
  --help                Show this help

Credentials are read from EVIDENCE_S3_ACCESS_KEY_ID and
EVIDENCE_S3_SECRET_ACCESS_KEY. The objects are retained for seven years in
COMPLIANCE mode. Existing object keys are never overwritten.
EOF
}

candidate=''
evidence=''
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
contract_file=${CERTIFICATION_CONTRACT_FILE:-"$script_dir/staging-certification-contract.json"}
lane=${CERTIFICATION_LANE:-automated}
bucket=${EVIDENCE_S3_BUCKET:-}
prefix=${EVIDENCE_S3_PREFIX:-}
endpoint=${EVIDENCE_S3_ENDPOINT:-}
region=${EVIDENCE_S3_REGION:-us-east-1}
expected_ids_csv=${EXPECTED_EVIDENCE_IDS:-}
local_only=false

while (($#)); do
	case "$1" in
		--candidate|--candidate-dir) [[ $# -ge 2 ]] || { echo "missing value for $1" >&2; exit 2; }; candidate=$2; shift 2 ;;
		--evidence|--evidence-dir) [[ $# -ge 2 ]] || { echo "missing value for $1" >&2; exit 2; }; evidence=$2; shift 2 ;;
		--bucket) [[ $# -ge 2 ]] || { echo "missing value for --bucket" >&2; exit 2; }; bucket=$2; shift 2 ;;
		--prefix) [[ $# -ge 2 ]] || { echo "missing value for --prefix" >&2; exit 2; }; prefix=$2; shift 2 ;;
		--endpoint-url) [[ $# -ge 2 ]] || { echo "missing value for --endpoint-url" >&2; exit 2; }; endpoint=$2; shift 2 ;;
		--region) [[ $# -ge 2 ]] || { echo "missing value for --region" >&2; exit 2; }; region=$2; shift 2 ;;
		--contract) [[ $# -ge 2 ]] || { echo "missing value for --contract" >&2; exit 2; }; contract_file=$2; shift 2 ;;
		--lane) [[ $# -ge 2 ]] || { echo "missing value for --lane" >&2; exit 2; }; lane=$2; shift 2 ;;
		--expected-evidence-ids) [[ $# -ge 2 ]] || { echo "missing value for --expected-evidence-ids" >&2; exit 2; }; expected_ids_csv=$2; shift 2 ;;
		--local-only) local_only=true; shift ;;
		-h|--help) usage; exit 0 ;;
		*) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
	esac
done

[[ -n "$candidate" && -d "$candidate" ]] || { echo "--candidate must name an existing directory" >&2; exit 2; }
[[ -n "$evidence" ]] || { echo "--evidence is required" >&2; exit 2; }
[[ "$lane" == automated || "$lane" == operator || "$lane" == all ]] || {
	echo "--lane must be automated, operator, or all" >&2
	exit 2
}
[[ -f "$contract_file" ]] || { echo "missing certification contract: $contract_file" >&2; exit 2; }
command -v sha256sum >/dev/null 2>&1 || { echo "sha256sum is required" >&2; exit 1; }

mkdir -p "$evidence"
if [[ -n "$(find "$evidence" -mindepth 1 -print -quit)" ]]; then
	echo "evidence directory must be empty: $evidence" >&2
	exit 2
fi

cp -a "$candidate"/. "$evidence"/

if [[ ! -f "$evidence/staging-certification.log" ]]; then
	printf 'staging certification log was not created before workflow failure\n' \
		>"$evidence/staging-certification.log"
fi

prefix=${prefix#/}
if [[ -z "$prefix" ]]; then
	prefix="staging-certification/${GITHUB_RUN_ID:-local}/${GITHUB_RUN_ATTEMPT:-1}/$(date -u +%Y%m%dT%H%M%SZ)-$$"
fi

# Redact common credential forms only in text files; binaries are left untouched.
while IFS= read -r -d '' file; do
	if grep -Iq . "$file" 2>/dev/null; then
		tmp=$(mktemp)
		sed -E \
			-e 's/(Bearer[[:space:]]+)[A-Za-z0-9._~+\/-]+/\1[REDACTED]/g' \
			-e 's/((AWS_SECRET_ACCESS_KEY|AWS_ACCESS_KEY_ID|SECRET_KEY|API_KEY|ACCESS_TOKEN|AUTH_TOKEN|PASSWORD|PRIVATE_KEY)[[:space:]]*[=:][[:space:]]*)[^,;[:space:]]+/\1[REDACTED]/Ig' \
			-e 's#(postgres(ql)?://[^:/[:space:]]+):[^@/[:space:]]+@#\1:[REDACTED]@#Ig' \
			-e 's#((https?://)[^:/[:space:]]+):[^@/[:space:]]+@#\1:[REDACTED]@#Ig' \
			-e 's/(("(secret|token|password|api[_-]?key|private[_-]?key)"|\x27(secret|token|password|api[_-]?key|private[_-]?key)\x27)[[:space:]]*:[[:space:]]*)"[^"]*"/\1"[REDACTED]"/Ig' \
			"$file" >"$tmp"
		mv "$tmp" "$file"
	fi
done < <(find "$evidence" -type f -print0)

# Produce a first digest manifest before optional parsing dependencies are
# checked. Even a runner missing jq therefore retains a verifiable local bundle.
manifest="$evidence/SHA256SUMS"
(cd "$evidence" && find . -type f ! -name SHA256SUMS -print0 | sort -z | xargs -0 -r sha256sum) >"$manifest"
[[ "$prefix" != *'..'* && "$prefix" != *$'\n'* ]] || {
	echo "invalid evidence prefix: $prefix" >&2
	exit 2
}

command -v jq >/dev/null 2>&1 || {
	echo 'jq is required to create evidence-index.json' >&2
	exit 1
}

# Keep the registry in one machine-readable contract. ENV-003 and ENV-004 are
# controls backed by other rows, not evidence rows of their own.
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

case "$lane" in
	automated|operator)
		contract_ids_filter="select(.lane == \"$lane\")"
		;;
	all)
		contract_ids_filter='.'
		;;
esac
mapfile -t contract_ids < <(jq -r ".evidence[] | $contract_ids_filter | .evidence_id" "$contract_file")
declare -A contract_seen=()
for id in "${contract_ids[@]}"; do
	contract_seen["$id"]=1
done

# An explicit list remains supported for callers that have not yet migrated,
# but every ID must still exist in the canonical contract.
if [[ -z "$expected_ids_csv" ]]; then
	expected_ids_csv=$(IFS=,; echo "${contract_ids[*]}")
fi

errors_file=$(mktemp)
records_file=$(mktemp)
files_file=$(mktemp)
collection_failed=0
declare -A seen_ids=()
declare -A expected_seen=()
actual_ids=()
expected_ids=()

record_error() {
	collection_failed=1
	printf '%s\n' "$1" | tee -a "$errors_file" >&2
}

log_file="$evidence/staging-certification.log"
log_sha=$(sha256sum "$log_file" | awk '{print $1}')
run_id=${GITHUB_RUN_ID:-unknown}
artifact_uri="local://staging-certification.log"
[[ -n "$bucket" ]] && artifact_uri="s3://$bucket/$prefix/staging-certification.log"

# The workflow deliberately keeps collecting evidence after gh run view fails
# so the failure artifacts survive. That failure must still invalidate the
# automated lane; otherwise a complete-looking set of rows could be published
# as PASS without the certification-run identity metadata.
if [[ -s "$evidence/certification-run.error" ]]; then
	record_error 'certification workflow metadata retrieval failed (see certification-run.error)'
fi

# Parse structured records emitted by the staging harness. Duplicate or
# malformed records are failures even when the Go test process exits 0.
while IFS= read -r line || [[ -n "$line" ]]; do
	if [[ "$line" =~ CERTIFICATION_EVIDENCE[[:space:]]+evidence_id=([A-Z][A-Z0-9-]+)[[:space:]]+(.+)$ ]]; then
		id=${BASH_REMATCH[1]}
		payload=${BASH_REMATCH[2]}
		if [[ -n "${seen_ids[$id]+set}" ]]; then
			record_error "duplicate certification evidence ID: $id"
			continue
		fi
		seen_ids["$id"]=1
		actual_ids+=("$id")
		if [[ -z "${contract_seen[$id]+set}" ]]; then
			record_error "unexpected certification evidence ID for $lane lane: $id"
		fi
		if ! jq -e --arg id "$id" '
			(type == "object") and (.evidence_id == $id) and
			(.result == "PASS" or .result == "FAIL" or .result == "N/A") and
			(.collected_utc | type == "string") and (.details | type == "string")
		' <<<"$payload" >/dev/null 2>&1; then
			record_error "invalid certification evidence payload for $id"
			continue
		fi
		result=$(jq -r '.result' <<<"$payload")
		[[ "$result" == PASS ]] || record_error "certification evidence $id has result $result"
		jq -cn --argjson record "$payload" --arg run_id "$run_id" \
			--arg artifact_uri "$artifact_uri" --arg sha256 "$log_sha" \
			--arg source "staging-certification.log" \
			'$record + {run_id: $run_id, artifact_uri: $artifact_uri, sha256: $sha256, source: $source}' \
			>>"$records_file"
	fi
done <"$log_file"

if [[ -n "$expected_ids_csv" ]]; then
	IFS=',' read -r -a expected_ids <<<"$expected_ids_csv"
	for id in "${expected_ids[@]}"; do
		id=${id//[[:space:]]/}
		[[ -n "$id" ]] || continue
		if [[ -n "${expected_seen[$id]+set}" ]]; then
			record_error "duplicate expected certification evidence ID: $id"
		else
			expected_seen["$id"]=1
		fi
	done
	for id in "${actual_ids[@]}"; do
		[[ -n "${expected_seen[$id]+set}" ]] || record_error "unexpected certification evidence ID: $id"
	done
	for id in "${expected_ids[@]}"; do
		id=${id//[[:space:]]/}
		[[ -n "$id" ]] || continue
		[[ -n "${contract_seen[$id]+set}" ]] || record_error "unknown expected certification evidence ID: $id"
		if [[ -z "${seen_ids[$id]+set}" ]]; then
			record_error "missing certification evidence ID: $id"
			jq -cn --arg id "$id" --arg run_id "$run_id" \
				--arg artifact_uri "$artifact_uri" --arg sha256 "$log_sha" \
				--arg collected_utc "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" \
				'{evidence_id:$id,result:"FAIL",collected_utc:$collected_utc,details:"missing certification evidence record in staging-certification.log",run_id:$run_id,artifact_uri:$artifact_uri,sha256:$sha256,source:"staging-certification.log"}' \
				>>"$records_file"
		fi
	done
fi

# Build a stable inventory before writing the index. The index excludes itself;
# SHA256SUMS below covers both the index and every payload file.
while IFS= read -r -d '' file; do
	rel=${file#"$evidence"/}
	[[ "$rel" == SHA256SUMS || "$rel" == evidence-index.json ]] && continue
	digest=$(sha256sum "$file" | awk '{print $1}')
	bytes=$(wc -c <"$file" | tr -d '[:space:]')
	jq -cn --arg path "$rel" --arg sha256 "$digest" --argjson bytes "$bytes" \
		'{path:$path,sha256:$sha256,size_bytes:$bytes}' >>"$files_file"
done < <(find "$evidence" -type f -print0 | sort -z)

records_json='[]'
files_json='[]'
errors_json='[]'
expected_json='[]'
[[ -s "$records_file" ]] && records_json=$(jq -sc 'sort_by(.evidence_id)' "$records_file")
[[ -s "$files_file" ]] && files_json=$(jq -sc 'sort_by(.path)' "$files_file")
[[ -s "$errors_file" ]] && errors_json=$(jq -Rsc 'split("\n")|map(select(length>0))' "$errors_file")
if ((${#expected_ids[@]} > 0)); then
	expected_json=$(printf '%s\n' "${expected_ids[@]}" | jq -Rsc 'split("\n")|map(select(length>0))|unique|sort')
fi

candidate_tag=${CANDIDATE_TAG:-unknown}
candidate_sha=${EXPECTED_SHA:-unknown}
if [[ -f "$evidence/candidate.txt" ]]; then
	resolved_sha=$(awk -F= '$1 == "sha" || $1 == "resolved_sha" {print $2; exit}' "$evidence/candidate.txt" || true)
	[[ -n "${resolved_sha:-}" ]] && candidate_sha=$resolved_sha
fi
index_tmp=$(mktemp)
jq -n --arg generated_utc "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" \
	--arg run_id "$run_id" --arg run_attempt "${GITHUB_RUN_ATTEMPT:-unknown}" \
	--arg repository "${GITHUB_REPOSITORY:-local}" --arg workflow "${GITHUB_WORKFLOW:-local}" \
	--arg candidate_tag "$candidate_tag" --arg candidate_sha "$candidate_sha" \
	--arg expected_sha "${EXPECTED_SHA:-unknown}" --arg prefix "$prefix" --arg lane "$lane" \
	--arg contract_file "$contract_file" --slurpfile contract "$contract_file" \
	--arg result "$([[ $collection_failed -eq 0 ]] && echo PASS || echo FAIL)" \
	--argjson records "$records_json" --argjson files "$files_json" --argjson errors "$errors_json" --argjson expected "$expected_json" \
	'{schema_version:"v0.10-core-staging-evidence.v1",contract:{schema_version:$contract[0].schema_version,file:$contract_file,lane:$lane},collection:{generated_utc:$generated_utc,result:$result,prefix:$prefix},run:{id:$run_id,attempt:$run_attempt,repository:$repository,workflow:$workflow},candidate:{tag:$candidate_tag,resolved_sha:$candidate_sha,expected_sha:$expected_sha,profile:"v0.10-core",migration_ceiling:"000124"},entries:$records,files:$files,validation:{errors:$errors,expected_ids:$expected}}' \
	>"$index_tmp"
mv "$index_tmp" "$evidence/evidence-index.json"

manifest="$evidence/SHA256SUMS"
(cd "$evidence" && find . -type f ! -name SHA256SUMS -print0 | sort -z | xargs -0 -r sha256sum) >"$manifest"

rm -f "$errors_file" "$records_file" "$files_file"

if [[ "$local_only" == true ]]; then
	if ((collection_failed)); then
		echo "local evidence bundle created with validation failures: $evidence" >&2
		exit 1
	fi
	echo "local evidence bundle created: $evidence"
	exit 0
fi

# Validate cloud configuration only after local collection. The caller's
# always() artifact step therefore retains evidence when this preflight fails.
missing=()
[[ -n "$bucket" ]] || missing+=("S3 bucket (--bucket or EVIDENCE_S3_BUCKET)")
[[ -n "$endpoint" ]] || missing+=("S3 endpoint (--endpoint-url or EVIDENCE_S3_ENDPOINT)")
export AWS_ACCESS_KEY_ID=${AWS_ACCESS_KEY_ID:-${EVIDENCE_S3_ACCESS_KEY_ID:-}}
export AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY:-${EVIDENCE_S3_SECRET_ACCESS_KEY:-}}
[[ -n "$AWS_ACCESS_KEY_ID" ]] || missing+=("EVIDENCE_S3_ACCESS_KEY_ID")
[[ -n "$AWS_SECRET_ACCESS_KEY" ]] || missing+=("EVIDENCE_S3_SECRET_ACCESS_KEY")
command -v aws >/dev/null 2>&1 || missing+=("aws CLI")
if ((${#missing[@]} > 0)); then
	printf 'immutable evidence upload preflight failed; missing: %s\n' "${missing[*]}" >&2
	missing_json=$(printf '%s\n' "${missing[@]}" | jq -Rsc 'split("\n")|map(select(length>0))')
	index_tmp=$(mktemp)
	jq --argjson errors "$missing_json" \
		'.collection.result = "FAIL" | .validation.errors += $errors' \
		"$evidence/evidence-index.json" >"$index_tmp"
	mv "$index_tmp" "$evidence/evidence-index.json"
	(cd "$evidence" && find . -type f ! -name SHA256SUMS -print0 | sort -z | xargs -0 -r sha256sum) >"$manifest"
	exit 1
fi

retain_until=$(date -u -d '+7 years' '+%Y-%m-%dT%H:%M:%SZ')
aws_args=(--region "$region")
[[ -n "$endpoint" ]] && aws_args+=(--endpoint-url "$endpoint")
base_args=(--bucket "$bucket" "${aws_args[@]}")
mapfile -d '' upload_files < <(find "$evidence" -type f -print0 | sort -z)
(( ${#upload_files[@]} > 0 )) || { echo 'evidence bundle contains no files' >&2; exit 1; }

# Preflight every key before uploading any object. This prevents a partial upload
# when a previous run already used one of the keys.
for file in "${upload_files[@]}"; do
	rel=${file#"$evidence"/}
	key="$prefix/$rel"
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

for file in "${upload_files[@]}"; do
	rel=${file#"$evidence"/}
	key="$prefix/$rel"
	digest=$(sha256sum "$file" | awk '{print $1}')
	aws s3api put-object "${base_args[@]}" --key "$key" --body "$file" \
		--metadata "sha256=$digest" \
		--object-lock-mode COMPLIANCE --object-lock-retain-until-date "$retain_until" >/dev/null
done

# Verify retention on every uploaded object, including the manifest.
for file in "${upload_files[@]}"; do
	rel=${file#"$evidence"/}; key="$prefix/$rel"
	mode=$(aws s3api head-object "${base_args[@]}" --key "$key" --query ObjectLockMode --output text)
	retained=$(aws s3api head-object "${base_args[@]}" --key "$key" --query ObjectLockRetainUntilDate --output text)
	remote_sha=$(aws s3api head-object "${base_args[@]}" --key "$key" --query 'Metadata.sha256' --output text)
	digest=$(sha256sum "$file" | awk '{print $1}')
	[[ "$mode" == COMPLIANCE ]] || { echo "invalid Object Lock mode for $key: $mode" >&2; exit 1; }
	[[ "$retained" != None && "$(date -u -d "$retained" +%s)" -ge "$(date -u -d "$retain_until" +%s)" ]] || { echo "retention too short for $key" >&2; exit 1; }
	[[ "$remote_sha" == "$digest" ]] || { echo "remote digest mismatch for $key" >&2; exit 1; }
done

echo "uploaded immutable evidence: s3://$bucket/$prefix (retain until $retain_until)"
if ((collection_failed)); then
	echo "uploaded immutable evidence with validation failures: $evidence" >&2
	exit 1
fi
