#!/usr/bin/env bash
set -euo pipefail

root_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root_dir"

status=0

fail() {
	printf 'production release: %s\n' "$1" >&2
	status=1
}

trim() {
	local value=$1
	value=${value#"${value%%[![:space:]]*}"}
	value=${value%"${value##*[![:space:]]}"}
	printf '%s' "$value"
}

table_row_result() {
	local section=$1
	local row_label=$2
	printf '%s\n' "$section" | awk -F'|' -v wanted="$row_label" '
		{
			label = $2
			gsub(/^[[:space:]]+|[[:space:]]+$/, "", label)
			if (label == wanted) {
				result = $(NF - 1)
				gsub(/^[[:space:]`]+|[[:space:]`]+$/, "", result)
				print result
				exit
			}
		}
	'
}

matrix="docs/reference/feature-matrix.md"

release_profile=${RELEASE_PROFILE:-}
case "$release_profile" in
	v0.10-core|full)
		;;
	'')
		fail "RELEASE_PROFILE must be set explicitly (v0.10-core or full)"
		;;
	*)
		fail "unsupported RELEASE_PROFILE=$release_profile (expected v0.10-core or full)"
		;;
esac

# Graphify output is generated workspace state and is intentionally excluded
# from the release cleanliness check. All source, migration, configuration, and
# documentation changes must still be committed before a tag is created.
non_graphify_status=$(git status --porcelain=v1 | grep -v 'graphify-out/' || true)
if [[ -n "$non_graphify_status" ]]; then
	fail "working tree contains uncommitted changes; review and commit the release candidate first"
	printf '%s\n' "$non_graphify_status" >&2
fi

candidate_tag=$(git describe --exact-match --tags HEAD 2>/dev/null || true)
if [[ -z "$candidate_tag" ]]; then
	fail "HEAD has no release tag; assign a version and create the reviewed release tag only after all gates pass"
elif [[ "$(git cat-file -t "refs/tags/$candidate_tag" 2>/dev/null || true)" != tag ]]; then
	fail "release tag $candidate_tag is not an annotated tag"
fi

if [[ -n "$candidate_tag" ]] && ! grep -Fq "**Current release candidate:** $candidate_tag" docs/releases/VERSION_HISTORY.md; then
	fail "docs/releases/VERSION_HISTORY.md does not identify tagged candidate $candidate_tag"
fi

if [[ ! -f "$matrix" ]]; then
	fail "missing authoritative feature matrix: $matrix"
else
	expected_header='| Capability | v0.10.0 scope | code-complete | integration-complete | production-certified | documented | Advertised production route source | Evidence / remaining gate |'
	if ! grep -Fqx "$expected_header" "$matrix"; then
		fail "feature matrix header does not define release scope and all four release statuses"
	fi

	required_row_count=0
	while IFS='|' read -r _ capability release_scope code_complete integration_complete production_certified documented _ _; do
		capability=$(trim "${capability:-}")
		[[ -z "$capability" || "$capability" == "Capability" || "$capability" == '---' ]] && continue
		release_scope=$(trim "${release_scope:-}")
		code_complete=$(trim "${code_complete:-}")
		integration_complete=$(trim "${integration_complete:-}")
		production_certified=$(trim "${production_certified:-}")
		documented=$(trim "${documented:-}")
		if [[ "$release_scope" != yes && "$release_scope" != no ]]; then
			fail "$capability has invalid v0.10.0 scope value: $release_scope (expected yes or no)"
			continue
		fi

		requires_certification=0
		if [[ "$release_profile" == full || "$release_scope" == yes ]]; then
			requires_certification=1
			required_row_count=$((required_row_count + 1))
		fi
		if (( requires_certification )) && [[ "$production_certified" != yes ]]; then
			fail "$capability is not production-certified (code=$code_complete integration=$integration_complete documented=$documented)"
		fi
		if (( requires_certification )) && [[ "$code_complete" != yes || "$integration_complete" != yes || "$documented" != yes ]]; then
			fail "$capability is in the $release_profile release scope without code, integration, and documentation completion"
		fi
	done < <(awk '/^\|/ { print }' "$matrix")
	if (( required_row_count == 0 )); then
		fail "$release_profile release profile does not select any feature-matrix rows"
	fi
