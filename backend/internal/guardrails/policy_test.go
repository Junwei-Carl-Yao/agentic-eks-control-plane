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

// Scenario: the configMap allowlist is exact-match. Substring or prefix matches
// would be a footgun for any operator who later names a CM `app-flags-prod`.
func TestPolicy_ConfigMapAllowlistIsExactMatch(t *testing.T) {
	if !configMapAllowed(FeatureFlagConfigMap) {
		t.Error("exact match was rejected")
	}
	if configMapAllowed(FeatureFlagConfigMap + "-prod") {
		t.Error("prefix match was accepted; allowlist must be exact")
	}
}

// Scenario: feature-flag key allowlist is exact-match.
func TestPolicy_FeatureFlagKeyAllowlistIsExactMatch(t *testing.T) {
	key := fixturePolicy.FeatureFlagKeys[0]
	if !fixturePolicy.featureFlagKeyAllowed(key) {
		t.Errorf("%q was rejected despite being on the allowlist", key)
	}
	if fixturePolicy.featureFlagKeyAllowed("UNLISTED") {
		t.Error("UNLISTED key was accepted")
	}
}

// Scenario: mutating the slices a caller passed into New() must not widen the
// live policy. The defensive copy in New is the load-bearing reason no
// downstream code can broaden the safety boundary after construction.
func TestPolicy_NewDefensivelyCopiesSlices(t *testing.T) {
	originalNamespaces := []string{"only-this"}
	originalKeys := []string{"ONLY_THIS"}
	enforcer := New(
		Policy{AllowedNamespaces: originalNamespaces, FeatureFlagKeys: originalKeys},
		func() (map[string]string, error) { return map[string]string{"MAX_REPLICAS": "1"}, nil },
		nil,
	)
	originalNamespaces[0] = "kube-system"
	originalKeys[0] = "WIDENED"

	if ok, _ := enforcer.policy.namespaceAllowed("kube-system"); ok {
		t.Error("post-construction slice mutation widened the namespace allowlist")
	}
	if enforcer.policy.featureFlagKeyAllowed("WIDENED") {
		t.Error("post-construction slice mutation widened the feature-flag-key allowlist")
	}
}
