package guardrails

import (
	"fmt"
	"log/slog"
	"strconv"

	"eks-control-plane/backend/internal/models"
)

// Decision is the structured audit record returned for every Enforce call.
// The same record is logged and returned in the API response body so the UI
// can render guardrail outcomes — denied actions are never silently dropped.
type Decision struct {
	Allow   bool   `json:"allow"`
	Action  string `json:"action"`
	Subject string `json:"subject"`
	Reason  string `json:"reason,omitempty"`
}

// Enforcer is the single chokepoint. The flags closure resolves the current
// feature-flag ConfigMap contents at request time, so changes take effect
// without restarting the binary. Adding a new flag means adding a parser
// method that consumes the same map; the constructor signature stays put.
type Enforcer struct {
	policy Policy
	flags  func() (map[string]string, error)
	logger *slog.Logger
}

// New returns an Enforcer bound to the supplied Policy and feature-flag
// loader. The Policy slices are copied defensively, so callers cannot widen
// the safety boundary by mutating slices they passed in. A nil logger falls
// back to the slog default so callers can omit it in tests.
func New(policy Policy, flags func() (map[string]string, error), logger *slog.Logger) *Enforcer {
	if logger == nil {
		logger = slog.Default()
	}
	sealed := Policy{
		AllowedNamespaces: append([]string(nil), policy.AllowedNamespaces...),
		FeatureFlagKeys:   append([]string(nil), policy.FeatureFlagKeys...),
	}
	return &Enforcer{policy: sealed, flags: flags, logger: logger}
}

// ListDeployments / ListPods / ListEvents / ListServices / ListIngresses /
// ListHorizontalPodAutoscalers / ListReplicaSets all share the
// namespace-scoped read shape: the only policy gate is namespace allowlist +
// DNS-1123. Each is a thin wrapper so the audit log carries the
// resource-specific action name.
func (enforcer *Enforcer) ListDeployments(namespace string) Decision {
	return enforcer.enforceNamespaceRead("list-deployments", namespace)
}

func (enforcer *Enforcer) ListPods(namespace string) Decision {
	return enforcer.enforceNamespaceRead("list-pods", namespace)
}

func (enforcer *Enforcer) ListEvents(namespace string) Decision {
	return enforcer.enforceNamespaceRead("list-events", namespace)
}

func (enforcer *Enforcer) ListServices(namespace string) Decision {
	return enforcer.enforceNamespaceRead("list-services", namespace)
}

func (enforcer *Enforcer) ListIngresses(namespace string) Decision {
	return enforcer.enforceNamespaceRead("list-ingresses", namespace)
}

func (enforcer *Enforcer) ListHorizontalPodAutoscalers(namespace string) Decision {
	return enforcer.enforceNamespaceRead("list-hpas", namespace)
}

func (enforcer *Enforcer) ListReplicaSets(namespace string) Decision {
	return enforcer.enforceNamespaceRead("list-replicasets", namespace)
}

// GetDeployment layers DNS-1123 on the resource name on top of the namespace
// check.
func (enforcer *Enforcer) GetDeployment(namespace, name string) Decision {
	return enforcer.namespaceAndNameRead("get-deployment", namespace, name)
}

// GetFeatureFlags is stricter than the other reads: only FeatureFlagConfigMap
// may be fetched. Every other ConfigMap in the namespace is invisible to
// callers — the binary is a feature-flag console, not a generic ConfigMap
// browser.
func (enforcer *Enforcer) GetFeatureFlags(namespace, name string) Decision {
	subject := namespace + "/" + name
	if reason := firstError(
		validNamespace(namespace),
		validFeatureFlagKey(name),
		enforcer.namespaceCheck(namespace),
	); reason != "" {
		return enforcer.deny("get-feature-flags", subject, reason)
	}
	if !configMapAllowed(name) {
		return enforcer.deny("get-feature-flags", subject,
			"configmap "+name+" is not on the feature-flag allowlist")
	}
	return enforcer.allow("get-feature-flags", subject)
}

