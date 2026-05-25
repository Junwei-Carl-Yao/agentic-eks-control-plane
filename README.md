# Agentic EKS Control Plane

**Live demo:** https://k8s-agent-demo.carlyao.dev/

Agentic control plane for Amazon EKS with guardrailed AI operations.

A web dashboard lets an operator issue natural-language requests ("scale `agent` to 3 replicas", "restart the `frontend` rollout in `control-plane`"). A single Claude agent maps each request to structured tool calls against a Go backend, which is the only path that touches the Kubernetes API and the place policy is enforced.

## Repository layout

- `infrastructure/` — Terraform (VPC, EKS, IAM, S3 remote state with native locking)
- `backend/` — Go API service: Kubernetes client, guardrail enforcer, HTTP routes
- `agent/` — TypeScript runtime (Claude Agent SDK) exposing backend routes as tools
- `frontend/` — React + Vite + TypeScript dashboard (cluster panel + chat panel)
- `deploy/` — Helm charts (`backend`, `agent`, `frontend`) and the ALB Ingress
- `scripts/` — Bootstrap, deploy, and verify scripts
- `docs/` — Requirements, architecture, guardrails, implementation plan

## Quick start

Provision infrastructure, build and push images, deploy to the cluster:

```
make bootstrap        # S3 state bucket (one-time per account)
make apply            # terraform apply against envs/dev
make backend agent frontend   # build and push container images
make deploy           # helm install all three charts + ALB Ingress
make deploy-verify    # asserts replicas, IRSA annotations, ALB hostname, /health
```

Local dev (no cluster apply needed; backend runs against your current kube context):

```
make dev-backend      # Go API on :8000
cd agent && npm run dev      # agent runtime on :8081
cd frontend && npm run dev   # Vite dev server on :5173
```

Tear down:

```
make destroy
make teardown-verify  # scans AWS for orphans
```

Override the environment with `TF_ENV=<name>` (default `dev`). Override the image registry/tag with `IMAGE_REGISTRY=...` and `IMAGE_TAG=...`. See `make help` for the full target list.

## Documentation

- [`docs/requirement.md`](docs/requirement.md) — goal, scope, agent permissions, success criteria
- [`docs/architecture.md`](docs/architecture.md) — component map, runtime flows, deployment topology
- [`docs/guardrails.md`](docs/guardrails.md) — allowed/blocked operations, policy and validation rules
- [`docs/implementation.md`](docs/implementation.md) — phased build order (Phase 0–6)
