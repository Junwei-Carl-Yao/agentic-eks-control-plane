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
