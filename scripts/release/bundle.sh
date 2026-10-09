#!/usr/bin/env bash
# Content-addressed v0.10-core release bundle for deploy-native.yml.
#
#   bundle.sh bundle <dist-dir> <candidate-sha> <out-dir>
#       Writes <dist-dir>/SHA256SUMS, then <out-dir>/odyssey-v010-core-<sha>.tar.gz
#       (deterministic tar: sorted names, fixed mtime/owner, gzip -n) and its
#       .sha256. Prints "name=<bundle>" and "sha256=<hex>" on stdout (ready for
#       $GITHUB_OUTPUT); diagnostics go to stderr. The SBOM is NOT part of the
#       bundle: an SPDX document carries a creation time and a random namespace,
#       so digesting it would make every rebuild of the same commit differ.
#       A dist directory that already holds an SBOM is refused.
#
#   bundle.sh sbom-digest <sbom-file>
#       Writes <sbom-file>.sha256 (basename form, next to the file) for the
#       SBOM that is generated after the bundle and attested beside it.
#       Prints "name=<basename>" and "sha256=<hex>".
#
#   bundle.sh compare <previous-dir> <current-dir> <bundle-name>
#       Reproducibility check between two build-only runs. Each dir holds
#       <bundle-name>.sha256 and optionally BUILD_ENV. A missing previous
#       digest is the baseline (exit 0); differing digests exit 1 and print
#       both digests and both build environments.
set -euo pipefail

fail() {
	echo "bundle.sh: $*" >&2
	exit 1
}

# Files digested into SHA256SUMS, in this order. Directories are covered by
# MIGRATION_SHA256SUMS and WEB_SHA256SUMS.
digested_files=(
	REVISION
	RELEASE_IDENTITY
	MIGRATION_SHA256SUMS
	WEB_SHA256SUMS
	ROUTE_PROFILE.json
	odyssey
	worker
	bootstrap-admin
	migrate
)
required_dirs=(migrations web)

cmd_bundle() {
	(($# == 3)) || fail 'usage: bundle.sh bundle <dist-dir> <candidate-sha> <out-dir>'
	local dist=$1 sha=$2 out=$3
	[[ "$sha" =~ ^[0-9a-f]{40}$ ]] || fail "candidate SHA must be 40 lowercase hex characters: $sha"
	[[ -d "$dist" ]] || fail "dist directory not found: $dist"
	[[ -d "$out" ]] || fail "output directory not found: $out"

	local name
	for name in "${digested_files[@]}"; do
		[[ -f "$dist/$name" ]] || fail "missing release file: $dist/$name"
	done
	for name in "${required_dirs[@]}"; do
		[[ -d "$dist/$name" ]] || fail "missing release directory: $dist/$name"
	done

	local sbom
	sbom=$(find "$dist" \( -iname '*.spdx*' -o -iname 'sbom*' \) -print -quit)
	[[ -z "$sbom" ]] || fail "SBOM must not be inside the digested bundle: $sbom (generate it after bundling, as a sibling)"

	(
		cd "$dist"
		sha256sum "${digested_files[@]}"
	) >"$dist/SHA256SUMS.tmp"
	mv "$dist/SHA256SUMS.tmp" "$dist/SHA256SUMS"

	local bundle="odyssey-v010-core-${sha}.tar.gz"
	tar \
		--sort=name \
		--mtime='UTC 1970-01-01' \
		--owner=0 \
		--group=0 \
		--numeric-owner \
		-C "$dist" \
		-cf - . | gzip -n >"$out/$bundle"
	(
		cd "$out"
		sha256sum "$bundle" >"${bundle}.sha256"
	)
	local digest
	digest=$(awk '{print $1}' "$out/${bundle}.sha256")
	printf 'name=%s\n' "$bundle"
	printf 'sha256=%s\n' "$digest"
}

cmd_sbom_digest() {
	(($# == 1)) || fail 'usage: bundle.sh sbom-digest <sbom-file>'
	local sbom=$1
	[[ -s "$sbom" ]] || fail "SBOM file missing or empty: $sbom"
	local dir base
	dir=$(dirname -- "$sbom")
	base=$(basename -- "$sbom")
	(
		cd "$dir"
		sha256sum "$base" >"${base}.sha256"
	)
	printf 'name=%s\n' "$base"
	printf 'sha256=%s\n' "$(awk '{print $1}' "$dir/${base}.sha256")"
}

cmd_compare() {
	(($# == 3)) || fail 'usage: bundle.sh compare <previous-dir> <current-dir> <bundle-name>'
	local previous=$1 current=$2 bundle=$3
	local current_file="$current/${bundle}.sha256"
	[[ -s "$current_file" ]] || fail "current digest missing: $current_file"
	local current_digest previous_digest
	current_digest=$(awk '{print $1}' "$current_file")
	[[ "$current_digest" =~ ^[0-9a-f]{64}$ ]] || fail "current digest is not a SHA-256: $current_digest"

	local previous_file="$previous/${bundle}.sha256"
	if [[ ! -e "$previous_file" ]]; then
		printf 'baseline: no previous digest for %s; recorded %s\n' "$bundle" "$current_digest"
		return 0
	fi
	previous_digest=$(awk '{print $1}' "$previous_file")
	if [[ "$previous_digest" == "$current_digest" ]]; then
		printf 'reproducible: %s %s matches the previous build\n' "$bundle" "$current_digest"
		return 0
	fi
	{
		printf 'bundle digest differs from the previous build of %s\n' "$bundle"
		printf '  previous: %s\n' "$previous_digest"
		printf '  current:  %s\n' "$current_digest"
		if [[ -f "$previous/BUILD_ENV" || -f "$current/BUILD_ENV" ]]; then
			echo '--- previous BUILD_ENV'
			cat "$previous/BUILD_ENV" 2>/dev/null || echo '(missing)'
			echo '--- current BUILD_ENV'
			cat "$current/BUILD_ENV" 2>/dev/null || echo '(missing)'
		fi
	} >&2
	return 1
}

(($# >= 1)) || fail 'usage: bundle.sh {bundle|sbom-digest|compare} ...'
command=$1
shift
case "$command" in
	bundle) cmd_bundle "$@" ;;
	sbom-digest) cmd_sbom_digest "$@" ;;
	compare) cmd_compare "$@" ;;
	*) fail "unknown command: $command" ;;
esac
