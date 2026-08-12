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

if ! git describe --exact-match --tags HEAD >/dev/null 2>&1; then
	fail "HEAD has no release tag; assign a version and create the reviewed release tag only after all gates pass"
fi

if ! grep -Fq '**Current release candidate:** v0.10.0-rc.3' docs/releases/VERSION_HISTORY.md; then
	fail "docs/releases/VERSION_HISTORY.md does not identify the current release candidate"
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
if [[ ! -f docs/releases/v0.10-core-staging-certification.md ]]; then
	fail 'missing v0.10-core staging certification checklist'
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
