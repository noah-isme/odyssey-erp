#!/usr/bin/env bash
set -euo pipefail

usage() {
	cat <<'EOF'
Usage: setup-evidence-bucket.sh --bucket NAME [options]

Create an immutable S3 evidence bucket with Object Lock enabled at creation
in COMPLIANCE mode with a 7-year retention period, test it with a probe object,
and optionally configure the GitHub staging environment secrets via gh CLI.

WARNING:
COMPLIANCE mode locks CANNOT be shortened, overwritten, or deleted by any user
including the AWS root account until the retention period expires (7 years / 2557 days).

Required:
  --bucket NAME         Bucket name to create (e.g. odyssey-staging-evidence-compliance)

Options:
  --region REGION       AWS region (default: us-east-1)
  --endpoint-url URL    S3-compatible endpoint URL (for MinIO, R2, Ceph, etc.)
  --apply-gh            Automatically set the 5 EVIDENCE_S3_* secrets in GitHub staging environment
  --skip-create         Skip bucket creation if the bucket already exists with Object Lock enabled
  -h, --help            Show this help message
EOF
}

bucket=''
region=${AWS_REGION:-${EVIDENCE_S3_REGION:-us-east-1}}
endpoint=${EVIDENCE_S3_ENDPOINT:-}
apply_gh=false
skip_create=false

