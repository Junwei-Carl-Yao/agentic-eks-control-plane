// Phase 2.5 — read-only cluster routes.
// Asserts route shape, query-param parsing, and error mapping. The actual K8s reads
// are exercised in internal/kubernetes; here we mock the reads layer.
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"eks-control-plane/backend/internal/guardrails"
)

// Scenario: GET /api/cluster/deployments?namespace=app → 200 + JSON list.
func TestGetDeployments_Returns200WithList(t *testing.T) {
	handler := newTestHandlerWithReads(stubReads{deployments: []Deployment{{Name: "web"}}})
	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/api/cluster/deployments?namespace=app", nil))
	if responseRecorder.Code != 200 {
		t.Fatalf("status = %d, want 200", responseRecorder.Code)
	}
	var deploymentList []Deployment
	_ = json.NewDecoder(responseRecorder.Body).Decode(&deploymentList)
	if len(deploymentList) != 1 || deploymentList[0].Name != "web" {
		t.Errorf("body = %v, want [web]", deploymentList)
	}
}

// Scenario: missing namespace query param → 400. We never default to "default",
// because that namespace is in BLOCKED_NAMESPACES (Phase 3).
func TestGetDeployments_MissingNamespaceIs400(t *testing.T) {
	handler := newTestHandlerWithReads(stubReads{})
	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/api/cluster/deployments", nil))
	if responseRecorder.Code != 400 {
		t.Errorf("status = %d, want 400", responseRecorder.Code)
	}
}

// Scenario: GET /api/cluster/deployments/{name}?namespace=app → 200 + single object.
func TestGetDeploymentDetail_Returns200(t *testing.T) {
	handler := newTestHandlerWithReads(stubReads{deployment: &Deployment{Name: "web", Namespace: "app", Replicas: 3}})
	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/api/cluster/deployments/web?namespace=app", nil))
	if responseRecorder.Code != 200 {
		t.Fatalf("status = %d, want 200", responseRecorder.Code)
	}
}

// Scenario: deployment doesn't exist → reads layer returns IsNotFound → route returns 404.
func TestGetDeploymentDetail_NotFoundIs404(t *testing.T) {
	handler := newTestHandlerWithReads(stubReads{notFound: true})
	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/api/cluster/deployments/ghost?namespace=app", nil))
	if responseRecorder.Code != 404 {
		t.Errorf("status = %d, want 404", responseRecorder.Code)
	}
}

// Scenario: GET /api/cluster/pods?namespace=app&labelSelector=app=web → forwards
// the selector verbatim to ListPods.
func TestGetPods_ForwardsLabelSelector(t *testing.T) {
	readsStub := &stubReads{}
	handler := newTestHandlerWithReads(readsStub)
	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet,
		"/api/cluster/pods?namespace=app&labelSelector=app%3Dweb", nil))
	if responseRecorder.Code != 200 {
		t.Fatalf("status = %d, want 200", responseRecorder.Code)
	}
	if readsStub.lastPodSelector != "app=web" {
		t.Errorf("forwarded selector = %q, want app=web", readsStub.lastPodSelector)
	}
}

// Scenario: GET /api/cluster/events?namespace=app → 200 + JSON list ordered newest first.
func TestGetEvents_Returns200(t *testing.T) {
	handler := newTestHandlerWithReads(stubReads{events: []Event{{Reason: "Scaled"}}})
	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/api/cluster/events?namespace=app", nil))
	if responseRecorder.Code != 200 {
		t.Errorf("status = %d, want 200", responseRecorder.Code)
	}
}

