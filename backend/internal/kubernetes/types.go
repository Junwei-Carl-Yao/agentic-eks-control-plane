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
