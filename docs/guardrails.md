# Guardrails

Authoritative reference for what the backend allows, what it blocks, and how each request is checked. The implementation lives in `backend/internal/guardrails/`; this document is the contract those packages enforce.

## Enforcement contract

Every read and write handler in `backend/internal/server/` follows the same pipeline:

1. Parse the structural input (query string for reads, JSON body for writes).
2. Call an `Enforcer` method.
3. If the `Decision` is `Allow`, dispatch to the Kubernetes layer. If `Deny`, return the decision in the response body and stop.

The `Enforcer` is the single chokepoint. Routes never reach `client-go` without an `Allow`. There is no fallback path, no admin escape hatch, and no env-driven widening — the `Policy` is sealed by value at construction in `guardrails.New` and the bounds (`MinReplicas`, `MaxReplicas`) are package constants set at compile time.

Two routes are exceptions by design:
- `GET /api/cluster/namespaces` **narrows** rather than denies: it lists the cluster's namespaces and filters down to those on the allowlist via `Enforcer.NamespaceAllowed`. Useful for the UI; never exposes anything the allowlist hides.

## Policy

Hardcoded in `guardrails/policy.go`:

| Setting | Value | Notes |
| --- | --- | --- |
| `AllowedNamespaces` | `["control-plane"]` | Empty list is default-deny. Widening requires a code change and rebuild. |
| `MaxReplicas` | `10` | Hard upper bound for `Scale`. |
| `MinReplicas` | `2` | Hard lower bound for `Scale`. Keeps a rolling restart from dropping the app to zero available pods. |
| `RollbackImageFloors` | `{"agent": 6, "backend": 4, "frontend": 4}` | Rollback targeting one of these Deployments must result in an image whose tag parses as `v<int>` and is at or above the floor. |

Tests construct their own `Policy`; the production binary uses `DefaultPolicy()`.

## Validation rules

In `guardrails/validation.go`:

- **DNS-1123 label** for namespace, deployment name, pod name, container name: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`, max 63 chars. This is the *label* form (no dots), which is what the apiserver actually accepts for Deployments, Pods, ConfigMaps, and Namespaces. Validating the laxer DNS-1123 *subdomain* form here would let names through that the apiserver then rejects.
- **Empty namespace** is rejected explicitly. An empty value would otherwise default to `default` downstream, which is not on the allowlist — failing here gives a clearer error.
- **Replica bounds**: `MinReplicas <= replicas <= MaxReplicas`, both inclusive. Duplicated from the K8s ops layer so the audit log records the reason code.
- **Revision**: `revision >= 0`. `0` means "previous revision" for `Rollback`. Existence in the deployment's revision history is checked at the K8s ops layer where the ReplicaSet list is already loaded.

## Operations

### Allowed reads

| Route | Enforcer method | Checks |
| --- | --- | --- |
| `GET /api/cluster/deployments` | `ListDeployments(ns)` | DNS-1123 + namespace allowlist |
| `GET /api/cluster/deployments/{name}` | `GetDeployment(ns, name)` | DNS-1123 (ns, name) + namespace allowlist |
| `GET /api/cluster/pods` | `ListPods(ns)` | DNS-1123 + namespace allowlist |
| `GET /api/cluster/events` | `ListEvents(ns)` | DNS-1123 + namespace allowlist |
| `GET /api/cluster/logs` | `TailLogs(ns, pod, container)` | DNS-1123 (ns, pod, container) + namespace allowlist |
| `GET /api/cluster/services` | `ListServices(ns)` | DNS-1123 + namespace allowlist |
| `GET /api/cluster/ingresses` | `ListIngresses(ns)` | DNS-1123 + namespace allowlist |
| `GET /api/cluster/hpas` | `ListHorizontalPodAutoscalers(ns)` | DNS-1123 + namespace allowlist |
| `GET /api/cluster/replicasets` | `ListReplicaSets(ns)` | DNS-1123 + namespace allowlist |
| `GET /api/cluster/namespaces` | `NamespaceAllowed(ns)` per item | Narrows the cluster list to the allowlist; never denies |
| `GET /api/cluster/nodes` | (none) | Reader returns names only |

### Allowed writes

| Route | Enforcer method | Checks beyond DNS-1123 + allowlist |
| --- | --- | --- |
| `POST /api/operations/scale` | `Scale(req)` | `MinReplicas <= replicas <= MaxReplicas` |
| `POST /api/operations/rollout-restart` | `RolloutRestart(req)` | — |
| `POST /api/operations/pause-rollout` | `PauseRollout(req)` | — |
| `POST /api/operations/resume-rollout` | `ResumeRollout(req)` | — |
| `POST /api/operations/rollback` | `Rollback(req)` | `revision >= 0`; image tag at or above the floor for deployments named `agent`/`backend`/`frontend` |

### Blocked, by design

The backend exposes no route for any of these — there is no tool to call, no handler to reach, no enforcer method that could ever return `Allow`:

- Delete: namespaces, PVCs, Deployments, Pods, Services, Ingresses, ConfigMaps, ReplicaSets.
- Read or modify: Secrets (no route, no client method, no logging path).
- RBAC: Roles, ClusterRoles, RoleBindings, ClusterRoleBindings, ServiceAccount tokens.
- `exec` / `attach` / `port-forward` into pods.
- Cluster-level: node mutations, cordoning/draining, CRDs, admission webhooks, API server config.
- Workload kinds outside Deployments (StatefulSets, DaemonSets, Jobs, CronJobs are not in the operation surface).

The backend's IRSA role and ClusterRole (Phase 6.1) are scoped to the same surface, so a bypass at the HTTP layer would still hit an apiserver `Forbidden` before doing anything destructive. The enforcer is the first line, the ClusterRole is the second.

## Audit and observability

Every `Enforce` call — Allow or Deny — emits a structured log entry and returns a `Decision`:

```go
type Decision struct {
    Allow   bool   `json:"allow"`
    Action  string `json:"action"`   // "scale", "rollout-restart", "list-pods", ...
    Subject string `json:"subject"`  // "control-plane/web" or "control-plane/web-7f:nginx" for logs
    Reason  string `json:"reason,omitempty"`
}
```

- Allow: returned in the response body alongside the operation result so the UI can show what was decided.
- Deny: returned in the response body with the reason; the route does not invoke the Kubernetes layer.
- Log line: `guardrail.decision` with `allow`, `action`, `subject`, `reason` (via `slog`). The deny reason is never silently dropped.

## Invariants the test suite locks in

The unit tests in `backend/internal/guardrails/` and the route tests in `backend/internal/server/` lock the following:

- A route handler that bypasses `Enforce` is detected by route-level tests asserting denies short-circuit the dispatcher.
- Widening the `Policy` via a slice mutation on the caller's copy does not affect the running enforcer (the slice is copied in `New`).
- A request with a valid namespace but invalid DNS-1123 name denies on validation, not on allowlist.
- A request for an off-allowlist namespace denies even when every other field is valid.
- `Scale` with `replicas < MinReplicas` or `replicas > MaxReplicas` denies even when the namespace and name are valid.
- `Rollback` with `revision < 0` denies before the K8s ops layer is consulted.
