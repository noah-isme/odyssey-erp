#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
observer="$script_dir/staging-certification-observe.sh"
collector="$script_dir/staging-certification-evidence.sh"
contract="$script_dir/staging-certification-contract.json"
command -v jq >/dev/null 2>&1 || { echo 'jq is required' >&2; exit 1; }
real_date=$(command -v date)

tmp=$(mktemp -d)
trap 'rm -rf -- "$tmp"' EXIT

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

expect_jq() {
	local file=$1 filter=$2 message=$3
	if ! jq -e "$filter" "$file" >/dev/null 2>&1; then
		echo "FAIL: $message" >&2
		echo "--- $file ($filter)" >&2
		jq . "$file" >&2 2>/dev/null || cat "$file" >&2
		exit 1
	fi
}

mu=$'\xc2\xb5'
candidate_sha=20cc13a0f028e3b09573944bb9f7a1f943461253
secret_pg_password=s3cr3tpw
secret_redis_password=redispw42
secret_text_password=hunter2
secret_jwt=eyJhbGciOiJIUzI1NiJ9

# ---------------------------------------------------------------------------
# Fake staging host: PATH shims for every external command the collector runs.
# The fake clock lives in $SHIM_STATE/epoch and advances only when the fake
# sleep is called, so a 60-minute window runs in seconds. $SHIM_STATE/tick
# counts sleeps; each shim serves the per-tick file <name>.<tick> when present
# and <name>.default otherwise.
# ---------------------------------------------------------------------------
shim_dir="$tmp/bin"
mkdir -p "$shim_dir"

cat >"$shim_dir/date" <<'EOF'
#!/usr/bin/env bash
args=("$@")
for a in "${args[@]}"; do
	case "$a" in -d|--date|--date=*) exec "$REAL_DATE" "${args[@]}" ;; esac
done
exec "$REAL_DATE" -d "@$(cat "$SHIM_STATE/epoch")" "${args[@]}"
EOF

cat >"$shim_dir/sleep" <<'EOF'
#!/usr/bin/env bash
state=$SHIM_STATE
count=$(( $(cat "$state/sleepcount") + 1 ))
echo "$count" >"$state/sleepcount"
if [[ -n "${SHIM_SLEEP_BLOCK_AFTER:-}" && $count -gt $SHIM_SLEEP_BLOCK_AFTER ]]; then
	touch "$state/blocking"
	exec /usr/bin/sleep 30
fi
tick=$(( $(cat "$state/tick") + 1 ))
echo "$tick" >"$state/tick"
echo $(( $(cat "$state/epoch") + ${1%%.*} )) >"$state/epoch"
for f in loadavg meminfo; do
	if [[ -f "$state/proc-scenario/$f.$tick" ]]; then cp "$state/proc-scenario/$f.$tick" "$state/proc/$f"; fi
done
EOF

cat >"$shim_dir/curl" <<'EOF'
#!/usr/bin/env bash
state=$SHIM_STATE
printf '%s\n' "$*" >>"$state/curl.calls"
tick=$(<"$state/tick")
f=$state/curl.$tick
[[ -f "$f" ]] || f=$state/curl.default
read -r code time rc <"$f"
body='{"status":"ok"}'
[[ "$code" == 200 ]] || body='{"status":"unavailable"}'
outfile='' write=''
args=("$@")
for ((i = 0; i < ${#args[@]}; i++)); do
	case "${args[i]}" in -o) outfile=${args[i+1]} ;; -w) write=${args[i+1]} ;; esac
done
[[ -z "$outfile" ]] || printf '%s' "$body" >"$outfile"
[[ "${rc:-0}" -eq 0 ]] || echo 'curl: (7) Failed to connect' >&2
[[ -z "$write" ]] || printf '%s %s' "$code" "$time"
exit "${rc:-0}"
EOF

cat >"$shim_dir/journalctl" <<'EOF'
#!/usr/bin/env bash
state=$SHIM_STATE
unit='' preflight=false
args=("$@")
for ((i = 0; i < ${#args[@]}; i++)); do
	case "${args[i]}" in -u) unit=${args[i+1]} ;; -n) preflight=true ;; esac
done
role=app
[[ "$unit" == *worker* ]] && role=worker
printf '%s %s\n' "$role" "$*" >>"$state/journal.calls"
tick=$(<"$state/tick")
if $preflight; then
	[[ ! -f "$state/journal.hint" ]] || echo 'Hint: You are currently not seeing messages from other users and the system.' >&2
	exit 0
fi
if [[ -f "$state/journal.fail.$role" ]]; then echo 'journalctl: simulated failure' >&2; exit 1; fi
f=$state/journal.$role.$tick
[[ -f "$f" ]] || f=$state/journal.$role.default
if [[ -s "$f" ]]; then
	cat "$f"
	echo "-- cursor: s=fake$tick;i=$tick"
else
	echo '-- No entries --'
fi
EOF

cat >"$shim_dir/redis-cli" <<'EOF'
#!/usr/bin/env bash
state=$SHIM_STATE
printf '%s\n' "$*" >>"$state/redis.argv"
printf '%s\n' "${REDISCLI_AUTH-<unset>}" >>"$state/redis.auth"
tick=$(<"$state/tick")
args=("$@")
cmd='' key=''
for ((i = 0; i < ${#args[@]}; i++)); do
	case "${args[i]}" in
		-h|-p|--user|-n) i=$((i + 1)) ;;
		--raw|--tls) ;;
		*) if [[ -z "$cmd" ]]; then cmd=${args[i]}; elif [[ -z "$key" ]]; then key=${args[i]}; fi ;;
	esac
done
printf '%s %s\n' "$cmd" "$key" >>"$state/redis.calls"
if [[ -f "$state/redis.fail.$tick" || -f "$state/redis.down" ]]; then
	echo 'Could not connect to Redis at redis-host:6380: Connection refused' >&2
	exit 1
fi
case "$cmd" in
	PING) echo PONG ;;
	LLEN|ZCARD|EXISTS)
		name=${key##*:}
		f=$state/queue.$name.$tick
		[[ -f "$f" ]] || f=$state/queue.$name.default
		if [[ -f "$f" ]]; then cat "$f"; else echo 0; fi
		;;
	*) echo "unexpected redis command: $cmd" >&2; exit 64 ;;
