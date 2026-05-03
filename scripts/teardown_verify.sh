#!/usr/bin/env bash
# Phase 1 teardown verification.
#
# Run AFTER `make destroy`. Scans AWS for resources that Terraform doesn't
# manage directly but that EKS + in-cluster controllers commonly create — the
# stuff that leaks and keeps the VPC from being deletable next time.
#
# Required inputs (env or tfvars):
#   CLUSTER_NAME   — name of the cluster that was destroyed
#   AWS_REGION     — region it lived in
#   STATE_BUCKET   — expected to STILL exist (bootstrap, not managed by apply)
#
# Exit codes: 0 = clean teardown, 1 = orphans found.

set -u -o pipefail

# Git Bash / MSYS rewrites arguments that look like Unix paths (e.g.
# "/aws/eks/foo") into Windows paths before handing them to native .exe
# binaries like aws.exe. That silently corrupts log-group prefixes, IAM
# paths, and any other leading-slash identifiers we pass to the AWS CLI.
export MSYS_NO_PATHCONV=1
export MSYS2_ARG_CONV_EXCL='*'

INFRA_DIR="${INFRA_DIR:-infrastructure}"
TF_ENV="${TF_ENV:-dev}"
TFVARS="$INFRA_DIR/envs/$TF_ENV/terraform.tfvars"

# Pull cluster_name / region from tfvars if not already in env.
read_tfvar() { grep -E "^[[:space:]]*$1[[:space:]]*=" "$TFVARS" 2>/dev/null | sed -E 's/.*=[[:space:]]*"([^"]+)".*/\1/' | head -1; }
: "${CLUSTER_NAME:=$(read_tfvar cluster_name)}"
: "${AWS_REGION:=$(read_tfvar region)}"
: "${CLUSTER_NAME:?set CLUSTER_NAME or cluster_name in $TFVARS}"
: "${AWS_REGION:?set AWS_REGION or region in $TFVARS}"
export AWS_REGION

need() { command -v "$1" >/dev/null 2>&1 || { echo "missing: $1" >&2; exit 2; }; }
need aws; need jq; need terraform

