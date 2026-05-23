# Architecture

How the running system is laid out: components, the path a request follows, and the deployment topology in EKS. For the *why* (goal, scope, success criteria) see `requirement.md`; for the *how it was built* see `implementation.md`.

## Components

```
                       ┌────────────────────────────┐
                       │       Browser (UI)         │
                       │  cluster panel + chat pane │
                       └────────────┬───────────────┘
                                    │  HTTPS
                                    ▼
                       ┌────────────────────────────┐
                       │      ALB Ingress           │
                       │  /api/agent/* → agent      │
                       │  /api/*       → backend    │
                       │  /*           → frontend   │
                       └──┬─────────┬─────────┬─────┘
                          │         │         │
              ┌───────────▼───┐     │       ┌─▼──────────┐
              │ agent runtime │     │       │  frontend  │
              │ (Node)        │     │       │  (nginx)   │
              └─┬───────────┬─┘     │       └────────────┘
                │           │       │
       Anthropic│           │ in-   │ /api/cluster/*
        API SDK│            │ cluster polling
                ▼           │ HTTP  │
        ┌───────────┐       ▼       ▼
        │ Anthropic │     ┌──────────────────────────┐
        │    API    │     │   backend (Go)           │
        └───────────┘     │   routes → enforcer →    │
                          │   kubernetes client      │
                          └────────────┬─────────────┘
                                       │  client-go (IRSA + RBAC)
                                       ▼
                          ┌───────────────────────────┐
                          │     EKS API server        │
                          └───────────────────────────┘
```

Two callers reach the backend: the agent runtime in-cluster (for tool calls during a chat turn) and the browser through the ALB (for the cluster panel's 5s polling). Both paths go through the same handlers and the same enforcer — there is no read shortcut that skips policy.

Component responsibilities:

- **Frontend** (`frontend/`) — Polls the backend read routes every 5s via react-query and renders cluster state. Chat panel `POST`s to the agent runtime and renders the SSE stream live. Owns the conversation transcript; resends it on every turn.
- **Agent runtime** (`agent/`) — Stateless TypeScript service running the Claude Agent SDK. Wraps every backend HTTP route as a structured tool. Streams tool calls, tool results, and assistant text back to the browser as SSE.
- **Backend** (`backend/`) — Go HTTP service. Holds the singleton Kubernetes client, the guardrail enforcer, and the typed mutation operations. Every write route runs the enforcer before calling the Kubernetes API.
- **Infrastructure** (`infrastructure/`) — Terraform provisions the VPC, the EKS cluster + managed node group, and three IAM roles: cluster, node, and backend IRSA (read-only EKS + read-only state bucket). The AWS Load Balancer Controller's IRSA role is provisioned here too so a clean `make apply` leaves the cluster Ingress-ready.

## Runtime flow — write operation

1. Operator types "scale `web` to 3 replicas" into the chat pane.
2. Browser `POST`s `/api/agent/chat` with the full prior transcript + new message.
3. ALB routes to the agent runtime. Runtime invokes the Claude Agent SDK with the registered tool set.
4. Claude picks the `scale` tool with `{namespace, name, replicas}`. The runtime streams a `tool_call` SSE frame.
5. Runtime calls `POST /api/operations/scale` on the backend.
6. Backend handler invokes `guardrails.Enforce(action)`:
   - schema/DNS-1123 validation,
   - namespace allowlist,
   - replica bounds (`MinReplicas`..`MaxReplicas`).
7. On `Allow`, the Kubernetes layer issues the patch via `client-go`. On `Deny`, the handler returns the deny reason without ever touching the API server.
8. Runtime streams a `tool_result` frame (success or deny reason). Claude composes a user-facing reply; runtime streams `text` deltas, then `done`.
9. Browser renders the streamed reply and the next cluster-panel poll reflects the new replica count.

The agent runtime never enforces policy locally — a deny coming back from the backend is authoritative and is surfaced verbatim. This holds even if the model misbehaves: there is no path from a tool call to the apiserver that skips the enforcer.

## Runtime flow — read operation

The cluster panel uses plain react-query polling against the backend read routes (`/api/cluster/deployments`, `/pods`, `/events`, `/services`, `/ingresses`, `/hpas`, `/namespaces`, `/nodes`, `/replicasets`, `/logs`). No agent in the loop. The agent runtime calls those same routes when it needs current context for a chat turn.

## Deployment topology

A single namespace `control-plane` hosts three Deployments (`backend`, `agent`, `frontend`), each at 2 replicas with a `PodDisruptionBudget` of `minAvailable: 1`. One Ingress in front, three path prefixes, one ALB.

- **Backend ServiceAccount** carries the IRSA annotation for AWS API permissions and a `ClusterRole` for the apiserver verbs the read/write paths need (`get/list/watch` on the read resources; `update` on `apps/deployments` for scale/rollout-restart/pause/resume/rollback; `get` on `pods/log`).
- **Agent → backend** traffic stays in-cluster: the agent's `BACKEND_URL` points at `http://backend.control-plane.svc.cluster.local:8000`, never the public ALB.
- **`ANTHROPIC_API_KEY`** is mounted from a Secret into the agent pod only.
- **Termination**: all three Deployments use `terminationGracePeriodSeconds: 30`. The backend's SIGTERM handler caps `httpServer.Shutdown` at 25s; the agent's does the same for in-flight SSE streams; nginx drains via SIGQUIT.

## Key invariants

- The guardrail enforcer is the single chokepoint for every route. Routes call `Enforce` *before* touching `client-go`, and the enforcer rejects on either schema validation or policy.
- The agent runtime is replaceable. Remove the LLM and the backend HTTP API still enforces the same rules — the agent is a UX over the same routes, not a privileged sidecar.
- Secrets are never read or written by the agent path. No tool reads Secrets, the backend exposes no Secret routes, and no Secret data is logged.
- Every deny is observable. The enforcer emits a structured audit log entry on Allow and Deny, and the deny reason rides back in the HTTP response body so the agent (and the UI) can surface it.
- The agent runtime is stateless. The browser owns the transcript and resends it on every turn, so any pod can serve any turn and replicas scale horizontally without session affinity.
