#!/usr/bin/env bash
# Tests for staging-schema-checksum.sh with fixture dumps and stub psql,
# pg_dump and migrate commands (no database needed): normalisation, the
# same-major guard, the ceiling and fresh-reference guards, and mismatch
# detection on the FIND-010 drift objects.
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
gate="$script_dir/staging-schema-checksum.sh"

tmp=$(mktemp -d)
trap 'rm -rf -- "$tmp"' EXIT

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

# --- stub tools -------------------------------------------------------------
# DSNs are the plain words "staging" and "reference". The stubs keep their
# state under $STUB_STATE and log every call there.
mkdir -p "$tmp/bin"
cat >"$tmp/bin/pg_dump" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1-}" == --version ]]; then
	echo "pg_dump (PostgreSQL) ${STUB_CLIENT_VERSION:-17.6 (Debian 17.6-1.pgdg120+1)}"
	exit 0
fi
db=''
for arg in "$@"; do
	case "$arg" in --dbname=*) db=${arg#--dbname=} ;; esac
done
printf 'pg_dump %s PGOPTIONS=%s args=%s\n' "$db" "${PGOPTIONS-}" "$*" >>"$STUB_STATE/calls.log"
cat "$STUB_FIXTURES/$db.sql"
EOF
cat >"$tmp/bin/psql" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
db='' sql=''
while (($# > 0)); do
	case "$1" in
	--dbname=*) db=${1#--dbname=} ;;
	-c) sql=$2; shift ;;
	esac
	shift
done
printf 'psql %s PGOPTIONS=%s sql=%s\n' "$db" "${PGOPTIONS-}" "$sql" >>"$STUB_STATE/calls.log"
upper=$(tr '[:lower:]' '[:upper:]' <<<"$db")
case "$sql" in
'SHOW server_version_num') var="STUB_${upper}_NUM"; echo "${!var:-170006}" ;;
'SHOW server_version') echo "17.6" ;;
*to_regclass*)
	if [[ "$db" == staging || -e "$STUB_STATE/$db.migrated" ]]; then echo f; else echo t; fi ;;
*schema_migrations*)
	if [[ "$db" == staging ]]; then
		printf '%s\n' "${STUB_STAGING_STATE:-124}"
	elif [[ -e "$STUB_STATE/$db.migrated" ]]; then
		echo 124
	else
		echo 'ERROR: relation "schema_migrations" does not exist' >&2
		exit 1
	fi ;;