while (($#)); do
	case "$1" in
		--bucket) [[ $# -ge 2 ]] || { echo "missing value for --bucket" >&2; exit 2; }; bucket=$2; shift 2 ;;
		--region) [[ $# -ge 2 ]] || { echo "missing value for --region" >&2; exit 2; }; region=$2; shift 2 ;;
		--endpoint-url) [[ $# -ge 2 ]] || { echo "missing value for --endpoint-url" >&2; exit 2; }; endpoint=$2; shift 2 ;;
		--apply-gh) apply_gh=true; shift ;;
		--skip-create) skip_create=true; shift ;;
		-h|--help) usage; exit 0 ;;
		*) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
	esac
done

[[ -n "$bucket" ]] || { echo "--bucket NAME is required" >&2; exit 2; }

command -v aws >/dev/null 2>&1 || { echo "aws CLI is required" >&2; exit 1; }
command -v sha256sum >/dev/null 2>&1 || { echo "sha256sum is required" >&2; exit 1; }

aws_args=(--region "$region")
[[ -n "$endpoint" ]] && aws_args+=(--endpoint-url "$endpoint")

echo "================================================================================"
echo "          IMMUTABLE EVIDENCE STORE PROVISIONING (S3 OBJECT LOCK)               "
echo "================================================================================"
echo "Bucket:   $bucket"
echo "Region:   $region"
echo "Endpoint: ${endpoint:-AWS standard}"
echo "Mode:     COMPLIANCE (7 years / 2557 days default retention)"
echo "================================================================================"

# Step 1: Create bucket with Object Lock enabled at creation
if [[ "$skip_create" == false ]]; then
	echo "→ [1/4] Creating bucket with Object Lock enabled at creation..."
	create_args=(s3api create-bucket --bucket "$bucket" "${aws_args[@]}" --object-lock-enabled-for-bucket)
	if [[ "$region" != "us-east-1" && -z "$endpoint" ]]; then
		create_args+=(--create-bucket-configuration LocationConstraint="$region")
	fi

	if aws "${create_args[@]}"; then
		echo "  ✓ Bucket created with Object Lock capability"
	else
		echo "  ℹ create-bucket returned non-zero; checking if bucket exists with Object Lock enabled..."
	fi
else
	echo "→ [1/4] Skipping bucket creation (--skip-create specified)..."
fi

# Step 2: Configure default retention rule: COMPLIANCE mode, 2557 days (7 years)
echo "→ [2/4] Setting bucket Object Lock configuration (COMPLIANCE, 2557 days)..."
config_json=$(cat <<'EOF'
{
  "ObjectLockEnabled": "Enabled",
  "Rule": {
    "DefaultRetention": {
      "Mode": "COMPLIANCE",
      "Days": 2557
    }
  }
}
EOF
)

aws s3api put-object-lock-configuration \
	--bucket "$bucket" \
	"${aws_args[@]}" \
	--object-lock-configuration "$config_json"

echo "  ✓ Default retention policy set"

# Verify configuration
echo "→ [3/4] Verifying bucket Object Lock configuration..."
verified_mode=$(aws s3api get-bucket-object-lock-configuration \
	--bucket "$bucket" \
	"${aws_args[@]}" \
	--query 'ObjectLockConfiguration.Rule.DefaultRetention.Mode' \
	--output text)

verified_days=$(aws s3api get-bucket-object-lock-configuration \
	--bucket "$bucket" \
	"${aws_args[@]}" \
	--query 'ObjectLockConfiguration.Rule.DefaultRetention.Days' \
	--output text)

if [[ "$verified_mode" != "COMPLIANCE" || "$verified_days" != "2557" ]]; then
	echo "ERROR: Object Lock verification failed: Mode=$verified_mode, Days=$verified_days (want COMPLIANCE, 2557)" >&2
	exit 1
fi
echo "  ✓ Verified: Mode=$verified_mode, Days=$verified_days"

# Step 3: Test Object Lock with one test probe object
echo "→ [4/4] Testing Object Lock with a probe object..."
probe_key="test-lock-probe/probe-$(date +%s).txt"
probe_file=$(mktemp)
trap 'rm -f "$probe_file"' EXIT
printf 'v0.10-core staging certification Object Lock probe: %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" > "$probe_file"
probe_sha=$(sha256sum "$probe_file" | awk '{print $1}')
retain_until=$(date -u -d '+7 years' '+%Y-%m-%dT%H:%M:%SZ')

aws s3api put-object \
	--bucket "$bucket" \
	"${aws_args[@]}" \
	--key "$probe_key" \
	--body "$probe_file" \
	--metadata "sha256=$probe_sha" \
	--object-lock-mode COMPLIANCE \
	--object-lock-retain-until-date "$retain_until" >/dev/null

echo "  ✓ Put test object: s3://$bucket/$probe_key"

# Verify head-object retention and metadata
remote_mode=$(aws s3api head-object --bucket "$bucket" "${aws_args[@]}" --key "$probe_key" --query ObjectLockMode --output text)
remote_date=$(aws s3api head-object --bucket "$bucket" "${aws_args[@]}" --key "$probe_key" --query ObjectLockRetainUntilDate --output text)
remote_sha=$(aws s3api head-object --bucket "$bucket" "${aws_args[@]}" --key "$probe_key" --query 'Metadata.sha256' --output text)

if [[ "$remote_mode" != "COMPLIANCE" ]]; then
	echo "ERROR: Test object ObjectLockMode is $remote_mode, want COMPLIANCE" >&2
	exit 1
fi
if [[ "$remote_sha" != "$probe_sha" ]]; then
	echo "ERROR: Test object remote sha mismatch: $remote_sha != $probe_sha" >&2
	exit 1
fi
echo "  ✓ Probe verified: ObjectLockMode=$remote_mode, RetainUntilDate=$remote_date, SHA256=$remote_sha"

echo "================================================================================"
echo "          EVIDENCE BUCKET READY AND VERIFIED                                    "
echo "================================================================================"

if [[ "$apply_gh" == true ]]; then
	command -v gh >/dev/null 2>&1 || { echo "gh CLI required for --apply-gh" >&2; exit 1; }
	echo "→ Applying evidence bucket secrets to GitHub staging environment..."

	gh secret set EVIDENCE_S3_BUCKET --body "$bucket" --env staging
	echo "  ✓ Set secret EVIDENCE_S3_BUCKET"

	gh secret set EVIDENCE_S3_REGION --body "$region" --env staging
	echo "  ✓ Set secret EVIDENCE_S3_REGION"

	endpoint_val=${endpoint:-"https://s3.${region}.amazonaws.com"}
	gh secret set EVIDENCE_S3_ENDPOINT --body "$endpoint_val" --env staging
	echo "  ✓ Set secret EVIDENCE_S3_ENDPOINT = $endpoint_val"

	if [[ -n "${AWS_ACCESS_KEY_ID:-}" ]]; then
		gh secret set EVIDENCE_S3_ACCESS_KEY_ID --body "$AWS_ACCESS_KEY_ID" --env staging
		echo "  ✓ Set secret EVIDENCE_S3_ACCESS_KEY_ID"
	fi
	if [[ -n "${AWS_SECRET_ACCESS_KEY:-}" ]]; then
		gh secret set EVIDENCE_S3_SECRET_ACCESS_KEY --body "$AWS_SECRET_ACCESS_KEY" --env staging
		echo "  ✓ Set secret EVIDENCE_S3_SECRET_ACCESS_KEY"
	fi
	echo "Done applying secrets to GitHub staging environment!"
else
	echo "To set the evidence bucket secrets in GitHub staging environment, run:"
	echo "  gh secret set EVIDENCE_S3_BUCKET --body \"$bucket\" --env staging"
	echo "  gh secret set EVIDENCE_S3_REGION --body \"$region\" --env staging"
	echo "  gh secret set EVIDENCE_S3_ENDPOINT --body \"${endpoint:-https://s3.${region}.amazonaws.com}\" --env staging"
	echo "  gh secret set EVIDENCE_S3_ACCESS_KEY_ID --body \"<ACCESS_KEY>\" --env staging"
	echo "  gh secret set EVIDENCE_S3_SECRET_ACCESS_KEY --body \"<SECRET_KEY>\" --env staging"
fi
