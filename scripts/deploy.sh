#!/usr/bin/env bash
# Phase 6 deploy:
#   1. Install the AWS Load Balancer Controller and wait for it to be Ready
#      (6.4) — without this the Ingress in 6.5 sits unreconciled.
#   2. helm upgrade --install the backend / agent / frontend charts (6.1–6.3).
#   3. kubectl apply the ALB Ingress (6.5) and resolve its hostname.
#   4. Re-render the backend with CORS_ORIGINS pinned to the ALB hostname so
#      cross-origin requests from the dashboard aren't dropped.
#
# Required Terraform outputs (from `make apply`):
#   cluster_name, region, vpc_id, irsa_backend_role_arn, irsa_lbc_role_arn
#
# Image tags:
#   IMAGE_TAG          — fallback for every component (default: dev)
#   BACKEND_IMAGE_TAG  — overrides IMAGE_TAG for backend only
#   AGENT_IMAGE_TAG    — overrides IMAGE_TAG for agent only
#   FRONTEND_IMAGE_TAG — overrides IMAGE_TAG for frontend only
#
# Other knobs:
#   REPLICA_COUNT — preserve a manually-scaled deployment across upgrade.
#                   Unset = use chart default (2).

set -euo pipefail

export MSYS_NO_PATHCONV=1
export MSYS2_ARG_CONV_EXCL='*'

INFRA_DIR="${INFRA_DIR:-infrastructure}"
HELM_DIR="${HELM_DIR:-deploy/helm}"
INGRESS_FILE="${INGRESS_FILE:-deploy/ingress/alb-ingress.yaml}"
NAMESPACE="${NAMESPACE:-control-plane}"
IMAGE_REGISTRY="${IMAGE_REGISTRY:-ghcr.io/your-org}"
IMAGE_TAG="${IMAGE_TAG:-dev}"
LBC_CHART_VERSION="${LBC_CHART_VERSION:-1.13.4}"

# Per-component image tag overrides. If unset, each falls back to IMAGE_TAG.
# Lets a redeploy bump only one component, e.g. `BACKEND_IMAGE_TAG=v5 make deploy`.
BACKEND_IMAGE_TAG="${BACKEND_IMAGE_TAG:-$IMAGE_TAG}"
AGENT_IMAGE_TAG="${AGENT_IMAGE_TAG:-$IMAGE_TAG}"
FRONTEND_IMAGE_TAG="${FRONTEND_IMAGE_TAG:-$IMAGE_TAG}"

# REPLICA_COUNT overrides the chart default (2) for every app chart. Set to
# preserve a manually-scaled deployment across a helm upgrade — without it,
# helm would attempt to scale back to the chart default and conflict with
# the `server` field manager that owns `.spec.replicas` after `kubectl scale`.
REPLICA_COUNT="${REPLICA_COUNT:-}"
REPLICA_FLAG=()
if [[ -n "$REPLICA_COUNT" ]]; then
  REPLICA_FLAG=(--set "replicaCount=$REPLICA_COUNT")
fi

# helm 4's --force-conflicts makes server-side apply claim ownership of any
# field that a non-helm manager (kubectl scale, kubectl set image, a prior
# direct PATCH from the agent's rollout-restart path) currently owns.
# Without this, a `make deploy` against a cluster that has been touched out
# of band fails with a CONFLICT response from kube-apiserver. The flag only
# affects fields the chart explicitly sets, so unrelated field ownership is
# preserved.
HELM_SSA_FLAGS=(--force-conflicts)

need() { command -v "$1" >/dev/null 2>&1 || { echo "missing dependency: $1" >&2; exit 2; }; }
need aws; need kubectl; need helm; need terraform

tfout() { terraform -chdir="$INFRA_DIR" output -raw "$1" 2>/dev/null; }

CLUSTER="$(tfout cluster_name)"
REGION="$(tfout region)"
VPC_ID="$(tfout vpc_id)"
BACKEND_ROLE_ARN="$(tfout irsa_backend_role_arn)"
LBC_ROLE_ARN="$(tfout irsa_lbc_role_arn)"
CERT_ARN="$(tfout certificate_arn || true)"
APP_URL="$(tfout app_url || true)"

: "${CLUSTER:?missing terraform output cluster_name}"
: "${REGION:?missing terraform output region}"
: "${BACKEND_ROLE_ARN:?missing terraform output irsa_backend_role_arn}"
: "${LBC_ROLE_ARN:?missing terraform output irsa_lbc_role_arn}"

# The Ingress YAML (deploy/ingress/alb-ingress.yaml) references __CERT_ARN__
# and __HOST__ placeholders that get substituted from Terraform outputs at
# apply time. Both must be set — if domain_name is empty in tfvars, the
# outputs come back blank and we bail rather than ship a half-templated
# manifest the apiserver would reject on the certificate-arn annotation.
: "${CERT_ARN:?missing terraform output certificate_arn — set domain_name, subdomain, hosted_zone_id in envs/$TF_ENV/terraform.tfvars and re-run terraform apply}"
: "${APP_URL:?missing terraform output app_url — set domain_name, subdomain, hosted_zone_id in envs/$TF_ENV/terraform.tfvars and re-run terraform apply}"
APP_HOST="${APP_URL#https://}"

echo "==> Updating kubeconfig for $CLUSTER ($REGION)"
aws eks update-kubeconfig --name "$CLUSTER" --region "$REGION" >/dev/null

kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

