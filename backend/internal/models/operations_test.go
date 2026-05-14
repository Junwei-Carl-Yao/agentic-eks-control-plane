// Phase 2.4 — typed request models with structural validation.
// Note: these are *structural* checks (required fields, type bounds). Policy checks
// (DNS-1123 names, MaxReplicas bound) are Phase 3 and belong in
// internal/guardrails, not here.
package models

import "testing"

// Scenario: empty namespace → Validate fails. Namespace is the targeting field; an
// empty value would fall back to "default" elsewhere, which is exactly what we
// don't want.
func TestScaleRequest_RequiresNamespace(t *testing.T) {
	scaleRequest := ScaleRequest{Namespace: "", Name: "web", Replicas: 3}
	if err := scaleRequest.Validate(); err == nil {
		t.Error("expected validation error for empty namespace")
	}
}

// Scenario: empty name → Validate fails.
func TestScaleRequest_RequiresName(t *testing.T) {
	scaleRequest := ScaleRequest{Namespace: "app", Name: "", Replicas: 3}
	if err := scaleRequest.Validate(); err == nil {
		t.Error("expected validation error for empty name")
	}
}

// Scenario: replicas < 1 → Validate fails. Zero is gated at the K8s op layer too,
// but failing here means a malformed request never reaches the op.
func TestScaleRequest_RejectsBelowOne(t *testing.T) {
	for _, replicaCount := range []int{-1, 0} {
		scaleRequest := ScaleRequest{Namespace: "app", Name: "web", Replicas: replicaCount}
		if err := scaleRequest.Validate(); err == nil {
			t.Errorf("replicas=%d: expected validation error", replicaCount)
		}
	}
}

// Scenario: minimal valid request → Validate succeeds.
func TestScaleRequest_HappyPath(t *testing.T) {
	scaleRequest := ScaleRequest{Namespace: "app", Name: "web", Replicas: 1}
	if err := scaleRequest.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

// Scenario: rollout-restart needs ns + name; nothing else.
func TestRolloutRestartRequest_RequiresNamespaceAndName(t *testing.T) {
	for _, rolloutRestartRequest := range []RolloutRestartRequest{
		{Namespace: "", Name: "web"},
		{Namespace: "app", Name: ""},
	} {
		if err := rolloutRestartRequest.Validate(); err == nil {
			t.Errorf("expected error for %+v", rolloutRestartRequest)
		}
	}
}

// Scenario: Revision = 0 means "previous" (well-defined sentinel). Negative is invalid.
func TestRollbackRequest_NegativeRevisionInvalid(t *testing.T) {
	rollbackRequest := RollbackRequest{Namespace: "app", Name: "web", Revision: -1}
	if err := rollbackRequest.Validate(); err == nil {
		t.Error("expected validation error for negative revision")
	}
}

// Scenario: Revision = 0 (default-to-previous) is valid.
func TestRollbackRequest_ZeroRevisionMeansPrevious(t *testing.T) {
	rollbackRequest := RollbackRequest{Namespace: "app", Name: "web", Revision: 0}
	if err := rollbackRequest.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}
