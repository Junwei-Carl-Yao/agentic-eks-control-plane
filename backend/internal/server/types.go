package server

import (
	"context"

	"eks-control-plane/backend/internal/guardrails"
	"eks-control-plane/backend/internal/kubernetes"
)

// Re-exports of the JSON-shaped DTOs the cluster reads layer returns. Aliases
// keep the route layer's signature self-contained without a second type.
type (
	Deployment              = kubernetes.Deployment
	Pod                     = kubernetes.Pod
	Event                   = kubernetes.Event
	Service                 = kubernetes.Service
	Ingress                 = kubernetes.Ingress
	HorizontalPodAutoscaler = kubernetes.HorizontalPodAutoscaler
	Namespace               = kubernetes.Namespace
	Node                    = kubernetes.Node
	ReplicaSet              = kubernetes.ReplicaSet
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
	ListServices(ctx context.Context, namespace string) ([]Service, error)
	ListIngresses(ctx context.Context, namespace string) ([]Ingress, error)
	ListHorizontalPodAutoscalers(ctx context.Context, namespace string) ([]HorizontalPodAutoscaler, error)
	ListNamespaces(ctx context.Context) ([]Namespace, error)
	ListNodes(ctx context.Context) ([]Node, error)
	GetFeatureFlags(ctx context.Context, namespace, name string) (map[string]string, error)
	ListReplicaSets(ctx context.Context, namespace string) ([]ReplicaSet, error)
}

// Operations is the mutation seam. The Phase 3 enforcer wraps each method on
// behalf of the route layer.
type Operations interface {
	Scale(ctx context.Context, namespace, name string, replicas int32) error
	RolloutRestart(ctx context.Context, namespace, name string) error
	PauseRollout(ctx context.Context, namespace, name string) error
	ResumeRollout(ctx context.Context, namespace, name string) error
	Rollback(ctx context.Context, namespace, name string, revision int64) error
	UpdateFeatureFlag(ctx context.Context, namespace, configMap, key, value string) error
}

// Deps bundles the optional clients the route layer needs. Any nil field
// silently disables that route group, which keeps NewServer composable for
// degraded environments and per-feature tests. Mounting Ops requires a
// non-nil Enforcer — operations may not be exposed without a chokepoint.
type Deps struct {
	Reader   ClusterReader
	Ops      Operations
	Enforcer *guardrails.Enforcer
}
