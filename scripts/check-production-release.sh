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

tag_exists() {
	git show-ref --verify --quiet "refs/tags/$1"
}

resolve_tag_commit() {
	git rev-parse --verify "refs/tags/$1^{commit}" 2>/dev/null
}

tag_type() {
	git cat-file -t "refs/tags/$1" 2>/dev/null || true
}

has_exact_tag() {
	local tags=$1
	local wanted=$2
	case $'\n'"$tags"$'\n' in
		*$'\n'"$wanted"$'\n'*) return 0 ;;
		*) return 1 ;;
	esac
}

closeout_path_allowed() {
	case "$1" in
		docs/releases/*|docs/reference/feature-matrix.md|docs/ROADMAP.md|docs/CHANGELOG.md|docs/INDEX.md|docs/README.md|docs/STAGING_DEPLOYMENT.md|docs/DEPLOYMENT.md|README.md)
			return 0
			;;
		*)
			return 1
			;;
	esac
}

matrix="docs/reference/feature-matrix.md"

certified_candidate_tag=${CERTIFIED_CANDIDATE_TAG:-}
release_version=${RELEASE_VERSION:-}

if [[ -n "$release_version" && ! "$release_version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
	fail "invalid RELEASE_VERSION=$release_version (expected vMAJOR.MINOR.PATCH)"
fi
if [[ -n "$certified_candidate_tag" && ! "$certified_candidate_tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+-rc\.[0-9]+$ ]]; then
	fail "invalid CERTIFIED_CANDIDATE_TAG=$certified_candidate_tag (expected vMAJOR.MINOR.PATCH-rc.N)"
fi

candidate_release_version=""
if [[ "$certified_candidate_tag" =~ ^(v[0-9]+\.[0-9]+\.[0-9]+)-rc\.[0-9]+$ ]]; then
	candidate_release_version=${BASH_REMATCH[1]}
fi
if [[ -n "$release_version" && -n "$candidate_release_version" && "$release_version" != "$candidate_release_version" ]]; then
	fail "CERTIFIED_CANDIDATE_TAG=$certified_candidate_tag belongs to $candidate_release_version, not RELEASE_VERSION=$release_version"
fi

release_profile=${RELEASE_PROFILE:-}
case "$release_profile" in
	v0.10-core)
		scope_column=v010
		;;
	v0.11-finance)
		scope_column=v011
		;;
	full)
		scope_column=all
		;;
	'')
		fail "RELEASE_PROFILE must be set explicitly (v0.10-core, v0.11-finance, or full)"
		;;
	*)
		fail "unsupported RELEASE_PROFILE=$release_profile (expected v0.10-core, v0.11-finance, or full)"
		;;
esac

# Graphify output is generated workspace state and is intentionally excluded
# from the release cleanliness check. All source, migration, configuration, and
# documentation changes must still be committed before a tag is created. Use a
# root-anchored Git pathspec so a similarly named source path cannot be hidden.
non_graphify_status=$(git status --porcelain=v1 --untracked-files=all -- . ':(exclude)graphify-out/**' || true)
if [[ -n "$non_graphify_status" ]]; then
	fail "working tree contains uncommitted changes; review and commit the release candidate first"
	printf '%s\n' "$non_graphify_status" >&2
fi

head_sha=$(git rev-parse --verify HEAD 2>/dev/null || true)
head_tags=$(git tag --points-at "$head_sha" 2>/dev/null | sort || true)

# CERTIFIED_CANDIDATE_TAG is the immutable artifact identity. When it is not
# supplied, retain the historical exact-HEAD-tag behavior for local/release
# checks that predate the explicit candidate contract.
candidate_tag=$certified_candidate_tag
if [[ -z "$candidate_tag" ]]; then
	candidate_tag=$(git describe --exact-match --tags "$head_sha" 2>/dev/null || true)
fi

candidate_sha=""
if [[ -z "$candidate_tag" ]]; then
	fail "HEAD has no release tag; set CERTIFIED_CANDIDATE_TAG or assign a version and create the reviewed release tag only after all gates pass"
elif ! tag_exists "$candidate_tag"; then
	fail "candidate tag $candidate_tag does not exist in refs/tags"
else
	candidate_sha=$(resolve_tag_commit "$candidate_tag" || true)
	if [[ -z "$candidate_sha" ]]; then
		fail "candidate tag $candidate_tag does not resolve to a commit"
	fi
	if [[ "$(tag_type "$candidate_tag")" != tag ]]; then
		fail "candidate tag $candidate_tag must be an annotated tag"
	fi
fi

final_sha=""
head_is_final=0

if [[ -z "$release_version" ]]; then
	:
elif ! tag_exists "$release_version"; then
	fail "RELEASE_VERSION tag $release_version does not exist in refs/tags"
else
	final_sha=$(resolve_tag_commit "$release_version" || true)
	if [[ -z "$final_sha" ]]; then
		fail "RELEASE_VERSION tag $release_version does not resolve to a commit"
	elif [[ "$(tag_type "$release_version")" != tag ]]; then
		fail "final release tag $release_version must be an annotated tag"
	fi
	if [[ -n "$candidate_sha" && -n "$final_sha" ]] && ! git merge-base --is-ancestor "$candidate_sha" "$final_sha"; then
		fail "final release tag $release_version at $final_sha is not descended from candidate tag $candidate_tag at $candidate_sha"
	fi
	if [[ "$head_sha" == "$final_sha" ]]; then
		head_is_final=1
	else
		fail "HEAD $head_sha does not match RELEASE_VERSION tag $release_version at $final_sha"
	fi
fi

head_is_candidate=0
closeout_mode=0
if [[ -n "$candidate_sha" ]]; then
	if [[ "$head_sha" == "$candidate_sha" ]]; then
		head_is_candidate=1
	elif git merge-base --is-ancestor "$candidate_sha" "$head_sha"; then
		# Evidence is intentionally committed after the immutable candidate. A
		# final tag may point at that closeout commit; otherwise only the
		# release-document allowlist below may differ from the candidate.
		closeout_mode=1
	else
		fail "HEAD $head_sha is not the certified candidate $candidate_sha or a descendant of it"
	fi
fi

if [[ -n "$certified_candidate_tag" && "$head_is_candidate" -eq 1 ]] && ! has_exact_tag "$head_tags" "$certified_candidate_tag"; then
	fail "HEAD $head_sha does not carry exact candidate tag $certified_candidate_tag"
fi
if [[ -n "$release_version" && "$head_is_final" -eq 1 ]] && ! has_exact_tag "$head_tags" "$release_version"; then
	fail "HEAD $head_sha does not carry exact final tag $release_version"
fi
if [[ -n "$head_tags" && "$head_is_candidate" -eq 0 && "$head_is_final" -eq 0 ]]; then
	fail "HEAD carries a tag, but neither CERTIFIED_CANDIDATE_TAG nor RELEASE_VERSION identifies it"
fi

if [[ "$head_is_final" -eq 1 ]]; then
	if [[ -z "$certified_candidate_tag" || -z "$candidate_sha" ]]; then
		fail "CERTIFIED_CANDIDATE_TAG is required when validating a final release tag"
	elif ! git merge-base --is-ancestor "$candidate_sha" "$head_sha"; then
		fail "final release tag $release_version is not descended from candidate tag $certified_candidate_tag"
	fi
fi

if [[ ( "$closeout_mode" -eq 1 || "$head_is_final" -eq 1 ) && -n "$candidate_sha" ]]; then
	while IFS= read -r closeout_path; do
		[[ -z "$closeout_path" ]] && continue
		if ! closeout_path_allowed "$closeout_path"; then
			fail "closeout diff contains non-release documentation path: $closeout_path"
		fi
	done < <(git diff --name-only "$candidate_sha..$head_sha")
fi

documented_candidate_tag=$candidate_tag
if [[ -n "$certified_candidate_tag" ]]; then
	documented_candidate_tag=$certified_candidate_tag
fi
if [[ -n "$documented_candidate_tag" ]] && ! grep -Fq "**Current release candidate:** $documented_candidate_tag" docs/releases/VERSION_HISTORY.md; then
	fail "docs/releases/VERSION_HISTORY.md does not identify certified candidate $documented_candidate_tag"
fi

if [[ ! -f "$matrix" ]]; then
	fail "missing authoritative feature matrix: $matrix"
else
	expected_header='| Capability | v0.10.0 scope | v0.11.0 scope | code-complete | integration-complete | production-certified | documented | Advertised production route source | Evidence / remaining gate |'
	if ! grep -Fqx "$expected_header" "$matrix"; then
		fail "feature matrix header does not define release scope and all four release statuses"
	fi

	required_row_count=0
	while IFS='|' read -r _ capability v010_scope v011_scope code_complete integration_complete production_certified documented _ _; do
		capability=$(trim "${capability:-}")
		[[ -z "$capability" || "$capability" == "Capability" || "$capability" == '---' ]] && continue
		v010_scope=$(trim "${v010_scope:-}")
		v011_scope=$(trim "${v011_scope:-}")
		code_complete=$(trim "${code_complete:-}")
		integration_complete=$(trim "${integration_complete:-}")
		production_certified=$(trim "${production_certified:-}")
		documented=$(trim "${documented:-}")
		if [[ "$v010_scope" != yes && "$v010_scope" != no ]]; then
			fail "$capability has invalid v0.10.0 scope value: $v010_scope (expected yes or no)"
			continue
		fi
		if [[ "$v011_scope" != yes && "$v011_scope" != no ]]; then
			fail "$capability has invalid v0.11.0 scope value: $v011_scope (expected yes or no)"
			continue
		fi

		release_scope="$v010_scope"
		if [[ "$scope_column" == v011 ]]; then
			release_scope="$v011_scope"
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
	if grep -Eq -- '- \[ \]|_record |_record\*|_pending_|<[^>]+>|\|[[:space:]]*`?PENDING`?[[:space:]]*\|' "$certification_record"; then
		fail 'v0.10-core staging certification record contains incomplete checklist, PENDING result, or evidence placeholder'
	fi
	if [[ -n "$candidate_tag" ]] && ! grep -Fq "**Candidate:** $candidate_tag" "$certification_record"; then
		fail "v0.10-core staging certification record does not identify tagged candidate $candidate_tag"
	fi
	if [[ -n "$certified_candidate_tag" ]]; then
		record_candidate_tag=$(awk -F: '/^\*\*Candidate:\*\*/ { value=$2; gsub(/[`*[:space:]]/, "", value); print value; exit }' "$certification_record")
		if [[ "$record_candidate_tag" != "$certified_candidate_tag" ]]; then
			fail "v0.10-core staging certification record candidate is ${record_candidate_tag:-missing}, expected $certified_candidate_tag"
		fi

		resolved_commit_line=$(awk -F'|' '$2 ~ /^[[:space:]]*Resolved([[:space:]]+candidate)?[[:space:]]+full commit[[:space:]]*$/ { print $3; exit }' "$certification_record")
		record_candidate_sha=$(printf '%s\n' "$resolved_commit_line" | grep -Eio '[0-9a-f]{40}' | head -n 1 || true)
		if [[ -z "$record_candidate_sha" ]]; then
			fail 'v0.10-core staging certification record does not record a 40-hex resolved candidate commit'
		elif [[ "$record_candidate_sha" != "$candidate_sha" ]]; then
			fail "v0.10-core staging certification commit $record_candidate_sha does not match candidate tag $certified_candidate_tag at $candidate_sha"
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
	if ! grep -Fq '000125' "$certification_record" || ! grep -Eiq 'absent|excluded|outside' "$certification_record"; then
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
