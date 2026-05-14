// Phase 2.2 — read-only cluster queries.
package kubernetes

import (
	"context"
	"testing"
)

// Scenario: namespace contains N deployments → ListDeployments returns all of them.
func TestListDeployments_ReturnsAllInNamespace(t *testing.T) {
	kubeClient := newFakeClient(t, withDeployments("app", "web", "api", "worker"))
	listedDeployments, err := kubeClient.ListDeployments(context.Background(), "app")
	if err != nil {
		t.Fatalf("ListDeployments: %v", err)
	}
	if len(listedDeployments) != 3 {
		t.Errorf("len = %d, want 3", len(listedDeployments))
	}
}

// Scenario: namespace has no deployments → returns empty slice, no error.
// (We never want a "not found" error on an empty list — only on missing single resource.)
func TestListDeployments_EmptyNamespace(t *testing.T) {
	kubeClient := newFakeClient(t)
	listedDeployments, err := kubeClient.ListDeployments(context.Background(), "app")
	if err != nil || len(listedDeployments) != 0 {
		t.Errorf("got (%v, %v), want ([], nil)", listedDeployments, err)
	}
}

// Scenario: deployment exists → GetDeployment returns it with current replica count + status.
func TestGetDeployment_ReturnsDetail(t *testing.T) {
	kubeClient := newFakeClient(t, withDeployments("app", "web"))
	deployment, err := kubeClient.GetDeployment(context.Background(), "app", "web")
	if err != nil {
		t.Fatalf("GetDeployment: %v", err)
	}
	if deployment.Name != "web" || deployment.Namespace != "app" {
		t.Errorf("got %+v, want web/app", deployment)
	}
}

// Scenario: deployment missing → returns a sentinel error the API layer can map to 404.
func TestGetDeployment_NotFound(t *testing.T) {
	kubeClient := newFakeClient(t)
	_, err := kubeClient.GetDeployment(context.Background(), "app", "ghost")
	if !IsNotFound(err) {
		t.Errorf("err = %v, want IsNotFound", err)
	}
}

// Scenario: label selector provided → ListPods filters server-side and returns only matches.
func TestListPods_FiltersByLabelSelector(t *testing.T) {
	kubeClient := newFakeClient(t,
		withPod("app", "web-1", map[string]string{"app": "web"}),
		withPod("app", "api-1", map[string]string{"app": "api"}),
	)
	matchingPods, err := kubeClient.ListPods(context.Background(), "app", "app=web")
	if err != nil {
		t.Fatalf("ListPods: %v", err)
	}
	if len(matchingPods) != 1 || matchingPods[0].Name != "web-1" {
		t.Errorf("got %+v, want [web-1]", matchingPods)
	}
}

// Scenario: empty selector → returns every pod in the namespace.
func TestListPods_EmptySelectorReturnsAll(t *testing.T) {
	kubeClient := newFakeClient(t,
		withPod("app", "web-1", nil),
		withPod("app", "api-1", nil),
	)
	allPods, _ := kubeClient.ListPods(context.Background(), "app", "")
	if len(allPods) != 2 {
		t.Errorf("len = %d, want 2", len(allPods))
	}
}

// Scenario: events exist → returned newest-first, so the UI's "recent events" panel is correct.
func TestListEvents_OrderedNewestFirst(t *testing.T) {
	kubeClient := newFakeClient(t, withEventsAtMinutes("app", 5, 1, 10))
	listedEvents, err := kubeClient.ListEvents(context.Background(), "app")
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(listedEvents) != 3 || !descending(listedEvents) {
		t.Errorf("events not in descending order: %+v", listedEvents)
	}
}

// Scenario: lines=N → TailLogs requests exactly N tail lines from the API.
func TestTailLogs_RespectsLineLimit(t *testing.T) {
	kubeClient := newFakeClient(t, withPodLogs("app", "web-1", "app", "L1\nL2\nL3\nL4\nL5"))
	tailedLogs, err := kubeClient.TailLogs(context.Background(), "app", "web-1", "app", 2)
	if err != nil {
		t.Fatalf("TailLogs: %v", err)
	}
	if tailedLogs != "L4\nL5" {
		t.Errorf("got %q, want last 2 lines", tailedLogs)
	}
}

// Scenario: pod has multiple containers → container arg selects which one's logs come back.
func TestTailLogs_SelectsNamedContainer(t *testing.T) {
	kubeClient := newFakeClient(t,
		withPodLogs("app", "web-1", "app", "app-out"),
		withPodLogs("app", "web-1", "sidecar", "sidecar-out"),
	)
	sidecarLogs, _ := kubeClient.TailLogs(context.Background(), "app", "web-1", "sidecar", 100)
	if sidecarLogs != "sidecar-out" {
		t.Errorf("got %q, want sidecar-out", sidecarLogs)
	}
}

