package guardrails

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"eks-control-plane/backend/internal/models"
)

// allowedNamespace is the canonical fixture namespace — matches DefaultPolicy
// so production and tests exercise the same allowlist.
var allowedNamespace = DefaultPolicy().AllowedNamespaces[0]

func staticFlags(values map[string]string) func() (map[string]string, error) {
	return func() (map[string]string, error) { return values, nil }
}

func newEnforcer(t *testing.T) (*Enforcer, *bytes.Buffer) {
	t.Helper()
	logBuffer := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logBuffer, nil))
	return New(DefaultPolicy(), staticFlags(map[string]string{"MAX_REPLICAS": "5"}), logger), logBuffer
}

// Scenario: scale of an allowed deployment with valid replicas → allow.
func TestEnforcer_ScaleAllow(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	decision := enforcer.Scale(models.ScaleRequest{Namespace: allowedNamespace, Name: "web", Replicas: 3})
	if !decision.Allow || decision.Subject != allowedNamespace+"/web" {
		t.Errorf("decision = %+v, want allow %s/web", decision, allowedNamespace)
	}
}

// Scenario: scale to a namespace not on the allowlist → deny with reason
// mentioning the namespace, even if the rest of the request is valid.
func TestEnforcer_ScaleUnallowlistedNamespace(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	decision := enforcer.Scale(models.ScaleRequest{Namespace: "kube-system", Name: "web", Replicas: 1})
	if decision.Allow || !strings.Contains(decision.Reason, "kube-system") {
		t.Errorf("decision = %+v, want deny mentioning kube-system", decision)
	}
}

// Scenario: scale beyond MAX_REPLICAS → deny. The reason must mention
// MAX_REPLICAS so the UI can render exactly which cap was hit.
func TestEnforcer_ScaleOverMaxReplicas(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	decision := enforcer.Scale(models.ScaleRequest{Namespace: allowedNamespace, Name: "web", Replicas: 99})
	if decision.Allow || !strings.Contains(decision.Reason, "MAX_REPLICAS") {
		t.Errorf("decision = %+v, want deny mentioning MAX_REPLICAS", decision)
	}
}

// Scenario: the maxReplicas resolver fails (e.g. ConfigMap missing) → deny
// with a reason naming MAX_REPLICAS. Failing closed is the safest behaviour
// when the live cap is unreadable.
func TestEnforcer_ScaleDeniesWhenMaxReplicasUnavailable(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	failing := func() (map[string]string, error) { return nil, errors.New("configmap missing") }
	enforcer := New(DefaultPolicy(), failing, logger)
	decision := enforcer.Scale(models.ScaleRequest{Namespace: allowedNamespace, Name: "web", Replicas: 1})
	if decision.Allow || !strings.Contains(decision.Reason, "MAX_REPLICAS unavailable") {
		t.Errorf("decision = %+v, want deny on MAX_REPLICAS unavailable", decision)
	}
}

// Scenario: invalid resource name (uppercase violates DNS-1123) → deny.
func TestEnforcer_ScaleInvalidName(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	decision := enforcer.Scale(models.ScaleRequest{Namespace: allowedNamespace, Name: "Web", Replicas: 1})
	if decision.Allow {
		t.Errorf("decision = %+v, want deny on invalid name", decision)
	}
}

// Scenario: rollout-restart on an allowed deployment → allow.
func TestEnforcer_RolloutRestartAllow(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	decision := enforcer.RolloutRestart(models.RolloutRestartRequest{Namespace: allowedNamespace, Name: "web"})
	if !decision.Allow || decision.Action != "rollout-restart" {
		t.Errorf("decision = %+v", decision)
	}
}

// Scenario: rollback with a valid revision → allow. Revision 0 means
// "previous"; the enforcer accepts both 0 and positive revisions.
func TestEnforcer_RollbackAllow(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	decision := enforcer.Rollback(models.RollbackRequest{Namespace: allowedNamespace, Name: "web", Revision: 2})
	if !decision.Allow {
		t.Errorf("decision = %+v", decision)
	}
}

// Scenario: feature-flag write to an allowlisted ConfigMap + key → allow.
func TestEnforcer_FeatureFlagAllow(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	decision := enforcer.UpdateFeatureFlag(models.UpdateFeatureFlagRequest{
		Namespace: allowedNamespace, ConfigMap: FeatureFlagConfigMap, Key: DefaultPolicy().FeatureFlagKeys[0], Value: "true",
	})
	if !decision.Allow {
		t.Errorf("decision = %+v", decision)
	}
}

// Scenario: feature-flag write to a ConfigMap that's not on the allowlist →
// deny. This is the load-bearing check that protects every other ConfigMap
// in the namespace from accidental writes.
func TestEnforcer_FeatureFlagDeniesUnallowlistedConfigMap(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	decision := enforcer.UpdateFeatureFlag(models.UpdateFeatureFlagRequest{
		Namespace: allowedNamespace, ConfigMap: "other-config", Key: DefaultPolicy().FeatureFlagKeys[0], Value: "true",
	})
	if decision.Allow || !strings.Contains(decision.Reason, "other-config") {
		t.Errorf("decision = %+v, want deny mentioning other-config", decision)
	}
}

// Scenario: feature-flag write with a key that's not on the allowlist → deny,
// even when the ConfigMap is allowed.
func TestEnforcer_FeatureFlagDeniesUnallowlistedKey(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	decision := enforcer.UpdateFeatureFlag(models.UpdateFeatureFlagRequest{
		Namespace: allowedNamespace, ConfigMap: FeatureFlagConfigMap, Key: "SECRET_KEY", Value: "v",
	})
	if decision.Allow || !strings.Contains(decision.Reason, "SECRET_KEY") {
		t.Errorf("decision = %+v, want deny mentioning SECRET_KEY", decision)
	}
}

// Scenario: every Enforce call emits a structured audit log entry, both for
// allow and for deny. Operators read this log when answering "what did we
// allow?" — silently dropped denies would defeat the audit trail.
func TestEnforcer_EmitsAuditLogForAllowAndDeny(t *testing.T) {
	enforcer, logBuffer := newEnforcer(t)
	enforcer.Scale(models.ScaleRequest{Namespace: allowedNamespace, Name: "web", Replicas: 1})
	enforcer.Scale(models.ScaleRequest{Namespace: "kube-system", Name: "web", Replicas: 1})

	logLines := strings.Split(strings.TrimSpace(logBuffer.String()), "\n")
	if len(logLines) != 2 {
		t.Fatalf("expected 2 audit lines, got %d:\n%s", len(logLines), logBuffer.String())
	}
	for _, logLine := range logLines {
		var auditEntry map[string]any
		if err := json.Unmarshal([]byte(logLine), &auditEntry); err != nil {
			t.Fatalf("audit line not JSON: %v", err)
		}
		if auditEntry["msg"] != "guardrail.decision" || auditEntry["action"] != "scale" {
			t.Errorf("audit shape = %v", auditEntry)
		}
	}
}
