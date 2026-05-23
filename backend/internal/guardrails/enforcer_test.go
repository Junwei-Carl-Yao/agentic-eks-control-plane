package guardrails

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"eks-control-plane/backend/internal/models"
)

// allowedNamespace is the canonical fixture namespace — matches DefaultPolicy
// so production and tests exercise the same allowlist.
var allowedNamespace = DefaultPolicy().AllowedNamespaces[0]

func newEnforcer(t *testing.T) (*Enforcer, *bytes.Buffer) {
	t.Helper()
	logBuffer := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logBuffer, nil))
	return New(DefaultPolicy(), logger), logBuffer
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
	decision := enforcer.Scale(models.ScaleRequest{Namespace: "kube-system", Name: "web", Replicas: MinReplicas})
	if decision.Allow || !strings.Contains(decision.Reason, "kube-system") {
		t.Errorf("decision = %+v, want deny mentioning kube-system", decision)
	}
}

// Scenario: scale beyond MaxReplicas → deny. The reason must mention
// MaxReplicas so the UI can render exactly which cap was hit.
func TestEnforcer_ScaleOverMaxReplicas(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	decision := enforcer.Scale(models.ScaleRequest{Namespace: allowedNamespace, Name: "web", Replicas: MaxReplicas + 1})
	if decision.Allow || !strings.Contains(decision.Reason, "MaxReplicas") {
		t.Errorf("decision = %+v, want deny mentioning MaxReplicas", decision)
	}
}

// Scenario: invalid resource name (uppercase violates DNS-1123) → deny.
func TestEnforcer_ScaleInvalidName(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	decision := enforcer.Scale(models.ScaleRequest{Namespace: allowedNamespace, Name: "Web", Replicas: MinReplicas})
	if decision.Allow {
		t.Errorf("decision = %+v, want deny on invalid name", decision)
	}
}

// Scenario: scale below MinReplicas → deny. The reason must mention the floor
// (MinReplicas) so operators and the UI can render which bound was hit.
// MinReplicas-1 covers the just-under boundary; 0 covers the legacy
// "positive" case that used to be the only floor.
func TestEnforcer_ScaleBelowMinReplicasDenied(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	for _, replicasBelowFloor := range []int{MinReplicas - 1, 0, -1} {
		decision := enforcer.Scale(models.ScaleRequest{Namespace: allowedNamespace, Name: "web", Replicas: replicasBelowFloor})
		if decision.Allow {
			t.Errorf("replicas=%d: decision = %+v, want deny", replicasBelowFloor, decision)
			continue
		}
		if !strings.Contains(decision.Reason, "MinReplicas") {
			t.Errorf("replicas=%d: reason = %q, want substring %q", replicasBelowFloor, decision.Reason, "MinReplicas")
		}
	}
}

// Scenario: scale at exactly MinReplicas → allow. The bound is inclusive per
// §3.2, so the floor itself must not be rejected.
func TestEnforcer_ScaleAtMinReplicasAllowed(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	decision := enforcer.Scale(models.ScaleRequest{Namespace: allowedNamespace, Name: "web", Replicas: MinReplicas})
	if !decision.Allow {
		t.Errorf("decision = %+v, want allow at MinReplicas=%d", decision, MinReplicas)
	}
}

// Scenario: scale at exactly MaxReplicas → allow. Mirrors the at-min test for
// the upper end of the inclusive bound.
func TestEnforcer_ScaleAtMaxReplicasAllowed(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	decision := enforcer.Scale(models.ScaleRequest{Namespace: allowedNamespace, Name: "web", Replicas: MaxReplicas})
	if !decision.Allow {
		t.Errorf("decision = %+v, want allow at MaxReplicas=%d", decision, MaxReplicas)
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
	decision := enforcer.Rollback(context.Background(), models.RollbackRequest{Namespace: allowedNamespace, Name: "web", Revision: 2}, nil)
	if !decision.Allow {
		t.Errorf("decision = %+v", decision)
	}
}

// Scenario: every Enforce call emits a structured audit log entry, both for
// allow and for deny. Operators read this log when answering "what did we
// allow?" — silently dropped denies would defeat the audit trail.
func TestEnforcer_EmitsAuditLogForAllowAndDeny(t *testing.T) {
	enforcer, logBuffer := newEnforcer(t)
	enforcer.Scale(models.ScaleRequest{Namespace: allowedNamespace, Name: "web", Replicas: MinReplicas})
	enforcer.Scale(models.ScaleRequest{Namespace: "kube-system", Name: "web", Replicas: MinReplicas})

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
