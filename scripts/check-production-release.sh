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

blocked_release_tests=$(rg -l 'BLOCKED_RELEASE' internal tests migrations --glob '*_test.go' 2>/dev/null || true)
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
