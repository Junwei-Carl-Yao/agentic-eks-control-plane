// Phase 2.2 — read-only cluster queries.
package kubernetes

import (
	"context"
	"errors"
	"testing"
	"time"
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

// Scenario: bare nodes (no labels, no allocatable, no Ready condition) still
// list — the DTO leaves the topology/capacity fields zero rather than failing.
func TestListNodes_BareNodeStillLists(t *testing.T) {
	kubeClient := newFakeClient(t, withNode("ip-10-0-0-1"), withNode("ip-10-0-0-2"))
	nodes, err := kubeClient.ListNodes(context.Background())
	if err != nil || len(nodes) != 2 {
		t.Fatalf("ListNodes: (%v, %v)", nodes, err)
	}
	for _, node := range nodes {
		if node.Name == "" {
			t.Errorf("node missing name: %+v", node)
		}
		if node.Ready {
			t.Errorf("bare node should not report ready: %+v", node)
		}
	}
}

// Scenario: detailed nodes → ListNodes carries zone, instance type, pod
// capacity, CPU/memory capacity, and Ready through to the DTO. These are the
// fields the operator console reads to render zones, capacity bars, and node
// status without synthesizing values on the frontend.
func TestListNodes_ExposesTopologyAndCapacity(t *testing.T) {
	kubeClient := newFakeClient(t,
		withNodeDetailed("ip-10-0-0-1", "us-east-1a", "m5.xlarge", 58, "4", "16Gi", true),
	)
	nodes, err := kubeClient.ListNodes(context.Background())
	if err != nil || len(nodes) != 1 {
		t.Fatalf("ListNodes: (%v, %v)", nodes, err)
	}
	node := nodes[0]
	if node.Zone != "us-east-1a" {
		t.Errorf("zone = %q, want us-east-1a", node.Zone)
	}
	if node.InstanceType != "m5.xlarge" {
		t.Errorf("instanceType = %q, want m5.xlarge", node.InstanceType)
	}
	if node.PodCapacity != 58 {
		t.Errorf("podCapacity = %d, want 58", node.PodCapacity)
	}
	if node.CPUCapacity != "4" || node.MemoryCapacity != "16Gi" {
		t.Errorf("cpu/mem = %q/%q, want 4/16Gi", node.CPUCapacity, node.MemoryCapacity)
	}
	if !node.Ready {
		t.Errorf("ready = false, want true")
	}
}

// Scenario: metrics-server reports usage for a node → ListNodes returns the
// utilization as a 0..1 fraction of allocatable. We check the fraction is in
// the expected range (1500m / 4 cores ≈ 0.375; 8Gi / 16Gi = 0.5) rather than
// asserting an exact float, since AsApproximateFloat64 is, well, approximate.
func TestListNodes_MergesMetricsServerUsage(t *testing.T) {
	kubeClient := newFakeClient(t,
		withNodeDetailed("ip-10-0-0-1", "us-east-1a", "m5.xlarge", 58, "4", "16Gi", true),
		withNodeMetrics("ip-10-0-0-1", "1500m", "8Gi"),
	)
	nodes, err := kubeClient.ListNodes(context.Background())
	if err != nil || len(nodes) != 1 {
		t.Fatalf("ListNodes: (%v, %v)", nodes, err)
	}
	if got := nodes[0].CPUUsage; got < 0.37 || got > 0.38 {
		t.Errorf("cpuUsage = %f, want ~0.375", got)
	}
	if got := nodes[0].MemoryUsage; got < 0.49 || got > 0.51 {
		t.Errorf("memoryUsage = %f, want ~0.5", got)
	}
}

// Scenario: no metrics record for a node → usage fields stay at zero, list
// still succeeds. metrics-server is optional; missing data should never sink
// the read.
func TestListNodes_MetricsMissingLeavesUsageZero(t *testing.T) {
	kubeClient := newFakeClient(t,
		withNodeDetailed("ip-10-0-0-1", "us-east-1a", "m5.xlarge", 58, "4", "16Gi", true),
	)
	nodes, err := kubeClient.ListNodes(context.Background())
	if err != nil || len(nodes) != 1 {
		t.Fatalf("ListNodes: (%v, %v)", nodes, err)
	}
	if nodes[0].CPUUsage != 0 || nodes[0].MemoryUsage != 0 {
		t.Errorf("usage = %f/%f, want 0/0", nodes[0].CPUUsage, nodes[0].MemoryUsage)
	}
}

// Scenario: pod has no limits or requests → host node's allocatable is used
// as the fallback denominator. 500m / 4 cores ≈ 0.125 cpu; 2Gi / 16Gi = 0.125
// mem. This is the "BestEffort QoS" path — at least operators see something
// instead of NaN for unbounded pods.
func TestListPods_UnboundedPodFallsBackToHostAllocatable(t *testing.T) {
	created := time.Now().Add(-1 * time.Hour).UTC()
	kubeClient := newFakeClient(t,
		withNodeDetailed("ip-10-0-0-1", "us-east-1a", "m5.xlarge", 58, "4", "16Gi", true),
		withScheduledPod("app", "web-1", "ip-10-0-0-1", 0, created),
		withPodMetrics("app", "web-1", "500m", "2Gi"),
	)
	pods, err := kubeClient.ListPods(context.Background(), "app", "")
	if err != nil || len(pods) != 1 {
		t.Fatalf("ListPods: (%v, %v)", pods, err)
	}
	if got := pods[0].CPUUsage; got < 0.12 || got > 0.13 {
		t.Errorf("cpuUsage = %f, want ~0.125", got)
	}
	if got := pods[0].MemoryUsage; got < 0.12 || got > 0.13 {
		t.Errorf("memoryUsage = %f, want ~0.125", got)
	}
}

// Scenario: pod sets container limits → those (not host allocatable) are the
// denominator. 50m usage against a 100m CPU limit = 0.5; 64Mi against 128Mi
// memory limit = 0.5. This is the change that makes "real" demo loads
// visible on the bar instead of rounding to zero.
func TestListPods_LimitsDriveTheBarDenominator(t *testing.T) {
	created := time.Now().Add(-1 * time.Hour).UTC()
	kubeClient := newFakeClient(t,
		withNodeDetailed("ip-10-0-0-1", "us-east-1a", "m5.xlarge", 58, "4", "16Gi", true),
		withScheduledPodLimits("app", "web-1", "ip-10-0-0-1", created, "100m", "128Mi", "10m", "32Mi"),
		withPodMetrics("app", "web-1", "50m", "64Mi"),
	)
	pods, err := kubeClient.ListPods(context.Background(), "app", "")
	if err != nil || len(pods) != 1 {
		t.Fatalf("ListPods: (%v, %v)", pods, err)
	}
	if got := pods[0].CPUUsage; got < 0.49 || got > 0.51 {
		t.Errorf("cpuUsage = %f, want ~0.5 (limits-relative)", got)
	}
	if got := pods[0].MemoryUsage; got < 0.49 || got > 0.51 {
		t.Errorf("memoryUsage = %f, want ~0.5 (limits-relative)", got)
	}
}

// Scenario: pod has requests but no limits → requests are the denominator
// (one tier softer than limits, still tighter than host allocatable).
func TestListPods_RequestsUsedWhenLimitsAbsent(t *testing.T) {
	created := time.Now().Add(-1 * time.Hour).UTC()
	kubeClient := newFakeClient(t,
		withNodeDetailed("ip-10-0-0-1", "us-east-1a", "m5.xlarge", 58, "4", "16Gi", true),
		withScheduledPodLimits("app", "web-1", "ip-10-0-0-1", created, "", "", "200m", "256Mi"),
		withPodMetrics("app", "web-1", "50m", "64Mi"),
	)
	pods, err := kubeClient.ListPods(context.Background(), "app", "")
	if err != nil || len(pods) != 1 {
		t.Fatalf("ListPods: (%v, %v)", pods, err)
	}
	// 50m / 200m = 0.25; 64Mi / 256Mi = 0.25.
	if got := pods[0].CPUUsage; got < 0.24 || got > 0.26 {
		t.Errorf("cpuUsage = %f, want ~0.25 (requests-relative)", got)
	}
	if got := pods[0].MemoryUsage; got < 0.24 || got > 0.26 {
		t.Errorf("memoryUsage = %f, want ~0.25 (requests-relative)", got)
	}
}

// Scenario: pods now carry the scheduling and restart info the UI needs to
// place them on nodes without hashing their names.
func TestListPods_CarriesSchedulingAndRestarts(t *testing.T) {
	created := time.Now().Add(-3 * time.Hour).UTC()
	kubeClient := newFakeClient(t,
		withScheduledPod("app", "web-1", "ip-10-0-0-1", 7, created),
	)
	pods, err := kubeClient.ListPods(context.Background(), "app", "")
	if err != nil || len(pods) != 1 {
		t.Fatalf("ListPods: (%v, %v)", pods, err)
	}
	pod := pods[0]
	if pod.NodeName != "ip-10-0-0-1" {
		t.Errorf("nodeName = %q, want ip-10-0-0-1", pod.NodeName)
	}
	if pod.RestartCount != 7 {
		t.Errorf("restartCount = %d, want 7", pod.RestartCount)
	}
	if !pod.CreatedAt.Equal(created) {
		t.Errorf("createdAt = %v, want %v", pod.CreatedAt, created)
	}
}

// Scenario: ClusterInfo returns the configured name/region verbatim and a
// healthy flag derived from the apiserver discovery probe.
func TestClusterInfo_ReturnsConfiguredIdentityAndHealth(t *testing.T) {
	kubeClient := newFakeClient(t)
	info, err := kubeClient.ClusterInfo(context.Background(), "eks-demo", "us-east-1")
	if err != nil {
		t.Fatalf("ClusterInfo: %v", err)
	}
	if info.Name != "eks-demo" || info.Region != "us-east-1" {
		t.Errorf("identity = %+v, want eks-demo/us-east-1", info)
	}
	if !info.Healthy {
		t.Errorf("fake clientset should report healthy")
	}
}

// Scenario: probe fails → ClusterInfo collapses the error into Healthy=false
// and still returns the configured identity. The probe error is NOT surfaced
// as a function-level error; ClusterInfo is total. This lets the UI keep
// rendering the cluster name/region even when the apiserver is unreachable.
func TestClusterInfo_UnhealthyWhenProbeFails(t *testing.T) {
	kubeClient := newFakeClient(t, withUnhealthyProbe(errors.New("apiserver unreachable")))
	info, err := kubeClient.ClusterInfo(context.Background(), "n", "r")
	if err != nil {
		t.Fatalf("ClusterInfo returned err = %v, want nil (probe error must collapse into Healthy)", err)
	}
	if info.Name != "n" || info.Region != "r" {
		t.Errorf("identity = %+v, want n/r", info)
	}
	if info.Healthy {
		t.Errorf("Healthy = true, want false when probe fails")
	}
}

// Scenario: default fake (no probe error) → ClusterHealth reports healthy.
// Verifies the new lightweight endpoint reuses the same seam ClusterInfo does
// and that the default in-memory probe is happy-path.
func TestClusterHealth_HealthyByDefault(t *testing.T) {
	kubeClient := newFakeClient(t)
	health, err := kubeClient.ClusterHealth(context.Background())
	if err != nil {
		t.Fatalf("ClusterHealth: %v", err)
	}
	if !health.Healthy {
		t.Errorf("Healthy = false, want true on default fake probe")
	}
}

// Scenario: probe fails → ClusterHealth flips Healthy to false but still
// returns nil function-level error. The probe error is collapsed into the
// flag the same way ClusterInfo collapses it; callers must read Healthy, not
// the error, to decide whether the apiserver answered.
func TestClusterHealth_UnhealthyWhenProbeFails(t *testing.T) {
	kubeClient := newFakeClient(t, withUnhealthyProbe(errors.New("apiserver unreachable")))
	health, err := kubeClient.ClusterHealth(context.Background())
	if err != nil {
		t.Fatalf("ClusterHealth returned err = %v, want nil (probe error must collapse into Healthy)", err)
	}
	if health.Healthy {
		t.Errorf("Healthy = true, want false when probe fails")
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
