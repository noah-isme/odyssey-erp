#!/usr/bin/env bash
# Tests for scripts/release/bundle.sh and its wiring in deploy-native.yml.
#
# Overrides (used to show the checks fail on the rc.10 layout):
#   BUNDLE_SH        bundle script under test (default: ./bundle.sh)
#   DEPLOY_WORKFLOW  workflow checked statically (default: the repo's deploy-native.yml)
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd -- "$script_dir/../.." && pwd)
bundle_sh=${BUNDLE_SH:-"$script_dir/bundle.sh"}
workflow=${DEPLOY_WORKFLOW:-"$repo_root/.github/workflows/deploy-native.yml"}

tmp=$(mktemp -d)
trap 'rm -rf -- "$tmp"' EXIT

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

# run <name> <expected-exit> <bundle.sh args...>: keeps stdout/stderr in
# $tmp/<name>.out and .err.
run() {
	local name=$1 want=$2
	shift 2
	local got=0
	bash "$bundle_sh" "$@" >"$tmp/$name.out" 2>"$tmp/$name.err" || got=$?
	if [[ "$got" != "$want" ]]; then
		echo "--- stdout" >&2; cat "$tmp/$name.out" >&2
		echo "--- stderr" >&2; cat "$tmp/$name.err" >&2
		fail "$name: exit $got, want $want"
	fi
}

out_value() {
	local name=$1 key=$2
	sed -n "s/^${key}=//p" "$tmp/$name.out"
}

candidate_sha=0123456789abcdef0123456789abcdef01234567
bundle_name="odyssey-v010-core-${candidate_sha}.tar.gz"

# ---------------------------------------------------------------------------
# 1. Workflow wiring (static): the SBOM is generated after the bundle, outside
#    dist, by a commit-pinned action, attested as a sibling; build-only runs
#    skip attestation and every host step.
# ---------------------------------------------------------------------------
[[ -f "$workflow" ]] || fail "workflow not found: $workflow"

line_of() {
	grep -n -F -- "$1" "$workflow" | head -n 1 | cut -d: -f1
}

# step_block <step name>: prints the step's lines up to the next step.
step_block() {
	awk -v want="      - name: $1" '
		$0 == want { on = 1; print; next }
		on && /^      - name: / { exit }
		on { print }
	' "$workflow"
}

grep -Eq 'uses: anchore/sbom-action@[0-9a-f]{40}([[:space:]]|$)' "$workflow" ||
	fail 'anchore/sbom-action must be pinned to a full commit SHA'
! grep -Eq 'anchore/sbom-action@v' "$workflow" ||
	fail 'anchore/sbom-action must not use a floating tag'

bundle_line=$(line_of '- name: Create content-addressed release bundle')
sbom_line=$(line_of '- name: Generate SPDX SBOM')
[[ -n "$bundle_line" && -n "$sbom_line" ]] || fail 'bundle or SBOM step missing from the workflow'
((sbom_line > bundle_line)) ||
	fail "SBOM step (line $sbom_line) must run after the bundle step (line $bundle_line)"

! grep -Fq 'SBOM.spdx.json' "$workflow" ||
	fail 'SBOM.spdx.json is still referenced (digest list, bundle or host check)'
step_block 'Generate SPDX SBOM' | grep -Eq 'output-file: odyssey-v010-core-\$\{\{ env\.CANDIDATE_SHA \}\}\.sbom\.spdx\.json$' ||
	fail 'SBOM must be written next to the bundle as odyssey-v010-core-<sha>.sbom.spdx.json'
step_block 'Create content-addressed release bundle' | grep -Fq 'scripts/release/bundle.sh bundle dist' ||
	fail 'bundle step must use scripts/release/bundle.sh'

attest=$(step_block 'Attest release provenance')
grep -Fq 'steps.bundle.outputs.name' <<<"$attest" || fail 'attestation must cover the bundle'
grep -Fq 'steps.sbom.outputs.name' <<<"$attest" || fail 'attestation must cover the SBOM as a sibling subject'
grep -Fq "!(github.event_name == 'workflow_dispatch' && inputs.build_only)" <<<"$attest" ||
	fail 'attestation must be skipped for build_only'

for step in 'Configure SSH' 'Upload release' 'Deploy release'; do
	step_block "$step" | sed -n 2p |
		grep -Fq "if: \${{ !(github.event_name == 'workflow_dispatch' && inputs.build_only) }}" ||
		fail "host step '$step' must be skipped for build_only"
done
grep -Fq "name: \${{ !(github.event_name == 'workflow_dispatch' && inputs.build_only) && 'staging' || '' }}" "$workflow" ||
	fail 'build_only must run without the staging environment'
grep -Fq 'existing release digest differs for candidate' "$workflow" ||
	fail 'the immutable-release digest check must remain'

