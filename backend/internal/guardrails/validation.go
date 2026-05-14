package guardrails

import (
	"fmt"
	"regexp"
)

// dns1123Label matches the rule kube-apiserver actually applies to Deployment,
// ConfigMap, and Namespace names: DNS-1123 *label* (max 63 chars, lowercase
// alphanumeric + '-', must start and end alphanumeric, no dots). The earlier
// subdomain regex (max 253, dots allowed) was laxer than what the server
// accepts for these kinds, so names like "foo.bar" passed the enforcer and
// then failed at the apiserver — defeating the point of validating here.
var dns1123Label = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

const dns1123LabelMaxLength = 63

// validNamespace accepts only DNS-1123 label names. Empty is rejected — an
// empty namespace would default to "default" downstream, and "default" is not
// on the allowlist anyway, so failing here is the clearer error.
func validNamespace(namespace string) error {
	return validDNS1123(namespace, "namespace")
}

func validResourceName(name string) error {
	return validDNS1123(name, "name")
}

func validDNS1123(value, label string) error {
	if value == "" {
		return fmt.Errorf("%s is required", label)
	}
	if len(value) > dns1123LabelMaxLength {
		return fmt.Errorf("%s exceeds %d characters", label, dns1123LabelMaxLength)
	}
	if !dns1123Label.MatchString(value) {
		return fmt.Errorf("%s %q is not a valid DNS-1123 label", label, value)
	}
	return nil
}

// validReplicas enforces the implementation §3.2 bounds: positive and within
// MaxReplicas. The "positive" half also lives at the K8s op layer, but we
// duplicate it here so the audit log carries the right reason code.
func validReplicas(requested, maximum int) error {
	if requested < 1 {
		return fmt.Errorf("replicas must be >= 1, got %d", requested)
	}
	if requested > maximum {
		return fmt.Errorf("replicas %d exceeds MaxReplicas=%d", requested, maximum)
	}
	return nil
}

// validRevision matches the §3.2 contract: revision >= 0 (0 means "previous").
// Existence in the deployment's history is checked by the K8s ops layer where
// the ReplicaSet list is already loaded.
func validRevision(revision int64) error {
	if revision < 0 {
		return fmt.Errorf("revision must be >= 0, got %d", revision)
	}
	return nil
}
