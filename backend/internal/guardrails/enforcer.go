package guardrails

import (
	"log/slog"

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

// Enforcer is the single chokepoint.
type Enforcer struct {
	policy Policy
	logger *slog.Logger
}

// New returns an Enforcer bound to the supplied Policy. The Policy slices are
// copied defensively, so callers cannot widen the safety boundary by mutating
// slices they passed in. A nil logger falls back to the slog default so
// callers can omit it in tests.
func New(policy Policy, logger *slog.Logger) *Enforcer {
	if logger == nil {
		logger = slog.Default()
	}
	sealed := Policy{
		AllowedNamespaces: append([]string(nil), policy.AllowedNamespaces...),
	}
	return &Enforcer{policy: sealed, logger: logger}
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

// Scale enforces namespace allowlist + DNS-1123 + replica bounds.
func (enforcer *Enforcer) Scale(request models.ScaleRequest) Decision {
	subject := request.Namespace + "/" + request.Name
	if reason := firstError(
		validNamespace(request.Namespace),
		validResourceName(request.Name),
		enforcer.namespaceCheck(request.Namespace),
	); reason != "" {
		return enforcer.deny("scale", subject, reason)
	}
	if err := validReplicas(request.Replicas, MaxReplicas); err != nil {
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