# ---------------------------------------------------------------------------
# 2. Bundle contents and reproducibility.
# ---------------------------------------------------------------------------

# make_dist <dir> <mtime>: the same release inputs each time; only file
# timestamps differ between builds.
make_dist() {
	local dist=$1 stamp=$2
	mkdir -p "$dist/migrations" "$dist/web/static"
	printf 'odyssey-binary\0\x01\x02' >"$dist/odyssey"
	printf 'worker-binary\0\x03' >"$dist/worker"
	printf 'bootstrap-admin-binary\0' >"$dist/bootstrap-admin"
	printf 'migrate-binary\0' >"$dist/migrate"
	chmod 0755 "$dist/odyssey" "$dist/worker" "$dist/bootstrap-admin" "$dist/migrate"
	printf 'SELECT 1;\n' >"$dist/migrations/000124_scoped_rbac_global_compatibility.up.sql"
	printf 'SELECT 2;\n' >"$dist/migrations/000124_scoped_rbac_global_compatibility.down.sql"
	printf 'body{}\n' >"$dist/web/static/app.css"
	printf '%s\n' "$candidate_sha" >"$dist/REVISION"
	printf 'tag=v0.10.0-rc.11-repro\ncommit=%s\nprofile=v0.10-core\nmigration_ceiling=000124_scoped_rbac_global_compatibility\n' \
		"$candidate_sha" >"$dist/RELEASE_IDENTITY"
	printf '{"profile":"v0.10-core","commit":"%s"}\n' "$candidate_sha" >"$dist/ROUTE_PROFILE.json"
	(cd "$dist" && find migrations -type f -print0 | sort -z | xargs -0 sha256sum) >"$dist/MIGRATION_SHA256SUMS"
	(cd "$dist" && find web -type f -print0 | sort -z | xargs -0 sha256sum) >"$dist/WEB_SHA256SUMS"
	find "$dist" -exec touch -d "$stamp" {} +
}

# fake_sbom <file>: what the SBOM generator does; every run differs
# (creation time and document namespace), like a real SPDX document.
fake_sbom() {
	printf '{"spdxVersion":"SPDX-2.3","creationInfo":{"created":"%s"},"documentNamespace":"https://anchore.com/syft/dir/dist-%s"}\n' \
		"$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)" "$RANDOM$RANDOM$RANDOM" >"$1"
}

# build <n> <mtime>: one pipeline run as the workflow orders it: dist, bundle,
# then the SBOM as a sibling of the bundle.
build() {
	local n=$1 stamp=$2
	local work="$tmp/build$n"
	mkdir -p "$work/out"
	make_dist "$work/dist" "$stamp"
	run "bundle$n" 0 bundle "$work/dist" "$candidate_sha" "$work/out"
	fake_sbom "$work/out/odyssey-v010-core-${candidate_sha}.sbom.spdx.json"
	run "sbom$n" 0 sbom-digest "$work/out/odyssey-v010-core-${candidate_sha}.sbom.spdx.json"
}

build 1 '2026-10-01 10:00:00 UTC'
build 2 '2026-10-09 23:59:59 UTC'

for n in 1 2; do
	[[ "$(out_value "bundle$n" name)" == "$bundle_name" ]] || fail "build $n: bundle name output"
	[[ "$(wc -l <"$tmp/bundle$n.out")" -eq 2 ]] || fail "build $n: stdout must be exactly name= and sha256= lines"
	sums="$tmp/build$n/dist/SHA256SUMS"
	grep -Eiq 'sbom|spdx' "$sums" && fail "build $n: SBOM listed in SHA256SUMS"
	[[ "$(awk '{print $2}' "$sums" | tr '\n' ' ')" == 'REVISION RELEASE_IDENTITY MIGRATION_SHA256SUMS WEB_SHA256SUMS ROUTE_PROFILE.json odyssey worker bootstrap-admin migrate ' ]] ||
		fail "build $n: SHA256SUMS lists unexpected files: $(awk '{print $2}' "$sums" | tr '\n' ' ')"
	tar -tzf "$tmp/build$n/out/$bundle_name" >"$tmp/list$n"
	grep -Eiq 'sbom|spdx' "$tmp/list$n" && fail "build $n: SBOM inside the bundle tar"
	grep -Fxq './SHA256SUMS' "$tmp/list$n" || fail "build $n: SHA256SUMS missing from the bundle"
	(cd "$tmp/build$n/out" && sha256sum -c --quiet "${bundle_name}.sha256") ||
		fail "build $n: bundle .sha256 does not verify from its directory"
	mkdir -p "$tmp/extract$n"
	tar -xzf "$tmp/build$n/out/$bundle_name" -C "$tmp/extract$n"
	(cd "$tmp/extract$n" && sha256sum -c --quiet SHA256SUMS && sha256sum -c --quiet MIGRATION_SHA256SUMS &&
		sha256sum -c --quiet WEB_SHA256SUMS) || fail "build $n: extracted manifests do not verify"
	[[ -x "$tmp/extract$n/odyssey" ]] || fail "build $n: binary mode lost"
	(cd "$tmp/build$n/out" && sha256sum -c --quiet "odyssey-v010-core-${candidate_sha}.sbom.spdx.json.sha256") ||
		fail "build $n: SBOM .sha256 does not verify from its directory"