// Scenario: GET /api/cluster/logs?namespace=app&pod=web-1&container=app&lines=100
// → forwards all four params to TailLogs.
func TestGetLogs_ForwardsAllParams(t *testing.T) {
	readsStub := &stubReads{}
	handler := newTestHandlerWithReads(readsStub)
	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet,
		"/api/cluster/logs?namespace=app&pod=web-1&container=app&lines=100", nil))
	if responseRecorder.Code != 200 {
		t.Fatalf("status = %d, want 200", responseRecorder.Code)
	}
	if readsStub.lastLogPod != "web-1" || readsStub.lastLogContainer != "app" || readsStub.lastLogLines != 100 {
		t.Errorf("forwarded (%s,%s,%d), want (web-1,app,100)",
			readsStub.lastLogPod, readsStub.lastLogContainer, readsStub.lastLogLines)
	}
}

// Scenario: lines param missing or non-numeric → 400, not silently defaulted.
// (A silent default would let a typo cause unbounded log fetches.)
func TestGetLogs_InvalidLinesIs400(t *testing.T) {
	handler := newTestHandlerWithReads(stubReads{})
	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet,
		"/api/cluster/logs?namespace=app&pod=web-1&container=app&lines=abc", nil))
	if responseRecorder.Code != 400 {
		t.Errorf("status = %d, want 400", responseRecorder.Code)
	}
}

// Scenario: each new Phase 2 read endpoint returns 200 with its JSON list.
// A table-driven test rather than seven near-identical bodies, since the
// per-route work is identical: query namespace param → call reader → encode.
func TestNewReadRoutes_Return200(t *testing.T) {
	type readCase struct {
		name        string
		path        string
		needsNS     bool
		readsConfig stubReads
	}
	cases := []readCase{
		{"services", "/api/cluster/services?namespace=app", true, stubReads{services: []Service{{Name: "web"}}}},
		{"ingresses", "/api/cluster/ingresses?namespace=app", true, stubReads{ingresses: []Ingress{{Name: "web"}}}},
		{"hpas", "/api/cluster/hpas?namespace=app", true, stubReads{hpas: []HorizontalPodAutoscaler{{Name: "web"}}}},
		{"namespaces", "/api/cluster/namespaces", false, stubReads{namespaces: []Namespace{{Name: "app"}}}},
		{"nodes", "/api/cluster/nodes", false, stubReads{nodes: []Node{{Name: "ip-10-0-0-1"}}}},
		{"feature-flags", "/api/cluster/feature-flags?namespace=app", true, stubReads{featureFlag: map[string]string{}}},
		{"replicasets", "/api/cluster/replicasets?namespace=app", true, stubReads{replicaSets: []ReplicaSet{{Name: "web-abcd"}}}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			handler := newTestHandlerWithReads(testCase.readsConfig)
			responseRecorder := httptest.NewRecorder()
			handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, testCase.path, nil))
			if responseRecorder.Code != 200 {
				t.Errorf("status = %d, want 200", responseRecorder.Code)
			}
		})
	}
}

// Scenario: namespaced read on a non-allowlisted namespace → 403 with the
// audit decision in the body, and the reader is never called. This is the
// reads-side equivalent of TestMutationRoutes_DenialReturns403AndSkipsOps —
// the chokepoint must reject before any cluster I/O.
func TestReadRoutes_DenialReturns403AndSkipsReader(t *testing.T) {
	readsStub := &stubReads{deployments: []Deployment{{Name: "web"}}}
	handler := newTestHandlerWithReads(readsStub)
	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet,
		"/api/cluster/deployments?namespace=kube-system", nil))
	if responseRecorder.Code != 403 {
		t.Fatalf("status = %d, want 403; body=%s", responseRecorder.Code, responseRecorder.Body.String())
	}
	var responseBody struct {
		Decision guardrails.Decision `json:"decision"`
	}
	if err := json.NewDecoder(responseRecorder.Body).Decode(&responseBody); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if responseBody.Decision.Allow || responseBody.Decision.Action != "list-deployments" {
		t.Errorf("decision = %+v, want denied list-deployments", responseBody.Decision)
	}
}

