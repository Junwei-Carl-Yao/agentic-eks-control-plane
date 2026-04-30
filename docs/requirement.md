# Agentic EKS Control Plane with Guardrailed AI Operations

## Goal

Design and build an agentic control plane for Amazon EKS that enables natural-language interaction with cluster infrastructure while enforcing strict guardrails on all AI-driven operations.

The system provisions infrastructure via Terraform and exposes a web dashboard where an AI agent performs safe, constrained infrastructure operations such as scaling, rollout restarting, and rolling back workloads.

## Stack

- **Infrastructure:** Terraform + AWS (EKS, VPC, IAM, S3 remote state with native locking)
- **Backend:** Go + Kubernetes client
- **Agent Runtime:** Claude Agent SDK
- **Frontend:** React + Vite + TypeScript
- **Deployment:** Helm, ALB Ingress Controller

## Scope

### Core Components

**Control Interface (UI):**
- Web dashboard for issuing natural-language commands and viewing cluster state
- Displays execution results and system feedback

**Agent Runtime (Claude Agent SDK):**
- Two-agent design:
  - **Planner Agent:** Interprets user intent and selects appropriate tools/actions
  - **Validator Agent:** Reviews proposed actions against safety constraints before execution
- Uses structured tool interfaces (e.g., scale, rollout restart, pause/resume rollout, rollback, env update)

**Guardrailed Execution Layer (Backend):**
- Acts as the single execution boundary for all operations
- Enforces:
  - Input validation (resource names, namespaces, replica bounds)
  - Policy checks (e.g., max replicas, namespace restrictions)
  - Rejection of disallowed or unsafe operations
- Executes validated actions via the Kubernetes API or Terraform CLI (read-only)

**Kubernetes Control Surface:**
- Exposes a constrained set of mutation operations on:
  - Deployments (scale, rollout restart, pause/resume rollout, rollback, env update)
- Read capabilities (pods, events, logs) are available to support decision-making

**Infrastructure Layer (Terraform):**
- Provisions the EKS cluster and networking (VPC, node groups)
- Maintains remote state with locking (S3 bucket + native S3 conditional-write locking)
- Supports drift detection via `terraform plan` (read-only)

### Execution Model

1. User issues a natural-language request via the UI
2. Planner Agent maps the request to a structured tool call
3. Validator Agent evaluates the proposed action against guardrails
4. Backend enforces validation and executes the action if approved
5. Results (success or failure) are returned to the user

### In Scope

- Guardrailed execution of infrastructure operations (scale, rollout restart, pause/resume rollout, rollback, env update)
- Two-agent architecture (planner + validator) using the Claude Agent SDK
- Backend-enforced safety constraints independent of LLM behavior
- Read-only Terraform integration (`plan`, `show`, `state`, `output`)
- Single-cluster EKS control plane

### Out of Scope

- Stateful workloads (StatefulSets, PVCs)
- Advanced Kubernetes extensions (CRDs, operators, service mesh, NetworkPolicies)
- Multi-cluster or multi-environment management
- CI/CD pipelines (focus is runtime operations)

## Agent Permissions

**Allowed:**
- Read cluster state
- Read logs and events
- Describe resources
- List resources
- Scale deployments
- Pause/resume rollout
- Roll back deployments
- Update environment variables (guardrailed)
- Run `terraform plan`
- Run Terraform read operations (`show`, `state`, `output`)

**Blocked:**
- `terraform apply` / `terraform destroy`
- Delete namespaces, PVCs, or deployments
- Modify or read Secrets
- Modify RBAC
- Exec into pods
- Cluster-level or node-level changes

## Success Criteria

- `make apply` provisions a working EKS cluster with networking and node groups
- Users can issue natural-language commands to perform guardrailed operations (e.g., scale, rollout restart, rollback)
- The backend correctly enforces guardrails and rejects unsafe or disallowed operations
- The Validator Agent is invoked for all write operations before execution
- The dashboard reflects live cluster state and operation results
- Terraform drift can be inspected via `terraform plan` (read-only)
- `make destroy` tears everything down cleanly without orphaned resources
- Comprehensive test coverage and evaluations

## Non-Goals

- Production-grade security hardening (focus is on guardrail design, not full security compliance)
- Multi-tenancy or fine-grained user access control
- Cost optimization beyond basic teardown