// FilterFeatureFlagData returns a new map containing only keys on the
// FeatureFlagKeys allowlist. Used by the feature-flag read route to narrow
// the returned data so non-flag keys (added directly to the ConfigMap,
// perhaps by a platform team for unrelated reasons) never reach the caller.
func (enforcer *Enforcer) FilterFeatureFlagData(data map[string]string) map[string]string {
	filtered := make(map[string]string, len(enforcer.policy.FeatureFlagKeys))
	for _, key := range enforcer.policy.FeatureFlagKeys {
		if value, ok := data[key]; ok {
			filtered[key] = value
		}
	}
	return filtered
}

// TailLogs validates pod and container names too — both are surfaced as
// path-like fragments to the kubernetes log stream API.
func (enforcer *Enforcer) TailLogs(namespace, pod, container string) Decision {
	subject := namespace + "/" + pod + ":" + container
	if reason := firstError(
		validNamespace(namespace),
		validResourceName(pod),
		validResourceName(container),
		enforcer.namespaceCheck(namespace),
	); reason != "" {
		return enforcer.deny("tail-logs", subject, reason)
	}
	return enforcer.allow("tail-logs", subject)
}

// NamespaceAllowed exposes the policy's namespace check as a boolean so the
// ListNamespaces route can filter the cluster-wide list down to allowlisted
// namespaces. ListNamespaces never denies — it narrows.
func (enforcer *Enforcer) NamespaceAllowed(namespace string) bool {
	ok, _ := enforcer.policy.namespaceAllowed(namespace)
	return ok
}

// Scale enforces namespace allowlist + DNS-1123 + replica bounds. The replica
// cap is read from the FeatureFlagConfigMap on every call so operators can
// adjust MAX_REPLICAS without redeploying.
func (enforcer *Enforcer) Scale(request models.ScaleRequest) Decision {
	subject := request.Namespace + "/" + request.Name
	if reason := firstError(
		validNamespace(request.Namespace),
		validResourceName(request.Name),
		enforcer.namespaceCheck(request.Namespace),
	); reason != "" {
		return enforcer.deny("scale", subject, reason)
	}
	maximum, err := enforcer.maxReplicas()
	if err != nil {
		return enforcer.deny("scale", subject, "MAX_REPLICAS unavailable: "+err.Error())
	}
	if err := validReplicas(request.Replicas, maximum); err != nil {
		return enforcer.deny("scale", subject, err.Error())
	}
	return enforcer.allow("scale", subject)
}

// RolloutRestart enforces namespace allowlist + DNS-1123. No payload to bound.
func (enforcer *Enforcer) RolloutRestart(request models.RolloutRestartRequest) Decision {
	return enforcer.simpleDeploymentMutation("rollout-restart", request.Namespace, request.Name)
}

func (enforcer *Enforcer) PauseRollout(request models.PauseRolloutRequest) Decision {
	return enforcer.simpleDeploymentMutation("pause-rollout", request.Namespace, request.Name)
}

func (enforcer *Enforcer) ResumeRollout(request models.ResumeRolloutRequest) Decision {
	return enforcer.simpleDeploymentMutation("resume-rollout", request.Namespace, request.Name)
}

func (enforcer *Enforcer) Rollback(request models.RollbackRequest) Decision {
	subject := request.Namespace + "/" + request.Name
	if reason := firstError(
		validNamespace(request.Namespace),
		validResourceName(request.Name),
		validRevision(request.Revision),
		enforcer.namespaceCheck(request.Namespace),
	); reason != "" {
		return enforcer.deny("rollback", subject, reason)
	}
	return enforcer.allow("rollback", subject)
}

