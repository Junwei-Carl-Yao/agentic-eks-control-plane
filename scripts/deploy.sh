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

need() { command -v "$1" >/dev/null 2>&1 || { echo "missing dependency: $1" >&2; exit 2; }; }
need aws; need kubectl; need helm; need terraform

tfout() { terraform -chdir="$INFRA_DIR" output -raw "$1" 2>/dev/null; }

CLUSTER="$(tfout cluster_name)"
REGION="$(tfout region)"
VPC_ID="$(tfout vpc_id)"
BACKEND_ROLE_ARN="$(tfout irsa_backend_role_arn)"
LBC_ROLE_ARN="$(tfout irsa_lbc_role_arn)"

: "${CLUSTER:?missing terraform output cluster_name}"
: "${REGION:?missing terraform output region}"
: "${BACKEND_ROLE_ARN:?missing terraform output irsa_backend_role_arn}"
: "${LBC_ROLE_ARN:?missing terraform output irsa_lbc_role_arn}"

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

echo "==> Installing backend (6.1)"
helm upgrade --install backend "$HELM_DIR/backend" \
  --namespace "$NAMESPACE" \
  --set image.repository="$IMAGE_REGISTRY/eks-control-plane-backend" \
  --set image.tag="$IMAGE_TAG" \
  --set serviceAccount.roleArn="$BACKEND_ROLE_ARN" \
  --set config.awsRegion="$REGION" \
  --set config.clusterName="$CLUSTER" \
  --wait --timeout 5m

echo "==> Installing agent (6.2)"
# The Anthropic API key Secret (agent-anthropic) must already exist in
# $NAMESPACE; the chart references it but never creates it.
helm upgrade --install agent "$HELM_DIR/agent" \
  --namespace "$NAMESPACE" \
  --set image.repository="$IMAGE_REGISTRY/eks-control-plane-agent" \
  --set image.tag="$IMAGE_TAG" \
  --wait --timeout 5m

echo "==> Installing frontend (6.3)"
helm upgrade --install frontend "$HELM_DIR/frontend" \
  --namespace "$NAMESPACE" \
  --set image.repository="$IMAGE_REGISTRY/eks-control-plane-frontend" \
  --set image.tag="$IMAGE_TAG" \
  --wait --timeout 5m

echo "==> Applying ALB Ingress (6.5)"
kubectl -n "$NAMESPACE" apply -f "$INGRESS_FILE"

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

echo "==> Re-rendering backend with CORS_ORIGINS=http://$HOST"
helm upgrade --install backend "$HELM_DIR/backend" \
  --namespace "$NAMESPACE" \
  --reuse-values \
  --set config.corsOrigins="http://$HOST" \
  --wait --timeout 5m

echo ""
echo "Deploy complete. ALB hostname: http://$HOST"