ORPHANS=0
CHECKS=0
red()   { printf '\033[31m%s\033[0m\n' "$*"; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
yellow(){ printf '\033[33m%s\033[0m\n' "$*"; }

ok()      { CHECKS=$((CHECKS+1)); green "  OK      $*"; }
orphan()  { CHECKS=$((CHECKS+1)); ORPHANS=$((ORPHANS+1)); red "  ORPHAN  $*"; }
section() { printf '\n== %s ==\n' "$*"; }

# count_or_orphan <label> <count> [<detail>]
count_or_orphan() {
  local label="$1" n="$2" detail="${3:-}"
  if [[ "$n" -eq 0 ]]; then ok "$label: 0"
  else orphan "$label: $n${detail:+  [$detail]}"; fi
}

CLUSTER_TAG="kubernetes.io/cluster/$CLUSTER_NAME"

# ---------------------------------------------------------------------------
section "Terraform state"
# ---------------------------------------------------------------------------
# After destroy, state should list zero managed resources.
state_count=$(terraform -chdir="$INFRA_DIR" state list 2>/dev/null | grep -c . || true)
count_or_orphan "terraform state entries" "$state_count"

# ---------------------------------------------------------------------------
section "EKS + cluster-tagged resources"
# ---------------------------------------------------------------------------
# Cluster itself
if aws eks describe-cluster --name "$CLUSTER_NAME" >/dev/null 2>&1; then
  orphan "EKS cluster $CLUSTER_NAME still exists"
else
  ok "EKS cluster gone"
fi

# Node groups (redundant if cluster is gone, but catches the half-destroy case)
ng=$(aws eks list-nodegroups --cluster-name "$CLUSTER_NAME" --query 'nodegroups | length(@)' --output text 2>/dev/null || echo 0)
count_or_orphan "node groups" "${ng:-0}"

# EC2 instances still tagged to the cluster
inst=$(aws ec2 describe-instances \
       --filters "Name=tag-key,Values=$CLUSTER_TAG" "Name=instance-state-name,Values=pending,running,stopping,stopped" \
       --query 'Reservations[].Instances[].InstanceId' --output text)
n=$(wc -w <<<"$inst"); count_or_orphan "EC2 instances tagged $CLUSTER_TAG" "$n" "$inst"

# EBS volumes from PVCs (CSI-created, cluster-tagged)
vols=$(aws ec2 describe-volumes --filters "Name=tag-key,Values=$CLUSTER_TAG" \
       --query 'Volumes[].VolumeId' --output text)
n=$(wc -w <<<"$vols"); count_or_orphan "EBS volumes tagged $CLUSTER_TAG" "$n" "$vols"

# Snapshots owned by us referencing the cluster
snaps=$(aws ec2 describe-snapshots --owner-ids self \
        --filters "Name=tag-key,Values=$CLUSTER_TAG" \
        --query 'Snapshots[].SnapshotId' --output text)
n=$(wc -w <<<"$snaps"); count_or_orphan "EBS snapshots tagged $CLUSTER_TAG" "$n" "$snaps"

# ---------------------------------------------------------------------------
section "Load balancers + target groups (in-cluster controllers)"
# ---------------------------------------------------------------------------
# ALBs/NLBs from aws-load-balancer-controller are tagged
# elbv2.k8s.aws/cluster=<cluster-name>. These are the #1 cause of stuck
# VPC deletes: the controller's finalizer never ran because the cluster
# was destroyed first.
lbs=$(aws elbv2 describe-load-balancers --output json \
      | jq -r --arg c "$CLUSTER_NAME" '.LoadBalancers[].LoadBalancerArn' \
      | while read -r arn; do
          tags=$(aws elbv2 describe-tags --resource-arns "$arn" --output json 2>/dev/null)
          match=$(echo "$tags" | jq -r --arg c "$CLUSTER_NAME" \
                  '.TagDescriptions[0].Tags[] | select(.Key=="elbv2.k8s.aws/cluster" and .Value==$c) | .Value')
          [[ -n "$match" ]] && echo "$arn"
        done)
n=$(wc -w <<<"$lbs"); count_or_orphan "load balancers from LBC" "$n" "$lbs"

# Classic ELBs — scan by tag (no describe-tags filter, have to list then tag-get)
elbs=$(aws elb describe-load-balancers --query 'LoadBalancerDescriptions[].LoadBalancerName' --output text)
elb_orphans=""
for name in $elbs; do
  t=$(aws elb describe-tags --load-balancer-names "$name" --output json 2>/dev/null \
      | jq -r --arg c "$CLUSTER_TAG" '.TagDescriptions[0].Tags[]? | select(.Key==$c) | .Value')
  [[ -n "$t" ]] && elb_orphans="$elb_orphans $name"
done
n=$(wc -w <<<"$elb_orphans"); count_or_orphan "classic ELBs tagged $CLUSTER_TAG" "$n" "$elb_orphans"

# Target groups
tgs=$(aws elbv2 describe-target-groups --output json \
      | jq -r '.TargetGroups[].TargetGroupArn' \
      | while read -r arn; do
          t=$(aws elbv2 describe-tags --resource-arns "$arn" --output json 2>/dev/null \
              | jq -r --arg c "$CLUSTER_NAME" '.TagDescriptions[0].Tags[] | select(.Key=="elbv2.k8s.aws/cluster" and .Value==$c) | .Value')
          [[ -n "$t" ]] && echo "$arn"
        done)
n=$(wc -w <<<"$tgs"); count_or_orphan "target groups from LBC" "$n" "$tgs"

# ---------------------------------------------------------------------------
section "VPC leftovers"
# ---------------------------------------------------------------------------
# If the VPC is gone, great. If it's still there, enumerate what's holding it.
vpcs=$(aws ec2 describe-vpcs --filters "Name=tag:Name,Values=${CLUSTER_NAME}*" \
       --query 'Vpcs[].VpcId' --output text)
if [[ -z "$vpcs" || "$vpcs" == "None" ]]; then
  ok "no VPC tagged with cluster name"
else
  for v in $vpcs; do
    orphan "VPC $v still exists — checking what's holding it"

    enis=$(aws ec2 describe-network-interfaces --filters "Name=vpc-id,Values=$v" \
           --query 'NetworkInterfaces[].NetworkInterfaceId' --output text)
    [[ -n "$enis" ]] && orphan "  ENIs in $v: $enis" || ok "  no ENIs in $v"

    nats=$(aws ec2 describe-nat-gateways --filter "Name=vpc-id,Values=$v" \
           --query 'NatGateways[?State!=`deleted`].NatGatewayId' --output text)
    [[ -n "$nats" ]] && orphan "  NAT gateways in $v: $nats" || ok "  no NAT gateways in $v"

    # Security groups other than the default "default" SG
    sgs=$(aws ec2 describe-security-groups --filters "Name=vpc-id,Values=$v" \
          --query 'SecurityGroups[?GroupName!=`default`].GroupId' --output text)
    [[ -n "$sgs" ]] && orphan "  non-default SGs in $v: $sgs" || ok "  only default SG in $v"
  done
fi

# Unassociated Elastic IPs that belong to this cluster (released EIPs leave no
# trace; unreleased ones have no association). Scope by Name tag to avoid
# flagging unrelated idle EIPs in the account/region.
eips=$(aws ec2 describe-addresses \
       --filters "Name=tag:Name,Values=${CLUSTER_NAME}-*" \
       --query 'Addresses[?AssociationId==null].AllocationId' --output text)
n=$(wc -w <<<"$eips"); count_or_orphan "unassociated Elastic IPs in region" "$n" "$eips"

# ---------------------------------------------------------------------------
section "IAM + OIDC"
# ---------------------------------------------------------------------------
# IAM OIDC provider for the destroyed cluster. EKS cluster deletion does NOT
# delete this — the Terraform iam module must (and we verify it did).
oidc_leftovers=$(aws iam list-open-id-connect-providers \
                 --query 'OpenIDConnectProviderList[].Arn' --output text \
  | tr '\t' '\n' | while read -r arn; do
      [[ -z "$arn" ]] && continue
      tags=$(aws iam list-open-id-connect-provider-tags --open-id-connect-provider-arn "$arn" 2>/dev/null \
             | jq -r --arg c "$CLUSTER_NAME" '.Tags[]? | select(.Value==$c) | .Value')
      [[ -n "$tags" ]] && echo "$arn"
    done)
n=$(wc -w <<<"$oidc_leftovers"); count_or_orphan "IAM OIDC providers tagged with cluster" "$n" "$oidc_leftovers"

# IAM roles commonly named for the cluster (cluster-, node-, irsa-*)
role_leftovers=$(aws iam list-roles \
                 --query "Roles[?contains(RoleName, '$CLUSTER_NAME')].RoleName" --output text)
n=$(wc -w <<<"$role_leftovers"); count_or_orphan "IAM roles referencing cluster name" "$n" "$role_leftovers"

# ---------------------------------------------------------------------------
section "CloudWatch log groups"
# ---------------------------------------------------------------------------
# EKS control plane logs land at /aws/eks/<cluster>/cluster. Cluster delete
# does NOT delete log groups unless the module does it explicitly.
lg=$(aws logs describe-log-groups --log-group-name-prefix "/aws/eks/$CLUSTER_NAME/" \
     --query 'logGroups[].logGroupName' --output text)
n=$(wc -w <<<"$lg"); count_or_orphan "/aws/eks/$CLUSTER_NAME/* log groups" "$n" "$lg"

# ---------------------------------------------------------------------------
section "Bootstrap resources (should STILL exist)"
# ---------------------------------------------------------------------------
if [[ -n "${STATE_BUCKET:-}" ]]; then
  if aws s3api head-bucket --bucket "$STATE_BUCKET" 2>/dev/null; then
    ok "state bucket $STATE_BUCKET present"
  else
    orphan "state bucket $STATE_BUCKET missing — destroy should never remove it"
  fi
fi
# State locking is S3-native (use_lockfile = true) — no DynamoDB table to verify.

# ---------------------------------------------------------------------------
printf '\n'
if [[ $ORPHANS -eq 0 ]]; then
  green "Clean teardown — $CHECKS checks, 0 orphans."
  exit 0
else
  red "$ORPHANS orphan(s) found across $CHECKS checks."
  yellow "Common causes:"
  yellow "  • in-cluster Service type=LoadBalancer / Ingress not deleted before destroy"
  yellow "  • PVCs not deleted → EBS volumes + snapshots linger"
  yellow "  • LBC / external-dns controllers destroyed with cluster → their finalizers never ran"
  exit 1
fi