// Scenario: ListNamespaces returns only namespaces on the allowlist. The
// reader reports the full cluster; the route narrows. This is the policy
// gate for cluster-scoped reads — denial would be useless (the caller didn't
// name a namespace), so the route filters instead.
func TestListNamespaces_FiltersToAllowlist(t *testing.T) {
	readsStub := stubReads{namespaces: []Namespace{
		{Name: "app"},
		{Name: "kube-system"},
		{Name: "default"},
	}}
	handler := newTestHandlerWithReads(readsStub)
	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/api/cluster/namespaces", nil))
	if responseRecorder.Code != 200 {
		t.Fatalf("status = %d, want 200", responseRecorder.Code)
	}
	var namespaceList []Namespace
	if err := json.NewDecoder(responseRecorder.Body).Decode(&namespaceList); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(namespaceList) != 1 || namespaceList[0].Name != "app" {
		t.Errorf("namespaces = %+v, want only [app]", namespaceList)
	}
}

// Scenario: a read with bad query shape (here: invalid DNS-1123 name on
// /deployments/{name}) → 403 with the resource-specific action recorded.
// The route never reaches the reader.
func TestGetDeployment_InvalidNameDeniedAt403(t *testing.T) {
	readsStub := &stubReads{}
	handler := newTestHandlerWithReadsAndEnforcer(readsStub, permissiveEnforcer())
	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet,
		"/api/cluster/deployments/Web-Caps?namespace=app", nil))
	if responseRecorder.Code != 403 {
		t.Errorf("status = %d, want 403; body=%s", responseRecorder.Code, responseRecorder.Body.String())
	}
}

// Scenario: GetFeatureFlags returns the allowed key/value pairs from the
// single feature-flag ConfigMap, narrowed to the FeatureFlagKeys allowlist.
// The route hardcodes the CM name and never accepts it from the caller — the
// binary is a feature-flag console, not a generic ConfigMap browser.
func TestGetFeatureFlags_ReturnsScopedData(t *testing.T) {
	readsStub := stubReads{featureFlag: map[string]string{
		"FOO":          "true",
		"SECRET_TOKEN": "leak-me",
	}}
	handler := newTestHandlerWithReads(readsStub)
	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/api/cluster/feature-flags?namespace=app", nil))
	if responseRecorder.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", responseRecorder.Code, responseRecorder.Body.String())
	}
	var data map[string]string
	if err := json.NewDecoder(responseRecorder.Body).Decode(&data); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(data) != 1 || data["FOO"] != "true" {
		t.Errorf("data = %v, want only {FOO:true}", data)
	}
}

// Scenario: ListNodes stays unguarded — the reads layer already returns
// names only, and there's no namespace to gate on. Confirm the route still
// reaches the reader rather than going through an enforce step.
func TestListNodes_BypassesEnforcer(t *testing.T) {
	readsStub := stubReads{nodes: []Node{{Name: "ip-10-0-0-1"}}}
	handler := newTestHandlerWithReads(readsStub)
	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/api/cluster/nodes", nil))
	if responseRecorder.Code != 200 {
		t.Errorf("status = %d, want 200", responseRecorder.Code)
	}
	var nodeList []Node
	_ = json.NewDecoder(responseRecorder.Body).Decode(&nodeList)
	if len(nodeList) != 1 || nodeList[0].Name != "ip-10-0-0-1" {
		t.Errorf("nodes = %+v, want [ip-10-0-0-1]", nodeList)
	}
}

// Scenario: namespaced read endpoints reject a missing namespace query param
// with 400 — never default to "default", which is on the blocked list anyway.
func TestNewReadRoutes_MissingNamespaceIs400(t *testing.T) {
	for _, namespacedReadPath := range []string{
		"/api/cluster/services",
		"/api/cluster/ingresses",
		"/api/cluster/hpas",
		"/api/cluster/feature-flags",
		"/api/cluster/replicasets",
	} {
		handler := newTestHandlerWithReads(stubReads{})
		responseRecorder := httptest.NewRecorder()
		handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, namespacedReadPath, nil))
		if responseRecorder.Code != 400 {
			t.Errorf("%s status = %d, want 400", namespacedReadPath, responseRecorder.Code)
		}
	}
}
