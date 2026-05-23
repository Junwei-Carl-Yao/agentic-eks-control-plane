# Agentic EKS Control Plane with Guardrailed AI Operations

## Goal

Design and build an agentic control plane for Amazon EKS that enables natural-language interaction with cluster infrastructure while enforcing strict guardrails on all AI-driven operations.

The system provisions infrastructure via Terraform and exposes a web dashboard where an AI agent performs safe, constrained infrastructure operations such as scaling, rollout restarting, and rolling back workloads.

## Stack

- **Infrastructure:** Terraform + AWS (EKS, VPC, IAM, S3 remote state with native locking)
- **Backend:** Go + Kubernetes client
- **Agent Runtime:** Claude Agent SDK
- **Frontend:** React + Vite + TypeScript
- **Deployment:** Helm, AWS Load Balancer Controller

## Scope

### Core Components

**Control Interface (UI):**
- Web dashboard for issuing natural-language commands and viewing cluster state
- Displays execution results and system feedback

**Agent Runtime (Claude Agent SDK):**
- Single agent that interprets user intent and selects appropriate tools/actions
- Safety is enforced by the backend guardrail layer, not by a separate validator agent
- Uses structured tool interfaces (e.g., scale, rollout restart, pause/resume rollout, rollback)

**Guardrailed Execution Layer (Backend):**
- Acts as the single execution boundary for all operations
- Enforces:
  - Input validation (resource names, namespaces, replica bounds)
  - Policy checks (e.g., max replicas, namespace restrictions)
  - Rejection of disallowed or unsafe operations
- Executes validated actions via the Kubernetes API

**Kubernetes Control Surface:**
- Exposes a constrained set of mutation operations on:
  - Deployments (scale, rollout restart, pause/resume rollout, rollback)
- Read capabilities support decision-making across deployments, pods, events, logs, services, ingresses, horizontal pod autoscalers, namespaces, nodes (names only), and replicasets

**Infrastructure Layer (Terraform):**
- Provisions the EKS cluster and networking (VPC, node groups)
- Maintains remote state with locking (S3 bucket + native S3 conditional-write locking)

### Execution Model

1. User issues a natural-language request via the UI
2. Agent maps the request to a structured tool call
3. Backend enforces validation and executes the action if approved
4. Results (success or failure) are returned to the user

### In Scope

- Guardrailed execution of infrastructure operations (scale, rollout restart, pause/resume rollout, rollback)
- Single-agent architecture using the Claude Agent SDK
- Backend-enforced safety constraints independent of LLM behavior
- Single-cluster EKS control plane

### Out of Scope

- Stateful workloads (StatefulSets, PVCs)
- Advanced Kubernetes extensions (CRDs, operators, service mesh, NetworkPolicies)
- Multi-cluster or multi-environment management
- CI/CD pipelines (focus is runtime operations)

## Agent Permissions

**Allowed:**
- Read cluster state (deployments, pods, events, logs, services, ingresses, horizontal pod autoscalers, namespaces, node names, replicasets)
- Scale deployments
- Rollout restart deployments
- Pause/resume rollout
- Roll back deployments

**Blocked:**
- Delete namespaces, PVCs, or deployments
- Modify or read Secrets
- Modify RBAC
- Exec into pods
- Cluster-level or node-level changes

## Success Criteria

- `make apply` provisions a working EKS cluster with networking and node groups
- Users can issue natural-language commands to perform guardrailed operations (e.g., scale, rollout restart, rollback)
- The backend correctly enforces guardrails and rejects unsafe or disallowed operations
- The backend enforcer is invoked for all operations before execution
- The dashboard reflects live cluster state and operation results
- `make destroy` tears everything down cleanly without orphaned resources
- Comprehensive test coverage and evaluations

## Non-Goals

- Production-grade security hardening (focus is on guardrail design, not full security compliance)
- Multi-tenancy or fine-grained user access control
- Cost optimization beyond basic teardown