// UpdateFeatureFlag is the most policy-heavy action: namespace + DNS-1123 on
// the ConfigMap name + the (configmap, key) allowlist + value length.
func (enforcer *Enforcer) UpdateFeatureFlag(request models.UpdateFeatureFlagRequest) Decision {
	subject := request.Namespace + "/" + request.ConfigMap + ":" + request.Key
	if reason := firstError(
		validNamespace(request.Namespace),
		validResourceName(request.ConfigMap),
		validFeatureFlagKey(request.Key),
		validFeatureFlagValue(request.Value),
		enforcer.namespaceCheck(request.Namespace),
	); reason != "" {
		return enforcer.deny("update-feature-flag", subject, reason)
	}
	if !configMapAllowed(request.ConfigMap) {
		return enforcer.deny("update-feature-flag", subject,
			"configmap "+request.ConfigMap+" is not on the feature-flag allowlist")
	}
	if !enforcer.policy.featureFlagKeyAllowed(request.Key) {
		return enforcer.deny("update-feature-flag", subject,
			"key "+request.Key+" is not on the feature-flag key allowlist")
	}
	return enforcer.allow("update-feature-flag", subject)
}

func (enforcer *Enforcer) enforceNamespaceRead(action, namespace string) Decision {
	subject := namespace
	if reason := firstError(
		validNamespace(namespace),
		enforcer.namespaceCheck(namespace),
	); reason != "" {
		return enforcer.deny(action, subject, reason)
	}
	return enforcer.allow(action, subject)
}

func (enforcer *Enforcer) namespaceAndNameRead(action, namespace, name string) Decision {
	subject := namespace + "/" + name
	if reason := firstError(
		validNamespace(namespace),
		validResourceName(name),
		enforcer.namespaceCheck(namespace),
	); reason != "" {
		return enforcer.deny(action, subject, reason)
	}
	return enforcer.allow(action, subject)
}

func (enforcer *Enforcer) simpleDeploymentMutation(action, namespace, name string) Decision {
	subject := namespace + "/" + name
	if reason := firstError(
		validNamespace(namespace),
		validResourceName(name),
		enforcer.namespaceCheck(namespace),
	); reason != "" {
		return enforcer.deny(action, subject, reason)
	}
	return enforcer.allow(action, subject)
}

func (enforcer *Enforcer) allow(action, subject string) Decision {
	decision := Decision{Allow: true, Action: action, Subject: subject}
	enforcer.audit(decision)
	return decision
}

func (enforcer *Enforcer) deny(action, subject, reason string) Decision {
	decision := Decision{Allow: false, Action: action, Subject: subject, Reason: reason}
	enforcer.audit(decision)
	return decision
}

func (enforcer *Enforcer) audit(decision Decision) {
	enforcer.logger.Info("guardrail.decision",
		"allow", decision.Allow,
		"action", decision.Action,
		"subject", decision.Subject,
		"reason", decision.Reason,
	)
}

// firstError returns the first non-nil error's message, or empty if all are nil.
// Lets each Enforce method enumerate its checks in one expression.
func firstError(errs ...error) string {
	for _, err := range errs {
		if err != nil {
			return err.Error()
		}
	}
	return ""
}

// namespaceCheck wraps Policy.namespaceAllowed in the error-shaped contract
// firstError understands. It's a method (not a package function) so the live
// Policy is the only source consulted; there is no package-level state to
// shadow it from elsewhere in the binary.
func (enforcer *Enforcer) namespaceCheck(namespace string) error {
	if ok, reason := enforcer.policy.namespaceAllowed(namespace); !ok {
		return errString(reason)
	}
	return nil
}

type errString string

func (message errString) Error() string { return string(message) }

// maxReplicas reads MAX_REPLICAS out of the live feature-flag map. Keeping
// the parser on the Enforcer (rather than at the call site) means the
// constructor signature does not grow when more flags are added.
func (enforcer *Enforcer) maxReplicas() (int, error) {
	data, err := enforcer.flags()
	if err != nil {
		return 0, err
	}
	raw, ok := data["MAX_REPLICAS"]
	if !ok {
		return 0, fmt.Errorf("MAX_REPLICAS not set in feature-flag ConfigMap")
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid MAX_REPLICAS=%q", raw)
	}
	return value, nil
}
