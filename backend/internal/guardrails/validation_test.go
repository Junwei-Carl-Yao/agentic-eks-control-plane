package guardrails

import (
	"strings"
	"testing"
)

// Scenario: DNS-1123 *labels* pass — the rule kube-apiserver actually applies
// to Deployment, ConfigMap, and Namespace names. Validating subdomain syntax
// here would let dotted names through that the apiserver then rejects, so we
// stick with the label rule.
func TestValidDNS1123_AcceptsValidNames(t *testing.T) {
	for _, validName := range []string{"app", "web-1", "service-with-many-hyphens"} {
		if err := validResourceName(validName); err != nil {
			t.Errorf("name %q rejected: %v", validName, err)
		}
	}
}

// Scenario: dotted subdomains, uppercase, underscores, leading/trailing
// dashes, empty, and names over the 63-char label cap all fail. The dotted
// case is the one that distinguishes label from subdomain semantics — it
// passed under the old subdomain regex.
func TestValidDNS1123_RejectsInvalid(t *testing.T) {
	for _, invalidName := range []string{
		"",
		"App",
		"web_1",
		"-leading",
		"trailing-",
		"api.svc.cluster.local", // dots are subdomain syntax; labels reject them
		strings.Repeat("a", 64), // 1 over the 63-char label cap
	} {
		if err := validResourceName(invalidName); err == nil {
			t.Errorf("name %q was accepted; want rejection", invalidName)
		}
	}
}

// Scenario: replicas must be >= 1 and <= MAX_REPLICAS. MaxReplicas == 0 means
// "no upper bound" — the loader defaults to 10 so this branch is mostly for
// tests that want to cover the lower bound in isolation.
func TestValidReplicas_BoundsCheck(t *testing.T) {
	cases := []struct {
		requested int
		maximum   int
		wantErr   bool
	}{
		{1, 10, false},
		{10, 10, false},
		{0, 10, true},
		{-1, 10, true},
		{11, 10, true},
	}
	for _, testCase := range cases {
		err := validReplicas(testCase.requested, testCase.maximum)
		if (err != nil) != testCase.wantErr {
			t.Errorf("requested=%d max=%d → err=%v, wantErr=%v", testCase.requested, testCase.maximum, err, testCase.wantErr)
		}
	}
}

// Scenario: feature-flag keys must obey ConfigMap-data-key character rules and
// the length cap. Empty is rejected (the model layer also rejects, but the
// enforcer's audit reason is what surfaces in the UI).
func TestValidFeatureFlagKey(t *testing.T) {
	if err := validFeatureFlagKey("FEATURE_X"); err != nil {
		t.Errorf("FEATURE_X rejected: %v", err)
	}
	for _, invalidKey := range []string{"", "has space", "bad/slash"} {
		if err := validFeatureFlagKey(invalidKey); err == nil {
			t.Errorf("key %q accepted; want rejection", invalidKey)
		}
	}
}

// Scenario: revision >= 0 (0 means "previous"); negative is invalid. The
// "exists in the deployment's history" half is enforced at the K8s ops layer.
func TestValidRevision(t *testing.T) {
	for _, validRevisionValue := range []int64{0, 1, 99} {
		if err := validRevision(validRevisionValue); err != nil {
			t.Errorf("revision %d rejected: %v", validRevisionValue, err)
		}
	}
	if err := validRevision(-1); err == nil {
		t.Error("revision -1 accepted; want rejection")
	}
}
