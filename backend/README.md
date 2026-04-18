# Backend

FastAPI service that hosts the agent runtime and the guardrailed execution layer.

## Layout

- `app/api/` — HTTP routes
- `app/agents/` — Planner and Validator agents (Claude Agent SDK)
- `app/guardrails/` — Policy checks and input validation
- `app/kubernetes/` — Kubernetes client wrapper
- `app/terraform/` — Terraform CLI wrapper (read-only)
- `app/models/` — Pydantic schemas
- `app/core/` — Config, logging, settings
- `tests/` — Unit, integration, and agent evaluations
