package server

import (
	"encoding/json"
	"io"
	"net/http"

	"eks-control-plane/backend/internal/guardrails"
	"eks-control-plane/backend/internal/kubernetes"
	"eks-control-plane/backend/internal/models"
)

// mountOperationRoutes wires mutation POSTs. Every handler runs the same
// pipeline: decode JSON → structural Validate → guardrail Enforce → dispatch.
// The Enforcer is the single chokepoint; routes never call Ops without it.
func mountOperationRoutes(serveMux *http.ServeMux, ops Operations, enforcer *guardrails.Enforcer) {
	serveMux.HandleFunc("POST /api/operations/scale", scaleHandler(ops, enforcer))
	serveMux.HandleFunc("POST /api/operations/rollout-restart", rolloutRestartHandler(ops, enforcer))
	serveMux.HandleFunc("POST /api/operations/pause-rollout", pauseRolloutHandler(ops, enforcer))
	serveMux.HandleFunc("POST /api/operations/resume-rollout", resumeRolloutHandler(ops, enforcer))
	serveMux.HandleFunc("POST /api/operations/rollback", rollbackHandler(ops, enforcer))
}

func scaleHandler(ops Operations, enforcer *guardrails.Enforcer) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		var body models.ScaleRequest
		if !decodeAndValidate(writer, request, &body) {
			return
		}
		decision := enforcer.Scale(body)
		if !writeIfDenied(writer, decision) {
			return
		}
		if err := ops.Scale(request.Context(), body.Namespace, body.Name, int32(body.Replicas)); err != nil {
			writeOpsError(writer, err)
			return
		}
		writeOk(writer, decision)
	}
}

func rolloutRestartHandler(ops Operations, enforcer *guardrails.Enforcer) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		var body models.RolloutRestartRequest
		if !decodeAndValidate(writer, request, &body) {
			return
		}
		decision := enforcer.RolloutRestart(body)
		if !writeIfDenied(writer, decision) {
			return
		}
		if err := ops.RolloutRestart(request.Context(), body.Namespace, body.Name); err != nil {
			writeOpsError(writer, err)
			return
		}
		writeOk(writer, decision)
	}
}

func pauseRolloutHandler(ops Operations, enforcer *guardrails.Enforcer) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		var body models.PauseRolloutRequest
		if !decodeAndValidate(writer, request, &body) {
			return
		}
		decision := enforcer.PauseRollout(body)
		if !writeIfDenied(writer, decision) {
			return
		}
		if err := ops.PauseRollout(request.Context(), body.Namespace, body.Name); err != nil {
			writeOpsError(writer, err)
			return
		}
		writeOk(writer, decision)
	}
}

func resumeRolloutHandler(ops Operations, enforcer *guardrails.Enforcer) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		var body models.ResumeRolloutRequest
		if !decodeAndValidate(writer, request, &body) {
			return
		}
		decision := enforcer.ResumeRollout(body)
		if !writeIfDenied(writer, decision) {
			return
		}
		if err := ops.ResumeRollout(request.Context(), body.Namespace, body.Name); err != nil {
			writeOpsError(writer, err)
			return
		}
		writeOk(writer, decision)
	}
}

func rollbackHandler(ops Operations, enforcer *guardrails.Enforcer) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		var body models.RollbackRequest
		if !decodeAndValidate(writer, request, &body) {
			return
		}
		decision := enforcer.Rollback(body)
		if !writeIfDenied(writer, decision) {
			return
		}
		if err := ops.Rollback(request.Context(), body.Namespace, body.Name, body.Revision); err != nil {
			writeOpsError(writer, err)
			return
		}
		writeOk(writer, decision)
	}
}

// validatable is anything our request models implement; the route layer does
// not care which model — it just decodes JSON and runs Validate.
type validatable interface {
	Validate() error
}

// decodeAndValidate returns true when the request was decoded + validated, in
// which case the caller proceeds. Otherwise it has already written the 4xx
// response and the caller must return.
func decodeAndValidate(writer http.ResponseWriter, request *http.Request, body validatable) bool {
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(body); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	if err := body.Validate(); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return false
	}

	var trailingToken any
	if err := decoder.Decode(&trailingToken); err != io.EOF {
		writeError(writer, http.StatusBadRequest, "invalid JSON body: must contain exactly one JSON object")
		return false
	}
	return true
}

// writeIfDenied returns true when the caller should proceed (decision allowed).
// On deny it writes a 403 with the audit decision in the body and returns false.
func writeIfDenied(writer http.ResponseWriter, decision guardrails.Decision) bool {
	if decision.Allow {
		return true
	}
	writeJSON(writer, http.StatusForbidden, map[string]any{
		"error":    decision.Reason,
		"decision": decision,
	})
	return false
}

// writeOk writes the success response shape that the UI consumes: status + the
// audit decision. Surfacing the decision on allow lets the UI render exactly
// what the enforcer recorded, with no separate fetch.
func writeOk(writer http.ResponseWriter, decision guardrails.Decision) {
	writeJSON(writer, http.StatusOK, map[string]any{
		"status":   "ok",
		"decision": decision,
	})
}

func writeOpsError(writer http.ResponseWriter, err error) {
	if kubernetes.IsNotFound(err) {
		writeError(writer, http.StatusNotFound, err.Error())
		return
	}
	writeError(writer, http.StatusInternalServerError, err.Error())
}
