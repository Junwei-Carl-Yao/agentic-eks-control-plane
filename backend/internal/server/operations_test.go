// Phase 2.5 + 3 — mutation routes.
// Routes run: decode → models.Validate → guardrails.Enforce → ops dispatch.
// These tests cover dispatch + structural validation + the deny short-circuit.
// Per-action policy semantics (allowlists, MAX_REPLICAS) are exercised in the
// guardrails package; here we just verify that a denial returns 403 and never
// reaches the ops layer.
package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"eks-control-plane/backend/internal/guardrails"
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
		"/api/operations/update-feature-flag",
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

// Scenario: update-feature-flag route dispatches with configmap + key + value.
// The op never touches envFrom or other ConfigMap keys — that invariant lives
// in the kubernetes layer's UpdateFeatureFlag and is exercised there.
func TestUpdateFeatureFlagRoute_Dispatches(t *testing.T) {
	operationsStub := &stubOps{}
	handler := newTestHandlerWithOps(operationsStub)
	requestBody := `{"namespace":"app","configmap":"app-flags","key":"FOO","value":"bar"}`
	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodPost, "/api/operations/update-feature-flag", strings.NewReader(requestBody)))
	if responseRecorder.Code != 200 || operationsStub.lastFlagKey != "FOO" || operationsStub.lastFlagValue != "bar" {
		t.Errorf("status=%d key=%s value=%s", responseRecorder.Code, operationsStub.lastFlagKey, operationsStub.lastFlagValue)
	}
}

// Scenario: a write the enforcer denies (here: namespace not on allowlist) →
// 403 with the audit decision in the body, and the ops layer is never called.
// The denial path is what makes the chokepoint a chokepoint, so this is the
// load-bearing test for Phase 3 wiring.
func TestMutationRoutes_DenialReturns403AndSkipsOps(t *testing.T) {
	operationsStub := &stubOps{}
	// The init override allows only `app`; the request below targets a
	// non-allowlisted namespace.
	enforcer := guardrails.New(testPolicy(), func() (map[string]string, error) { return map[string]string{"MAX_REPLICAS": "10"}, nil }, slog.Default())
	handler := newTestHandlerWithOpsAndEnforcer(operationsStub, enforcer)
	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodPost, "/api/operations/scale",
		strings.NewReader(`{"namespace":"kube-system","name":"web","replicas":1}`)))
	if responseRecorder.Code != 403 {
		t.Fatalf("status = %d, want 403; body=%s", responseRecorder.Code, responseRecorder.Body.String())
	}
	if operationsStub.scaleCalls != 0 {
		t.Errorf("ops.Scale was called %d times despite denial", operationsStub.scaleCalls)
	}
	var responseBody struct {
		Error    string              `json:"error"`
		Decision guardrails.Decision `json:"decision"`
	}
	if err := json.NewDecoder(responseRecorder.Body).Decode(&responseBody); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if responseBody.Decision.Allow || responseBody.Decision.Action != "scale" || responseBody.Decision.Reason == "" {
		t.Errorf("decision = %+v, want denied scale with reason", responseBody.Decision)
	}
}

// Scenario: replicas above MAX_REPLICAS → 403 with a reason naming the cap, so
// the UI can render exactly what was rejected.
func TestScaleRoute_OverMaxReplicasIs403(t *testing.T) {
	operationsStub := &stubOps{}
	enforcer := guardrails.New(testPolicy(), func() (map[string]string, error) { return map[string]string{"MAX_REPLICAS": "5"}, nil }, slog.Default())
	handler := newTestHandlerWithOpsAndEnforcer(operationsStub, enforcer)
	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodPost, "/api/operations/scale",
		strings.NewReader(`{"namespace":"app","name":"web","replicas":6}`)))
	if responseRecorder.Code != 403 || operationsStub.scaleCalls != 0 {
		t.Errorf("status=%d calls=%d, want 403/0", responseRecorder.Code, operationsStub.scaleCalls)
	}
}

// Scenario: feature-flag write to a ConfigMap that's not on the allowlist →
// 403, even with a valid namespace. update-feature-flag has the strictest
// policy of any mutation, so we cover both the CM and the key denial paths.
func TestUpdateFeatureFlagRoute_DeniesUnallowlistedConfigMap(t *testing.T) {
	operationsStub := &stubOps{}
	enforcer := guardrails.New(testPolicy(), func() (map[string]string, error) { return map[string]string{"MAX_REPLICAS": "10"}, nil }, slog.Default())
	handler := newTestHandlerWithOpsAndEnforcer(operationsStub, enforcer)
	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodPost, "/api/operations/update-feature-flag",
		strings.NewReader(`{"namespace":"app","configmap":"other-config","key":"FOO","value":"v"}`)))
	if responseRecorder.Code != 403 || operationsStub.lastFlagKey != "" {
		t.Errorf("status=%d key=%q, want 403/empty", responseRecorder.Code, operationsStub.lastFlagKey)
	}
}

func TestUpdateFeatureFlagRoute_DeniesUnallowlistedKey(t *testing.T) {
	operationsStub := &stubOps{}
	enforcer := guardrails.New(testPolicy(), func() (map[string]string, error) { return map[string]string{"MAX_REPLICAS": "10"}, nil }, slog.Default())
	handler := newTestHandlerWithOpsAndEnforcer(operationsStub, enforcer)
	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodPost, "/api/operations/update-feature-flag",
		strings.NewReader(`{"namespace":"app","configmap":"app-flags","key":"BAR","value":"v"}`)))
	if responseRecorder.Code != 403 || operationsStub.lastFlagKey != "" {
		t.Errorf("status=%d key=%q, want 403/empty", responseRecorder.Code, operationsStub.lastFlagKey)
	}
}

// Scenario: an allowed mutation returns 200 and the audit decision is included
// in the response body. The UI relies on this to render the guardrail badge
// without a second round-trip.
func TestScaleRoute_AllowResponseIncludesDecision(t *testing.T) {
	operationsStub := &stubOps{}
	handler := newTestHandlerWithOps(operationsStub)
	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodPost, "/api/operations/scale",
		strings.NewReader(`{"namespace":"app","name":"web","replicas":2}`)))
	if responseRecorder.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", responseRecorder.Code, responseRecorder.Body.String())
	}
	var responseBody struct {
		Status   string              `json:"status"`
		Decision guardrails.Decision `json:"decision"`
	}
	if err := json.NewDecoder(responseRecorder.Body).Decode(&responseBody); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if responseBody.Status != "ok" || !responseBody.Decision.Allow || responseBody.Decision.Action != "scale" {
		t.Errorf("body = %+v", responseBody)
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
