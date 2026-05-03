package server

import (
	"errors"
	"net/http"
	"strconv"

	"eks-control-plane/backend/internal/kubernetes"
)

// mountClusterRoutes wires the read-only cluster GETs onto the supplied serveMux.
// Method-prefixed patterns rely on Go 1.22+ ServeMux semantics: a registered
// path with the wrong method returns 405 automatically.
func mountClusterRoutes(serveMux *http.ServeMux, reader ClusterReader) {
	serveMux.HandleFunc("GET /api/cluster/deployments", listDeploymentsHandler(reader))
	serveMux.HandleFunc("GET /api/cluster/deployments/{name}", getDeploymentHandler(reader))
	serveMux.HandleFunc("GET /api/cluster/pods", listPodsHandler(reader))
	serveMux.HandleFunc("GET /api/cluster/events", listEventsHandler(reader))
	serveMux.HandleFunc("GET /api/cluster/logs", tailLogsHandler(reader))
}

func listDeploymentsHandler(reader ClusterReader) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		namespace := request.URL.Query().Get("namespace")
		if namespace == "" {
			writeError(writer, http.StatusBadRequest, "namespace is required")
			return
		}
		deployments, err := reader.ListDeployments(request.Context(), namespace)
		if err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(writer, http.StatusOK, deployments)
	}
}

func getDeploymentHandler(reader ClusterReader) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		namespace := request.URL.Query().Get("namespace")
		if namespace == "" {
			writeError(writer, http.StatusBadRequest, "namespace is required")
			return
		}
		name := request.PathValue("name")
		deployment, err := reader.GetDeployment(request.Context(), namespace, name)
		if err != nil {
			writeClusterError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, deployment)
	}
}

func listPodsHandler(reader ClusterReader) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		namespace := request.URL.Query().Get("namespace")
		if namespace == "" {
			writeError(writer, http.StatusBadRequest, "namespace is required")
			return
		}
		selector := request.URL.Query().Get("labelSelector")
		pods, err := reader.ListPods(request.Context(), namespace, selector)
		if err != nil {
			writeClusterError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, pods)
	}
}

func listEventsHandler(reader ClusterReader) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		namespace := request.URL.Query().Get("namespace")
		if namespace == "" {
			writeError(writer, http.StatusBadRequest, "namespace is required")
			return
		}
		events, err := reader.ListEvents(request.Context(), namespace)
		if err != nil {
			writeClusterError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, events)
	}
}

func tailLogsHandler(reader ClusterReader) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		namespace := query.Get("namespace")
		pod := query.Get("pod")
		container := query.Get("container")
		linesStr := query.Get("lines")
		if namespace == "" || pod == "" || container == "" || linesStr == "" {
			writeError(writer, http.StatusBadRequest, "namespace, pod, container, lines are all required")
			return
		}
		lines, err := strconv.ParseInt(linesStr, 10, 64)
		if err != nil || lines <= 0 {
			writeError(writer, http.StatusBadRequest, "lines must be a positive integer")
			return
		}
		body, err := reader.TailLogs(request.Context(), namespace, pod, container, lines)
		if err != nil {
			writeClusterError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]string{"logs": body})
	}
}

// writeClusterError maps a reads-layer error to the right status code. Missing
// resources are 404; everything else is treated as upstream failure.
func writeClusterError(writer http.ResponseWriter, err error) {
	if kubernetes.IsNotFound(err) || errors.Is(err, kubernetes.ErrNotFound) {
		writeError(writer, http.StatusNotFound, err.Error())
		return
	}
	writeError(writer, http.StatusInternalServerError, err.Error())
}
