package server

import (
	"context"

	"eks-control-plane/backend/internal/kubernetes"
)

// Re-exports of the JSON-shaped DTOs the cluster reads layer returns. Aliases
// keep the route layer's signature self-contained without a second type.
type (
	Deployment = kubernetes.Deployment
	Pod        = kubernetes.Pod
	Event      = kubernetes.Event
)

// ClusterReader is the read-only seam for the cluster routes. It exists so
// tests can swap in a stub without standing up the K8s fake clientset, and so
// the route layer doesn't import kubernetes directly.
type ClusterReader interface {
	ListDeployments(ctx context.Context, namespace string) ([]Deployment, error)
	GetDeployment(ctx context.Context, namespace, name string) (Deployment, error)
	ListPods(ctx context.Context, namespace, labelSelector string) ([]Pod, error)
	ListEvents(ctx context.Context, namespace string) ([]Event, error)
	TailLogs(ctx context.Context, namespace, pod, container string, lines int64) (string, error)
}

// Operations is the mutation seam. Phase 3 will insert a guardrail enforcer
// between the route layer and the kubernetes.Client implementation.
type Operations interface {
	Scale(ctx context.Context, namespace, name string, replicas int32) error
	RolloutRestart(ctx context.Context, namespace, name string) error
	PauseRollout(ctx context.Context, namespace, name string) error
	ResumeRollout(ctx context.Context, namespace, name string) error
	Rollback(ctx context.Context, namespace, name string, revision int64) error
	UpdateEnv(ctx context.Context, namespace, name, container string, env map[string]string) error
}

// Deps bundles the optional clients the route layer needs. Any nil field
// silently disables that route group, which keeps NewServer composable for
// degraded environments and per-feature tests.
type Deps struct {
	Reader ClusterReader
	Ops    Operations
}
