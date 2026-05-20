package server

import (
	"errors"
	"net/http"
	"strconv"

	"eks-control-plane/backend/internal/guardrails"
	"eks-control-plane/backend/internal/kubernetes"
)

// mountClusterRoutes wires the read-only cluster GETs onto the supplied serveMux.
// Every namespaced read runs the same pipeline as a mutation: structural query
// parsing → guardrail Enforce → reader dispatch. ListNamespaces is the one
// route that does not deny — it narrows the cluster-wide list down to the
// allowlist. ListNodes is unguarded; the reads layer already returns names
// only and nothing else.
func mountClusterRoutes(serveMux *http.ServeMux, reader ClusterReader, enforcer *guardrails.Enforcer, clusterName, clusterRegion string) {
	serveMux.HandleFunc("GET /api/cluster/info", clusterInfoHandler(reader, clusterName, clusterRegion))
	serveMux.HandleFunc("GET /api/cluster/health", clusterHealthHandler(reader))
	serveMux.HandleFunc("GET /api/cluster/deployments", listDeploymentsHandler(reader, enforcer))
	serveMux.HandleFunc("GET /api/cluster/deployments/{name}", getDeploymentHandler(reader, enforcer))
	serveMux.HandleFunc("GET /api/cluster/pods", listPodsHandler(reader, enforcer))
	serveMux.HandleFunc("GET /api/cluster/events", listEventsHandler(reader, enforcer))
	serveMux.HandleFunc("GET /api/cluster/logs", tailLogsHandler(reader, enforcer))
	serveMux.HandleFunc("GET /api/cluster/services", listServicesHandler(reader, enforcer))
	serveMux.HandleFunc("GET /api/cluster/ingresses", listIngressesHandler(reader, enforcer))
	serveMux.HandleFunc("GET /api/cluster/hpas", listHPAsHandler(reader, enforcer))
	serveMux.HandleFunc("GET /api/cluster/namespaces", listNamespacesHandler(reader, enforcer))
	serveMux.HandleFunc("GET /api/cluster/nodes", listNodesHandler(reader))
	serveMux.HandleFunc("GET /api/cluster/replicasets", listReplicaSetsHandler(reader, enforcer))
}

func listDeploymentsHandler(reader ClusterReader, enforcer *guardrails.Enforcer) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		namespace := request.URL.Query().Get("namespace")
		if namespace == "" {
			writeError(writer, http.StatusBadRequest, "namespace is required")
			return
		}
		if !writeIfDenied(writer, enforcer.ListDeployments(namespace)) {
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

func getDeploymentHandler(reader ClusterReader, enforcer *guardrails.Enforcer) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		namespace := request.URL.Query().Get("namespace")
		if namespace == "" {
			writeError(writer, http.StatusBadRequest, "namespace is required")
			return
		}
		name := request.PathValue("name")
		if !writeIfDenied(writer, enforcer.GetDeployment(namespace, name)) {
			return
		}
		deployment, err := reader.GetDeployment(request.Context(), namespace, name)
		if err != nil {
			writeClusterError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, deployment)
	}
}

func listPodsHandler(reader ClusterReader, enforcer *guardrails.Enforcer) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		namespace := request.URL.Query().Get("namespace")
		if namespace == "" {
			writeError(writer, http.StatusBadRequest, "namespace is required")
			return
		}
		if !writeIfDenied(writer, enforcer.ListPods(namespace)) {
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

func listEventsHandler(reader ClusterReader, enforcer *guardrails.Enforcer) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		namespace := request.URL.Query().Get("namespace")
		if namespace == "" {
			writeError(writer, http.StatusBadRequest, "namespace is required")
			return
		}
		if !writeIfDenied(writer, enforcer.ListEvents(namespace)) {
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

func tailLogsHandler(reader ClusterReader, enforcer *guardrails.Enforcer) http.HandlerFunc {
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
		if !writeIfDenied(writer, enforcer.TailLogs(namespace, pod, container)) {
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

func listServicesHandler(reader ClusterReader, enforcer *guardrails.Enforcer) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		namespace := request.URL.Query().Get("namespace")
		if namespace == "" {
			writeError(writer, http.StatusBadRequest, "namespace is required")
			return
		}
		if !writeIfDenied(writer, enforcer.ListServices(namespace)) {
			return
		}
		services, err := reader.ListServices(request.Context(), namespace)
		if err != nil {
			writeClusterError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, services)
	}
}

func listIngressesHandler(reader ClusterReader, enforcer *guardrails.Enforcer) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		namespace := request.URL.Query().Get("namespace")
		if namespace == "" {
			writeError(writer, http.StatusBadRequest, "namespace is required")
			return
		}
		if !writeIfDenied(writer, enforcer.ListIngresses(namespace)) {
			return
		}
		ingresses, err := reader.ListIngresses(request.Context(), namespace)
		if err != nil {
			writeClusterError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, ingresses)
	}
}

func listHPAsHandler(reader ClusterReader, enforcer *guardrails.Enforcer) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		namespace := request.URL.Query().Get("namespace")
		if namespace == "" {
			writeError(writer, http.StatusBadRequest, "namespace is required")
			return
		}
		if !writeIfDenied(writer, enforcer.ListHorizontalPodAutoscalers(namespace)) {
			return
		}
		hpas, err := reader.ListHorizontalPodAutoscalers(request.Context(), namespace)
		if err != nil {
			writeClusterError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, hpas)
	}
}

// listNamespacesHandler narrows the cluster-wide list down to the allowlist.
// The route never denies — callers see only namespaces the Policy permits, so
// the UI cannot accidentally pivot into one it isn't allowed to act on.
func listNamespacesHandler(reader ClusterReader, enforcer *guardrails.Enforcer) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		namespaces, err := reader.ListNamespaces(request.Context())
		if err != nil {
			writeClusterError(writer, err)
			return
		}
		allowed := make([]Namespace, 0, len(namespaces))
		for _, namespace := range namespaces {
			if enforcer.NamespaceAllowed(namespace.Name) {
				allowed = append(allowed, namespace)
			}
		}
		writeJSON(writer, http.StatusOK, allowed)
	}
}

func listNodesHandler(reader ClusterReader) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		nodes, err := reader.ListNodes(request.Context())
		if err != nil {
			writeClusterError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, nodes)
	}
}

// clusterInfoHandler returns the configured cluster identity plus a live
// healthy flag. It bypasses the enforcer because the response carries no
// namespace-scoped data — only the operator's view of which cluster they are
// connected to.
func clusterInfoHandler(reader ClusterReader, name, region string) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		info, err := reader.ClusterInfo(request.Context(), name, region)
		if err != nil {
			writeClusterError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, info)
	}
}

// clusterHealthHandler returns just the live /livez verdict. Same enforcer
// bypass rationale as clusterInfoHandler — no namespace-scoped data leaves
// here. Split off so the UI can poll health on a tight cadence without
// re-fetching identity each tick.
func clusterHealthHandler(reader ClusterReader) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		health, err := reader.ClusterHealth(request.Context())
		if err != nil {
			writeClusterError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, health)
	}
}

func listReplicaSetsHandler(reader ClusterReader, enforcer *guardrails.Enforcer) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		namespace := request.URL.Query().Get("namespace")
		if namespace == "" {
			writeError(writer, http.StatusBadRequest, "namespace is required")
			return
		}
		if !writeIfDenied(writer, enforcer.ListReplicaSets(namespace)) {
			return
		}
		replicaSets, err := reader.ListReplicaSets(request.Context(), namespace)
		if err != nil {
			writeClusterError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, replicaSets)
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
