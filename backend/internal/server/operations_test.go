// Phase 2.5 - mutation routes.
// Phase 2 wires routes -> operation calls. Phase 3 inserts the enforcer between
// them. These tests cover dispatch + structural validation; they do NOT test policy.
package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Scenario: POST /api/operations/scale with valid JSON body -> calls Scale and returns 200.
func TestScaleRoute_DispatchesToOperation(t *testing.T) {
	operationsStub := &stubOps{}
	handler := newTestHandlerWithOps(operationsStub)
	responseRecorder := httptest.NewRecorder()
	requestBody := `{"namespace":"app","name":"web","replicas":3}`
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodPost, "/api/operations/scale", strings.NewReader(requestBody)))
	if responseRecorder.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", responseRecorder.Code, responseRecorder.Body.String())
	}
	if operationsStub.scaleCalls != 1 || operationsStub.lastScaleReplicas != 3 {
		t.Errorf("ops calls = %+v", operationsStub)
	}
}

// Scenario: malformed JSON -> 400, no operation invoked.
func TestScaleRoute_MalformedBodyIs400(t *testing.T) {
	operationsStub := &stubOps{}
	handler := newTestHandlerWithOps(operationsStub)
	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodPost, "/api/operations/scale", strings.NewReader("{not json")))
	if responseRecorder.Code != 400 || operationsStub.scaleCalls != 0 {
		t.Errorf("status=%d calls=%d, want 400/0", responseRecorder.Code, operationsStub.scaleCalls)
	}
}

// Scenario: body fails models.Validate -> 400, no operation invoked.
func TestScaleRoute_ValidationFailureIs400(t *testing.T) {
	operationsStub := &stubOps{}
	handler := newTestHandlerWithOps(operationsStub)
	responseRecorder := httptest.NewRecorder()
	emptyNamespaceBody := `{"namespace":"","name":"web","replicas":3}` // empty namespace fails Validate
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodPost, "/api/operations/scale", strings.NewReader(emptyNamespaceBody)))
	if responseRecorder.Code != 400 || operationsStub.scaleCalls != 0 {
		t.Errorf("status=%d calls=%d, want 400/0", responseRecorder.Code, operationsStub.scaleCalls)
	}
}

// Scenario: a valid object followed by trailing JSON tokens -> 400 and no dispatch.
func TestScaleRoute_TrailingTokensIs400(t *testing.T) {
	operationsStub := &stubOps{}
	handler := newTestHandlerWithOps(operationsStub)
	responseRecorder := httptest.NewRecorder()
	requestBody := `{"namespace":"app","name":"web","replicas":3}{"extra":true}`
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodPost, "/api/operations/scale", strings.NewReader(requestBody)))
	if responseRecorder.Code != 400 || operationsStub.scaleCalls != 0 {
		t.Errorf("status=%d calls=%d, want 400/0", responseRecorder.Code, operationsStub.scaleCalls)
	}
}

// Scenario: GET on a mutation route -> 405. Mutations are POST-only.
func TestMutationRoutes_RejectNonPost(t *testing.T) {
	handler := newTestHandlerWithOps(&stubOps{})
	for _, mutationRoutePath := range []string{
		"/api/operations/scale",
		"/api/operations/rollout-restart",
		"/api/operations/pause-rollout",
		"/api/operations/resume-rollout",
		"/api/operations/rollback",
		"/api/operations/update-env",
	} {
		responseRecorder := httptest.NewRecorder()
		handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, mutationRoutePath, nil))
		if responseRecorder.Code != 405 {
			t.Errorf("%s GET status = %d, want 405", mutationRoutePath, responseRecorder.Code)
		}
	}
}

// Scenario: rollout-restart route dispatches to RolloutRestart with the right ns/name.
func TestRolloutRestartRoute_Dispatches(t *testing.T) {
	operationsStub := &stubOps{}
	handler := newTestHandlerWithOps(operationsStub)
	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodPost, "/api/operations/rollout-restart",
		strings.NewReader(`{"namespace":"app","name":"web"}`)))
	if responseRecorder.Code != 200 || operationsStub.lastRestart != "app/web" {
		t.Errorf("status=%d last=%q", responseRecorder.Code, operationsStub.lastRestart)
	}
}

// Scenario: pause + resume routes dispatch to their respective ops.
func TestPauseAndResumeRoutes_Dispatch(t *testing.T) {
	operationsStub := &stubOps{}
	handler := newTestHandlerWithOps(operationsStub)
	requestBody := `{"namespace":"app","name":"web"}`
	for _, pauseOrResumePath := range []string{"/api/operations/pause-rollout", "/api/operations/resume-rollout"} {
		responseRecorder := httptest.NewRecorder()
		handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodPost, pauseOrResumePath, strings.NewReader(requestBody)))
		if responseRecorder.Code != 200 {
			t.Errorf("%s status = %d, want 200", pauseOrResumePath, responseRecorder.Code)
		}
	}
	if !operationsStub.paused || !operationsStub.resumed {
		t.Errorf("paused=%v resumed=%v", operationsStub.paused, operationsStub.resumed)
	}
}

// Scenario: rollback route accepts revision; omitted revision means "previous".
func TestRollbackRoute_OmittedRevisionMeansPrevious(t *testing.T) {
	operationsStub := &stubOps{}
	handler := newTestHandlerWithOps(operationsStub)
	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodPost, "/api/operations/rollback",
		strings.NewReader(`{"namespace":"app","name":"web"}`)))
	if responseRecorder.Code != 200 || operationsStub.lastRollbackRevision != 0 {
		t.Errorf("status=%d revision=%d, want 200/0", responseRecorder.Code, operationsStub.lastRollbackRevision)
	}
}

// Scenario: update-env route dispatches with container + env map; envFrom is not in
// the request schema at all (can't be set via this API).
func TestUpdateEnvRoute_Dispatches(t *testing.T) {
	operationsStub := &stubOps{}
	handler := newTestHandlerWithOps(operationsStub)
	requestBody := `{"namespace":"app","name":"web","container":"app","env":{"FOO":"bar"}}`
	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodPost, "/api/operations/update-env", strings.NewReader(requestBody)))
	if responseRecorder.Code != 200 || operationsStub.lastEnv["FOO"] != "bar" {
		t.Errorf("status=%d env=%v", responseRecorder.Code, operationsStub.lastEnv)
	}
}

// Scenario: K8s layer surfaces IsNotFound -> route returns 404 (not 500).
func TestMutationRoutes_NotFoundMapsTo404(t *testing.T) {
	operationsStub := &stubOps{notFound: true}
	handler := newTestHandlerWithOps(operationsStub)
	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodPost, "/api/operations/scale",
		strings.NewReader(`{"namespace":"app","name":"ghost","replicas":1}`)))
	if responseRecorder.Code != 404 {
		t.Errorf("status = %d, want 404", responseRecorder.Code)
	}
}
