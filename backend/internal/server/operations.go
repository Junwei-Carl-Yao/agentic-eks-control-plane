package server

import (
	"encoding/json"
	"io"
	"net/http"

	"eks-control-plane/backend/internal/kubernetes"
	"eks-control-plane/backend/internal/models"
)

// mountOperationRoutes wires mutation POSTs. Each route does the same three
// things: decode JSON, run structural validation, dispatch. Phase 3 will
// insert a guardrail enforcer between validation and dispatch.
func mountOperationRoutes(serveMux *http.ServeMux, ops Operations) {
	serveMux.HandleFunc("POST /api/operations/scale", scaleHandler(ops))
	serveMux.HandleFunc("POST /api/operations/rollout-restart", rolloutRestartHandler(ops))
	serveMux.HandleFunc("POST /api/operations/pause-rollout", pauseRolloutHandler(ops))
	serveMux.HandleFunc("POST /api/operations/resume-rollout", resumeRolloutHandler(ops))
	serveMux.HandleFunc("POST /api/operations/rollback", rollbackHandler(ops))
	serveMux.HandleFunc("POST /api/operations/update-env", updateEnvHandler(ops))
}

func scaleHandler(ops Operations) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		var body models.ScaleRequest
		if !decodeAndValidate(writer, request, &body) {
			return
		}
		if err := ops.Scale(request.Context(), body.Namespace, body.Name, int32(body.Replicas)); err != nil {
			writeOpsError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func rolloutRestartHandler(ops Operations) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		var body models.RolloutRestartRequest
		if !decodeAndValidate(writer, request, &body) {
			return
		}
		if err := ops.RolloutRestart(request.Context(), body.Namespace, body.Name); err != nil {
			writeOpsError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func pauseRolloutHandler(ops Operations) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		var body models.PauseRolloutRequest
		if !decodeAndValidate(writer, request, &body) {
			return
		}
		if err := ops.PauseRollout(request.Context(), body.Namespace, body.Name); err != nil {
			writeOpsError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func resumeRolloutHandler(ops Operations) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		var body models.ResumeRolloutRequest
		if !decodeAndValidate(writer, request, &body) {
			return
		}
		if err := ops.ResumeRollout(request.Context(), body.Namespace, body.Name); err != nil {
			writeOpsError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func rollbackHandler(ops Operations) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		var body models.RollbackRequest
		if !decodeAndValidate(writer, request, &body) {
			return
		}
		if err := ops.Rollback(request.Context(), body.Namespace, body.Name, body.Revision); err != nil {
			writeOpsError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func updateEnvHandler(ops Operations) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		var body models.UpdateEnvRequest
		if !decodeAndValidate(writer, request, &body) {
			return
		}
		if err := ops.UpdateEnv(request.Context(), body.Namespace, body.Name, body.Container, body.Env); err != nil {
			writeOpsError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// validatable is anything our request models implement; the route layer does
// not care which model - it just decodes JSON and runs Validate.
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

func writeOpsError(writer http.ResponseWriter, err error) {
	if kubernetes.IsNotFound(err) {
		writeError(writer, http.StatusNotFound, err.Error())
		return
	}
	writeError(writer, http.StatusInternalServerError, err.Error())
}
