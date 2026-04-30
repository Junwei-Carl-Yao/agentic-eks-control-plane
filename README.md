# Agentic EKS Control Plane

Agentic control plane for Amazon EKS with guardrailed AI operations.

## Structure

- `infrastructure/` — Terraform (EKS, VPC, IAM, S3 remote state with native locking)
- `backend/` — Go API service (guardrails, K8s/Terraform clients)
- `agent-runtime/` — Planner/validator runtime (Claude Agent SDK)
- `frontend/` — React + Vite + TypeScript dashboard
- `deploy/` — Helm charts, ALB Ingress manifests
- `scripts/` — Operational scripts
- `docs/` — Design notes

## Quick Start

See `Makefile` for top-level targets (`apply`, `destroy`, `dev`, `test`).
