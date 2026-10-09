#!/usr/bin/env bash
# Schema checksum gate for the v0.10 staging certification (DB-002).
#
# Migrates a fresh, disposable reference database with the candidate's
# migrations, dumps the reference and the staging schema with a pg_dump client
# of the same major version as both servers, normalises the dumps and compares
# their SHA-256 digests. Any difference fails the gate, so no certification test
# runs against a schema that the candidate's migrations do not produce
# (FIND-010: out-of-band triggers, functions, a view and column defaults).
#
# The staging database is only read (SELECTs and pg_dump, in read-only
# sessions). The reference database must be empty: the script refuses to
# migrate a database that already has schema_migrations, so it can never run
# migrations against staging by mistake.
#
# Usage:
#   staging-schema-checksum.sh --staging-dsn DSN --reference-dsn DSN \
#     --migrations DIR --out DIR [--ceiling N]
#
# Outputs in --out: reference.sql, staging.sql (normalised dumps),
# schema.diff (unified diff, empty on a match) and schema-checksum.txt
# (both digests, client and server versions, migration ceiling, result).
#
# Exit status: 0 schemas match, 1 schemas differ, 2 usage, guard or tool error.
#
# Tool overrides (default in parentheses), each a command line split on
# whitespace: PG_DUMP_CMD (pg_dump), PSQL_CMD (psql), MIGRATE_CMD (migrate).
# For example, with only a newer local client:
#   PG_DUMP_CMD='docker run --rm -i --network host -e PGOPTIONS postgres:17 pg_dump'
set -euo pipefail

usage() {
	cat >&2 <<'USAGE'
usage: staging-schema-checksum.sh --staging-dsn DSN --reference-dsn DSN \
         --migrations DIR --out DIR [--ceiling N]
USAGE
	exit 2
}

die() {
	echo "staging-schema-checksum: $*" >&2
	exit 2
}

staging_dsn=''
reference_dsn=''
migrations=''
out=''
ceiling=''
while (($# > 0)); do
	case "$1" in
	--staging-dsn) staging_dsn=${2-}; shift 2 || usage ;;
	--reference-dsn) reference_dsn=${2-}; shift 2 || usage ;;
	--migrations) migrations=${2-}; shift 2 || usage ;;
	--out) out=${2-}; shift 2 || usage ;;
	--ceiling) ceiling=${2-}; shift 2 || usage ;;
	-h | --help) usage ;;
	*) echo "unknown argument: $1" >&2; usage ;;
	esac
done

[[ -n "$staging_dsn" && -n "$reference_dsn" && -n "$migrations" && -n "$out" ]] || usage
[[ "$staging_dsn" != "$reference_dsn" ]] || die 'the reference DSN must not be the staging DSN'
[[ -d "$migrations" ]] || die "migrations directory not found: $migrations"

read -r -a pg_dump_cmd <<<"${PG_DUMP_CMD:-pg_dump}"
read -r -a psql_cmd <<<"${PSQL_CMD:-psql}"
read -r -a migrate_cmd <<<"${MIGRATE_CMD:-migrate}"

# The newest migration in the directory is the ceiling; --ceiling must agree.
latest=$(find "$migrations" -maxdepth 1 -type f -name '[0-9]*_*.up.sql' -printf '%f\n' |
	sed -E 's/^0*([0-9]+)_.*/\1/' | sort -n | tail -n 1)
[[ -n "$latest" ]] || die "no *.up.sql migrations in $migrations"
if [[ -z "$ceiling" ]]; then
	ceiling=$latest
