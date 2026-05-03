// Phase 2.5 — read-only cluster routes.
// Asserts route shape, query-param parsing, and error mapping. The actual K8s reads
// are exercised in internal/kubernetes; here we mock the reads layer.
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
