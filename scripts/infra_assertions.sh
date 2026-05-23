#!/usr/bin/env bash
# Phase 1 infrastructure assertions.
#
# Checks reality against spec (docs/implementation.md §1.1–1.5), independent of
# what Terraform's state file claims. Run AFTER `make apply` completes.
#
# Required terraform outputs:
#   cluster_name, region, vpc_id, oidc_issuer,
#   state_bucket_name,
#   cluster_role_name, node_role_name, irsa_backend_role_name,
#   backend_service_account  (e.g. "control-plane:backend")
#
# Exit codes: 0 = all pass, 1 = one or more failures.

set -u -o pipefail

# Stop Git Bash from rewriting Unix-style paths (e.g. /readyz, /aws/eks/...)
# into Windows paths before they reach native .exe binaries. No-op on
# Linux/macOS.
export MSYS_NO_PATHCONV=1
export MSYS2_ARG_CONV_EXCL='*'

INFRA_DIR="${INFRA_DIR:-infrastructure}"
FAILURES=0
CHECKS=0

red()    { printf '\033[31m%s\033[0m\n' "$*"; }
green()  { printf '\033[32m%s\033[0m\n' "$*"; }
yellow() { printf '\033[33m%s\033[0m\n' "$*"; }

pass() { CHECKS=$((CHECKS+1)); green  "  PASS  $*"; }
fail() { CHECKS=$((CHECKS+1)); FAILURES=$((FAILURES+1)); red "  FAIL  $*"; }
skip() { yellow "  SKIP  $*"; }
section() { printf '\n== %s ==\n' "$*"; }

# assert_eq <actual> <expected> <message>
assert_eq() {
  if [[ "$1" == "$2" ]]; then pass "$3 (= $2)"
  else fail "$3 (got '$1', expected '$2')"; fi
}

# assert_nonempty <value> <message>
assert_nonempty() {
  if [[ -n "${1// /}" && "$1" != "null" ]]; then pass "$2"
  else fail "$2 (empty/null)"; fi
}

need() {
  command -v "$1" >/dev/null 2>&1 || { red "missing dependency: $1"; exit 2; }
}
need aws; need jq; need terraform; need kubectl

tfout() { terraform -chdir="$INFRA_DIR" output -raw "$1" 2>/dev/null; }

CLUSTER=$(tfout cluster_name)
REGION=$(tfout region)
VPC=$(tfout vpc_id)
OIDC=$(tfout oidc_issuer)
STATE_BUCKET=$(tfout state_bucket_name)
CLUSTER_ROLE=$(tfout cluster_role_name)
NODE_ROLE=$(tfout node_role_name)
IRSA_ROLE=$(tfout irsa_backend_role_name)
BACKEND_SA=$(tfout backend_service_account)  # "namespace:serviceaccount"

: "${CLUSTER:?missing terraform output cluster_name}"
: "${REGION:?missing terraform output region}"
: "${VPC:?missing terraform output vpc_id}"

export AWS_REGION="$REGION"

# ---------------------------------------------------------------------------
section "1.1 Remote state bootstrap"
# ---------------------------------------------------------------------------
if [[ -n "${STATE_BUCKET:-}" ]]; then
  v=$(aws s3api get-bucket-versioning --bucket "$STATE_BUCKET" --query Status --output text 2>/dev/null || echo "")
  assert_eq "$v" "Enabled" "state bucket versioning"

  enc=$(aws s3api get-bucket-encryption --bucket "$STATE_BUCKET" \
        --query 'ServerSideEncryptionConfiguration.Rules[0].ApplyServerSideEncryptionByDefault.SSEAlgorithm' \
        --output text 2>/dev/null || echo "")
  assert_nonempty "$enc" "state bucket has SSE configured"

  pab=$(aws s3api get-public-access-block --bucket "$STATE_BUCKET" \
        --query 'PublicAccessBlockConfiguration.[BlockPublicAcls,IgnorePublicAcls,BlockPublicPolicy,RestrictPublicBuckets]' \
        --output text 2>/dev/null || echo "")
  assert_eq "$pab" $'True\tTrue\tTrue\tTrue' "state bucket public access fully blocked"
else
  skip "state_bucket_name output not set"
fi

