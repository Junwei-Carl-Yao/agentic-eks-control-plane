# Remote state backend.
#
# `bucket` is intentionally omitted here because the state bucket is created
# out-of-band by scripts/bootstrap.sh and injected at init time via
# `-backend-config=envs/<env>/backend.hcl`. State locking uses S3 native
# conditional writes (use_lockfile = true) — no DynamoDB table required.
# Keeping environment-specific values out of this file lets the same root
# module target multiple environments without edits.
terraform {
  backend "s3" {
    key          = "eks-control-plane/terraform.tfstate"
    encrypt      = true
    use_lockfile = true
  }
}
