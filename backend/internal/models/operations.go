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

// UpdateEnvRequest merges env vars into a named container. envFrom is not in
// the schema and is never touched.
type UpdateEnvRequest struct {
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Container string            `json:"container"`
	Env       map[string]string `json:"env"`
}

func (request UpdateEnvRequest) Validate() error {
	if request.Namespace == "" {
		return errors.New("namespace is required")
	}
	if request.Name == "" {
		return errors.New("name is required")
	}
	if request.Container == "" {
		return errors.New("container is required")
	}
	if len(request.Env) == 0 {
		return errors.New("env must not be empty")
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