# State locking is S3-native (use_lockfile = true in backend.hcl) — no
# DynamoDB table to verify. Lock contention manifests as a transient .tflock
# object in the state bucket itself.

# ---------------------------------------------------------------------------
section "1.3 VPC module"
# ---------------------------------------------------------------------------
SUBNETS_JSON=$(aws ec2 describe-subnets --filters "Name=vpc-id,Values=$VPC" --output json)

total=$(echo "$SUBNETS_JSON" | jq '.Subnets | length')
assert_eq "$total" "4" "subnet count"

azs=$(echo "$SUBNETS_JSON" | jq -r '[.Subnets[].AvailabilityZone] | unique | length')
assert_eq "$azs" "2" "subnet AZ spread"

public=$(echo "$SUBNETS_JSON" | jq '[.Subnets[] | select(.Tags[]? | select(.Key=="kubernetes.io/role/elb" and .Value=="1"))] | length')
assert_eq "$public" "2" "public subnets tagged kubernetes.io/role/elb=1"

private=$(echo "$SUBNETS_JSON" | jq '[.Subnets[] | select(.Tags[]? | select(.Key=="kubernetes.io/role/internal-elb" and .Value=="1"))] | length')
assert_eq "$private" "2" "private subnets tagged kubernetes.io/role/internal-elb=1"

igw=$(aws ec2 describe-internet-gateways --filters "Name=attachment.vpc-id,Values=$VPC" \
      --query 'InternetGateways | length(@)' --output text)
assert_eq "$igw" "1" "one internet gateway attached"

nat=$(aws ec2 describe-nat-gateways --filter "Name=vpc-id,Values=$VPC" \
      --query 'NatGateways[?State==`available`] | length(@)' --output text)
[[ "$nat" -ge 1 ]] && pass "at least one NAT gateway available ($nat)" \
                   || fail "no NAT gateway available"

# ---------------------------------------------------------------------------
section "1.4 EKS cluster + OIDC"
# ---------------------------------------------------------------------------
CLUSTER_JSON=$(aws eks describe-cluster --name "$CLUSTER" --output json 2>/dev/null || echo "{}")

status=$(echo "$CLUSTER_JSON" | jq -r '.cluster.status // "MISSING"')
assert_eq "$status" "ACTIVE" "cluster status"

issuer=$(echo "$CLUSTER_JSON" | jq -r '.cluster.identity.oidc.issuer // empty')
assert_nonempty "$issuer" "OIDC issuer present on cluster"

# IAM OIDC provider for this issuer must exist (IRSA depends on it)
oidc_host="${issuer#https://}"
oidc_arn=$(aws iam list-open-id-connect-providers \
           --query "OpenIDConnectProviderList[?contains(Arn, \`$oidc_host\`)].Arn | [0]" --output text)
assert_nonempty "$oidc_arn" "IAM OIDC provider registered for cluster issuer"

priv=$(echo "$CLUSTER_JSON" | jq -r '.cluster.resourcesVpcConfig.endpointPrivateAccess')
assert_eq "$priv" "true" "cluster private endpoint enabled"

# Managed node group on private subnets
NG_NAME=$(aws eks list-nodegroups --cluster-name "$CLUSTER" \
          --query 'nodegroups[0]' --output text)
assert_nonempty "$NG_NAME" "at least one managed node group"

if [[ "$NG_NAME" != "None" && -n "$NG_NAME" ]]; then
  NG_JSON=$(aws eks describe-nodegroup --cluster-name "$CLUSTER" --nodegroup-name "$NG_NAME" --output json)
  ng_subnets=$(echo "$NG_JSON" | jq -r '.nodegroup.subnets[]' | tr -d '\r' | sort -u)
  private_ids=$(echo "$SUBNETS_JSON" | jq -r '.Subnets[] | select(.Tags[]? | select(.Key=="kubernetes.io/role/internal-elb" and .Value=="1")) | .SubnetId' | tr -d '\r' | sort -u)
  extra=$(comm -23 <(echo "$ng_subnets") <(echo "$private_ids"))
  if [[ -z "$extra" ]]; then
    pass "node group subnets are all private"
  else
    fail "node group contains a non-private subnet (offending: $(echo $extra))"
  fi
