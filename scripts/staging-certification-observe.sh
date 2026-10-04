#!/usr/bin/env bash
set -euo pipefail

# Raw-evidence collector for certification row OPS-005 (v0.10-core staging):
# "At least 60-minute observation metrics, incident log, and operator handoff".
#
# The collector is strictly read-only against the staging host. It runs only:
#   curl GET <health-url>; journalctl -u <unit> (read); systemctl show;
#   redis-cli PING / LLEN / ZCARD / EXISTS; pg_isready (or the operator-supplied
#   --pg-check-cmd); df; readlink; and reads of /proc/loadavg and /proc/meminfo.
#   It never restarts a unit and never writes to Redis or Postgres. It reports
#   observations only; PASS/FAIL belongs to the reviewer.
#
# Asynq v0.25.1 key names (hibiken/asynq internal/base/base.go, queue "default"):
#   QueueKeyPrefix  asynq:{<queue>}:                  base.go:106
#   PendingKey      asynq:{<queue>}:pending   LIST    base.go:121  -> LLEN
#   ActiveKey       asynq:{<queue>}:active    LIST    base.go:126  -> LLEN
#   ScheduledKey    asynq:{<queue>}:scheduled ZSET    base.go:131  -> ZCARD
#   RetryKey        asynq:{<queue>}:retry     ZSET    base.go:136  -> ZCARD
#   ArchivedKey     asynq:{<queue>}:archived  ZSET    base.go:141  -> ZCARD
#   CompletedKey    asynq:{<queue>}:completed ZSET    base.go:150  -> ZCARD
#   PausedKey       asynq:{<queue>}:paused    string  base.go:155  -> EXISTS
# The types match the Inspector's own CurrentStats script (internal/rdb/inspect.go:97-120).

export LC_ALL=C

readonly SCHEMA_VERSION='v0.10-core-ops005-observation.v1'
readonly MIN_OPS005_SECONDS=3600

usage() {
	cat <<'EOF'
Usage: staging-certification-observe.sh --out DIR --handoff-owner NAME --confirm-staging [options]

Collect the raw OPS-005 observation record on the staging host: one sample per
interval of health, request log, queue depth, Postgres/Redis health, resource
saturation, and unit state. Read-only; refuses to run without --confirm-staging.
Writes DIR/samples.jsonl, DIR/summary.json, DIR/raw/<NNNN>/..., and DIR/SHA256SUMS.
The summary reports observations and anomalies; it never decides PASS or FAIL.

Required:
  --out DIR              New or empty output directory
  --handoff-owner NAME   Operator who owns the handoff at the end of the window
  --confirm-staging      Acknowledge that this host is the isolated staging host

Options:
  --duration SECONDS     Observation window (default: 3600). Values below 3600
                         require --rehearsal.
  --interval SECONDS     Sampling interval (default: 60)
  --rehearsal            Dry run; the summary is marked eligible_for_ops005: false
  --health-url URL       Health endpoint (default: http://127.0.0.1:8180/healthz)
  --app-unit NAME        Application unit (default: odyssey-staging.service)
  --worker-unit NAME     Worker unit (default: odyssey-staging-worker.service)
  --install-dir DIR      Staging root holding .env, current, releases (default: /opt/odyssey-staging)
  --env-file FILE        Read PG_DSN / REDIS_ADDR / APP_ENV from FILE without sourcing it
                         (default: INSTALL_DIR/.env; the ambient PG_DSN and REDIS_ADDR
                         variables are the fallback)
  --queue NAME           Asynq queue to read (default: default)
  --pg-check-cmd CMD     Run CMD (via bash -c) instead of pg_isready; exit 0 means healthy
  --alerts-file FILE     Operator-supplied alert log; must exist at start, read at the end
  --incidents-file FILE  Operator-supplied incident log; must exist at start, read at the end
  --candidate-tag TAG    Candidate tag to record (for example v0.10.0-rc.8)
  --expected-sha SHA     40-hex candidate commit to record and compare to the current release
  --max-error-rate N     5xx error-rate threshold that raises an anomaly (default: 0.01)
  --help                 Show this help

Alerts and incidents that are not supplied are recorded as "not supplied", never as none.
SIGINT, SIGTERM, and SIGHUP end the window early and write a summary with
completed: false and eligible_for_ops005: false. Run inside tmux or nohup.

Credentials are never printed or stored: DSN and Redis URL userinfo is parsed only to
derive host and port, Redis AUTH is passed through REDISCLI_AUTH, and every captured
text is redacted. redis-cli 6.0 or newer is required for REDISCLI_AUTH and --user.

Testing hook: OBSERVE_PROC_DIR overrides /proc for loadavg and meminfo.
EOF
}

need_value() {
	[[ $# -ge 2 ]] || { echo "missing value for $1" >&2; exit 2; }
}

out=''
handoff_owner=''
confirm_staging=false
duration=3600
interval=60
rehearsal=false
health_url='http://127.0.0.1:8180/healthz'
app_unit='odyssey-staging.service'
worker_unit='odyssey-staging-worker.service'
install_dir='/opt/odyssey-staging'
env_file=''
queue_name='default'
pg_check_cmd=''
alerts_file=''
incidents_file=''
candidate_tag=''
expected_sha=''
max_error_rate='0.01'
proc_dir=${OBSERVE_PROC_DIR:-/proc}

while (($#)); do
	case "$1" in
		--out) need_value "$@"; out=$2; shift 2 ;;
		--handoff-owner) need_value "$@"; handoff_owner=$2; shift 2 ;;
		--confirm-staging) confirm_staging=true; shift ;;
		--duration) need_value "$@"; duration=$2; shift 2 ;;
		--interval) need_value "$@"; interval=$2; shift 2 ;;
		--rehearsal) rehearsal=true; shift ;;
		--health-url) need_value "$@"; health_url=$2; shift 2 ;;
		--app-unit) need_value "$@"; app_unit=$2; shift 2 ;;
		--worker-unit) need_value "$@"; worker_unit=$2; shift 2 ;;
		--install-dir) need_value "$@"; install_dir=$2; shift 2 ;;
		--env-file) need_value "$@"; env_file=$2; shift 2 ;;
		--queue) need_value "$@"; queue_name=$2; shift 2 ;;
		--pg-check-cmd) need_value "$@"; pg_check_cmd=$2; shift 2 ;;
		--alerts-file) need_value "$@"; alerts_file=$2; shift 2 ;;
		--incidents-file) need_value "$@"; incidents_file=$2; shift 2 ;;
		--candidate-tag) need_value "$@"; candidate_tag=$2; shift 2 ;;
		--expected-sha) need_value "$@"; expected_sha=$2; shift 2 ;;
		--max-error-rate) need_value "$@"; max_error_rate=$2; shift 2 ;;
		-h|--help) usage; exit 0 ;;
		*) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
	esac
done

die_usage() { echo "$1" >&2; exit 2; }

