#!/usr/bin/env bash
set -euo pipefail

usage() {
	cat <<'EOF'
Usage: staging-certification-evidence.sh --candidate DIR --evidence DIR [options]

Copy a staging certification candidate into an evidence bundle, redact secrets from
text files, create a SHA-256 manifest, and store the bundle in S3 Object Lock.

Required:
  --candidate DIR       Candidate artifacts/logs to collect
  --evidence DIR        Local output directory for the collected evidence

Options:
  --bucket NAME         S3 bucket (default: EVIDENCE_S3_BUCKET)
  --prefix PREFIX       Object prefix (default: EVIDENCE_S3_PREFIX or timestamp)
  --endpoint-url URL    S3-compatible endpoint (default: EVIDENCE_S3_ENDPOINT)
  --region REGION       S3 region (default: EVIDENCE_S3_REGION or us-east-1)
  --help                Show this help

Credentials are read from EVIDENCE_S3_ACCESS_KEY_ID and
EVIDENCE_S3_SECRET_ACCESS_KEY. The objects are retained for seven years in
COMPLIANCE mode. Existing object keys are never overwritten.
EOF
}

candidate=''
evidence=''
bucket=${EVIDENCE_S3_BUCKET:-}
prefix=${EVIDENCE_S3_PREFIX:-}
endpoint=${EVIDENCE_S3_ENDPOINT:-}
region=${EVIDENCE_S3_REGION:-us-east-1}

while (($#)); do
	case "$1" in
		--candidate|--candidate-dir) [[ $# -ge 2 ]] || { echo "missing value for $1" >&2; exit 2; }; candidate=$2; shift 2 ;;
		--evidence|--evidence-dir) [[ $# -ge 2 ]] || { echo "missing value for $1" >&2; exit 2; }; evidence=$2; shift 2 ;;
		--bucket) [[ $# -ge 2 ]] || { echo "missing value for --bucket" >&2; exit 2; }; bucket=$2; shift 2 ;;
		--prefix) [[ $# -ge 2 ]] || { echo "missing value for --prefix" >&2; exit 2; }; prefix=$2; shift 2 ;;
		--endpoint-url) [[ $# -ge 2 ]] || { echo "missing value for --endpoint-url" >&2; exit 2; }; endpoint=$2; shift 2 ;;
		--region) [[ $# -ge 2 ]] || { echo "missing value for --region" >&2; exit 2; }; region=$2; shift 2 ;;
		-h|--help) usage; exit 0 ;;
		*) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
	esac
done

[[ -n "$candidate" && -d "$candidate" ]] || { echo "--candidate must name an existing directory" >&2; exit 2; }
[[ -n "$evidence" ]] || { echo "--evidence is required" >&2; exit 2; }
[[ -n "$bucket" ]] || { echo "S3 bucket is required (--bucket or EVIDENCE_S3_BUCKET)" >&2; exit 2; }
command -v aws >/dev/null 2>&1 || { echo "aws CLI is required" >&2; exit 1; }
command -v sha256sum >/dev/null 2>&1 || { echo "sha256sum is required" >&2; exit 1; }
export AWS_ACCESS_KEY_ID=${AWS_ACCESS_KEY_ID:-${EVIDENCE_S3_ACCESS_KEY_ID:-}}
export AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY:-${EVIDENCE_S3_SECRET_ACCESS_KEY:-}}
[[ -n "$AWS_ACCESS_KEY_ID" ]] || { echo "S3 access key is required (EVIDENCE_S3_ACCESS_KEY_ID)" >&2; exit 2; }
[[ -n "$AWS_SECRET_ACCESS_KEY" ]] || { echo "S3 secret key is required (EVIDENCE_S3_SECRET_ACCESS_KEY)" >&2; exit 2; }

mkdir -p "$evidence"
if [[ -n "$(find "$evidence" -mindepth 1 -print -quit)" ]]; then
	echo "evidence directory must be empty: $evidence" >&2
	exit 2
fi

cp -a "$candidate"/. "$evidence"/

# Redact common credential forms only in text files; binaries are left untouched.
while IFS= read -r -d '' file; do
	if grep -Iq . "$file" 2>/dev/null; then
		tmp=$(mktemp)
		sed -E \
			-e 's/(Bearer[[:space:]]+)[A-Za-z0-9._~+\/-]+/\1[REDACTED]/g' \
			-e 's/((AWS_SECRET_ACCESS_KEY|AWS_ACCESS_KEY_ID|SECRET_KEY|API_KEY|ACCESS_TOKEN|AUTH_TOKEN|PASSWORD|PRIVATE_KEY)[[:space:]]*[=:][[:space:]]*)[^,;[:space:]]+/\1[REDACTED]/Ig' \
			-e 's#(postgres(ql)?://[^:/[:space:]]+):[^@/[:space:]]+@#\1:[REDACTED]@#Ig' \
			-e 's#((https?://)[^:/[:space:]]+):[^@/[:space:]]+@#\1:[REDACTED]@#Ig' \
			-e 's/(("(secret|token|password|api[_-]?key|private[_-]?key)"|\x27(secret|token|password|api[_-]?key|private[_-]?key)\x27)[[:space:]]*:[[:space:]]*)"[^"]*"/\1"[REDACTED]"/Ig' \
			"$file" >"$tmp"
		mv "$tmp" "$file"
	fi
done < <(find "$evidence" -type f -print0)

manifest="$evidence/SHA256SUMS"
(cd "$evidence" && find . -type f ! -name SHA256SUMS -print0 | sort -z | xargs -0 sha256sum) >"$manifest"

if [[ -z "$prefix" ]]; then
	prefix="staging-certification/$(date -u +%Y%m%dT%H%M%SZ)"
fi
prefix=${prefix#/}
retain_until=$(date -u -d '+7 years' '+%Y-%m-%dT%H:%M:%SZ')
aws_args=(--region "$region")
[[ -n "$endpoint" ]] && aws_args+=(--endpoint-url "$endpoint")
base_args=(--bucket "$bucket" "${aws_args[@]}")

while IFS= read -r -d '' file; do
	rel=${file#"$evidence"/}
	key="$prefix/$rel"
	if aws s3api head-object "${base_args[@]}" --key "$key" >/dev/null 2>&1; then
		echo "refusing to overwrite existing object: s3://$bucket/$key" >&2
		exit 1
	fi
	aws s3api put-object "${base_args[@]}" --key "$key" --body "$file" \
		--object-lock-mode COMPLIANCE --object-lock-retain-until-date "$retain_until" >/dev/null
	done < <(find "$evidence" -type f -print0)

# Verify retention on every uploaded object, including the manifest.
while IFS= read -r -d '' file; do
	rel=${file#"$evidence"/}; key="$prefix/$rel"
	mode=$(aws s3api head-object "${base_args[@]}" --key "$key" --query ObjectLockMode --output text)
	retained=$(aws s3api head-object "${base_args[@]}" --key "$key" --query ObjectLockRetainUntilDate --output text)
	[[ "$mode" == COMPLIANCE ]] || { echo "invalid Object Lock mode for $key: $mode" >&2; exit 1; }
	[[ "$(date -u -d "$retained" +%s)" -ge "$(date -u -d "$retain_until" +%s)" ]] || { echo "retention too short for $key" >&2; exit 1; }
done < <(find "$evidence" -type f -print0)

echo "uploaded immutable evidence: s3://$bucket/$prefix (retain until $retain_until)"