esac
EOF

cat >"$shim_dir/pg_isready" <<'EOF'
#!/usr/bin/env bash
state=$SHIM_STATE
printf '%s\n' "$*" >>"$state/pg.argv"
tick=$(<"$state/tick")
f=$state/pg.$tick
[[ -f "$f" ]] || f=$state/pg.default
rc=$(<"$f")
if [[ -f "$state/pg.message" ]]; then cat "$state/pg.message"; else echo 'db-host:5432 - accepting connections'; fi
exit "$rc"
EOF

cat >"$shim_dir/systemctl" <<'EOF'
#!/usr/bin/env bash
state=$SHIM_STATE
printf '%s\n' "$*" >>"$state/systemctl.calls"
[[ "$1" == show ]] || { echo "unexpected systemctl subcommand: $1" >&2; exit 64; }
role=app
[[ "$2" == *worker* ]] && role=worker
tick=$(<"$state/tick")
f=$state/systemctl.$role.$tick
[[ -f "$f" ]] || f=$state/systemctl.$role.default
cat "$f"
EOF

cat >"$shim_dir/df" <<'EOF'
#!/usr/bin/env bash
state=$SHIM_STATE
tick=$(<"$state/tick")
echo 'Filesystem     1024-blocks      Used Available Capacity Mounted on'
for arg in "$@"; do
	case "$arg" in -*) continue ;; esac
	name=release
	[[ "$arg" == / ]] && name=root
	f=$state/df.$name.$tick
	[[ -f "$f" ]] || f=$state/df.$name.default
	cat "$f"
