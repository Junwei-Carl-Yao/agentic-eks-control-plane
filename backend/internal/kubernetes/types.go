package kubernetes

import "time"

// Deployment is the JSON shape returned by reads.
type Deployment struct {
	Name              string `json:"name"`
	Namespace         string `json:"namespace"`
	Replicas          int32  `json:"replicas"`
	AvailableReplicas int32  `json:"availableReplicas"`
	UpdatedReplicas   int32  `json:"updatedReplicas"`
	Paused            bool   `json:"paused"`
}

// Pod is the JSON shape returned by ListPods.
type Pod struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Phase     string            `json:"phase"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// Event is the JSON shape returned by ListEvents.
type Event struct {
	Namespace string    `json:"namespace"`
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
	Type      string    `json:"type"`
	Time      time.Time `json:"time"`
	Object    string    `json:"object,omitempty"`
}

// Service is the JSON shape returned by ListServices. Includes only the fields a
// human or planner agent needs to reason about routing — no annotations, no
// internal selectors, no spec fragments.
type Service struct {
	Name      string        `json:"name"`
	Namespace string        `json:"namespace"`
	Type      string        `json:"type"`
	ClusterIP string        `json:"clusterIP"`
	Ports     []ServicePort `json:"ports,omitempty"`
}

type ServicePort struct {
	Name       string `json:"name,omitempty"`
	Port       int32  `json:"port"`
	TargetPort string `json:"targetPort,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
	NodePort   int32  `json:"nodePort,omitempty"`
}

// Ingress collapses an Ingress's host rules into a flat hostnames list.
type Ingress struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Class     string   `json:"class,omitempty"`
	Hosts     []string `json:"hosts,omitempty"`
}

// HorizontalPodAutoscaler reports min/max/current replicas plus the workload it
// targets.
type HorizontalPodAutoscaler struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	MinReplicas     int32  `json:"minReplicas"`
	MaxReplicas     int32  `json:"maxReplicas"`
	CurrentReplicas int32  `json:"currentReplicas"`
	TargetRef       string `json:"targetRef,omitempty"`
}

// Namespace returns just identity + lifecycle phase.
type Namespace struct {
	Name  string `json:"name"`
	Phase string `json:"phase,omitempty"`
}

// Node is intentionally name-only. Per implementation §2.2 we never expose
// addresses, capacity, or labels — those would leak topology to any caller.
type Node struct {
	Name string `json:"name"`
}

// ReplicaSet carries enough revision info for the planner to map ReplicaSets
// back to a Deployment's rollout history.
type ReplicaSet struct {
	Name              string `json:"name"`
	Namespace         string `json:"namespace"`
	Replicas          int32  `json:"replicas"`
	AvailableReplicas int32  `json:"availableReplicas"`
	Revision          int64  `json:"revision,omitempty"`
	Owner             string `json:"owner,omitempty"`
}
