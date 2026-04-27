#!/usr/bin/env bash
# Remote-state bootstrap for the EKS control plane.
#
# Creates (idempotently) an S3 bucket for state with versioning, SSE
# (AES256), and public access fully blocked.
#
# State locking uses S3 native conditional writes (use_lockfile = true,
# Terraform >= 1.11) — no DynamoDB table required.
#
# Writes:
#   • infrastructure/envs/<env>/backend.hcl   (Terraform init -backend-config)
#   • prints the values to paste into terraform.tfvars
#
# This bucket is intentionally NOT managed by Terraform: a cluster
# `terraform destroy` must never remove the state it lives in.
#
# Usage:
#   TF_ENV=dev AWS_REGION=us-east-1 scripts/bootstrap.sh
#   scripts/bootstrap.sh --env dev --region us-east-1
#
# Idempotent: safe to re-run. Existing resources are kept as-is.

set -euo pipefail

TF_ENV="${TF_ENV:-dev}"
REGION="${AWS_REGION:-}"
PREFIX="${BOOTSTRAP_PREFIX:-eks-control-plane}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --env)    TF_ENV="$2"; shift 2 ;;
    --region) REGION="$2"; shift 2 ;;
    --prefix) PREFIX="$2"; shift 2 ;;
    -h|--help)
      sed -n '2,22p' "$0"; exit 0 ;;
    *)
      echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

need() { command -v "$1" >/dev/null 2>&1 || { echo "missing: $1" >&2; exit 2; }; }
need aws

if [[ -z "$REGION" ]]; then
  REGION="$(aws configure get region 2>/dev/null || true)"
fi
: "${REGION:?set AWS_REGION or pass --region}"

ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
BUCKET="${PREFIX}-tfstate-${ACCOUNT_ID}-${REGION}"
KEY="eks-control-plane/${TF_ENV}/terraform.tfstate"

echo "==> bootstrap: env=${TF_ENV} region=${REGION} account=${ACCOUNT_ID}"
echo "    bucket=${BUCKET}"

# --------------------------------------------------------------------------
# S3 bucket
# --------------------------------------------------------------------------
if aws s3api head-bucket --bucket "$BUCKET" 2>/dev/null; then
  echo "  [skip] bucket exists"
else
  echo "  [create] bucket"
  if [[ "$REGION" == "us-east-1" ]]; then
    aws s3api create-bucket --bucket "$BUCKET" --region "$REGION" >/dev/null
  else
    aws s3api create-bucket \
      --bucket "$BUCKET" \
      --region "$REGION" \
      --create-bucket-configuration "LocationConstraint=${REGION}" >/dev/null
  fi
fi

aws s3api put-bucket-versioning \
  --bucket "$BUCKET" \
  --versioning-configuration Status=Enabled >/dev/null

aws s3api put-bucket-encryption \
  --bucket "$BUCKET" \
  --server-side-encryption-configuration '{
    "Rules": [{"ApplyServerSideEncryptionByDefault": {"SSEAlgorithm": "AES256"}}]
  }' >/dev/null

aws s3api put-public-access-block \
  --bucket "$BUCKET" \
  --public-access-block-configuration \
    BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true \
  >/dev/null

echo "  [ok] bucket configured (versioning, SSE, public-access-block)"

# --------------------------------------------------------------------------
# Write backend.hcl
# --------------------------------------------------------------------------
BACKEND_DIR="infrastructure/envs/${TF_ENV}"
BACKEND_FILE="${BACKEND_DIR}/backend.hcl"
mkdir -p "$BACKEND_DIR"
cat >"$BACKEND_FILE" <<EOF
bucket       = "${BUCKET}"
use_lockfile = true
region       = "${REGION}"
key          = "${KEY}"
encrypt      = true
EOF

echo "==> wrote ${BACKEND_FILE}"
echo
echo "Add these to infrastructure/envs/${TF_ENV}/terraform.tfvars:"
echo "  region            = \"${REGION}\""
echo "  state_bucket_name = \"${BUCKET}\""
