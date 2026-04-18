# Agentic EKS Control Plane

Agentic control plane for Amazon EKS with guardrailed AI operations.

## Structure

- `infrastructure/` — Terraform (EKS, VPC, IAM, S3, DynamoDB)
- `backend/` — FastAPI service (agents, guardrails, K8s/Terraform clients)
- `frontend/` — React + Vite + TypeScript dashboard
- `deploy/` — Helm charts, ALB Ingress manifests
- `scripts/` — Operational scripts
- `docs/` — Design notes

## Quick Start

See `Makefile` for top-level targets (`apply`, `destroy`, `dev`, `test`).