fi

# ---------------------------------------------------------------------------
section "1.5 IAM least-privilege"
# ---------------------------------------------------------------------------
has_policy() {
  local role="$1" policy="$2"
  aws iam list-attached-role-policies --role-name "$role" \
    --query "AttachedPolicies[?PolicyName=='$policy'] | length(@)" --output text 2>/dev/null
}

if [[ -n "${CLUSTER_ROLE:-}" ]]; then
  assert_eq "$(has_policy "$CLUSTER_ROLE" AmazonEKSClusterPolicy)" "1" \
            "cluster role has AmazonEKSClusterPolicy"
fi

if [[ -n "${NODE_ROLE:-}" ]]; then
  for p in AmazonEKSWorkerNodePolicy AmazonEC2ContainerRegistryReadOnly AmazonEKS_CNI_Policy; do
    assert_eq "$(has_policy "$NODE_ROLE" "$p")" "1" "node role has $p"
  done
fi

if [[ -n "${IRSA_ROLE:-}" && -n "${BACKEND_SA:-}" ]]; then
  trust=$(aws iam get-role --role-name "$IRSA_ROLE" \
          --query 'Role.AssumeRolePolicyDocument' --output json)

  # Federated principal must be the cluster's OIDC provider
  fed=$(echo "$trust" | jq -r '.Statement[].Principal.Federated' | head -1)
  [[ "$fed" == *"$oidc_host"* ]] && pass "IRSA role trust federated on cluster OIDC" \
                                 || fail "IRSA role trust not bound to cluster OIDC ($fed)"

  # Subject condition must pin to the backend service account
  sub=$(echo "$trust" | jq -r '.. | .["StringEquals"]? // empty | to_entries[] | select(.key | endswith(":sub")) | .value')
  expected="system:serviceaccount:${BACKEND_SA}"
  [[ "$sub" == "$expected" ]] && pass "IRSA trust sub pinned to $expected" \
                              || fail "IRSA trust sub = '$sub', expected '$expected'"

  # Inline policy must not use Resource:"*" on broad actions
  stmts=$(aws iam list-role-policies --role-name "$IRSA_ROLE" --query 'PolicyNames[]' --output text)
  broad=0
  for name in $stmts; do
    doc=$(aws iam get-role-policy --role-name "$IRSA_ROLE" --policy-name "$name" \
          --query 'PolicyDocument' --output json)
    # flag any statement with Action containing "*" AND Resource == "*"
    if echo "$doc" | jq -e '.Statement[] | select((.Action|tostring)|contains("*")) | select(.Resource=="*" or (.Resource|type=="array" and any(.[]; .=="*")))' >/dev/null; then
      broad=1
      fail "IRSA inline policy '$name' uses wildcard action + Resource:*"
    fi
  done
  [[ $broad -eq 0 ]] && pass "IRSA inline policies avoid wildcard Action + Resource:*"
fi

# ---------------------------------------------------------------------------
section "Cluster reachability (exit criterion)"
# ---------------------------------------------------------------------------
aws eks update-kubeconfig --name "$CLUSTER" --region "$REGION" >/dev/null
# Poll briefly: apiserver poststart hooks can lag a few seconds behind ACTIVE.
readyz_ok=0
last_err=""
for _ in $(seq 1 12); do
  if out=$(kubectl get --raw=/readyz 2>&1); then readyz_ok=1; break; fi
  last_err="$out"
  sleep 5
done
if [[ $readyz_ok -eq 1 ]]; then
  pass "control plane /readyz"
else
  fail "control plane /readyz not ready after 60s — last error: ${last_err}"
fi

ready_nodes=$(kubectl get nodes -o json | jq '[.items[] | select(.status.conditions[]? | select(.type=="Ready" and .status=="True"))] | length')
[[ "$ready_nodes" -ge 2 ]] && pass "at least 2 Ready nodes ($ready_nodes)" \
                           || fail "fewer than 2 Ready nodes ($ready_nodes)"

# ---------------------------------------------------------------------------
printf '\n'
if [[ $FAILURES -eq 0 ]]; then
  green "All $CHECKS assertions passed."
  exit 0
else
  red "$FAILURES of $CHECKS assertions failed."
  exit 1
fi
