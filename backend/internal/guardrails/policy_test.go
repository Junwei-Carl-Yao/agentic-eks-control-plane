package guardrails

import "testing"

// fixturePolicy mirrors DefaultPolicy. Tests assert behaviour against the
// production policy directly, so a change to DefaultPolicy that drops a
// scenario from coverage shows up here.
var fixturePolicy = DefaultPolicy()

// Scenario: namespace not on AllowedNamespaces → deny. The policy's allowlist
// is the only thing consulted; anything outside it is rejected.
func TestPolicy_NamespaceNotInAllowlistDenied(t *testing.T) {
	if ok, _ := fixturePolicy.namespaceAllowed("kube-system"); ok {
		t.Error("kube-system was permitted under default allowlist")
	}
}

// Scenario: namespace on the allowlist → permit.
func TestPolicy_AllowlistedNamespaceAllowed(t *testing.T) {
	namespace := fixturePolicy.AllowedNamespaces[0]
	if ok, _ := fixturePolicy.namespaceAllowed(namespace); !ok {
		t.Errorf("%q was rejected despite being on the allowlist", namespace)
	}
}

// Scenario: mutating the slice a caller passed into New() must not widen the
// live policy. The defensive copy in New is the load-bearing reason no
// downstream code can broaden the safety boundary after construction.
func TestPolicy_NewDefensivelyCopiesSlices(t *testing.T) {
	originalNamespaces := []string{"only-this"}
	enforcer := New(Policy{AllowedNamespaces: originalNamespaces}, nil)
	originalNamespaces[0] = "kube-system"

	if ok, _ := enforcer.policy.namespaceAllowed("kube-system"); ok {
		t.Error("post-construction slice mutation widened the namespace allowlist")
	}
}