echo "==> Installing AWS Load Balancer Controller (6.4)"
helm repo add eks https://aws.github.io/eks-charts >/dev/null 2>&1 || true
helm repo update eks >/dev/null
helm upgrade --install aws-load-balancer-controller eks/aws-load-balancer-controller \
  --namespace kube-system \
  --version "$LBC_CHART_VERSION" \
  --set clusterName="$CLUSTER" \
  --set region="$REGION" \
  --set vpcId="$VPC_ID" \
  --set serviceAccount.create=true \
  --set serviceAccount.name=aws-load-balancer-controller \
  --set "serviceAccount.annotations.eks\.amazonaws\.com/role-arn=$LBC_ROLE_ARN" \
  --wait --timeout 5m

# helm --wait blocks on pod readiness, but the Available condition is what
# the spec gates on. Double-check before applying the Ingress.
kubectl -n kube-system rollout status deploy/aws-load-balancer-controller --timeout=180s

echo "==> Installing metrics-server"
# EKS does not ship metrics-server; without it the backend's metrics.k8s.io
# RBAC is dead weight and `kubectl top` returns NotFound. Installed via the
# upstream chart into kube-system so cluster autoscaling decisions and the
# frontend's pod/node metrics panels both work.
helm repo add metrics-server https://kubernetes-sigs.github.io/metrics-server/ >/dev/null 2>&1 || true
helm repo update metrics-server >/dev/null
helm upgrade --install metrics-server metrics-server/metrics-server \
  --namespace kube-system \
  --wait --timeout 5m

echo "==> Installing backend (6.1) at tag $BACKEND_IMAGE_TAG"
helm upgrade --install backend "$HELM_DIR/backend" \
  --namespace "$NAMESPACE" \
  "${HELM_SSA_FLAGS[@]}" \
  "${REPLICA_FLAG[@]}" \
  --set image.repository="$IMAGE_REGISTRY/eks-control-plane-backend" \
  --set image.tag="$BACKEND_IMAGE_TAG" \
  --set serviceAccount.roleArn="$BACKEND_ROLE_ARN" \
  --set config.awsRegion="$REGION" \
  --set config.clusterName="$CLUSTER" \
  --wait --timeout 5m

echo "==> Installing agent (6.2) at tag $AGENT_IMAGE_TAG"
# The Anthropic API key Secret (agent-anthropic) must already exist in
# $NAMESPACE; the chart references it but never creates it.
helm upgrade --install agent "$HELM_DIR/agent" \
  --namespace "$NAMESPACE" \
  "${HELM_SSA_FLAGS[@]}" \
  "${REPLICA_FLAG[@]}" \
  --set image.repository="$IMAGE_REGISTRY/eks-control-plane-agent" \
  --set image.tag="$AGENT_IMAGE_TAG" \
  --wait --timeout 5m

echo "==> Installing frontend (6.3) at tag $FRONTEND_IMAGE_TAG"
helm upgrade --install frontend "$HELM_DIR/frontend" \
  --namespace "$NAMESPACE" \
  "${HELM_SSA_FLAGS[@]}" \
  "${REPLICA_FLAG[@]}" \
  --set image.repository="$IMAGE_REGISTRY/eks-control-plane-frontend" \
  --set image.tag="$FRONTEND_IMAGE_TAG" \
  --wait --timeout 5m

echo "==> Applying ALB Ingress (6.5) with cert $CERT_ARN and host $APP_HOST"
INGRESS_RENDERED="$(mktemp)"
CORS_VALUES="$(mktemp)"
trap 'rm -f "$INGRESS_RENDERED" "$CORS_VALUES"' EXIT
sed -e "s|__CERT_ARN__|$CERT_ARN|g" -e "s|__HOST__|$APP_HOST|g" "$INGRESS_FILE" > "$INGRESS_RENDERED"
kubectl -n "$NAMESPACE" apply -f "$INGRESS_RENDERED"

echo "==> Waiting for ALB hostname to be assigned"
HOST=""
for _ in $(seq 1 60); do
  HOST="$(kubectl -n "$NAMESPACE" get ingress eks-control-plane \
          -o jsonpath='{.status.loadBalancer.ingress[0].hostname}' 2>/dev/null || true)"
  if [[ -n "$HOST" ]]; then break; fi
  sleep 5
done
if [[ -z "$HOST" ]]; then
  echo "ERROR: Ingress never got an ALB hostname after 5 minutes" >&2
  exit 1
fi
echo "    ALB hostname: $HOST"

echo "==> Re-rendering backend with CORS_ORIGINS=http://$HOST,$APP_URL"
# helm --set splits the value on commas, so a CSV like
# "http://host1,https://host2" would be parsed as two key=value pairs and
# fail. Write to a tmp values file instead — YAML strings don't have that
# problem.
cat > "$CORS_VALUES" <<EOF
config:
  corsOrigins: "http://$HOST,$APP_URL"
EOF
helm upgrade --install backend "$HELM_DIR/backend" \
  --namespace "$NAMESPACE" \
  --reuse-values \
  "${HELM_SSA_FLAGS[@]}" \
  -f "$CORS_VALUES" \
  --wait --timeout 5m

echo ""
echo "ALB hostname (raw): http://$HOST"
echo "Custom URL:         $APP_URL"
echo ""
echo "Phase 3 (if not yet run): write the Route53 alias for $APP_HOST →"
echo "  terraform -chdir=$INFRA_DIR apply -var-file=envs/${TF_ENV:-dev}/terraform.tfvars -var='create_dns_alias=true'"
echo ""
echo "Deploy complete."
