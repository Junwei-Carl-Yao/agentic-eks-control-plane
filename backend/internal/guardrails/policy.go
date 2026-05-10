// Package guardrails is the single chokepoint that decides whether a mutation
// is allowed. The route layer must call an Enforcer method for every write
// before dispatching to the kubernetes ops layer, so the blast radius is
// bounded regardless of who proposes the action (human or LLM).
package guardrails

// FeatureFlagConfigMap is the only ConfigMap name UpdateFeatureFlag may write
// to. Any other ConfigMap is rejected at the enforcer.
const FeatureFlagConfigMap = "app-flags"

// Policy is the static guardrail policy. It is passed by value into New and
// copied defensively, so widening the policy is impossible without
// constructing a fresh Enforcer at the binary's boot path. There is no
// package-level mutable state that participates in any decision.
type Policy struct {
	AllowedNamespaces []string
	FeatureFlagKeys   []string
}

// DefaultPolicy returns the production policy. Values are fixed at compile
// time; environment cannot widen them. Tests construct their own Policy
// rather than mutating this one.
func DefaultPolicy() Policy {
	return Policy{
		AllowedNamespaces: []string{"api-smoke"},
		FeatureFlagKeys:   []string{"MAX_REPLICAS"},
	}
}

// namespaceAllowed encodes the §3.1 contract: namespaces are permitted only
// when explicitly listed.
func (policy Policy) namespaceAllowed(namespace string) (bool, string) {
	for _, allowed := range policy.AllowedNamespaces {
		if allowed == namespace {
			return true, ""
		}
	}
	return false, "namespace " + namespace + " is not on the allowed list"
}

func (policy Policy) featureFlagKeyAllowed(key string) bool {
	for _, allowed := range policy.FeatureFlagKeys {
		if allowed == key {
			return true
		}
	}
	return false
}

func configMapAllowed(name string) bool {
	return FeatureFlagConfigMap == name
}
