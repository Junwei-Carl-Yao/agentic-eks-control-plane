// Package models defines the request/response shapes for mutation operations.
//
// Validate methods here perform *structural* checks only (required fields, type
// bounds). Policy enforcement (DNS-1123 names, MAX_REPLICAS per namespace,
// env-var denylists, blocked namespaces) lives in internal/guardrails (Phase 3).
package models

import "errors"

// ScaleRequest sets a deployment's replica count.
type ScaleRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Replicas  int    `json:"replicas"`
}

func (request ScaleRequest) Validate() error {
	if request.Namespace == "" {
		return errors.New("namespace is required")
	}
	if request.Name == "" {
		return errors.New("name is required")
	}
	if request.Replicas < 1 {
		return errors.New("replicas must be >= 1")
	}
	return nil
}

// RolloutRestartRequest triggers a rolling restart by patching the pod-template
// annotation kubectl uses, so existing tooling treats it as a normal restart.
type RolloutRestartRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

func (request RolloutRestartRequest) Validate() error {
	if request.Namespace == "" {
		return errors.New("namespace is required")
	}
	if request.Name == "" {
		return errors.New("name is required")
	}
	return nil
}

// PauseRolloutRequest / ResumeRolloutRequest share the same shape as restart.
type PauseRolloutRequest = RolloutRestartRequest
type ResumeRolloutRequest = RolloutRestartRequest

// UpdateFeatureFlagRequest writes a single key into a named ConfigMap's data.
// Other keys, binaryData, and ConfigMaps not on the enforcer's allowlist are
// untouched.
type UpdateFeatureFlagRequest struct {
	Namespace string `json:"namespace"`
	ConfigMap string `json:"configmap"`
	Key       string `json:"key"`
	Value     string `json:"value"`
}

func (request UpdateFeatureFlagRequest) Validate() error {
	if request.Namespace == "" {
		return errors.New("namespace is required")
	}
	if request.ConfigMap == "" {
		return errors.New("configmap is required")
	}
	if request.Key == "" {
		return errors.New("key is required")
	}
	return nil
}

// RollbackRequest reverts to a specific revision. Revision == 0 means "previous"
// (matches `kubectl rollout undo` semantics). Negative revisions are invalid.
type RollbackRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Revision  int64  `json:"revision"`
}

func (request RollbackRequest) Validate() error {
	if request.Namespace == "" {
		return errors.New("namespace is required")
	}
	if request.Name == "" {
		return errors.New("name is required")
	}
	if request.Revision < 0 {
		return errors.New("revision must be >= 0 (0 means previous)")
	}
	return nil
}