done
EOF
chmod +x "$shim_dir"/*

# A staging install tree with the environment file the collector reads.
install="$tmp/install"
mkdir -p "$install/releases/20cc13a"
ln -s releases/20cc13a "$install/current"
cat >"$install/.env" <<EOF
APP_ENV=staging
PG_DSN=postgres://odyssey_staging:${secret_pg_password}@db-host:5432/odyssey_staging?sslmode=require
REDIS_ADDR=redis://:${secret_redis_password}@redis-host:6380/2
EOF

new_state() {
	local d="$tmp/state-$1"
	mkdir -p "$d/proc" "$d/proc-scenario"
	echo 1790000000 >"$d/epoch"
	echo 0 >"$d/tick"
	echo 0 >"$d/sleepcount"
	printf '200 0.004 0\n' >"$d/curl.default"
	printf 'LoadState=loaded\nActiveState=active\nSubState=running\nNRestarts=0\nMainPID=1111\n' >"$d/systemctl.app.default"
	printf 'LoadState=loaded\nActiveState=active\nSubState=running\nNRestarts=0\nMainPID=2222\n' >"$d/systemctl.worker.default"
	echo 0 >"$d/pg.default"
	echo '/dev/vda1 41152812 20576406 20576406 50% /' >"$d/df.root.default"
	echo '/dev/vdb1 41152812 16461124 24691688 40% /opt' >"$d/df.release.default"
	echo '0.50 0.40 0.30 1/200 12345' >"$d/proc/loadavg"
	printf 'MemTotal:        4096000 kB\nMemAvailable:    2048000 kB\n' >"$d/proc/meminfo"
	echo "$d"
}

run_rc=0
# run_observe STATE OUT [extra flags...]: runs the collector against the fake host.
run_observe() {
	local st=$1 o=$2
	shift 2
	SHIM_STATE=$st REAL_DATE=$real_date OBSERVE_PROC_DIR=$st/proc PATH="$shim_dir:$PATH" \
		"$observer" --out "$o" --handoff-owner 'Test Operator' --confirm-staging --install-dir "$install" "$@" \
		>"$st/stdout.txt" 2>"$st/stderr.txt" && run_rc=0 || run_rc=$?
}

# acc STATUS DURATION [PATH] [PREFIX]: one chi access line as journald shows it.
acc() {
	printf '2026-09-21T13:33:21+0000 stg odyssey-staging[1111]: 2026/09/21 13:33:21 %s"GET http://127.0.0.1:8180%s HTTP/1.1" from 127.0.0.1:54321 - %s 1234B in %s\n' \
		"${4:-}" "${3:-/dashboard}" "$1" "$2"
}

# ---------------------------------------------------------------------------
# 1. Usage errors never start a window.
# ---------------------------------------------------------------------------
st=$(new_state usage)
run_observe "$st" "$tmp/out-usage-short" --duration 120
[[ $run_rc -eq 2 ]] || fail "duration below 3600 without --rehearsal must exit 2, got $run_rc"
grep -q -- '--rehearsal' "$st/stderr.txt" || fail 'short duration rejection must mention --rehearsal'
[[ ! -e "$tmp/out-usage-short/summary.json" ]] || fail 'rejected run wrote a summary'

for missing in handoff confirm out; do
	args=(--out "$tmp/out-usage-$missing" --handoff-owner 'Test Operator' --confirm-staging --install-dir "$install" --rehearsal --duration 60 --interval 60)
	case "$missing" in
		handoff) args=(--out "$tmp/out-usage-$missing" --confirm-staging --install-dir "$install" --rehearsal --duration 60 --interval 60) ;;
		confirm) args=(--out "$tmp/out-usage-$missing" --handoff-owner 'Test Operator' --install-dir "$install" --rehearsal --duration 60 --interval 60) ;;
		out) args=(--handoff-owner 'Test Operator' --confirm-staging --install-dir "$install" --rehearsal --duration 60 --interval 60) ;;
	esac
	if SHIM_STATE=$st REAL_DATE=$real_date PATH="$shim_dir:$PATH" "$observer" "${args[@]}" >"$st/stdout.txt" 2>"$st/stderr.txt"; then
		fail "missing $missing must be rejected"
	else
		rc=$?
		[[ $rc -eq 2 ]] || fail "missing $missing must exit 2, got $rc"
	fi
done

mkdir -p "$tmp/out-nonempty"
echo keep >"$tmp/out-nonempty/existing"
run_observe "$st" "$tmp/out-nonempty" --rehearsal --duration 60 --interval 60
[[ $run_rc -eq 2 ]] || fail "non-empty output directory must exit 2, got $run_rc"

run_observe "$st" "$tmp/out-usage-interval" --rehearsal --duration 60 --interval 120
[[ $run_rc -eq 2 ]] || fail "interval above duration must exit 2, got $run_rc"

run_observe "$st" "$tmp/out-usage-sha" --rehearsal --duration 60 --interval 60 --expected-sha 20cc13a
[[ $run_rc -eq 2 ]] || fail "short --expected-sha must exit 2, got $run_rc"

prod_install="$tmp/prod-install"
mkdir -p "$prod_install"
printf 'APP_ENV=production\nPG_DSN=postgres://u:p@db:5432/x\n' >"$prod_install/.env"
run_observe "$st" "$tmp/out-prod" --install-dir "$prod_install" --rehearsal --duration 60 --interval 60
[[ $run_rc -eq 2 ]] || fail "APP_ENV=production must be refused with exit 2, got $run_rc"
grep -q 'staging only' "$st/stderr.txt" || fail 'production refusal must say staging only'

run_observe "$st" "$tmp/out-prod-unit" --app-unit odyssey-production.service --rehearsal --duration 60 --interval 60
[[ $run_rc -eq 2 ]] || fail "a prod-named unit must be refused with exit 2, got $run_rc"

# ---------------------------------------------------------------------------
# 2. Preflight: no journal access or a missing unit stops before sampling.
# ---------------------------------------------------------------------------
st=$(new_state preflight-journal)
touch "$st/journal.hint"
run_observe "$st" "$tmp/out-preflight-journal" --rehearsal --duration 60 --interval 60
[[ $run_rc -eq 1 ]] || fail "unreadable journal must exit 1, got $run_rc"
grep -q 'cannot read the system journal' "$st/stderr.txt" || fail 'journal preflight message missing'

st=$(new_state preflight-unit)
printf 'LoadState=not-found\nActiveState=inactive\nSubState=dead\n' >"$st/systemctl.app.default"
run_observe "$st" "$tmp/out-preflight-unit" --rehearsal --duration 60 --interval 60
[[ $run_rc -eq 1 ]] || fail "an unloaded unit must exit 1, got $run_rc"

# ---------------------------------------------------------------------------
# 3. Main scenario (rehearsal, 5 samples): latency units, percentile math,
#    request counts, error lines, redaction, derivation of PG/Redis targets,
#    queue extremes, saturation, manifest, and read-only behavior.
# ---------------------------------------------------------------------------
st=$(new_state main)
lat=("1000${mu}s" 2000000ns 3ms 0.004s 5ms "6000${mu}s" 7ms 8000000ns 9ms $'\033[32m10ms\033[0m'
	11ms "12000${mu}s" 13ms 0.014s 15ms 16ms 17000000ns 18ms 19ms 0.02s)
{
	for i in 0 1 2 3 4 5 6 7 8 9; do
		path=/dashboard
		prefix=''
		((i < 4)) && path=/healthz
		((i == 4)) && prefix='[stg/abc-000005] '
		((i == 5)) && path="/reports?password=${secret_text_password}&token=abc123"
		acc 200 "${lat[i]}" "$path" "$prefix"
	done
} >"$st/journal.app.0"
{
	for i in 10 11 12 13 14 15 16 17 18 19; do
		acc 200 "${lat[i]}" /sales/orders
	done
	printf '2026-09-21T13:35:00+0000 stg odyssey-staging[1111]: {"time":"2026-09-21T13:35:00Z","level":"ERROR","msg":"upstream failed","password":"%s","detail":"authentication failed using %s"}\n' \
		"$secret_text_password" "$secret_pg_password"
	printf '2026-09-21T13:35:01+0000 stg odyssey-staging[1111]: dial postgres://odyssey_staging:%s@db-host:5432/odyssey_staging?sslmode=require failed password=%s level=ERROR\n' \
		"$secret_pg_password" "$secret_text_password"
} >"$st/journal.app.2"
printf '2026-09-21T13:35:02+0000 stg odyssey-staging-worker[2222]: asynq: pid=2222 2026/09/21 13:35:02 ERROR: Failed to process task id=abc: connect redis://:%s@redis-host:6380/2 refused; Authorization: Bearer %s.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dBjftJeZ4CVP\n' \
	"$secret_redis_password" "$secret_jwt" >"$st/journal.worker.3"
echo 1 >"$st/queue.pending.0"
echo 7 >"$st/queue.pending.1"
echo 2 >"$st/queue.pending.default"
echo 4 >"$st/queue.active.default"
echo 9 >"$st/queue.retry.2"
echo 1 >"$st/queue.retry.default"
echo '2.00 1.00 0.50 1/200 1' >"$st/proc-scenario/loadavg.2"
echo '9.25 5.00 2.00 1/200 1' >"$st/proc-scenario/loadavg.3"
echo '0.10 0.10 0.10 1/200 1' >"$st/proc-scenario/loadavg.4"
printf 'MemTotal:        4096000 kB\nMemAvailable:     512000 kB\n' >"$st/proc-scenario/meminfo.3"
echo '/dev/vda1 41152812 37037531 4115281 91% /' >"$st/df.root.4"
printf 'secret leak redispw42 here\n' >"$st/pg.message"
alerts_input="$tmp/alerts.txt"
incidents_input="$tmp/incidents.txt"
printf 'none fired; checked console at 13:40Z token=alertsecret99 password=hunter2\n' >"$alerts_input"
: >"$incidents_input"

out_main="$tmp/out-main"
run_observe "$st" "$out_main" --rehearsal --duration 240 --interval 60 \
	--candidate-tag v0.10.0-rc.8 --expected-sha "$candidate_sha" \
	--alerts-file "$alerts_input" --incidents-file "$incidents_input"
[[ $run_rc -eq 0 ]] || { cat "$st/stderr.txt" >&2; fail "main scenario must exit 0 even with anomalies, got $run_rc"; }
summary="$out_main/summary.json"
expect_jq "$summary" '.sample_count == 5' 'main: five samples at 60s over 240s'
expect_jq "$summary" '.completed == true and .interrupted_by == null' 'main: window completed'
expect_jq "$summary" '.rehearsal == true and .eligible_for_ops005 == false' 'main: rehearsal must be ineligible'
expect_jq "$summary" '.ineligible_reasons | any(. == "rehearsal run")' 'main: rehearsal reason recorded'
expect_jq "$summary" '.utc_start == "'"$("$real_date" -u -d @1790000000 '+%Y-%m-%dT%H:%M:%SZ')"'" and .utc_end == "'"$("$real_date" -u -d @1790000240 '+%Y-%m-%dT%H:%M:%SZ')"'"' 'main: UTC start/end from the clock'
expect_jq "$summary" '.duration_seconds == 240 and .requested_duration_seconds == 240 and .interval_seconds == 60' 'main: durations'
expect_jq "$summary" '.requests_total == 20 and .requests_healthz == 4 and .requests_5xx == 0 and .requests_4xx == 0' 'main: request counts include health probes separately'
expect_jq "$summary" '.error_rate_5xx == 0' 'main: zero 5xx rate'
expect_jq "$summary" '.latency_ms.p50 == 10 and .latency_ms.p95 == 19 and .latency_ms.max == 20 and .latency_ms.samples == 20' 'main: us/ms/s/ns parsing and nearest-rank percentiles over 1..20 ms'
expect_jq "$summary" '.health.ok == 5 and .health.fail == 0 and .redis.ok == 5 and .redis.fail == 0 and .postgres.ok == 5 and .postgres.fail == 0' 'main: healthy dependencies'
expect_jq "$summary" '.redis.target == "redis-host:6380" and .postgres.target == "db-host:5432"' 'main: targets derived from the environment file without credentials'
expect_jq "$summary" '.queue.pending.first == 1 and .queue.pending.max == 7 and .queue.pending.last == 2' 'main: pending first/max/last'
expect_jq "$summary" '.queue.retry.max == 9 and .queue.retry.last == 1 and .queue.active.last == 4 and .queue.archived.max == 0' 'main: per-state queue extremes'
expect_jq "$summary" '.saturation.max_load1 == 9.25 and .saturation.min_mem_available_mb == 500 and .saturation.max_disk_used_pct == 91' 'main: saturation extremes'
expect_jq "$summary" '.unit_restarts.app.delta == 0 and .unit_restarts.worker.delta == 0 and .unit_restarts.app.main_pid_changes == 0' 'main: no restarts'
expect_jq "$summary" '.error_log_lines.app == 2 and .error_log_lines.worker == 1' 'main: error-level lines counted per unit'
expect_jq "$summary" '.handoff_owner == "Test Operator" and .candidate_tag == "v0.10.0-rc.8" and .expected_sha == "'"$candidate_sha"'"' 'main: handoff owner and candidate recorded'
expect_jq "$summary" '.alerts.status == "supplied" and .alerts.file == "alerts.txt" and .incidents.status == "supplied" and .incidents.lines == 0' 'main: supplied alerts and incidents'
expect_jq "$summary" '(.anomalies | any(contains("error-level"))) and (.anomalies | all(contains("5xx error rate") | not))' 'main: error lines flagged, no 5xx anomaly'
expect_jq "$summary" '.release.first == "20cc13a" and .release.changes == 0 and (.anomalies | all(contains("release symlink") | not))' 'main: release identity matches the expected sha'
expect_jq "$summary" '.note | contains("PASS or FAIL is decided by the reviewer")' 'main: summary must not decide PASS/FAIL'
if jq -e 'has("result") or has("pass") or has("verdict")' "$summary" >/dev/null; then fail 'summary must not carry a verdict'; fi
[[ -s "$out_main/raw/0001/healthz.txt" && -s "$out_main/raw/0005/systemd-worker.txt" && -s "$out_main/raw/0003/redis.txt" ]] || fail 'main: raw per-sample files missing'
[[ $(wc -l <"$out_main/samples.jsonl") -eq 5 ]] || fail 'main: samples.jsonl must hold one object per sample'
expect_jq "$out_main/samples.jsonl" '.redis.queue.pending != null' 'main: sample lines carry queue counts'
grep -q '\[QUERY-REDACTED\]' "$out_main/raw/0001/app-access.log" || fail 'main: query strings must be stripped from stored access lines'

# Secrets: nothing credential-like may appear anywhere in the bundle or output.
for secret in "$secret_pg_password" "$secret_redis_password" "$secret_text_password" "$secret_jwt" alertsecret99; do
	if grep -rqF -- "$secret" "$out_main"; then fail "main: secret '$secret' leaked into the evidence bundle"; fi
	if grep -qF -- "$secret" "$st/stdout.txt" "$st/stderr.txt"; then fail "main: secret '$secret' leaked into stdout or stderr"; fi
done
grep -q '\[REDACTED\]' "$out_main/raw/0003/app-errors.log" || fail 'main: redaction marker expected in the error lines'
grep -q 'ERROR: Failed to process task' "$out_main/raw/0004/worker-errors.log" || fail 'main: worker error text must survive redaction'
# Credentials reach redis-cli only through REDISCLI_AUTH, never through argv.
if grep -qF -- "$secret_redis_password" "$st/redis.argv" "$st/pg.argv" "$st/curl.calls"; then fail 'main: credential passed on a command line'; fi
if grep -Eq -- '(^| )-a( |$)|--pass|--no-auth-warning' "$st/redis.argv"; then fail 'main: redis-cli must not receive a password option'; fi
sort -u "$st/redis.auth" | grep -qx "$secret_redis_password" || fail 'main: REDISCLI_AUTH was not used'
grep -q -- '-h redis-host -p 6380 --raw -n 2' "$st/redis.argv" || fail 'main: redis host, port and db must be derived from REDIS_ADDR'
grep -q -- '-t 5 -h db-host -p 5432' "$st/pg.argv" || fail 'main: pg_isready must use the host and port from PG_DSN'

# Read-only: only the allowed commands ran.
cut -d' ' -f1 "$st/redis.calls" | sort -u | tr '\n' ' ' | grep -qx 'EXISTS LLEN PING ZCARD ' || fail "main: redis commands outside PING/LLEN/ZCARD/EXISTS: $(sort -u "$st/redis.calls")"
for state_name in pending active; do
	grep -q "LLEN asynq:{default}:$state_name" "$st/redis.calls" || fail "main: $state_name must be read with LLEN"
done
for state_name in scheduled retry archived completed; do
	grep -q "ZCARD asynq:{default}:$state_name" "$st/redis.calls" || fail "main: $state_name must be read with ZCARD"
done
grep -q 'EXISTS asynq:{default}:paused' "$st/redis.calls" || fail 'main: paused must be read with EXISTS'
if grep -qv '^show ' "$st/systemctl.calls"; then fail 'main: systemctl was used for something other than show'; fi
if grep -Eq -- '(^| )(-X|-d|--data|--request|-T|--upload-file)( |$)' "$st/curl.calls"; then fail 'main: health probe must be a plain GET'; fi
# The journal window continues from the previous cursor instead of overlapping.
grep -q '^app .*--since' "$st/journal.calls" || fail 'main: first journal read must use --since'
grep -q -- '--after-cursor=s=fake0' "$st/journal.calls" || fail 'main: later journal reads must resume from the cursor'

# Manifest verifies and excludes itself.
(cd "$out_main" && sha256sum -c SHA256SUMS >/dev/null) || fail 'main: SHA256SUMS does not verify'
if grep -q 'SHA256SUMS' "$out_main/SHA256SUMS"; then fail 'main: SHA256SUMS must not list itself'; fi
for covered in ./summary.json ./samples.jsonl ./raw/0001/healthz.txt; do
	grep -qF "  $covered" "$out_main/SHA256SUMS" || fail "main: manifest must cover $covered"
done
echo '# tampered' >>"$out_main/summary.json"
if (cd "$out_main" && sha256sum -c SHA256SUMS >/dev/null 2>&1); then fail 'main: SHA256SUMS accepted a modified summary'; fi
head -n -1 "$out_main/summary.json" >"$tmp/summary.restored" && cp "$tmp/summary.restored" "$out_main/summary.json"
(cd "$out_main" && sha256sum -c SHA256SUMS >/dev/null) || fail 'main: manifest should verify again after restoring the summary'

# ---------------------------------------------------------------------------
# 4. Alerts and incidents are "not supplied", never silently "none".
# ---------------------------------------------------------------------------
st=$(new_state unsupplied)
out_unsupplied="$tmp/out-unsupplied"
run_observe "$st" "$out_unsupplied" --rehearsal --duration 120 --interval 60 --expected-sha "$(printf 'f%.0s' {1..40})"
[[ $run_rc -eq 0 ]] || fail "unsupplied scenario must exit 0, got $run_rc"
expect_jq "$out_unsupplied/summary.json" '.alerts.status == "not supplied" and .incidents.status == "not supplied"' 'unsupplied: alerts and incidents must read "not supplied"'
expect_jq "$out_unsupplied/summary.json" '(.anomalies | any(startswith("alerts not supplied"))) and (.anomalies | any(startswith("incidents not supplied")))' 'unsupplied: anomalies must flag the missing logs'
expect_jq "$out_unsupplied/summary.json" '.candidate_tag == "not supplied"' 'unsupplied: candidate tag absent means "not supplied"'
expect_jq "$out_unsupplied/summary.json" '.anomalies | any(contains("not a prefix of the expected sha"))' 'unsupplied: a different deployed release must be flagged'
expect_jq "$out_unsupplied/summary.json" '.anomalies | any(contains("no application requests other than /healthz"))' 'unsupplied: a window with only health probes must be flagged'
expect_jq "$out_unsupplied/summary.json" '.requests_total == 0 and .error_rate_5xx == null and .latency_ms.p50 == null' 'unsupplied: no traffic yields null rates, not zero'
[[ ! -e "$out_unsupplied/alerts.txt" && ! -e "$out_unsupplied/incidents.txt" ]] || fail 'unsupplied: alerts/incidents files must not be invented'

# ---------------------------------------------------------------------------
# 5. Duration grammar and the 5xx error rate.
# ---------------------------------------------------------------------------
st=$(new_state units)
{
	acc 200 "250ns"
	acc 200 "1.5${mu}s"
	acc 200 "750${mu}s"
	acc 200 "1.234ms"
	acc 200 "1.5s"
	acc 200 "1m0.5s"
} >"$st/journal.app.0"
run_observe "$st" "$tmp/out-units" --rehearsal --duration 60 --interval 60
[[ $run_rc -eq 0 ]] || fail "units scenario must exit 0, got $run_rc"
expect_jq "$tmp/out-units/summary.json" '.latency_ms.samples == 6 and .latency_ms.p50 == 0.75 and .latency_ms.p95 == 60500 and .latency_ms.max == 60500' 'units: ns/us/ms/s and compound durations'

st=$(new_state errors)
{
	for _ in $(seq 1 90); do acc 200 2ms; done
	for _ in $(seq 1 5); do acc 404 3ms /missing; done
	for _ in $(seq 1 5); do acc 503 4ms /api/orders; done
} >"$st/journal.app.1"
run_observe "$st" "$tmp/out-errors" --rehearsal --duration 120 --interval 60
[[ $run_rc -eq 0 ]] || fail "error-rate scenario must exit 0, got $run_rc"
expect_jq "$tmp/out-errors/summary.json" '.requests_total == 100 and .requests_5xx == 5 and .requests_4xx == 5 and .error_rate_5xx == 0.05' 'errors: 5xx/4xx counts and error rate'
expect_jq "$tmp/out-errors/summary.json" '.anomalies | any(contains("5xx error rate") and contains("0.01"))' 'errors: rate above the default threshold flagged'
run_observe "$st" "$tmp/out-errors-high" --rehearsal --duration 120 --interval 60 --max-error-rate 0.06
expect_jq "$tmp/out-errors-high/summary.json" '.anomalies | all(contains("5xx error rate") | not)' 'errors: rate below --max-error-rate not flagged'

# ---------------------------------------------------------------------------
# 6. Health, Redis and Postgres failures are counted and flagged.
# ---------------------------------------------------------------------------
st=$(new_state failures)
printf '503 0.010 0\n' >"$st/curl.2"
touch "$st/redis.fail.4"
echo 2 >"$st/pg.1"
run_observe "$st" "$tmp/out-failures" --rehearsal --duration 240 --interval 60
[[ $run_rc -eq 0 ]] || fail "failure scenario must exit 0 (anomalies are data), got $run_rc"
summary="$tmp/out-failures/summary.json"
expect_jq "$summary" '.health.ok == 4 and .health.fail == 1 and .redis.ok == 4 and .redis.fail == 1 and .postgres.ok == 4 and .postgres.fail == 1' 'failures: ok/fail counters'
expect_jq "$summary" '(.anomalies | any(startswith("health endpoint was not 200 in 1 of 5 samples (first at sample 3)"))) and (.anomalies | any(startswith("redis PING failed in 1 of 5 samples (first at sample 5)"))) and (.anomalies | any(startswith("postgres readiness failed in 1 of 5 samples (first at sample 2)")))' 'failures: anomalies name each failing check and sample'
expect_jq "$tmp/out-failures/samples.jsonl" 'select(.seq == 5) | .redis.ok == false and .redis.queue.pending == null' 'failures: a failed Redis sample records null depth, not zero'
expect_jq "$summary" '.queue.pending.last == 0 and .queue.pending.max == 0' 'failures: null depths are skipped in extremes, even for the final sample'
expect_jq "$summary" '.health.latency_ms.samples == 4' 'failures: failed probes excluded from health latency'

# A journal that fails mid-window must not silently look like zero errors.
st=$(new_state journal-fail)
touch "$st/journal.fail.app"
run_observe "$st" "$tmp/out-journal-fail" --rehearsal --duration 60 --interval 60
[[ $run_rc -eq 0 ]] || fail "journal failure scenario must exit 0, got $run_rc"
expect_jq "$tmp/out-journal-fail/summary.json" '.anomalies | any(startswith("journal capture for the app unit failed"))' 'journal-fail: failed capture flagged'

# ---------------------------------------------------------------------------
# 7. Restart delta and archived growth.
# ---------------------------------------------------------------------------
st=$(new_state restart)
for t in 2 3 4; do
	printf 'LoadState=loaded\nActiveState=active\nSubState=running\nNRestarts=1\nMainPID=3333\n' >"$st/systemctl.app.$t"
done
printf 'LoadState=loaded\nActiveState=activating\nSubState=auto-restart\nNRestarts=1\nMainPID=0\n' >"$st/systemctl.app.1"
echo 0 >"$st/queue.archived.0"
for t in 2 3 4; do echo 3 >"$st/queue.archived.$t"; done
run_observe "$st" "$tmp/out-restart" --rehearsal --duration 240 --interval 60
[[ $run_rc -eq 0 ]] || fail "restart scenario must exit 0, got $run_rc"
summary="$tmp/out-restart/summary.json"
expect_jq "$summary" '.unit_restarts.app.delta == 1 and .unit_restarts.app.main_pid_changes == 1 and .unit_restarts.worker.delta == 0' 'restart: NRestarts delta and MainPID change per unit'
expect_jq "$summary" '(.anomalies | any(contains("odyssey-staging.service NRestarts changed by 1"))) and (.anomalies | any(contains("MainPID changed 1")))' 'restart: restart flagged'
expect_jq "$summary" '.anomalies | any(contains("was not active in 1 samples"))' 'restart: inactive sample flagged'
expect_jq "$summary" '.queue.archived.first == 0 and .queue.archived.max == 3 and .queue.archived.last == 3' 'archived: first/max/last'
expect_jq "$summary" '.anomalies | any(. == "queue archived count increased from 0 to 3")' 'archived: growth flagged'

# ---------------------------------------------------------------------------
# 8. A full-length run is eligible only when completed and not a rehearsal.
# ---------------------------------------------------------------------------
st=$(new_state full)
out_full="$tmp/out-full"
run_observe "$st" "$out_full" --duration 3600 --interval 300 \
	--candidate-tag v0.10.0-rc.8 --expected-sha "$candidate_sha" \
	--alerts-file "$alerts_input" --incidents-file "$incidents_input"
[[ $run_rc -eq 0 ]] || fail "full-length run must exit 0, got $run_rc"
expect_jq "$out_full/summary.json" '.completed == true and .rehearsal == false and .eligible_for_ops005 == true and (.ineligible_reasons | length) == 0' 'full: a completed 3600s non-rehearsal window is eligible'
expect_jq "$out_full/summary.json" '.duration_seconds == 3600 and .sample_count == 13' 'full: samples every 300s across 3600s'
(cd "$out_full" && sha256sum -c SHA256SUMS >/dev/null) || fail 'full: SHA256SUMS does not verify'

# ---------------------------------------------------------------------------
# 9. SIGTERM (and SIGHUP from a dropped SSH session) mid-window writes a
#    summary marked incomplete and ineligible. SIGINT shares the handler but
#    cannot be delivered here: a non-interactive shell starts background jobs
#    with SIGINT ignored.
# ---------------------------------------------------------------------------
signal_test() {
	local sig=$1 name=$2 want_rc=$3
	local sst out pid rc=0
	sst=$(new_state "signal-$sig")
	out="$tmp/out-signal-$sig"
	SHIM_SLEEP_BLOCK_AFTER=2 SHIM_STATE=$sst REAL_DATE=$real_date OBSERVE_PROC_DIR=$sst/proc PATH="$shim_dir:$PATH" 		"$observer" --out "$out" --handoff-owner 'Test Operator' --confirm-staging --install-dir "$install" 		--duration 3600 --interval 60 >"$sst/stdout.txt" 2>"$sst/stderr.txt" &
	pid=$!
	for _ in $(seq 1 200); do
		[[ -f "$sst/blocking" ]] && break
		/usr/bin/sleep 0.1
	done
	[[ -f "$sst/blocking" ]] || { kill "$pid" 2>/dev/null || true; fail "$sig: collector never reached the blocking sleep"; }
	kill -"$sig" "$pid"
	wait "$pid" || rc=$?
	[[ $rc -eq "$want_rc" ]] || fail "$sig: expected exit $want_rc, got $rc"
	expect_jq "$out/summary.json" '.completed == false and .eligible_for_ops005 == false and .interrupted_by == "'"$name"'" and .sample_count == 3' "$sig: summary must be incomplete and ineligible"
	expect_jq "$out/summary.json" '.ineligible_reasons | any(contains("interrupted by '"$name"'"))' "$sig: reason recorded"
	expect_jq "$out/summary.json" '.duration_seconds == 120 and .rehearsal == false' "$sig: elapsed time at interruption recorded"
	(cd "$out" && sha256sum -c SHA256SUMS >/dev/null) || fail "$sig: SHA256SUMS does not verify"
}
signal_test TERM SIGTERM 143
signal_test HUP SIGHUP 129

# ---------------------------------------------------------------------------
# 10. Alternative dependency wiring: keyword/value PG DSN, plain Redis address
#     without credentials, and an operator-supplied --pg-check-cmd.
# ---------------------------------------------------------------------------
kw_install="$tmp/kw-install"
mkdir -p "$kw_install/releases/20cc13a"
ln -s releases/20cc13a "$kw_install/current"
printf 'APP_ENV=staging
PG_DSN="host=pghost port=6543 user=odyssey password='"'"'sp aced pw'"'"' dbname=odyssey"
' >"$kw_install/.env"
st=$(new_state keyword-dsn)
printf '2026-09-21T13:35:00+0000 stg odyssey-staging[1111]: {"level":"ERROR","msg":"x sp aced pw y"}
' >"$st/journal.app.1"
run_observe "$st" "$tmp/out-kw" --install-dir "$kw_install" --rehearsal --duration 60 --interval 60
[[ $run_rc -eq 0 ]] || { cat "$st/stderr.txt" >&2; fail "keyword DSN scenario must exit 0, got $run_rc"; }
expect_jq "$tmp/out-kw/summary.json" '.postgres.target == "pghost:6543" and .postgres.method == "pg_isready" and .redis.target == "127.0.0.1:6379"' 'keyword-dsn: host and port parsed, Redis defaults to the app default'
grep -q -- '-t 5 -h pghost -p 6543' "$st/pg.argv" || fail 'keyword-dsn: pg_isready host and port'
if grep -rqF 'sp aced pw' "$tmp/out-kw"; then fail 'keyword-dsn: quoted password leaked'; fi
grep -qvx '<unset>' "$st/redis.auth" && fail 'keyword-dsn: REDISCLI_AUTH must stay unset without a Redis password'

st=$(new_state pg-cmd)
run_observe "$st" "$tmp/out-pgcmd" --rehearsal --duration 120 --interval 60 --pg-check-cmd 'test ! -e /nonexistent-ops005-marker'
[[ $run_rc -eq 0 ]] || fail "pg-check-cmd scenario must exit 0, got $run_rc"
expect_jq "$tmp/out-pgcmd/summary.json" '.postgres.method == "pg-check-cmd" and .postgres.ok == 3' 'pg-cmd: override command drives the Postgres check'
[[ ! -e "$st/pg.argv" ]] || fail 'pg-cmd: pg_isready must not run when --pg-check-cmd is supplied'
st=$(new_state pg-cmd-fail)
run_observe "$st" "$tmp/out-pgcmd-fail" --rehearsal --duration 120 --interval 60 --pg-check-cmd 'echo down; exit 3'
expect_jq "$tmp/out-pgcmd-fail/summary.json" '.postgres.ok == 0 and .postgres.fail == 3' 'pg-cmd: failing override command counted'

# ---------------------------------------------------------------------------
# 11. The record plugs into the evidence collector's operator lane: the copy it
#     redacts and indexes must still verify against the inner manifest.
# ---------------------------------------------------------------------------
candidate="$tmp/operator-candidate"
mkdir -p "$candidate"
cp -a "$out_full" "$candidate/ops-005-observation"
utc_end=$(jq -r '.utc_end' "$out_full/summary.json")
record=$(jq -cn --arg collected "$utc_end" \
	'{evidence_id:"OPS-005",result:"PASS",collected_utc:$collected,details:"ops-005-observation/summary.json reviewed"}')
printf 'CERTIFICATION_EVIDENCE evidence_id=OPS-005 %s\n' "$record" >"$candidate/staging-certification.log"
evidence="$tmp/operator-evidence"
env CANDIDATE_TAG=v0.10.0-rc.8 EXPECTED_SHA="$candidate_sha" \
	"$collector" --candidate "$candidate" --evidence "$evidence" --contract "$contract" \
	--lane operator --expected-evidence-ids OPS-005 --local-only >"$tmp/evidence.log" 2>&1 \
	|| { cat "$tmp/evidence.log" >&2; fail 'evidence collector rejected the OPS-005 observation bundle'; }
expect_jq "$evidence/evidence-index.json" '.collection.result == "PASS" and (.entries | length) == 1 and .entries[0].evidence_id == "OPS-005"' 'evidence: OPS-005 indexed in the operator lane'
expect_jq "$evidence/evidence-index.json" '.files | any(.path == "ops-005-observation/summary.json")' 'evidence: observation summary listed in the index'
(cd "$evidence/ops-005-observation" && sha256sum -c SHA256SUMS >/dev/null) || fail 'evidence: inner manifest no longer verifies after the collector redaction pass'
(cd "$evidence" && sha256sum -c SHA256SUMS >/dev/null) || fail 'evidence: top-level manifest does not verify'

echo 'staging certification observe tests passed'