done

diff -r "$tmp/extract1" "$tmp/extract2" >/dev/null ||
	fail 'bundle contents (ROUTE_PROFILE.json, manifests, binaries) differ across runs of the same inputs'
digest1=$(out_value bundle1 sha256)
digest2=$(out_value bundle2 sha256)
[[ "$digest1" =~ ^[0-9a-f]{64}$ ]] || fail "bundle digest is not a SHA-256: $digest1"
[[ "$digest1" == "$digest2" ]] ||
	fail "same inputs with different SBOM contents produced different bundle digests: $digest1 vs $digest2"
[[ "$(out_value sbom1 sha256)" != "$(out_value sbom2 sha256)" ]] ||
	fail 'fixture error: the two SBOMs should differ'
cmp -s "$tmp/build1/out/${bundle_name}.sha256" "$tmp/build2/out/${bundle_name}.sha256" ||
	fail 'bundle .sha256 files differ across runs'

# ---------------------------------------------------------------------------
# 3. Refusals.
# ---------------------------------------------------------------------------
mkdir -p "$tmp/tagdist/out"
make_dist "$tmp/tagdist/dist" '2026-10-01 10:00:00 UTC'
fake_sbom "$tmp/tagdist/dist/SBOM.spdx.json"
run sbom_in_dist 1 bundle "$tmp/tagdist/dist" "$candidate_sha" "$tmp/tagdist/out"
grep -Fq 'SBOM must not be inside the digested bundle' "$tmp/sbom_in_dist.err" ||
	fail 'an SBOM inside dist must be refused with a clear message'
[[ ! -e "$tmp/tagdist/out/$bundle_name" ]] || fail 'no bundle may be written when dist holds an SBOM'

mkdir -p "$tmp/missing/out"
make_dist "$tmp/missing/dist" '2026-10-01 10:00:00 UTC'
rm "$tmp/missing/dist/ROUTE_PROFILE.json"
run missing_file 1 bundle "$tmp/missing/dist" "$candidate_sha" "$tmp/missing/out"
grep -Fq 'missing release file' "$tmp/missing_file.err" || fail 'missing release file message'

run bad_sha 1 bundle "$tmp/build1/dist" "${candidate_sha:0:7}" "$tmp/build1/out"
: >"$tmp/empty.spdx.json"
run empty_sbom 1 sbom-digest "$tmp/empty.spdx.json"

# ---------------------------------------------------------------------------
# 4. Build-only digest comparison.
# ---------------------------------------------------------------------------
mkdir -p "$tmp/cmp/none" "$tmp/cmp/same" "$tmp/cmp/other" "$tmp/cmp/current"
cp "$tmp/build1/out/${bundle_name}.sha256" "$tmp/cmp/current/"
printf 'go_version=go version go1.25.0 linux/amd64\n' >"$tmp/cmp/current/BUILD_ENV"
cp "$tmp/build2/out/${bundle_name}.sha256" "$tmp/cmp/same/"

run cmp_baseline 0 compare "$tmp/cmp/none" "$tmp/cmp/current" "$bundle_name"
grep -Fq "baseline: no previous digest for $bundle_name; recorded $digest1" "$tmp/cmp_baseline.out" ||
	fail 'first build-only run must record a baseline'

run cmp_same 0 compare "$tmp/cmp/same" "$tmp/cmp/current" "$bundle_name"
grep -Fq "reproducible: $bundle_name $digest1" "$tmp/cmp_same.out" || fail 'matching digests must pass'

other=$(printf 'x' | sha256sum | awk '{print $1}')
printf '%s  %s\n' "$other" "$bundle_name" >"$tmp/cmp/other/${bundle_name}.sha256"
printf 'go_version=go version go1.25.9 linux/amd64\n' >"$tmp/cmp/other/BUILD_ENV"
run cmp_differs 1 compare "$tmp/cmp/other" "$tmp/cmp/current" "$bundle_name"
grep -Fq "previous: $other" "$tmp/cmp_differs.err" || fail 'mismatch must print the previous digest'
grep -Fq "current:  $digest1" "$tmp/cmp_differs.err" || fail 'mismatch must print the current digest'
grep -Fq 'go1.25.9' "$tmp/cmp_differs.err" || fail 'mismatch must print the build environments'

echo 'bundle_test.sh: all checks passed'
