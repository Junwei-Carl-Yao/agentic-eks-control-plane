#!/usr/bin/env bash
# Phase 6 exit-criteria assertions. Mirrors the shape of
# scripts/infra_assertions.sh so success is a clean exit code.
#
# Checks:
#   - backend, agent, frontend Deployments report availableReplicas == 2
#   - backend ServiceAccount carries irsa_backend_role_arn
#   - LBC ServiceAccount in kube-system carries irsa_lbc_role_arn
#   - Ingress has a non-empty status.loadBalancer.ingress[0].hostname
#   - curl $hostname/health and $hostname/api/agent/health each return 200

set -u -o pipefail

export MSYS_NO_PATHCONV=1
export MSYS2_ARG_CONV_EXCL='*'

INFRA_DIR="${INFRA_DIR:-infrastructure}"
NAMESPACE="${NAMESPACE:-control-plane}"

FAILURES=0
CHECKS=0

red()   { printf '\033[31m%s\033[0m\n' "$*"; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
pass()  { CHECKS=$((CHECKS+1)); green "  PASS  $*"; }
fail()  { CHECKS=$((CHECKS+1)); FAILURES=$((FAILURES+1)); red "  FAIL  $*"; }
section() { printf '\n== %s ==\n' "$*"; }

need() { command -v "$1" >/dev/null 2>&1 || { red "missing dependency: $1"; exit 2; }; }
need aws; need kubectl; need terraform; need curl

tfout() { terraform -chdir="$INFRA_DIR" output -raw "$1" 2>/dev/null; }

CLUSTER="$(tfout cluster_name)"
REGION="$(tfout region)"
BACKEND_ROLE_ARN="$(tfout irsa_backend_role_arn)"
LBC_ROLE_ARN="$(tfout irsa_lbc_role_arn)"

: "${CLUSTER:?missing terraform output cluster_name}"
: "${REGION:?missing terraform output region}"

aws eks update-kubeconfig --name "$CLUSTER" --region "$REGION" >/dev/null

# ---------------------------------------------------------------------------
section "Deployments: availableReplicas == 2"
# ---------------------------------------------------------------------------
for deployment in backend agent frontend; do
  available=$(kubectl -n "$NAMESPACE" get deploy "$deployment" \
              -o jsonpath='{.status.availableReplicas}' 2>/dev/null || echo "")
  if [[ "$available" == "2" ]]; then
    pass "$deployment availableReplicas = 2"
  else
    fail "$deployment availableReplicas = '$available' (want 2)"
  fi
done

# ---------------------------------------------------------------------------
section "ServiceAccount IRSA annotations"
# ---------------------------------------------------------------------------
backend_anno=$(kubectl -n "$NAMESPACE" get sa backend \
               -o jsonpath='{.metadata.annotations.eks\.amazonaws\.com/role-arn}' 2>/dev/null || echo "")
if [[ "$backend_anno" == "$BACKEND_ROLE_ARN" ]]; then
  pass "backend SA role-arn matches Terraform output"
else
  fail "backend SA role-arn = '$backend_anno', want '$BACKEND_ROLE_ARN'"
fi

lbc_anno=$(kubectl -n kube-system get sa aws-load-balancer-controller \
           -o jsonpath='{.metadata.annotations.eks\.amazonaws\.com/role-arn}' 2>/dev/null || echo "")
if [[ "$lbc_anno" == "$LBC_ROLE_ARN" ]]; then
  pass "aws-load-balancer-controller SA role-arn matches Terraform output"
else
  fail "LBC SA role-arn = '$lbc_anno', want '$LBC_ROLE_ARN'"
fi

# ---------------------------------------------------------------------------
section "Ingress + ALB reachability"
# ---------------------------------------------------------------------------
HOST=""
for _ in $(seq 1 12); do
  HOST="$(kubectl -n "$NAMESPACE" get ingress eks-control-plane \
          -o jsonpath='{.status.loadBalancer.ingress[0].hostname}' 2>/dev/null || true)"
  [[ -n "$HOST" ]] && break
  sleep 5
done
if [[ -n "$HOST" ]]; then
  pass "Ingress status.loadBalancer.ingress[0].hostname = $HOST"
else
  fail "Ingress never got a hostname"
fi

poll_http() {
  local url="$1" label="$2"
  local code="000"
  local raw
  for _ in $(seq 1 30); do
    # Git-bash curl can exit non-zero (e.g. CR/LF or transient ALB warm-up)
    # while still printing a usable http_code. Capture both, then keep only
    # the last 3 digits so the comparison never sees concatenated garbage
    # like "200000" or "404000".
    raw=$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 "$url" 2>/dev/null; echo "_$?")
    code="${raw%_*}"
    code="${code//[^0-9]/}"
    code="${code: -3}"
    if [[ "$code" == "200" ]]; then pass "$label -> 200"; return 0; fi
    sleep 5
  done
  fail "$label never returned 200 (last=$code)"
  return 1
}

if [[ -n "$HOST" ]]; then
  poll_http "http://$HOST/health" "GET /health"
  poll_http "http://$HOST/api/agent/health" "GET /api/agent/health"
fi

printf '\n'
if [[ $FAILURES -eq 0 ]]; then
  green "All $CHECKS assertions passed."
  exit 0
else
  red "$FAILURES of $CHECKS assertions failed."
  exit 1
fi
