#!/usr/bin/env bash
set -euo pipefail

root_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root_dir"

matrix="docs/reference/feature-matrix.md"
status=0

fail() {
	printf 'release hygiene: %s\n' "$1" >&2
	status=1
}

trim() {
	local value=$1
	value=${value#"${value%%[![:space:]]*}"}
	value=${value%"${value##*[![:space:]]}"}
	printf '%s' "$value"
}

if [[ ! -f "$matrix" ]]; then
	fail "missing authoritative feature matrix: $matrix"
	exit "$status"
fi

expected_header='| Capability | code-complete | integration-complete | production-certified | documented | Advertised production route source | Evidence / remaining gate |'
if ! grep -Fqx "$expected_header" "$matrix"; then
	fail "feature matrix header does not define all four release statuses"
fi

declare -A seen_capabilities=()
row_count=0
while IFS='|' read -r _ capability code_complete integration_complete production_certified documented route_sources evidence _; do
	capability=$(trim "${capability:-}")
	[[ -z "$capability" || "$capability" == "Capability" || "$capability" == '---' ]] && continue
	[[ "$capability" == \** ]] && continue

	code_complete=$(trim "${code_complete:-}")
	integration_complete=$(trim "${integration_complete:-}")
	production_certified=$(trim "${production_certified:-}")
	documented=$(trim "${documented:-}")
	route_sources=$(trim "${route_sources:-}")
	evidence=$(trim "${evidence:-}")
	row_count=$((row_count + 1))

	if [[ -n "${seen_capabilities[$capability]+x}" ]]; then
		fail "duplicate feature-matrix capability: $capability"
	fi
	seen_capabilities["$capability"]=1

	for field in code_complete integration_complete production_certified documented; do
		value=${!field}
		if [[ "$value" != yes && "$value" != partial && "$value" != no ]]; then
			fail "$capability has invalid $field value: $value"
		fi
	done

	if [[ "$production_certified" == yes && ( "$code_complete" != yes || "$integration_complete" != yes || "$documented" != yes ) ]]; then
		fail "$capability is production-certified without code, integration, and documentation completion"
	fi
	if [[ "$route_sources" != none && "$documented" != yes ]]; then
		fail "$capability advertises route sources but is not documented"
	fi
	if [[ -z "$evidence" ]]; then
		fail "$capability is missing evidence or a remaining release gate"
	fi

	if [[ "$route_sources" != none ]]; then
		while IFS= read -r route_source; do
			route_source=$(trim "$route_source")
			[[ -z "$route_source" ]] && continue
			if [[ "$route_source" != *' -> '* ]]; then
				fail "$capability has a route source without 'route -> source': $route_source"
				continue
			fi
			source=${route_source##* -> }
			source=$(trim "$source")
			source=${source#\`}
			source=${source%\`}
			if [[ ! -f "$source" ]]; then
				fail "$capability names a missing advertised route source: $source"
				continue
			fi
			if [[ "$integration_complete" == yes ]] && rg -n -i -e 'StatusNotImplemented' -e '"not implemented"' -e "'not implemented'" -e 'status[^[:cntrl:]]*not implemented' "$source" >/dev/null; then
				matches=$(rg -n -i -e 'StatusNotImplemented' -e '"not implemented"' -e "'not implemented'" -e 'status[^[:cntrl:]]*not implemented' "$source" || true)
				fail "$capability has a placeholder response in advertised route source $source:\n$matches"
			fi
			done < <(printf '%s\n' "$route_sources" | tr ';' '\n')
	fi
done < <(awk '/^\|/ { print }' "$matrix")

if (( row_count == 0 )); then
	fail "feature matrix contains no capability rows"
fi

if ! grep -Fq '[Authoritative Feature Matrix](reference/feature-matrix.md)' docs/README.md; then
	fail 'docs/README.md does not link the authoritative feature matrix'
fi
if ! grep -Fq 'docs/reference/feature-matrix.md' README.md; then
	fail 'README.md does not link the authoritative feature matrix'
fi

if grep -Fq 'RBAC middleware integration' NEXT_STEPS.md; then
	fail 'NEXT_STEPS.md still presents RBAC middleware integration as unfinished'
fi
if grep -Eqi 'three days to production|all changes committed: yes|ready for production launch' NEXT_STEPS.md; then
	fail 'NEXT_STEPS.md still contains a superseded production-readiness claim'
fi
if grep -Fqi 'staging and production certification successfully passed' docs/guides/phase14-p7-acceptance-evidence.md; then
	fail 'Phase 14/P7 evidence still claims staging and production certification'
fi

exit "$status"
