package guardrails

import (
	"strings"
	"testing"
)

// Scenario: namespace-scoped reads share one policy gate (namespace allowlist
// + DNS-1123). Allowlisted namespace → allow; non-allowlisted → deny with the
// namespace named. The action string distinguishes resources in the audit log.
func TestEnforcer_ReadAllowsAllowlistedNamespace(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	for _, decision := range []Decision{
		enforcer.ListDeployments(allowedNamespace),
		enforcer.ListPods(allowedNamespace),
		enforcer.ListEvents(allowedNamespace),
		enforcer.ListServices(allowedNamespace),
		enforcer.ListIngresses(allowedNamespace),
		enforcer.ListHorizontalPodAutoscalers(allowedNamespace),
		enforcer.ListReplicaSets(allowedNamespace),
	} {
		if !decision.Allow {
			t.Errorf("decision = %+v, want allow", decision)
		}
	}
}

func TestEnforcer_ReadDeniesUnallowlistedNamespace(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	decision := enforcer.ListDeployments("kube-system")
	if decision.Allow || !strings.Contains(decision.Reason, "kube-system") {
		t.Errorf("decision = %+v, want deny mentioning kube-system", decision)
	}
}

// Scenario: GetDeployment validates the resource name too — an invalid name
// on an allowed namespace must still be rejected.
func TestEnforcer_GetReadDeniesInvalidName(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	decision := enforcer.GetDeployment(allowedNamespace, "Web")
	if decision.Allow {
		t.Errorf("decision = %+v, want deny on invalid name", decision)
	}
}

// Scenario: TailLogs validates pod and container as DNS-1123 labels because
// both are surfaced as path-like fragments to the k8s log stream API.
func TestEnforcer_TailLogsValidatesPodAndContainer(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	allowed := enforcer.TailLogs(allowedNamespace, "web-1", "app")
	if !allowed.Allow {
		t.Errorf("decision = %+v, want allow", allowed)
	}
	denied := enforcer.TailLogs(allowedNamespace, "Web-1", "app")
	if denied.Allow {
		t.Errorf("decision = %+v, want deny on invalid pod name", denied)
	}
}

// Scenario: NamespaceAllowed mirrors the allowlist exactly. ListNamespaces
// uses this to narrow the cluster-wide list, so the equivalence is the
// load-bearing invariant for that route.
func TestEnforcer_NamespaceAllowedMatchesPolicy(t *testing.T) {
	enforcer, _ := newEnforcer(t)
	if !enforcer.NamespaceAllowed(allowedNamespace) {
		t.Errorf("%q rejected despite being on the allowlist", allowedNamespace)
	}
	if enforcer.NamespaceAllowed("kube-system") {
		t.Error("kube-system was allowed under DefaultPolicy")
	}
}
