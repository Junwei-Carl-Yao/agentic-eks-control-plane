package kubernetes

import "time"

// Deployment is the JSON shape returned by reads.
type Deployment struct {
	Name              string                `json:"name"`
	Namespace         string                `json:"namespace"`
	Replicas          int32                 `json:"replicas"`
	AvailableReplicas int32                 `json:"availableReplicas"`
	UpdatedReplicas   int32                 `json:"updatedReplicas"`
	Paused            bool                  `json:"paused"`
	Containers        []DeploymentContainer `json:"containers,omitempty"`
}

// DeploymentContainer is the per-container slice of a Deployment's pod template
// the read DTO exposes — enough for the agent and UI to answer "what image is
// running" without separately fetching pod specs.
type DeploymentContainer struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

// Pod is the JSON shape returned by ListPods. NodeName, RestartCount, and
// CreatedAt are populated so the UI can map pods onto nodes, render restart
// counts, and compute pod age without synthesizing those fields client-side.
// CPUUsage/MemoryUsage are 0..1 fractions of the pod's own ceiling: sum of
// container limits when set, sum of requests otherwise, host allocatable as
// a final fallback for unbounded pods. Zero when metrics-server is missing.
type Pod struct {
	Name         string            `json:"name"`
	Namespace    string            `json:"namespace"`
	Phase        string            `json:"phase"`
	Labels       map[string]string `json:"labels,omitempty"`
	NodeName     string            `json:"nodeName,omitempty"`
	RestartCount int32             `json:"restartCount"`
	CreatedAt    time.Time         `json:"createdAt"`
	CPUUsage     float64           `json:"cpuUsage"`
	MemoryUsage  float64           `json:"memoryUsage"`
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

// Node carries the topology + capacity fields the UI needs to render a real
// cluster map. Addresses and arbitrary labels stay off the wire — those would
// leak topology beyond what the operator console needs. CPUUsage/MemoryUsage
// are 0..1 fractions of allocatable derived from metrics-server; they stay
// zero when metrics-server is unreachable or hasn't reported yet.
type Node struct {
	Name           string  `json:"name"`
	Zone           string  `json:"zone,omitempty"`
	InstanceType   string  `json:"instanceType,omitempty"`
	PodCapacity    int64   `json:"podCapacity"`
	CPUCapacity    string  `json:"cpuCapacity,omitempty"`
	MemoryCapacity string  `json:"memoryCapacity,omitempty"`
	CPUUsage       float64 `json:"cpuUsage"`
	MemoryUsage    float64 `json:"memoryUsage"`
	Ready          bool    `json:"ready"`
}

// ClusterInfo identifies the cluster the backend is talking to. Name and
// Region come from configuration; Healthy is computed from a discovery probe
// so a stale/disconnected cluster is visible to operators at a glance.
type ClusterInfo struct {
	Name    string `json:"name"`
	Region  string `json:"region"`
	Healthy bool   `json:"healthy"`
}

// ClusterHealth is the slim payload returned by GET /api/cluster/health. It
// carries only the live /livez verdict so the UI can poll health on a tight
// cadence without re-fetching the static identity fields on every tick.
type ClusterHealth struct {
	Healthy bool `json:"healthy"`
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