fi
ceiling=$((10#$ceiling))
((latest == ceiling)) || die "newest migration in $migrations is $latest, expected ceiling $ceiling"

mkdir -p "$out"

# query <dsn> <sql> [read-only]: prints the unaligned, tuples-only result.
query() {
	local dsn=$1 sql=$2
	if [[ "${3-}" == read-only ]]; then
		PGOPTIONS='-c default_transaction_read_only=on' \
			"${psql_cmd[@]}" -X -A -t -q -v ON_ERROR_STOP=1 --dbname="$dsn" -c "$sql"
	else
		"${psql_cmd[@]}" -X -A -t -q -v ON_ERROR_STOP=1 --dbname="$dsn" -c "$sql"
	fi
}

# major_of_version_num 170006 -> 17
major_of_version_num() {
	[[ "$1" =~ ^[0-9]{5,6}$ ]] || return 1
	echo $(($1 / 10000))
}

# Normalise a dump: drop the version banner, session SETs, set_config and
# the per-dump random \restrict/\unrestrict keys, strip trailing whitespace.
normalise() {
	sed -E \
		-e '/^-- Dumped (from|by) /d' \
		-e '/^SET /d' \
		-e '/^SELECT pg_catalog\.set_config/d' \
		-e '/^\\(un)?restrict /d' \
		-e 's/[[:space:]]+$//' \
		"$1"
}

# 1. Same major everywhere: staging server, reference server, pg_dump client.
client_version=$("${pg_dump_cmd[@]}" --version) || die 'pg_dump --version failed'
[[ "$client_version" =~ \(PostgreSQL\)\ ([0-9]+)(\.[0-9]+)? ]] || die "cannot parse pg_dump version: $client_version"
client_major=${BASH_REMATCH[1]}

staging_num=$(query "$staging_dsn" 'SHOW server_version_num' read-only) || die 'cannot read the staging server version'
reference_num=$(query "$reference_dsn" 'SHOW server_version_num') || die 'cannot read the reference server version'
staging_major=$(major_of_version_num "$staging_num") || die "unexpected staging server_version_num: $staging_num"
reference_major=$(major_of_version_num "$reference_num") || die "unexpected reference server_version_num: $reference_num"
staging_version=$(query "$staging_dsn" 'SHOW server_version' read-only)
reference_version=$(query "$reference_dsn" 'SHOW server_version')

echo "pg_dump client major $client_major, staging server $staging_version, reference server $reference_version"
if [[ "$client_major" != "$staging_major" || "$reference_major" != "$staging_major" ]]; then
	die "major version mismatch: pg_dump client $client_major, reference server $reference_major, staging server $staging_major; all three must be equal (dumps of different majors are not comparable)"
fi

# 2. Staging must be exactly at the ceiling and clean.
migration_state_sql="SELECT version::text || CASE WHEN dirty THEN ' dirty' ELSE '' END FROM schema_migrations ORDER BY version"
staging_state=$(query "$staging_dsn" "$migration_state_sql" read-only) ||
	die 'cannot read staging schema_migrations'
[[ "$staging_state" == "$ceiling" ]] || die "staging schema_migrations reads '${staging_state//$'\n'/,}', expected exactly $ceiling (clean)"

# 3. Migrate the fresh reference database to the ceiling.
fresh=$(query "$reference_dsn" "SELECT to_regclass('public.schema_migrations') IS NULL") || die 'cannot inspect the reference database'
[[ "$fresh" == t ]] || die 'the reference database already has schema_migrations; it must be a fresh, empty database'
"${migrate_cmd[@]}" -path "$migrations" -database "$reference_dsn" up >&2 || die 'migrate up on the reference database failed'
reference_state=$(query "$reference_dsn" "$migration_state_sql")
[[ "$reference_state" == "$ceiling" ]] || die "reference schema_migrations reads '$reference_state' after migrate up, expected $ceiling"

# 4. Dump, normalise and compare.
dump_flags=(--schema-only --no-owner --no-privileges --no-comments --no-security-labels
	--no-tablespaces --no-publications --no-subscriptions)
raw_dir=$(mktemp -d)
trap 'rm -rf -- "$raw_dir"' EXIT
"${pg_dump_cmd[@]}" "${dump_flags[@]}" --dbname="$reference_dsn" >"$raw_dir/reference.sql" ||
	die 'pg_dump of the reference database failed'
PGOPTIONS='-c default_transaction_read_only=on' \
	"${pg_dump_cmd[@]}" "${dump_flags[@]}" --dbname="$staging_dsn" >"$raw_dir/staging.sql" ||
	die 'pg_dump of the staging database failed'
[[ -s "$raw_dir/reference.sql" && -s "$raw_dir/staging.sql" ]] || die 'pg_dump produced an empty dump'

normalise "$raw_dir/reference.sql" >"$out/reference.sql"
normalise "$raw_dir/staging.sql" >"$out/staging.sql"
reference_sha=$(sha256sum "$out/reference.sql" | awk '{print $1}')
staging_sha=$(sha256sum "$out/staging.sql" | awk '{print $1}')

result=MATCH
diff -u --label reference.sql --label staging.sql "$out/reference.sql" "$out/staging.sql" >"$out/schema.diff" || result=MISMATCH

{
	printf 'algorithm=sha256 (normalised pg_dump --schema-only)\n'
	printf 'reference_sha256=%s\n' "$reference_sha"
	printf 'staging_sha256=%s\n' "$staging_sha"
	printf 'pg_dump_client=%s\n' "$client_version"
	printf 'reference_server_version=%s\n' "$reference_version"
	printf 'staging_server_version=%s\n' "$staging_version"
	printf 'major=%s\n' "$staging_major"
	printf 'migration_ceiling=%s\n' "$ceiling"
	printf 'result=%s\n' "$result"
} >"$out/schema-checksum.txt"

echo "reference sha256 $reference_sha"
echo "staging   sha256 $staging_sha"
if [[ "$result" != MATCH ]]; then
	echo "schema checksum gate FAILED: staging differs from the schema migrations 1..$ceiling produce; see $out/schema.diff" >&2
	exit 1
fi
echo "schema checksum gate passed (migration ceiling $ceiling)"