fi

for profile_doc in docs/STAGING_DEPLOYMENT.md docs/DEPLOYMENT.md docs/releases/production-release-checklist.md; do
	if [[ ! -f "$profile_doc" ]]; then
		fail "missing RELEASE_PROFILE contract document: $profile_doc"
		continue
	fi
	if ! grep -Fq 'RELEASE_PROFILE' "$profile_doc"; then
		fail "$profile_doc does not document RELEASE_PROFILE"
	fi
done
if ! grep -Fq 'RELEASE_PROFILE=v0.10-core' docs/STAGING_DEPLOYMENT.md; then
	fail 'staging deployment guide does not pin RELEASE_PROFILE=v0.10-core'
fi
if ! grep -Fq 'RELEASE_PROFILE=v0.10-core' docs/DEPLOYMENT.md; then
	fail 'production deployment guide does not document RELEASE_PROFILE=v0.10-core'
fi
certification_record="docs/releases/v0.10-core-staging-certification.md"
if [[ ! -f "$certification_record" ]]; then
	fail "missing v0.10-core staging certification checklist"
else
	if grep -Fq '**Status:** Evidence template' "$certification_record"; then
		fail 'v0.10-core staging certification record is still marked as an evidence template'
	fi
	if grep -Eiq -- '- \[ \]|_record |_record\*|_pending_|<[^>]+>|\b(TBD|TODO|PLACEHOLDER)\b|\|[[:space:]]*`?PENDING`?[[:space:]]*\|' "$certification_record"; then
		fail 'v0.10-core staging certification record contains an incomplete checklist, PENDING result, or evidence placeholder'
	fi
	if grep -Eq -- '\|[[:space:]]*`?(FAIL|NO-GO)`?[[:space:]]*\|[[:space:]]*$' "$certification_record"; then
		fail 'v0.10-core staging certification record contains a FAIL or NO-GO result'
	fi
	if [[ -n "$candidate_tag" ]] && ! grep -Fq "**Candidate:** $candidate_tag" "$certification_record"; then
		fail "v0.10-core staging certification record does not identify tagged candidate $candidate_tag"
	fi

	evidence_section=$(awk '
		/^## Evidence registry$/ { in_section = 1; next }
		in_section && /^## / { exit }
		in_section { print }
	' "$certification_record")
	evidence_row_count=0
	while IFS='|' read -r _ evidence_id evidence_description evidence_url evidence_sha collected_utc evidence_owner evidence_result evidence_reviewer _; do
		evidence_id=$(trim "${evidence_id:-}")
		evidence_id=${evidence_id//\`/}
		[[ "$evidence_id" =~ ^[A-Z][A-Z0-9-]*-[0-9]+$ ]] || continue
		evidence_row_count=$((evidence_row_count + 1))
		evidence_description=$(trim "${evidence_description:-}")
		evidence_url=$(trim "${evidence_url:-}")
		evidence_sha=$(trim "${evidence_sha:-}")
		collected_utc=$(trim "${collected_utc:-}")
		evidence_owner=$(trim "${evidence_owner:-}")
		evidence_result=$(trim "${evidence_result:-}")
		evidence_result=${evidence_result//\`/}
		evidence_reviewer=$(trim "${evidence_reviewer:-}")
		if [[ -z "$evidence_url" || -z "$collected_utc" || -z "$evidence_owner" || -z "$evidence_reviewer" ]]; then
			fail "$evidence_id has incomplete evidence metadata"
		fi
		if [[ ! "$evidence_sha" =~ ^[0-9a-f]{64}$ ]]; then
			fail "$evidence_id does not record a valid lowercase SHA-256 digest"
		fi
		if [[ "$evidence_result" != PASS && "$evidence_result" != N/A ]]; then
			fail "$evidence_id evidence result must be PASS or justified N/A (found ${evidence_result:-missing})"
		fi
		if [[ "$evidence_result" == N/A && ! "$evidence_description" =~ N/A:[[:space:]].+ ]]; then
			fail "$evidence_id N/A result must include an 'N/A: reason' justification in Required proof"
		fi
		if [[ "$evidence_id" == APR-001 && "$evidence_result" != PASS ]]; then
			fail "APR-001 approval evidence must be PASS (found ${evidence_result:-missing})"
		fi
	done <<< "$evidence_section"
	if (( evidence_row_count == 0 )); then
		fail 'v0.10-core staging certification record has no evidence registry rows'
	fi

	approval_section=$(awk '
		/^## Approval and promotion decision$/ { in_section = 1; next }
		in_section && /^## / { exit }
		in_section { print }
	' "$certification_record")
	for approval in 'Test lead' 'Security approver' 'Operations approver' 'Release owner'; do
		approval_result=$(table_row_result "$approval_section" "$approval")
		if [[ "$approval_result" != PASS ]]; then
			fail "$approval result must be PASS (found ${approval_result:-missing})"
		fi
	done
	go_no_go_result=$(table_row_result "$approval_section" 'Go/no-go')
	if [[ "$go_no_go_result" != GO ]]; then
		fail "Go/no-go decision must be GO (found ${go_no_go_result:-missing})"
	fi

	findings_section=$(awk '
		/^## Findings$/ { in_section = 1; next }
		in_section && /^## / { exit }
		in_section { print }
	' "$certification_record")
	while IFS='|' read -r _ finding_id severity finding_evidence finding_description finding_owner finding_disposition finding_impact finding_status _; do
		finding_id=$(trim "${finding_id:-}")
		finding_id=${finding_id//\`/}
		[[ "$finding_id" =~ ^FIND-[0-9]+$ ]] || continue
		severity=$(trim "${severity:-}")
		severity=${severity//\`/}
		finding_evidence=$(trim "${finding_evidence:-}")
		finding_description=$(trim "${finding_description:-}")
		finding_owner=$(trim "${finding_owner:-}")
		finding_disposition=$(trim "${finding_disposition:-}")
		finding_impact=$(trim "${finding_impact:-}")
		finding_status=$(trim "${finding_status:-}")
		finding_status=${finding_status//\`/}
		if [[ ! "$severity" =~ ^(critical|high|medium|low)$ ]]; then
			fail "$finding_id has invalid severity ${severity:-missing}"
		fi
		if [[ -z "$finding_evidence" || -z "$finding_description" || -z "$finding_owner" || -z "$finding_disposition" || -z "$finding_impact" ]]; then
			fail "$finding_id has incomplete finding evidence or disposition"
		fi
		case "$finding_status" in
			CLOSED|RESOLVED|ACCEPTED)
				;;
			*)
				fail "$finding_id is unresolved (status ${finding_status:-missing})"
				;;
		esac
		if [[ "$severity" =~ ^(critical|high)$ && "$finding_status" == ACCEPTED ]]; then
			fail "$finding_id is a $severity finding and cannot be accepted for release"
		fi
	done <<< "$findings_section"
fi

if [[ "$release_profile" == v0.10-core ]]; then
	# The human-readable certification record is not sufficient by itself: a
	# release gate must consume the exact, write-once machine index produced by
	# the closeout validator. Requiring both the local copy and its immutable
	# object URI prevents a copied or hand-edited checklist from becoming the
	# source of truth.
	final_index_file=${CERTIFICATION_EVIDENCE_INDEX_FILE:-}
	final_index_uri=${CERTIFICATION_EVIDENCE_INDEX_URI:-}
	if [[ -z "$final_index_file" ]]; then
		fail 'CERTIFICATION_EVIDENCE_INDEX_FILE must point to the final closeout evidence-index.json'
	elif [[ ! -f "$final_index_file" ]]; then
		fail "final certification evidence index does not exist: $final_index_file"
	fi
	if [[ -z "$final_index_uri" ]]; then
		fail 'CERTIFICATION_EVIDENCE_INDEX_URI must identify the immutable s3:// final evidence-index.json'
	fi
	if ! command -v jq >/dev/null 2>&1; then
		fail 'jq is required to validate the machine-readable certification index'
	elif [[ -f "$final_index_file" ]]; then
		contract_file="scripts/staging-certification-contract.json"
		if [[ ! -f "$contract_file" ]]; then
			fail "missing certification contract: $contract_file"
		elif ! jq -e 'type == "object" and (.evidence | type == "array" and length == 25)' "$contract_file" >/dev/null 2>&1; then
			fail "invalid certification contract: $contract_file"
		else
			head_sha=$(git rev-parse HEAD)
			expected_prefix="${candidate_tag}/${head_sha:0:7}/"
			if ! jq -e --arg tag "$candidate_tag" --arg sha "$head_sha" --arg expected_prefix "$expected_prefix" '
				type == "object" and
				.schema_version == "v0.10-core-staging-closeout.v1" and
				.contract.schema_version == "v0.10-core-certification-contract.v1" and
				.contract.profile == "v0.10-core" and
				.contract.migration_ceiling == "000124" and
				.collection.result == "PASS" and
				.collection.decision == "GO" and
				(.collection.generated_utc | type == "string" and test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\\.[0-9]{1,9})?Z$")) and
				(.collection.prefix | type == "string" and startswith($expected_prefix)) and
				.candidate.tag == $tag and
				.candidate.resolved_sha == $sha and
				.candidate.profile == "v0.10-core" and
				.candidate.migration_ceiling == "000124" and
				(.entries | type == "array" and length == 25) and
				(.sources | type == "array" and length == 2) and
				all(.entries[];
					.evidence_id | type == "string" and length > 0) and
				all(.entries[];
					.result == "PASS" and
					(.run_id | type == "string" and length > 0 and . != "PENDING" and . != "UNKNOWN" and . != "unknown") and
					(.reviewer | type == "string" and length > 0 and . != "PENDING" and . != "UNKNOWN" and . != "unknown") and
					(.details | type == "string" and length > 0 and . != "PENDING") and
					(.collected_utc | type == "string" and test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\\.[0-9]{1,9})?Z$")) and
					(.artifact_uri | type == "string" and test("^s3://[A-Za-z0-9][A-Za-z0-9._-]*/[^[:space:]?#]+$") and (contains("..") | not)) and
					(.sha256 | type == "string" and test("^[0-9a-f]{64}$"))) and
				(.validation.errors | type == "array" and length == 0) and
				(.validation.missing_ids | type == "array" and length == 0) and
				(.validation.duplicate_ids | type == "array" and length == 0) and
				(.validation.unknown_ids | type == "array" and length == 0)
			' "$final_index_file" >/dev/null 2>&1; then
				fail 'final certification evidence index does not satisfy the v0.10-core closeout contract'
			fi

			contract_ids=$(jq -c '[.evidence[].evidence_id] | sort' "$contract_file")
			index_ids=$(jq -c '[.entries[].evidence_id] | sort' "$final_index_file" 2>/dev/null || printf '[]')
			if [[ "$index_ids" != "$contract_ids" ]]; then
				fail 'final certification evidence index IDs do not exactly match the 25-row contract'
			fi
			if ! jq -e '[.entries[].evidence_id] | length == (unique | length)' "$final_index_file" >/dev/null 2>&1; then
				fail 'final certification evidence index contains duplicate evidence IDs'
			fi

			index_prefix=$(jq -r '.collection.prefix // ""' "$final_index_file")
			if [[ ! "$final_index_uri" =~ ^s3://[^/]+/[^[:space:]?#]+/evidence-index\.json$ || "$final_index_uri" == *'..'* ]]; then
				fail "final certification evidence index URI is not an immutable s3:// object: $final_index_uri"
			else
				uri_rest=${final_index_uri#s3://}
				uri_bucket=${uri_rest%%/*}
				uri_key=${uri_rest#*/}
				if [[ "$uri_key" != "$index_prefix/evidence-index.json" ]]; then
					fail 'final certification evidence URI does not match collection.prefix/evidence-index.json'
				fi
			fi
			if [[ -n "${uri_bucket:-}" ]]; then
				while IFS= read -r artifact_uri; do
					artifact_rest=${artifact_uri#s3://}
					artifact_bucket=${artifact_rest%%/*}
					if [[ "$artifact_bucket" != "$uri_bucket" ]]; then
						fail "certification artifact URI uses a different bucket than the final evidence index: $artifact_uri"
					fi
				 done < <(jq -r '.entries[].artifact_uri' "$final_index_file" 2>/dev/null || true)
			fi

			manifest_file="$(dirname "$final_index_file")/SHA256SUMS"
			if [[ ! -f "$manifest_file" ]]; then
				fail "final certification evidence manifest is missing: $manifest_file"
			elif ! (cd "$(dirname "$final_index_file")" && sha256sum -c SHA256SUMS >/dev/null 2>&1); then
				fail 'final certification evidence SHA256SUMS does not verify the local index'
			fi

			# A URI and a local file are still only claims until the immutable
			# store confirms them. Verify every indexed artifact plus the index
			# and manifest when the S3/Object-Lock credentials are available;
			# missing remote configuration is itself a release-gate failure.
			remote_missing=()
			[[ -n "${EVIDENCE_S3_ENDPOINT:-}" ]] || remote_missing+=(EVIDENCE_S3_ENDPOINT)
			[[ -n "${EVIDENCE_S3_REGION:-}" ]] || remote_missing+=(EVIDENCE_S3_REGION)
			[[ -n "${AWS_ACCESS_KEY_ID:-${EVIDENCE_S3_ACCESS_KEY_ID:-}}" ]] || remote_missing+=(EVIDENCE_S3_ACCESS_KEY_ID)
			[[ -n "${AWS_SECRET_ACCESS_KEY:-${EVIDENCE_S3_SECRET_ACCESS_KEY:-}}" ]] || remote_missing+=(EVIDENCE_S3_SECRET_ACCESS_KEY)
			command -v aws >/dev/null 2>&1 || remote_missing+=(aws-cli)
			if ((${#remote_missing[@]} > 0)); then
				fail "immutable certification evidence reachability cannot be verified; missing ${remote_missing[*]}"
			else
				remote_region=${EVIDENCE_S3_REGION:-us-east-1}
				remote_endpoint_args=(--endpoint-url "$EVIDENCE_S3_ENDPOINT")
				remote_aws_args=(--region "$remote_region" "${remote_endpoint_args[@]}")
				remote_access_key=${AWS_ACCESS_KEY_ID:-${EVIDENCE_S3_ACCESS_KEY_ID:-}}
				remote_secret_key=${AWS_SECRET_ACCESS_KEY:-${EVIDENCE_S3_SECRET_ACCESS_KEY:-}}
				remote_head() {
					local uri=$1
					local rest=${uri#s3://}
					local bucket_name=${rest%%/*}
					local key_name=${rest#*/}
					AWS_ACCESS_KEY_ID="$remote_access_key" AWS_SECRET_ACCESS_KEY="$remote_secret_key" \
						aws s3api head-object --bucket "$bucket_name" --key "$key_name" "${remote_aws_args[@]}" --output json
				}
				remote_verify() {
					local uri=$1 expected_sha=${2:-}
					local head_json lock_mode retained remote_sha minimum_retention
					if ! head_json=$(remote_head "$uri" 2>/dev/null); then
						fail "immutable certification artifact is unreachable: $uri"
						return
					fi
					lock_mode=$(jq -r '.ObjectLockMode // ""' <<<"$head_json")
					retained=$(jq -r '.ObjectLockRetainUntilDate // ""' <<<"$head_json")
					remote_sha=$(jq -r '.Metadata.sha256 // ""' <<<"$head_json")
					minimum_retention=$(date -u -d '+7 years' '+%s')
					[[ "$lock_mode" == COMPLIANCE ]] || fail "artifact is not protected by Object Lock COMPLIANCE: $uri"
					if [[ -z "$retained" ]] || ! date -u -d "$retained" +%s >/dev/null 2>&1 || (( $(date -u -d "$retained" +%s) < minimum_retention )); then
						fail "artifact Object Lock retention is shorter than the required seven years: $uri"
					fi
					if [[ -n "$expected_sha" && "$remote_sha" != "$expected_sha" ]]; then
						fail "artifact metadata SHA-256 does not match the closeout index: $uri"
					fi
				}
				if [[ -n "$uri_bucket" && -n "$uri_key" ]]; then
					remote_verify "$final_index_uri" "$(sha256sum "$final_index_file" | awk '{print $1}')"
					remote_verify "s3://$uri_bucket/${uri_key%/evidence-index.json}/SHA256SUMS" "$(sha256sum "$manifest_file" | awk '{print $1}')"
					declare -A verified_artifacts=()
					while IFS= read -r artifact_uri; do
						artifact_sha=$(jq -r --arg uri "$artifact_uri" '.entries[] | select(.artifact_uri == $uri) | .sha256' "$final_index_file" | head -n 1)
						if [[ -z "${verified_artifacts[$artifact_uri]+set}" ]]; then
							verified_artifacts["$artifact_uri"]=1
							remote_verify "$artifact_uri" "$artifact_sha"
						fi
					 done < <(jq -r '.entries[].artifact_uri' "$final_index_file" | sort -u)
				fi
			fi
		fi
	fi
fi

if [[ "$release_profile" == v0.10-core ]]; then
	for direction in up down; do
		if [[ ! -f "migrations/000124_scoped_rbac_global_compatibility.$direction.sql" ]]; then
			fail "v0.10-core candidate is missing the 000124 $direction migration"
		fi
	done
	if compgen -G 'migrations/000125_*.sql' >/dev/null; then
		fail 'v0.10-core candidate contains forbidden migration 000125'
	fi
	latest_migration=$(find migrations -maxdepth 1 -type f -name '[0-9][0-9][0-9][0-9][0-9][0-9]_*.up.sql' -printf '%f\n' | sort | tail -n 1)
	latest_migration_number=${latest_migration%%_*}
	if [[ -z "$latest_migration" || ! "$latest_migration_number" =~ ^[0-9]{6}$ ]]; then
		fail 'could not determine the v0.10-core migration ceiling'
	elif (( 10#$latest_migration_number > 124 )); then
		fail "v0.10-core migration ceiling exceeds 000124 (found $latest_migration_number)"
	fi
	if ! grep -Fq 'Migration `000125` must be absent' "$certification_record"; then
		fail 'v0.10-core certification record does not preserve the 000124 ceiling / 000125 exclusion'
	fi
fi
profile_config="internal/app/config.go"
if [[ ! -f "$profile_config" ]]; then
	fail "missing RELEASE_PROFILE runtime configuration: $profile_config"
else
	if ! grep -Fq 'envconfig:"RELEASE_PROFILE"' "$profile_config"; then
		fail "$profile_config does not expose RELEASE_PROFILE"
	fi
	if ! grep -Fq 'ParseReleaseProfile' "$profile_config"; then
		fail "$profile_config does not validate RELEASE_PROFILE"
	fi
fi

if command -v rg >/dev/null 2>&1; then
	blocked_release_tests=$(rg -l 'BLOCKED_RELEASE' internal tests migrations --glob '*_test.go' 2>/dev/null || true)
else
	blocked_release_tests=$(grep -Rsl --include='*_test.go' 'BLOCKED_RELEASE' internal tests migrations 2>/dev/null || true)
fi
if [[ -n "$blocked_release_tests" ]]; then
	fail "release-gated tests are still explicitly blocked"
	printf '%s\n' "$blocked_release_tests" >&2
fi

for target in cmd/odyssey cmd/worker cmd/bootstrap-admin scripts/seed; do
	if ! grep -Fq "go build -tags production" Dockerfile || ! grep -Fq "./$target" Dockerfile; then
		fail "Dockerfile does not show a production-tagged build for ./$target"
	fi
done

vps_guide="docs/DEPLOYMENT.md"
if [[ ! -f "$vps_guide" ]]; then
	fail "missing self-managed VPS deployment guide: $vps_guide"
else
	for requirement in systemd nginx backup rollback; do
		if ! grep -Eiq "$requirement" "$vps_guide"; then
			fail "$vps_guide does not document the VPS $requirement control"
		fi
	done
fi

if [[ $status -eq 0 ]]; then
	printf 'production release: static final-release gate passed\n'
else
	printf 'production release: static final-release gate is blocked\n'
fi
exit "$status"
