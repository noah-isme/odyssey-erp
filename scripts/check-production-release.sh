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

if ! grep -Fq '**Current release candidate:** v0.10.0-rc.1' docs/releases/VERSION_HISTORY.md; then
	fail "docs/releases/VERSION_HISTORY.md does not identify the current release candidate"
fi

if [[ ! -f "$matrix" ]]; then
	fail "missing authoritative feature matrix: $matrix"
else
	while IFS='|' read -r _ capability code_complete integration_complete production_certified documented _ _; do
		capability=$(trim "${capability:-}")
		[[ -z "$capability" || "$capability" == "Capability" || "$capability" == '---' ]] && continue
		code_complete=$(trim "${code_complete:-}")
		integration_complete=$(trim "${integration_complete:-}")
		production_certified=$(trim "${production_certified:-}")
		documented=$(trim "${documented:-}")
		if [[ "$production_certified" != yes ]]; then
			fail "$capability is not production-certified (code=$code_complete integration=$integration_complete documented=$documented)"
		fi
	done < <(awk '/^\|/ { print }' "$matrix")
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

if grep -Eq '^[[:space:]]+plan:[[:space:]]+free[[:space:]]*$' render.yaml 2>/dev/null; then
	fail "render.yaml is a demo/staging Free blueprint, not a production infrastructure definition"
fi

if [[ $status -eq 0 ]]; then
	printf 'production release: static final-release gate passed\n'
else
	printf 'production release: static final-release gate is blocked\n'
fi
exit "$status"