*) echo "unexpected sql: $sql" >&2; exit 1 ;;
esac
EOF
cat >"$tmp/bin/migrate" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
db=''
while (($# > 0)); do
	case "$1" in -database) db=$2; shift ;; esac
	shift
done
printf 'migrate %s\n' "$db" >>"$STUB_STATE/calls.log"
touch "$STUB_STATE/$db.migrated"
EOF
chmod +x "$tmp/bin/"*

export PG_DUMP_CMD="$tmp/bin/pg_dump" PSQL_CMD="$tmp/bin/psql" MIGRATE_CMD="$tmp/bin/migrate"

mkdir -p "$tmp/migrations"
for f in 000001_init 000123_previous 000124_scoped_rbac_global_compatibility; do
	: >"$tmp/migrations/$f.up.sql"
	: >"$tmp/migrations/$f.down.sql"
done

# --- fixture dumps ----------------------------------------------------------
fixtures="$tmp/fixtures"
mkdir -p "$fixtures"

# A clean schema as pg_dump 17 prints it, with per-dump noise.
write_dump() { # write_dump <file> <restrict-key> <dumped-by> <extra-sql-file-or-empty>
	{
		printf -- '--\n-- PostgreSQL database dump\n--\n\n'
		printf '\\restrict %s\n\n' "$2"
		printf -- '-- Dumped from database version 17.6\n-- Dumped by pg_dump version %s\n\n' "$3"
		printf "SET statement_timeout = 0;\nSET client_encoding = 'UTF8';\n"
		printf "SELECT pg_catalog.set_config('search_path', '', false);\n\n"
		cat <<'SQL'
--
-- Name: document_signature_challenges; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.document_signature_challenges (
    id bigint NOT NULL,
    challenge_id uuid NOT NULL,
    meaning character varying(200) NOT NULL
);

--
-- Name: ar_invoices; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ar_invoices (
    id bigint NOT NULL,
    status text NOT NULL
);
SQL
		[[ -z "$4" ]] || cat "$4"
		printf '\n--\n-- PostgreSQL database dump complete\n--\n\n\\unrestrict %s\n\n' "$2"
	} >"$1"
}

cat >"$tmp/drift.sql" <<'SQL'

--
-- Name: document_blobs; Type: VIEW; Schema: public; Owner: -
--

CREATE VIEW public.document_blobs AS
 SELECT id FROM public.storage_blobs;

ALTER TABLE ONLY public.document_signature_challenges ALTER COLUMN challenge_id SET DEFAULT gen_random_uuid();

--
-- Name: ar_invoices trg_set_ar_invoice_subtotal; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_set_ar_invoice_subtotal BEFORE INSERT OR UPDATE ON public.ar_invoices FOR EACH ROW EXECUTE FUNCTION public.set_ar_invoice_subtotal_default();
SQL

write_dump "$fixtures/reference.sql" aaaaKEYaaaa '17.6 (Debian 17.6-1.pgdg120+1)' ''
# Same schema, other noise: different restrict key, client banner, trailing
# whitespace on a line.
write_dump "$fixtures/staging-noise.sql" bbbbKEYbbbb '17.4' ''
sed -i 's/^    status text NOT NULL$/    status text NOT NULL   \t/' "$fixtures/staging-noise.sql"
write_dump "$fixtures/staging-drift.sql" ccccKEYcccc '17.6' "$tmp/drift.sql"

# run <name> <expected-exit> <staging-fixture> [extra gate args...]
run() {
	local name=$1 want=$2 fixture=$3
	shift 3
	export STUB_STATE="$tmp/state-$name" STUB_FIXTURES="$tmp/fx-$name"
	mkdir -p "$STUB_STATE" "$STUB_FIXTURES"
	cp "$fixtures/reference.sql" "$STUB_FIXTURES/reference.sql"
	cp "$fixtures/$fixture.sql" "$STUB_FIXTURES/staging.sql"
	out="$tmp/out-$name"
	local got=0
	bash "$gate" --staging-dsn staging --reference-dsn reference --migrations "$tmp/migrations" \
		--out "$out" "$@" >"$tmp/$name.out" 2>"$tmp/$name.err" || got=$?
	if [[ "$got" != "$want" ]]; then
		echo "--- stdout" >&2; cat "$tmp/$name.out" >&2
		echo "--- stderr" >&2; cat "$tmp/$name.err" >&2
		fail "$name: exit $got, want $want"
	fi
}

field() { sed -n "s/^$1=//p" "$out/schema-checksum.txt"; }

# 1. Normalisation: dumps that differ only in banner, SET/set_config,
#    \restrict keys and trailing whitespace match.
run noise 0 staging-noise --ceiling 124
[[ "$(field result)" == MATCH ]] || fail 'noise: result is not MATCH'
[[ "$(field reference_sha256)" == "$(field staging_sha256)" ]] || fail 'noise: digests differ'
[[ "$(field reference_sha256)" =~ ^[0-9a-f]{64}$ ]] || fail 'noise: digest is not sha256'
[[ "$(field migration_ceiling)" == 124 && "$(field major)" == 17 ]] || fail 'noise: ceiling/major not recorded'
[[ ! -s "$out/schema.diff" ]] || fail 'noise: schema.diff is not empty'
if grep -Eq '^(SET |SELECT pg_catalog\.set_config|-- Dumped |\\(un)?restrict )|[[:space:]]$' "$out/staging.sql" "$out/reference.sql"; then
	fail 'noise: normalised dumps still contain banner, SET, set_config, restrict or trailing whitespace'
fi
grep -q 'CREATE TABLE public.ar_invoices' "$out/staging.sql" || fail 'noise: normalisation dropped schema lines'
grep -q 'schema checksum gate passed' "$tmp/noise.out" || fail 'noise: no pass message'
# Staging is only read, in read-only sessions; only the reference is migrated.
grep -q '^migrate reference$' "$STUB_STATE/calls.log" || fail 'noise: reference was not migrated'
if grep -q '^migrate staging' "$STUB_STATE/calls.log"; then fail 'noise: staging was migrated'; fi
if grep -E '^(psql|pg_dump) staging ' "$STUB_STATE/calls.log" | grep -vq 'PGOPTIONS=-c default_transaction_read_only=on'; then
	fail 'noise: a staging session was not read-only'
fi
grep -q -- '--schema-only --no-owner --no-privileges --no-comments --no-security-labels --no-tablespaces --no-publications --no-subscriptions' \
	"$STUB_STATE/calls.log" || fail 'noise: pg_dump flags changed'

# 2. Mismatch: the FIND-010 drift (view, default, trigger) fails the gate and
#    schema.diff names every object.
run drift 1 staging-drift
[[ "$(field result)" == MISMATCH ]] || fail 'drift: result is not MISMATCH'
[[ "$(field reference_sha256)" != "$(field staging_sha256)" ]] || fail 'drift: digests are equal'
for object in 'VIEW public.document_blobs' 'challenge_id SET DEFAULT gen_random_uuid()' 'TRIGGER trg_set_ar_invoice_subtotal'; do
	grep -q "^+.*$object" "$out/schema.diff" || fail "drift: schema.diff does not name $object"
done
grep -q 'schema checksum gate FAILED' "$tmp/drift.err" || fail 'drift: no failure message'
[[ -s "$out/reference.sql" && -s "$out/staging.sql" ]] || fail 'drift: dumps not written'

# 3. Same-major guard: client, reference and staging majors must be equal;
#    nothing is migrated or dumped when they differ.
guard_case() { # guard_case <name> <env assignment>
	local name=$1
	export "${2?}"
	run "$name" 2 staging-noise
	unset "${2%%=*}"
	grep -q 'major version mismatch' "$tmp/$name.err" || fail "$name: no major mismatch message"
	if grep -Eq '^(migrate|pg_dump) ' "$tmp/state-$name/calls.log"; then fail "$name: migrated or dumped despite the guard"; fi
	[[ ! -e "$tmp/out-$name/schema-checksum.txt" ]] || fail "$name: wrote a checksum"
}
guard_case client18 'STUB_CLIENT_VERSION=18.6'
guard_case client15 'STUB_CLIENT_VERSION=15.10 (Ubuntu 15.10-0ubuntu0.24.04.1)'
guard_case reference16 'STUB_REFERENCE_NUM=160004'
guard_case staging15 'STUB_STAGING_NUM=150008'
grep -q 'pg_dump client 18, reference server 17, staging server 17' "$tmp/client18.err" || fail 'client18: majors not named'

# 4. Staging must be exactly at the ceiling and clean.
STUB_STAGING_STATE=125 run staging125 2 staging-noise
grep -q "expected exactly 124" "$tmp/staging125.err" || fail 'staging125: no ceiling message'
STUB_STAGING_STATE='124 dirty' run stagingdirty 2 staging-noise
grep -q "reads '124 dirty'" "$tmp/stagingdirty.err" || fail 'stagingdirty: no dirty message'

# 5. --ceiling must equal the newest migration in the directory.
run ceiling 2 staging-noise --ceiling 123
grep -q 'newest migration .* is 124, expected ceiling 123' "$tmp/ceiling.err" || fail 'ceiling: no message'

# 6. The reference must be fresh; it is never migrated twice.
export STUB_STATE="$tmp/state-notfresh"
mkdir -p "$STUB_STATE" && touch "$STUB_STATE/reference.migrated"
got=0
STUB_FIXTURES="$tmp/fx-noise" bash "$gate" --staging-dsn staging --reference-dsn reference \
	--migrations "$tmp/migrations" --out "$tmp/out-notfresh" >"$tmp/notfresh.out" 2>"$tmp/notfresh.err" || got=$?
[[ "$got" == 2 ]] || { cat "$tmp/notfresh.err" >&2; fail "notfresh: exit $got, want 2"; }
grep -q 'must be a fresh, empty database' "$tmp/notfresh.err" || fail 'notfresh: no message'
if grep -q '^migrate ' "$STUB_STATE/calls.log"; then fail 'notfresh: migrated a non-fresh reference'; fi

# 7. The reference DSN must not be the staging DSN; usage errors exit 2.
got=0
bash "$gate" --staging-dsn staging --reference-dsn staging --migrations "$tmp/migrations" --out "$tmp/out-same" \
	>/dev/null 2>"$tmp/same.err" || got=$?
[[ "$got" == 2 ]] || fail "same DSN: exit $got, want 2"
grep -q 'must not be the staging DSN' "$tmp/same.err" || fail 'same DSN: no message'
got=0
bash "$gate" --staging-dsn staging >/dev/null 2>&1 || got=$?
[[ "$got" == 2 ]] || fail "missing arguments: exit $got, want 2"

echo 'staging-schema-checksum tests passed'
