// Package guardrails is the single chokepoint that decides whether a mutation
// is allowed. The route layer must call an Enforcer method for every write
// before dispatching to the kubernetes ops layer, so the blast radius is
// bounded regardless of who proposes the action (human or LLM).
package guardrails

import "slices"

// MaxReplicas is the hard upper bound on Scale.Replicas. Hardcoded at compile
// time so no caller (route, agent, future internal use) can widen it without
// rebuilding the binary.
const MaxReplicas = 10

// MinReplicas is the hard lower bound on Scale.Replicas. Set to 2 so a
// rolling restart of a single deployment can never drop the app to zero
// available pods; the default RollingUpdate strategy keeps at least one
// replica serving while the other is replaced.
const MinReplicas = 2

// Policy is the static guardrail policy. It is passed by value into New and
// copied defensively, so widening the policy is impossible without
// constructing a fresh Enforcer at the binary's boot path. There is no
// package-level mutable state that participates in any decision.
type Policy struct {
	AllowedNamespaces []string
	// RollbackImageFloors maps a Deployment name to the minimum acceptable
	// image version. A rollback targeting one of these Deployments is denied
	// unless the resolved target image carries a `v<int>` tag at or above the
	// floor — preventing a rollback from undoing a security or compatibility
	// fix baked into a known-good release.
	RollbackImageFloors map[string]int
}

// DefaultPolicy returns the production policy. Values are fixed at compile
// time; environment cannot widen them. Tests construct their own Policy
// rather than mutating this one.
func DefaultPolicy() Policy {
	return Policy{
		AllowedNamespaces: []string{"control-plane"},
		RollbackImageFloors: map[string]int{
			"agent":    6,
			"backend":  4,
			"frontend": 4,
		},
	}
}

// namespaceAllowed encodes the §3.1 contract: namespaces are permitted only
// when explicitly listed.
func (policy Policy) namespaceAllowed(namespace string) (bool, string) {
	if slices.Contains(policy.AllowedNamespaces, namespace) {
		return true, ""
	}
	return false, "namespace " + namespace + " is not on the allowed list"
}