// Scenario: services seeded → ListServices returns them with port + clusterIP.
func TestListServices_ReturnsServices(t *testing.T) {
	kubeClient := newFakeClient(t, withService("app", "web", 80))
	services, err := kubeClient.ListServices(context.Background(), "app")
	if err != nil || len(services) != 1 {
		t.Fatalf("ListServices: (%v, %v)", services, err)
	}
	if services[0].Name != "web" || services[0].ClusterIP != "10.0.0.1" || len(services[0].Ports) != 1 || services[0].Ports[0].Port != 80 {
		t.Errorf("unexpected service shape: %+v", services[0])
	}
}

// Scenario: ingresses seeded → ListIngresses returns Hosts collapsed from rules.
func TestListIngresses_CollapsesHosts(t *testing.T) {
	kubeClient := newFakeClient(t, withIngress("app", "web", "example.com"))
	ingresses, err := kubeClient.ListIngresses(context.Background(), "app")
	if err != nil || len(ingresses) != 1 {
		t.Fatalf("ListIngresses: (%v, %v)", ingresses, err)
	}
	if ingresses[0].Name != "web" || len(ingresses[0].Hosts) != 1 || ingresses[0].Hosts[0] != "example.com" {
		t.Errorf("unexpected ingress shape: %+v", ingresses[0])
	}
}

// Scenario: HPAs seeded → ListHorizontalPodAutoscalers returns min/max + target.
func TestListHorizontalPodAutoscalers_ReturnsBounds(t *testing.T) {
	kubeClient := newFakeClient(t, withHorizontalPodAutoscaler("app", "web-hpa", "web", 1, 5))
	hpas, err := kubeClient.ListHorizontalPodAutoscalers(context.Background(), "app")
	if err != nil || len(hpas) != 1 {
		t.Fatalf("ListHorizontalPodAutoscalers: (%v, %v)", hpas, err)
	}
	if hpas[0].MinReplicas != 1 || hpas[0].MaxReplicas != 5 || hpas[0].TargetRef != "Deployment/web" {
		t.Errorf("unexpected hpa shape: %+v", hpas[0])
	}
}

// Scenario: namespaces seeded → ListNamespaces returns them with phase populated.
func TestListNamespaces_ReturnsAll(t *testing.T) {
	kubeClient := newFakeClient(t, withNamespace("app"), withNamespace("api"))
	namespaces, err := kubeClient.ListNamespaces(context.Background())
	if err != nil || len(namespaces) != 2 {
		t.Fatalf("ListNamespaces: (%v, %v)", namespaces, err)
	}
}

// Scenario: nodes seeded → ListNodes returns names only. The Phase 2.2 contract
// says we never expose addresses/capacity/labels — guard the contract here so
// a future refactor doesn't accidentally widen the projection.
func TestListNodes_ReturnsNamesOnly(t *testing.T) {
	kubeClient := newFakeClient(t, withNode("ip-10-0-0-1"), withNode("ip-10-0-0-2"))
	nodes, err := kubeClient.ListNodes(context.Background())
	if err != nil || len(nodes) != 2 {
		t.Fatalf("ListNodes: (%v, %v)", nodes, err)
	}
	if nodes[0].Name == "" || nodes[1].Name == "" {
		t.Errorf("nodes missing names: %+v", nodes)
	}
}

// Scenario: replicasets seeded with revision history → ListReplicaSets returns
// each RS's revision and its owning Deployment's name.
func TestListReplicaSets_CarriesRevisionAndOwner(t *testing.T) {
	kubeClient := newFakeClient(t, withRevisionHistory("app", "web", []int64{1, 2}))
	replicaSets, err := kubeClient.ListReplicaSets(context.Background(), "app")
	if err != nil || len(replicaSets) != 2 {
		t.Fatalf("ListReplicaSets: (%v, %v)", replicaSets, err)
	}
	for _, replicaSet := range replicaSets {
		if replicaSet.Owner != "web" {
			t.Errorf("rs %q owner = %q, want web", replicaSet.Name, replicaSet.Owner)
		}
		if replicaSet.Revision != 1 && replicaSet.Revision != 2 {
			t.Errorf("rs %q revision = %d, want 1 or 2", replicaSet.Name, replicaSet.Revision)
		}
	}
}
