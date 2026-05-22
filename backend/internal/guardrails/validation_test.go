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

// Scenario: replicas must be >= minimum and <= maximum. Covers below-minimum,
// at-minimum, an interior value, at-maximum, and above-maximum so each branch
// of the §3.2 bounds check has a witness.
func TestValidReplicas_BoundsCheck(t *testing.T) {
	cases := []struct {
		name      string
		requested int
		minimum   int
		maximum   int
		wantErr   bool
		errSubstr string
	}{
		{"below minimum", 1, 2, 10, true, "MinReplicas"},
		{"at minimum", 2, 2, 10, false, ""},
		{"interior", 5, 2, 10, false, ""},
		{"at maximum", 10, 2, 10, false, ""},
		{"above maximum", 11, 2, 10, true, "MaxReplicas"},
		{"zero below minimum", 0, 2, 10, true, "MinReplicas"},
		{"negative below minimum", -1, 2, 10, true, "MinReplicas"},
	}
	for _, testCase := range cases {
		err := validReplicas(testCase.requested, testCase.minimum, testCase.maximum)
		if (err != nil) != testCase.wantErr {
			t.Errorf("%s: requested=%d min=%d max=%d → err=%v, wantErr=%v",
				testCase.name, testCase.requested, testCase.minimum, testCase.maximum, err, testCase.wantErr)
			continue
		}
		if testCase.wantErr && !strings.Contains(err.Error(), testCase.errSubstr) {
			t.Errorf("%s: err = %q, want substring %q", testCase.name, err.Error(), testCase.errSubstr)
		}
	}
}

// Scenario: validReplicas called with the production MinReplicas/MaxReplicas
// constants honors both bounds. Pins the wiring between the constants and the
// validator so a future rename of either constant has to update this test.
func TestValidReplicas_UsesProductionBounds(t *testing.T) {
	if err := validReplicas(MinReplicas, MinReplicas, MaxReplicas); err != nil {
		t.Errorf("requested=MinReplicas should pass: %v", err)
	}
	if err := validReplicas(MinReplicas-1, MinReplicas, MaxReplicas); err == nil {
		t.Errorf("requested=MinReplicas-1 should fail")
	}
	if err := validReplicas(MaxReplicas+1, MinReplicas, MaxReplicas); err == nil {
		t.Errorf("requested=MaxReplicas+1 should fail")
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