[[ "$confirm_staging" == true ]] || die_usage '--confirm-staging is required: this collector must only run on the isolated staging host'
[[ -n "$out" ]] || die_usage '--out is required'
[[ -n "${handoff_owner//[[:space:]]/}" ]] || die_usage '--handoff-owner is required'
[[ ${#handoff_owner} -le 200 && "$handoff_owner" != *$'\n'* ]] || die_usage '--handoff-owner must be a single line of at most 200 characters'
[[ "$duration" =~ ^[0-9]+$ ]] || die_usage '--duration must be a positive integer number of seconds'
[[ "$interval" =~ ^[0-9]+$ ]] || die_usage '--interval must be a positive integer number of seconds'
duration=$((10#$duration))
interval=$((10#$interval))
((duration >= 1)) || die_usage '--duration must be at least 1 second'
((interval >= 1)) || die_usage '--interval must be at least 1 second'
if ((duration < MIN_OPS005_SECONDS)) && [[ "$rehearsal" != true ]]; then
	die_usage "--duration below $MIN_OPS005_SECONDS seconds requires --rehearsal (the OPS-005 window is at least 60 minutes)"
fi
((interval <= duration)) || die_usage '--interval must not exceed --duration'
[[ "$max_error_rate" =~ ^(0|1|0?\.[0-9]+|1\.0+)$ ]] || die_usage '--max-error-rate must be a decimal between 0 and 1'
[[ "$max_error_rate" != .* ]] || max_error_rate="0$max_error_rate"
[[ -z "$candidate_tag" || "$candidate_tag" =~ ^[A-Za-z0-9._-]+$ ]] || die_usage '--candidate-tag contains unsupported characters'
[[ -z "$expected_sha" || "$expected_sha" =~ ^[0-9a-f]{40}$ ]] || die_usage '--expected-sha must be a 40-hex lowercase commit'
[[ "$queue_name" =~ ^[A-Za-z0-9_.-]+$ ]] || die_usage '--queue contains unsupported characters'
[[ "$app_unit" =~ ^[A-Za-z0-9@:_.-]+$ && "$worker_unit" =~ ^[A-Za-z0-9@:_.-]+$ ]] || die_usage 'unit names contain unsupported characters'
shopt -s nocasematch
if [[ "$app_unit" == *prod* || "$worker_unit" == *prod* ]]; then
	die_usage 'refusing to observe a unit whose name contains "prod"; staging only'
fi
shopt -u nocasematch
[[ -z "$alerts_file" || -f "$alerts_file" && -r "$alerts_file" ]] || die_usage "--alerts-file must name a readable file (create it empty before the run if no alerts have fired yet): $alerts_file"
[[ -z "$incidents_file" || -f "$incidents_file" && -r "$incidents_file" ]] || die_usage "--incidents-file must name a readable file (create it empty before the run if no incident has occurred yet): $incidents_file"

if [[ -e "$out" ]]; then
	[[ -d "$out" ]] || die_usage "--out must be a directory: $out"
	if [[ -n "$(find "$out" -mindepth 1 -print -quit)" ]]; then
		die_usage "output directory must be empty: $out"
	fi
fi

require_tool() {
	command -v "$1" >/dev/null 2>&1 || { echo "$1 is required" >&2; exit 1; }
}
for tool in curl jq journalctl systemctl redis-cli sha256sum awk sed sort timeout df readlink find xargs; do
	require_tool "$tool"
done
[[ -n "$pg_check_cmd" ]] || require_tool pg_isready

# ---------------------------------------------------------------------------
# Target derivation. Credentials are parsed only to learn host and port and to
# build the literal-scrub list; they are never printed or written.
# ---------------------------------------------------------------------------
secret_literals=''
add_secret() {
	local value=$1
	((${#value} >= 4)) || return 0
	secret_literals+="$value"$'\n'
	return 0
}

pct_decode() {
	local value=$1
	printf '%b' "${value//%/\\x}"
}

env_value() {
	local name=$1 file=$2 line value
	[[ -r "$file" ]] || return 1
	line=$(grep -E "^[[:space:]]*(export[[:space:]]+)?${name}=" "$file" | tail -n 1) || return 1
	value=${line#*=}
	value=${value%$'\r'}
	if [[ "$value" == \"*\" && ${#value} -ge 2 ]]; then
		value=${value:1:${#value}-2}
	elif [[ "$value" == \'*\' && ${#value} -ge 2 ]]; then
		value=${value:1:${#value}-2}
	fi
	printf '%s' "$value"
}

[[ -n "$env_file" ]] || env_file="$install_dir/.env"
env_app_env=$(env_value APP_ENV "$env_file" 2>/dev/null || true)
if [[ -n "$env_app_env" && "$env_app_env" != staging ]]; then
	die_usage "refusing to run: APP_ENV=$env_app_env in the environment file (staging only)"
fi

pg_dsn=$(env_value PG_DSN "$env_file" 2>/dev/null || true)
[[ -n "$pg_dsn" ]] || pg_dsn=${PG_DSN:-}
redis_addr=$(env_value REDIS_ADDR "$env_file" 2>/dev/null || true)
[[ -n "$redis_addr" ]] || redis_addr=${REDIS_ADDR:-127.0.0.1:6379}

pg_host=''
pg_port=''
parse_pg_dsn() {
	local dsn=$1 userinfo='' pass=''
	if [[ "$dsn" =~ ^postgres(ql)?://([^/?]*@)?(\[[^]]+\]|[^:/?]*)(:([0-9]+))?([/?]|$) ]]; then
		userinfo=${BASH_REMATCH[2]%@}
		pg_host=${BASH_REMATCH[3]}
		pg_port=${BASH_REMATCH[5]}
		[[ "$userinfo" != *:* ]] || pass=${userinfo#*:}
		pg_host=${pg_host#[}
		pg_host=${pg_host%]}
		if [[ "$dsn" =~ [\?\&]host=([^\&]+) ]]; then pg_host=$(pct_decode "${BASH_REMATCH[1]}"); fi
		if [[ "$dsn" =~ [\?\&]port=([0-9]+) ]]; then pg_port=${BASH_REMATCH[1]}; fi
	else
		if [[ "$dsn" =~ (^|[[:space:]])host=([^[:space:]]+) ]]; then pg_host=${BASH_REMATCH[2]//\'/}; fi
		if [[ "$dsn" =~ (^|[[:space:]])port=([0-9]+) ]]; then pg_port=${BASH_REMATCH[2]}; fi
		if [[ "$dsn" =~ (^|[[:space:]])password=\'([^\']*)\' ]]; then
			pass=${BASH_REMATCH[2]}
		elif [[ "$dsn" =~ (^|[[:space:]])password=([^[:space:]]+) ]]; then
			pass=${BASH_REMATCH[2]}
		fi
	fi
	if [[ -n "$pass" ]]; then
		add_secret "$pass"
		add_secret "$(pct_decode "$pass")"
	fi
	return 0
}

redis_host=''
redis_port=6379
redis_user=''
redis_pass=''
redis_db=''
redis_tls=false
parse_redis_addr() {
	local addr=$1 scheme rest authority path userinfo hostport
	if [[ "$addr" == *://* ]]; then
		scheme=${addr%%://*}
		case "$scheme" in
			redis) ;;
			rediss) redis_tls=true ;;
			*) die_usage "unsupported REDIS_ADDR scheme: $scheme" ;;
		esac
		rest=${addr#*://}
		authority=${rest%%/*}
		path=${rest#"$authority"}
		path=${path#/}
		redis_db=${path%%\?*}
		hostport=$authority
		if [[ "$authority" == *@* ]]; then
			userinfo=${authority%@*}
			hostport=${authority##*@}
			redis_user=${userinfo%%:*}
			[[ "$userinfo" != *:* ]] || redis_pass=${userinfo#*:}
		fi
	else
		hostport=$addr
	fi
	if [[ "$hostport" =~ ^(\[[^]]+\]|[^:]+)(:([0-9]+))?$ ]]; then
		redis_host=${BASH_REMATCH[1]#[}
		redis_host=${redis_host%]}
		[[ -z "${BASH_REMATCH[3]}" ]] || redis_port=${BASH_REMATCH[3]}
	else
		die_usage 'cannot derive a Redis host and port from REDIS_ADDR'
	fi
	if [[ -n "$redis_pass" ]]; then
		add_secret "$redis_pass"
		add_secret "$(pct_decode "$redis_pass")"
	fi
	return 0
}

parse_redis_addr "$redis_addr"
redis_args=(-h "$redis_host" -p "$redis_port" --raw)
[[ -z "$redis_user" ]] || redis_args+=(--user "$redis_user")
[[ -z "$redis_db" ]] || redis_args+=(-n "$redis_db")
[[ "$redis_tls" != true ]] || redis_args+=(--tls)
redis_target="$redis_host:$redis_port"
unset redis_addr

pg_args=()
pg_target='custom check command'
pg_method='pg-check-cmd'
if [[ -z "$pg_check_cmd" ]]; then
	[[ -n "$pg_dsn" ]] || die_usage 'cannot derive the PostgreSQL host and port: set PG_DSN in the environment file or environment, or pass --pg-check-cmd'
	parse_pg_dsn "$pg_dsn"
	[[ -n "$pg_host" || -n "$pg_port" ]] || die_usage 'cannot derive a PostgreSQL host or port from PG_DSN; pass --pg-check-cmd'
	pg_args=(-t 5)
	[[ -z "$pg_host" ]] || pg_args+=(-h "$pg_host")
	[[ -z "$pg_port" ]] || pg_args+=(-p "$pg_port")
	pg_target="${pg_host:-local-socket}:${pg_port:-5432}"
	pg_method='pg_isready'
fi
unset pg_dsn

# ---------------------------------------------------------------------------
# Redaction. Every captured text passes through redact before it is stored.
# The patterns mirror staging-certification-evidence.sh (so the evidence
# collector's own pass is a no-op on this output) and add URL userinfo of any
# scheme, generic key=value credentials, JWTs, and the literal secrets above.
# ---------------------------------------------------------------------------
cred_keys='passw(or)?d|pwd|secret|token|api[_-]?key|access[_-]?key|private[_-]?key|credential|dsn'
redact() {
	sed -E \
		-e 's/\x1b\[[0-9;]*[A-Za-z]//g' \
		-e 's/(Bearer[[:space:]]+)[A-Za-z0-9._~+\/=-]+/\1[REDACTED]/g' \
		-e 's/AKIA[0-9A-Z]{16}/[REDACTED]/g' \
		-e 's/eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]*/[REDACTED]/g' \
		-e 's#([A-Za-z][A-Za-z0-9+.-]*://[^:/@[:space:]]*):[^@/[:space:]]+@#\1:[REDACTED]@#g' \
		-e 's/(authorization[[:space:]]*[=:][[:space:]]*).*/\1[REDACTED]/Ig' \
		-e "s/(\"[A-Za-z0-9_.-]*($cred_keys)[A-Za-z0-9_.-]*\"[[:space:]]*:[[:space:]]*)\"[^\"]*\"/\\1\"[REDACTED]\"/Ig" \
		-e "s/([A-Za-z0-9_.-]*($cred_keys)[A-Za-z0-9_.-]*[[:space:]]*[=:][[:space:]]*)\"[^\"]*\"/\\1\"[REDACTED]\"/Ig" \
		-e "s/([A-Za-z0-9_.-]*($cred_keys)[A-Za-z0-9_.-]*[[:space:]]*[=:][[:space:]]*)'[^']*'/\\1'[REDACTED]'/Ig" \
		-e "s/([A-Za-z0-9_.-]*($cred_keys)[A-Za-z0-9_.-]*[[:space:]]*[=:][[:space:]]*)[^,;[:space:]\"'&]+/\\1[REDACTED]/Ig" \
		-e 's/(AUTH[[:space:]]+)[^[:space:]]+/\1[REDACTED]/g' \
	| OBSERVE_LITERALS="$secret_literals" awk '
		function scrub(line, s,   res, p) {
			res = ""
			while ((p = index(line, s)) > 0) {
				res = res substr(line, 1, p - 1) "[REDACTED]"
				line = substr(line, p + length(s))
			}
			return res line
		}
		BEGIN { n = split(ENVIRON["OBSERVE_LITERALS"], lit, "\n") }
		{
			for (i = 1; i <= n; i++) if (lit[i] != "") $0 = scrub($0, lit[i])
			print
		}'
}

# ---------------------------------------------------------------------------
# Parsers. chi middleware.Logger (router.go) writes access lines such as
#   "GET http://host/path HTTP/1.1" from 127.0.0.1:54321 - 200 1234B in 1.234ms
# Go prints durations with ns, µs, ms, s, m, or h units.
# ---------------------------------------------------------------------------
access_regex='"[A-Z]+ [^" ]+ HTTP/[0-9.]+" from [^ ]+ - [0-9][0-9][0-9] [0-9]+B in [^ ]+'
# slog writes upper-case levels (JSON "level":"ERROR" or text level=ERROR); the
# asynq library logger prefixes " ERROR:"; the Go runtime prints panic:/fatal error:.
error_regex='"level"[[:space:]]*:[[:space:]]*"(ERROR|FATAL|PANIC)"|level=(ERROR|FATAL|PANIC)([[:space:]]|$)|[[:space:]]ERROR:|panic:|fatal error:'

# stdin: text; stdout: "status<TAB>latency_ms<TAB>is_healthz" per access line.
access_tsv() {
	sed -e 's/\xc2\xb5s/us/g' -e 's/\xce\xbcs/us/g' | awk -v re="$access_regex" '
		function to_ms(d,   total, num, unit, seen) {
			total = 0; seen = 0
			while (match(d, /^[0-9.]+(ns|us|ms|s|m|h)/)) {
				seg = substr(d, 1, RLENGTH); d = substr(d, RLENGTH + 1)
				if (match(seg, /(ns|us|ms|s|m|h)$/)) {
					unit = substr(seg, RSTART); num = substr(seg, 1, RSTART - 1) + 0
					if (unit == "ns") total += num / 1000000
					else if (unit == "us") total += num / 1000
					else if (unit == "ms") total += num
					else if (unit == "s") total += num * 1000
					else if (unit == "m") total += num * 60000
					else if (unit == "h") total += num * 3600000
					seen = 1
				}
			}
			if (!seen || d != "") return -1
			return total
		}
		match($0, re) {
			n = split(substr($0, RSTART, RLENGTH), f, " ")
			url = f[2]; status = f[7]; ms = to_ms(f[10])
			sub(/^[A-Za-z]+:\/\/[^\/]*/, "", url)
			health = (url == "/healthz" || index(url, "/healthz?") == 1) ? 1 : 0
			printf "%s\t%s\t%d\n", status, (ms < 0 ? "-" : sprintf("%.6f", ms)), health
		}'
}

# stdin: access_tsv output; stdout: "total c4xx c5xx healthz"
count_requests() {
	awk -F'\t' '
		{ total++; if ($1 >= 500 && $1 < 600) c5++; else if ($1 >= 400 && $1 < 500) c4++; if ($3 == 1) h++ }
		END { printf "%d %d %d %d\n", total, c4, c5, h }'
}

# stdin: ascending-sorted numbers; stdout: JSON {n,p50,p95,max}. Nearest-rank:
# the value at rank ceil(p/100 * n).
latency_stats_json() {
	awk '
		{ v[NR] = $1 + 0 }
		END {
			if (NR == 0) { print "{\"n\":0,\"p50\":null,\"p95\":null,\"max\":null}"; exit }
			r = 50 * NR / 100; k = int(r); if (k < r) k++; if (k < 1) k = 1; p50 = v[k]
			r = 95 * NR / 100; k = int(r); if (k < r) k++; if (k < 1) k = 1; p95 = v[k]
			printf "{\"n\":%d,\"p50\":%.3f,\"p95\":%.3f,\"max\":%.3f}\n", NR, p50, p95, v[NR]
		}'
}

# ---------------------------------------------------------------------------
# Preflight: fail fast, before the window starts, on problems that would
# silently produce an empty record (no journal access, wrong unit names).
# ---------------------------------------------------------------------------
tmp=$(mktemp -d)
# shellcheck disable=SC2329 # invoked through the EXIT trap
cleanup() { rm -rf -- "$tmp"; }
trap cleanup EXIT

for unit in "$app_unit" "$worker_unit"; do
	if ! show=$(systemctl show "$unit" -p LoadState --no-pager 2>&1) || [[ "$show" != *LoadState=loaded* ]]; then
		echo "preflight failed: systemd unit is not loaded: $unit" >&2
		exit 1
	fi
	if ! journalctl -u "$unit" -n 1 --no-pager -o cat >/dev/null 2>"$tmp/preflight-journal.err"; then
		echo "preflight failed: cannot read the journal for $unit" >&2
		exit 1
	fi
	if grep -qi 'not seeing messages' "$tmp/preflight-journal.err"; then
		echo "preflight failed: this user cannot read the system journal (run with sudo or join the systemd-journal group)" >&2
		exit 1
	fi
done

mkdir -p "$out/raw"
out=$(cd "$out" && pwd)
: >"$out/samples.jsonl"
: >"$out/observe.log"

log() {
	local line
	line="$(date -u '+%Y-%m-%dT%H:%M:%SZ') $*"
	printf '%s\n' "$line"
	printf '%s\n' "$line" >>"$out/observe.log"
}

# ---------------------------------------------------------------------------
# Sampling
# ---------------------------------------------------------------------------
queue_states=(pending active scheduled retry archived completed)
declare -A queue_cmd=([pending]=LLEN [active]=LLEN [scheduled]=ZCARD [retry]=ZCARD [archived]=ZCARD [completed]=ZCARD)
release_dir="$install_dir/releases"
cursor_app=''
cursor_worker=''
since_start=''
internal_failures=0

redis_run() {
	if [[ -n "$redis_pass" ]]; then
		REDISCLI_AUTH="$redis_pass" timeout 5 redis-cli "${redis_args[@]}" "$@" </dev/null
	else
		timeout 5 redis-cli "${redis_args[@]}" "$@" </dev/null
	fi
}

meminfo_mb() {
	awk -v key="$1:" '$1 == key { printf "%d", $2 / 1024 }' "$proc_dir/meminfo" 2>/dev/null || true
}

df_used_pct() {
	local listing
	listing=$(df -P -- "$1" 2>/dev/null) || return 0
	printf '%s\n' "$listing" | awk 'NR == 2 { gsub(/%/, "", $5); print $5 }'
}

# capture_journal ROLE UNIT -> sets j_ok and writes the filtered, redacted
# lines to $2dir. Access lines are kept only for the application unit.
capture_journal() {
	local role=$1 unit=$2 dir=$3 cursor new_cursor rc=0
	local -a args=(-u "$unit" --no-pager -a -o short-iso --utc --show-cursor)
	if [[ "$role" == app ]]; then cursor=$cursor_app; else cursor=$cursor_worker; fi
	if [[ -n "$cursor" ]]; then
		args+=(--after-cursor="$cursor")
	else
		args+=(--since "$since_start")
	fi
	journalctl "${args[@]}" 2>"$tmp/journal-$role.err" | redact >"$tmp/journal-$role.txt" || rc=$?
	if [[ -s "$tmp/journal-$role.err" ]]; then
		redact <"$tmp/journal-$role.err" >"$dir/journal-$role.stderr"
	fi
	j_ok=true
	if ((rc != 0)) || grep -qi 'not seeing messages' "$tmp/journal-$role.err"; then
		j_ok=false
	fi
	new_cursor=$(sed -n 's/^-- cursor: //p' "$tmp/journal-$role.txt" | tail -n 1)
	if [[ -n "$new_cursor" ]]; then
		if [[ "$role" == app ]]; then cursor_app=$new_cursor; else cursor_worker=$new_cursor; fi
	fi
	grep -Ev '^-- ' "$tmp/journal-$role.txt" >"$tmp/journal-$role.body" || true
	if [[ "$role" == app ]]; then
		# Query strings can carry tokens or personal data; request rate and
		# latency do not need them.
		grep -E "$access_regex" "$tmp/journal-$role.body" \
			| sed -E 's/("[A-Z]+ [^" ?]*)\?[^" ]*/\1?[QUERY-REDACTED]/' >"$dir/app-access.log" || true
		grep -E "$error_regex" "$tmp/journal-$role.body" | grep -Ev "$access_regex" >"$dir/app-errors.log" || true
	else
		grep -E "$error_regex" "$tmp/journal-$role.body" >"$dir/worker-errors.log" || true
	fi
	return 0
}

unit_state() {
	local unit=$1 dir=$2 role=$3 raw key value
	u_ok=false u_active='' u_sub='' u_restarts='' u_pid=''
	if raw=$(systemctl show "$unit" -p LoadState -p ActiveState -p SubState -p NRestarts -p MainPID --no-pager 2>&1); then
		u_ok=true
	fi
	printf '%s\n' "$raw" | redact >"$dir/systemd-$role.txt"
	while IFS='=' read -r key value; do
		case "$key" in
			ActiveState) u_active=$value ;;
			SubState) u_sub=$value ;;
			NRestarts) [[ "$value" =~ ^[0-9]+$ ]] && u_restarts=$value ;;
			MainPID) [[ "$value" =~ ^[0-9]+$ ]] && u_pid=$value ;;
		esac
	done <<<"$raw"
	[[ -n "$u_active" ]] || u_ok=false
	return 0
}

take_sample() {
	local seq=$1 epoch=$2 dir utc st val rc
	dir=$(printf '%s/raw/%04d' "$out" "$seq")
	mkdir -p "$dir"
	utc=$(date -u -d "@$epoch" '+%Y-%m-%dT%H:%M:%SZ')

	# --- health endpoint ---
	local hout hcode=000 htime='' hrc=0 hms='' hok=false
	hout=$(curl -sS --noproxy '*' --max-time 5 -o "$tmp/health-body" -w '%{http_code} %{time_total}' -- "$health_url" 2>"$tmp/health-err" </dev/null) || hrc=$?
	read -r hcode htime <<<"${hout:-000 0}" || true
	[[ "$hcode" =~ ^[0-9]{3}$ ]] || hcode=000
	[[ "$htime" =~ ^[0-9.]+$ ]] && hms=$(awk -v t="$htime" 'BEGIN { printf "%.3f", t * 1000 }')
	[[ "$hcode" == 200 && $hrc -eq 0 ]] && hok=true
	{
		printf 'GET %s\nhttp_code=%s\ntime_total_s=%s\ncurl_exit=%s\n' "$health_url" "$hcode" "${htime:-}" "$hrc"
		printf 'body='
		head -c 512 "$tmp/health-body" 2>/dev/null || true
		printf '\n'
		head -c 512 "$tmp/health-err" 2>/dev/null || true
	} | redact >"$dir/healthz.txt"
	: >"$tmp/health-body"

	# --- redis ping and asynq queue depth (read-only commands) ---
	local rok=false qok=false rping=''
	local -A qv=()
	for st in "${queue_states[@]}"; do qv[$st]=''; done
	local paused=''
	rping=$(redis_run PING 2>&1) && rc=0 || rc=$?
	printf 'PING => %s (exit %s)\n' "$rping" "$rc" | redact >"$dir/redis.txt"
	if ((rc == 0)) && [[ "$rping" == PONG ]]; then
		rok=true
		qok=true
		for st in "${queue_states[@]}"; do
			val=$(redis_run "${queue_cmd[$st]}" "asynq:{${queue_name}}:${st}" 2>&1) && rc=0 || rc=$?
			printf '%s asynq:{%s}:%s => %s (exit %s)\n' "${queue_cmd[$st]}" "$queue_name" "$st" "$val" "$rc" | redact >>"$dir/redis.txt"
			if ((rc == 0)) && [[ "$val" =~ ^[0-9]+$ ]]; then qv[$st]=$val; else qok=false; fi
		done
		val=$(redis_run EXISTS "asynq:{${queue_name}}:paused" 2>&1) && rc=0 || rc=$?
		printf 'EXISTS asynq:{%s}:paused => %s (exit %s)\n' "$queue_name" "$val" "$rc" | redact >>"$dir/redis.txt"
		if ((rc == 0)) && [[ "$val" =~ ^[0-9]+$ ]]; then paused=$val; else qok=false; fi
	fi

	# --- postgres readiness (no credentials involved) ---
	local pgout pgok=false
	if [[ -n "$pg_check_cmd" ]]; then
		pgout=$(timeout 30 bash -c "$pg_check_cmd" 2>&1 </dev/null) && pgok=true || true
	else
		pgout=$(timeout 10 pg_isready "${pg_args[@]}" 2>&1 </dev/null) && pgok=true || true
	fi
	printf '%s\nready=%s\n' "$pgout" "$pgok" | redact >"$dir/postgres.txt"

	# --- resource saturation ---
	local l1='' l5='' l15='' mem_avail mem_total droot drel
	read -r l1 l5 l15 _ 2>/dev/null <"$proc_dir/loadavg" || { l1=''; l5=''; l15=''; }
	mem_avail=$(meminfo_mb MemAvailable)
	mem_total=$(meminfo_mb MemTotal)
	droot=$(df_used_pct /)
	drel=$(df_used_pct "$release_dir")
	{
		printf 'loadavg=%s %s %s\nmem_available_mb=%s\nmem_total_mb=%s\n' "$l1" "$l5" "$l15" "$mem_avail" "$mem_total"
		df -P -- / "$release_dir" 2>&1 || true
	} >"$dir/resources.txt"

	# --- units and deployed release ---
	local app_ok app_active app_sub app_restarts app_pid wk_ok wk_active wk_sub wk_restarts wk_pid
	unit_state "$app_unit" "$dir" app
	app_ok=$u_ok app_active=$u_active app_sub=$u_sub app_restarts=$u_restarts app_pid=$u_pid
	unit_state "$worker_unit" "$dir" worker
	wk_ok=$u_ok wk_active=$u_active wk_sub=$u_sub wk_restarts=$u_restarts wk_pid=$u_pid
	local current='' current_real=''
	current_real=$(readlink -f -- "$install_dir/current" 2>/dev/null || true)
	[[ -z "$current_real" ]] || current=${current_real##*/}
	printf 'current -> %s\n' "$current_real" | redact >"$dir/release.txt"

	# --- journal (since the previous sample) ---
	local japp_ok jwk_ok
	capture_journal app "$app_unit" "$dir"
	japp_ok=$j_ok
	capture_journal worker "$worker_unit" "$dir"
	jwk_ok=$j_ok
	local rq_total=0 rq_c4=0 rq_c5=0 rq_health=0 err_app err_worker
	access_tsv <"$dir/app-access.log" >"$tmp/sample.tsv"
	read -r rq_total rq_c4 rq_c5 rq_health < <(count_requests <"$tmp/sample.tsv")
	err_app=$(wc -l <"$dir/app-errors.log" | tr -d '[:space:]')
	err_worker=$(wc -l <"$dir/worker-errors.log" | tr -d '[:space:]')

	jq -cn \
		--arg seq "$seq" --arg utc "$utc" --arg epoch "$epoch" \
		--arg h_code "$hcode" --arg h_ms "$hms" --arg h_rc "$hrc" --argjson h_ok "$hok" \
		--argjson r_ok "$rok" --argjson q_ok "$qok" --arg q_paused "$paused" \
		--arg q_pending "${qv[pending]}" --arg q_active "${qv[active]}" --arg q_scheduled "${qv[scheduled]}" \
		--arg q_retry "${qv[retry]}" --arg q_archived "${qv[archived]}" --arg q_completed "${qv[completed]}" \
		--argjson pg_ok "$pgok" \
		--arg l1 "$l1" --arg l5 "$l5" --arg l15 "$l15" --arg mem_avail "$mem_avail" --arg mem_total "$mem_total" \
		--arg d_root "$droot" --arg d_rel "$drel" --arg current "$current" \
		--argjson app_ok "$app_ok" --arg app_active "$app_active" --arg app_sub "$app_sub" --arg app_restarts "$app_restarts" --arg app_pid "$app_pid" \
		--argjson wk_ok "$wk_ok" --arg wk_active "$wk_active" --arg wk_sub "$wk_sub" --arg wk_restarts "$wk_restarts" --arg wk_pid "$wk_pid" \
		--argjson japp_ok "$japp_ok" --argjson jwk_ok "$jwk_ok" \
		--arg rq_total "$rq_total" --arg rq_c4 "$rq_c4" --arg rq_c5 "$rq_c5" --arg rq_health "$rq_health" \
		--arg err_app "$err_app" --arg err_worker "$err_worker" \
		'def n: if . == "" then null else tonumber end;
		 def s: if . == "" then null else . end;
		 {
		   seq: ($seq | tonumber), utc: $utc, epoch: ($epoch | tonumber),
		   health: {ok: $h_ok, http_code: ($h_code | tonumber), time_ms: ($h_ms | n), curl_exit: ($h_rc | tonumber)},
		   redis: {ok: $r_ok, queue_ok: $q_ok,
		     queue: {pending: ($q_pending | n), active: ($q_active | n), scheduled: ($q_scheduled | n),
		             retry: ($q_retry | n), archived: ($q_archived | n), completed: ($q_completed | n)},
		     paused: ($q_paused | n)},
		   postgres: {ok: $pg_ok},
		   load: {load1: ($l1 | n), load5: ($l5 | n), load15: ($l15 | n)},
		   memory: {available_mb: ($mem_avail | n), total_mb: ($mem_total | n)},
		   disk: {root_used_pct: ($d_root | n), release_used_pct: ($d_rel | n)},
		   release: {current: ($current | s)},
		   units: {
		     app: {ok: $app_ok, active_state: ($app_active | s), sub_state: ($app_sub | s), n_restarts: ($app_restarts | n), main_pid: ($app_pid | n)},
		     worker: {ok: $wk_ok, active_state: ($wk_active | s), sub_state: ($wk_sub | s), n_restarts: ($wk_restarts | n), main_pid: ($wk_pid | n)}
		   },
		   journal: {app_ok: $japp_ok, worker_ok: $jwk_ok},
		   requests: {total: ($rq_total | tonumber), c4xx: ($rq_c4 | tonumber), c5xx: ($rq_c5 | tonumber), healthz: ($rq_health | tonumber)},
		   error_log_lines: {app: ($err_app | tonumber), worker: ($err_worker | tonumber)}
		 }' >>"$out/samples.jsonl"

	log "sample $seq: health=$hcode redis=$([[ $rok == true ]] && echo ok || echo FAIL) postgres=$([[ $pgok == true ]] && echo ok || echo FAIL) queue(pending/active/retry/archived)=${qv[pending]:--}/${qv[active]:--}/${qv[retry]:--}/${qv[archived]:--} requests=$rq_total 5xx=$rq_c5 app=${app_active:-?} worker=${wk_active:-?}"
	return 0
}

now_epoch() { date -u +%s; }

interrupted_by=''
sleep_pid=''
# shellcheck disable=SC2329 # invoked through the signal traps
on_signal() {
	interrupted_by=$1
	[[ -z "$sleep_pid" ]] || kill "$sleep_pid" 2>/dev/null || true
}
trap 'on_signal SIGINT' INT
trap 'on_signal SIGTERM' TERM
trap 'on_signal SIGHUP' HUP

start_epoch=$(now_epoch)
utc_start=$(date -u -d "@$start_epoch" '+%Y-%m-%dT%H:%M:%SZ')
since_start=$(date -u -d "@$start_epoch" '+%Y-%m-%d %H:%M:%S UTC')
log "OPS-005 observation started: duration=${duration}s interval=${interval}s rehearsal=$rehearsal handoff_owner=$handoff_owner"

seq=0
while :; do
	seq=$((seq + 1))
	sample_epoch=$(now_epoch)
	if ! take_sample "$seq" "$sample_epoch"; then
		internal_failures=$((internal_failures + 1))
		log "sample $seq: internal capture failure"
	fi
	[[ -z "$interrupted_by" ]] || break
	now_e=$(now_epoch)
	elapsed=$((now_e - start_epoch))
	((elapsed < duration)) || break
	wait_secs=$((interval - elapsed % interval))
	remaining=$((duration - elapsed))
	((wait_secs <= remaining)) || wait_secs=$remaining
	sleep "$wait_secs" &
	sleep_pid=$!
	wait "$sleep_pid" || true
	sleep_pid=''
	[[ -z "$interrupted_by" ]] || break
done

end_epoch=$(now_epoch)
utc_end=$(date -u -d "@$end_epoch" '+%Y-%m-%dT%H:%M:%SZ')
elapsed_total=$((end_epoch - start_epoch))
if [[ -n "$interrupted_by" ]]; then
	log "OPS-005 observation interrupted by $interrupted_by after ${elapsed_total}s"
fi

# ---------------------------------------------------------------------------
# Operator-supplied alerts and incidents: copied (redacted) at the end so they
# can be filled in during the window. Absent files are "not supplied".
# ---------------------------------------------------------------------------
supplied_json() {
	local source=$1 name=$2
	if [[ -z "$source" ]]; then
		jq -cn '{status: "not supplied"}'
		return 0
	fi
	redact <"$source" >"$out/$name"
	jq -cn --arg file "$name" --argjson lines "$(wc -l <"$out/$name" | tr -d '[:space:]')" \
		--arg sha256 "$(sha256sum "$out/$name" | awk '{print $1}')" \
		'{status: "supplied", file: $file, lines: $lines, sha256: $sha256}'
}
alerts_json=$(supplied_json "$alerts_file" alerts.txt)
incidents_json=$(supplied_json "$incidents_file" incidents.txt)

# ---------------------------------------------------------------------------
# Summary. Requests and latency are re-derived from the stored raw access
# lines so the reviewer can reproduce every figure from the bundle.
# ---------------------------------------------------------------------------
find "$out/raw" -type f -name app-access.log -print0 | sort -z | xargs -0 -r cat | access_tsv >"$tmp/all.tsv"
read -r req_total req_c4 req_c5 req_health < <(count_requests <"$tmp/all.tsv")
{ cut -f2 "$tmp/all.tsv" | grep -v '^-$' || true; } | sort -g | latency_stats_json >"$tmp/latency.json"
jq -r 'select(.health.ok) | .health.time_ms | select(. != null)' "$out/samples.jsonl" | sort -g | latency_stats_json >"$tmp/health-latency.json"
req_json=$(jq -cn --argjson total "$req_total" --argjson c4 "$req_c4" --argjson c5 "$req_c5" --argjson healthz "$req_health" \
	--slurpfile latency "$tmp/latency.json" --slurpfile health_latency "$tmp/health-latency.json" \
	'{total: $total, c4xx: $c4, c5xx: $c5, healthz: $healthz, latency: $latency[0], health_latency: $health_latency[0]}')

read -r -d '' summary_filter <<'JQ' || true
def stat(f):
  [ .[] | f | select(. != null) ] as $v
  | {first: $v[0],
     min: (if ($v | length) > 0 then ($v | min) else null end),
     max: (if ($v | length) > 0 then ($v | max) else null end),
     last: $v[-1]};
def unit_summary($role; $unit):
  [ .[] | .units[$role] ] as $u
  | [ $u[] | .n_restarts | select(. != null) ] as $nr
  | [ $u[] | .main_pid | select(. != null and . > 0) ] as $pids
  | {
      unit: $unit,
      n_restarts_first: $nr[0],
      n_restarts_last: $nr[-1],
      delta: (if ($nr | length) > 0 then ($nr[-1] - $nr[0]) else null end),
      main_pid_changes: ([ range(1; ($pids | length)) | select($pids[.] != $pids[. - 1]) ] | length),
      not_active_samples: ([ $u[] | select(.ok and .active_state != "active") ] | length),
      capture_failed_samples: ([ $u[] | select(.ok | not) ] | length),
      last_active_state: ([ $u[] | .active_state | select(. != null) ] | .[-1]),
      last_sub_state: ([ $u[] | .sub_state | select(. != null) ] | .[-1])
    };
def first_seq(f): [ .[] | select(f) | .seq ] | .[0];
def count(f): [ .[] | select(f) ] | length;
def round4: . * 10000 | round / 10000;

. as $s
| ($s | length) as $n
| ($req.total) as $rt
| ({app: ([ .[] | .error_log_lines.app ] | add // 0), worker: ([ .[] | .error_log_lines.worker ] | add // 0)}) as $errs
| (if $rt > 0 then ($req.c5xx / $rt) else null end) as $rate
| ($s | stat(.load.load1)) as $load1
| ($s | stat(.memory.available_mb)) as $mem
| ($s | stat(.disk.root_used_pct)) as $droot
| ($s | stat(.disk.release_used_pct)) as $drel
| ([ $droot.max, $drel.max ] | map(select(. != null)) | if length > 0 then max else null end) as $dmax
| ($s | unit_summary("app"; $meta.app_unit)) as $app
| ($s | unit_summary("worker"; $meta.worker_unit)) as $worker
| ($s | stat(.redis.queue.archived)) as $archived
| ([ $s[] | .release.current | select(. != null) ]) as $rels
| ([ range(1; $n) | select($s[.].epoch - $s[. - 1].epoch > 2 * $meta.interval_seconds) ]) as $gaps
| ([ range(1; $n) | ($s[.].epoch - $s[. - 1].epoch) ] | max) as $max_gap
| {
    queue: ({name: $meta.queue_name}
      + (reduce ["pending", "active", "scheduled", "retry", "archived", "completed"][] as $k ({}; . + {($k): ($s | stat(.redis.queue[$k]))}))
      + {paused: ($s | stat(.redis.paused))}),
    release: {first: $rels[0], last: $rels[-1], changes: ([ range(1; ($rels | length)) | select($rels[.] != $rels[. - 1]) ] | length)}
  } as $derived
| ([
    (if ($meta.rehearsal) then "rehearsal run" else empty end),
    (if ($meta.completed | not) then "observation window did not complete (interrupted by \($meta.interrupted_by))" else empty end),
    (if $meta.duration_seconds < $meta.min_seconds then "observed window \($meta.duration_seconds)s is shorter than the required \($meta.min_seconds)s" else empty end),
    (if $n < 2 then "fewer than two samples were captured" else empty end)
  ]) as $reasons
| ([
    ((count(.health.ok | not)) as $f | if $f > 0 then "health endpoint was not 200 in \($f) of \($n) samples (first at sample \(first_seq(.health.ok | not)))" else empty end),
    ((count(.redis.ok | not)) as $f | if $f > 0 then "redis PING failed in \($f) of \($n) samples (first at sample \(first_seq(.redis.ok | not)))" else empty end),
    ((count(.redis.ok and (.redis.queue_ok | not))) as $f | if $f > 0 then "queue depth capture failed in \($f) of \($n) samples" else empty end),
    ((count(.postgres.ok | not)) as $f | if $f > 0 then "postgres readiness failed in \($f) of \($n) samples (first at sample \(first_seq(.postgres.ok | not)))" else empty end),
    ((count(.journal.app_ok | not)) as $f | if $f > 0 then "journal capture for the app unit failed in \($f) samples; request and error figures are incomplete" else empty end),
    ((count(.journal.worker_ok | not)) as $f | if $f > 0 then "journal capture for the worker unit failed in \($f) samples; error figures are incomplete" else empty end),
    (if $app.capture_failed_samples > 0 then "systemctl show failed for \($app.unit) in \($app.capture_failed_samples) samples" else empty end),
    (if $worker.capture_failed_samples > 0 then "systemctl show failed for \($worker.unit) in \($worker.capture_failed_samples) samples" else empty end),
    (if $app.not_active_samples > 0 then "\($app.unit) was not active in \($app.not_active_samples) samples (last ActiveState=\($app.last_active_state))" else empty end),
    (if $worker.not_active_samples > 0 then "\($worker.unit) was not active in \($worker.not_active_samples) samples (last ActiveState=\($worker.last_active_state))" else empty end),
    (if $app.delta != null and $app.delta != 0 then "\($app.unit) NRestarts changed by \($app.delta) (\($app.n_restarts_first) -> \($app.n_restarts_last))" else empty end),
    (if $worker.delta != null and $worker.delta != 0 then "\($worker.unit) NRestarts changed by \($worker.delta) (\($worker.n_restarts_first) -> \($worker.n_restarts_last))" else empty end),
    (if $app.main_pid_changes > 0 then "\($app.unit) MainPID changed \($app.main_pid_changes) time(s); the service restarted during the window" else empty end),
    (if $worker.main_pid_changes > 0 then "\($worker.unit) MainPID changed \($worker.main_pid_changes) time(s); the service restarted during the window" else empty end),
    (if $archived.max != null and $archived.first != null and $archived.max > $archived.first then "queue archived count increased from \($archived.first) to \($archived.max)" else empty end),
    (if $rate != null and $rate > $meta.max_error_rate then "5xx error rate \($rate | round4) (\($req.c5xx)/\($rt)) exceeds the threshold \($meta.max_error_rate)" else empty end),
    (if $rt - $req.healthz == 0 then "no application requests other than /healthz were logged; error-rate and latency figures reflect health probes only" else empty end),
    (if $errs.app > 0 then "app logged \($errs.app) error-level line(s)" else empty end),
    (if $errs.worker > 0 then "worker logged \($errs.worker) error-level line(s)" else empty end),
    (if ($gaps | length) > 0 then "\($gaps | length) sampling gap(s) exceeded twice the interval (largest \($max_gap)s)" else empty end),
    (if $load1.max == null then "load average was not captured" else empty end),
    (if $mem.min == null then "memory availability was not captured" else empty end),
    (if $dmax == null then "disk usage was not captured" else empty end),
    (if $derived.release.changes > 0 then "the current release symlink changed \($derived.release.changes) time(s) during the window" else empty end),
    (if $derived.release.last == null then "the current release symlink could not be resolved" else empty end),
    (if $meta.expected_sha != "not supplied" and $derived.release.last != null
        and ($derived.release.last | test("^[0-9a-f]{7,40}$"))
        and ($meta.expected_sha | startswith($derived.release.last) | not)
      then "current release \($derived.release.last) is not a prefix of the expected sha" else empty end),
    (if $alerts.status != "supplied" then "alerts not supplied; record the alert console state separately" else empty end),
    (if $incidents.status != "supplied" then "incidents not supplied; record the incident log separately" else empty end),
    (if $meta.internal_failures > 0 then "collector failed internally during \($meta.internal_failures) sample(s)" else empty end)
  ]) as $anomalies
| {
    schema_version: $meta.schema_version,
    evidence_id: "OPS-005",
    note: "Observations only. PASS or FAIL is decided by the reviewer.",
    completed: $meta.completed,
    interrupted_by: $meta.interrupted_by,
    eligible_for_ops005: ($reasons | length == 0),
    ineligible_reasons: $reasons,
    rehearsal: $meta.rehearsal,
    utc_start: $meta.utc_start,
    utc_end: $meta.utc_end,
    duration_seconds: $meta.duration_seconds,
    requested_duration_seconds: $meta.requested_duration_seconds,
    interval_seconds: $meta.interval_seconds,
    sample_count: $n,
    host: $meta.host,
    candidate_tag: $meta.candidate_tag,
    expected_sha: $meta.expected_sha,
    handoff_owner: $meta.handoff_owner,
    requests_total: $rt,
    requests_5xx: $req.c5xx,
    requests_4xx: $req.c4xx,
    requests_healthz: $req.healthz,
    error_rate_5xx: $rate,
    max_error_rate: $meta.max_error_rate,
    latency_ms: ($req.latency | {p50, p95, max, samples: .n, method: "nearest-rank over access-log durations, health probes included"}),
    health: {ok: count(.health.ok), fail: count(.health.ok | not),
             latency_ms: ($req.health_latency | {p50, p95, max, samples: .n})},
    redis: {ok: count(.redis.ok), fail: count(.redis.ok | not), target: $meta.redis_target},
    postgres: {ok: count(.postgres.ok), fail: count(.postgres.ok | not), target: $meta.postgres_target, method: $meta.postgres_method},
    queue: $derived.queue,
    saturation: {
      max_load1: $load1.max,
      min_mem_available_mb: $mem.min,
      max_disk_used_pct: $dmax,
      max_disk_used_pct_root: $droot.max,
      max_disk_used_pct_release_dir: $drel.max,
      cpu_count: $meta.cpu_count
    },
    unit_restarts: {app: $app, worker: $worker},
    error_log_lines: $errs,
    release: $derived.release,
    alerts: $alerts,
    incidents: $incidents,
    anomalies: $anomalies,
    collector: {script: "scripts/staging-certification-observe.sh", internal_failures: $meta.internal_failures}
  }
JQ

cpu_count=$(nproc 2>/dev/null || true)
completed=true
[[ -z "$interrupted_by" ]] || completed=false

meta_json=$(jq -cn \
	--arg schema_version "$SCHEMA_VERSION" --argjson completed "$completed" --arg interrupted_by "$interrupted_by" \
	--argjson rehearsal "$rehearsal" --arg utc_start "$utc_start" --arg utc_end "$utc_end" \
	--argjson duration_seconds "$elapsed_total" --argjson requested "$duration" --argjson interval "$interval" \
	--argjson min_seconds "$MIN_OPS005_SECONDS" --arg host "$(uname -n 2>/dev/null || echo unknown)" \
	--arg candidate_tag "${candidate_tag:-not supplied}" --arg expected_sha "${expected_sha:-not supplied}" \
	--arg handoff_owner "$handoff_owner" --argjson max_error_rate "$max_error_rate" \
	--arg redis_target "$redis_target" --arg postgres_target "$pg_target" --arg postgres_method "$pg_method" \
	--arg queue_name "$queue_name" --arg app_unit "$app_unit" --arg worker_unit "$worker_unit" \
	--arg cpu_count "$cpu_count" --argjson internal_failures "$internal_failures" \
	'{schema_version: $schema_version, completed: $completed,
	  interrupted_by: (if $interrupted_by == "" then null else $interrupted_by end),
	  rehearsal: $rehearsal, utc_start: $utc_start, utc_end: $utc_end,
	  duration_seconds: $duration_seconds, requested_duration_seconds: $requested, interval_seconds: $interval,
	  min_seconds: $min_seconds, host: $host, candidate_tag: $candidate_tag, expected_sha: $expected_sha,
	  handoff_owner: $handoff_owner, max_error_rate: $max_error_rate, redis_target: $redis_target,
	  postgres_target: $postgres_target, postgres_method: $postgres_method, queue_name: $queue_name,
	  app_unit: $app_unit, worker_unit: $worker_unit,
	  cpu_count: (if $cpu_count == "" then null else ($cpu_count | tonumber) end),
	  internal_failures: $internal_failures}')

jq -s \
	--argjson meta "$meta_json" --argjson req "$req_json" \
	--argjson alerts "$alerts_json" --argjson incidents "$incidents_json" \
	"$summary_filter" "$out/samples.jsonl" >"$out/summary.json"

# Manifest of every file, excluding itself (same form as the evidence collector).
(cd "$out" && find . -type f ! -name SHA256SUMS -print0 | sort -z | xargs -0 -r sha256sum) >"$out/SHA256SUMS"

anomaly_count=$(jq '.anomalies | length' "$out/summary.json")
eligible=$(jq -r '.eligible_for_ops005' "$out/summary.json")
printf 'OPS-005 observation record: %s\n' "$out"
printf 'completed=%s eligible_for_ops005=%s samples=%s anomalies=%s\n' "$completed" "$eligible" "$(jq '.sample_count' "$out/summary.json")" "$anomaly_count"

case "$interrupted_by" in
	SIGINT) exit 130 ;;
	SIGTERM) exit 143 ;;
	SIGHUP) exit 129 ;;
esac
exit 0
