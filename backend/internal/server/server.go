// Package server hosts the HTTP API surface. Routes here are *plumbing* -
// they parse, validate-structurally, dispatch, and translate errors. Policy
// enforcement is Phase 3's job and lives in internal/guardrails.
package server

import (
	"encoding/json"
	"net/http"

	"eks-control-plane/backend/internal/config"
)

// New builds the API handler. Optional Deps wire the cluster and operations
// route groups; nil deps simply disable that group, which keeps the
// constructor composable for degraded modes and per-feature tests.
func New(settings config.Settings, deps ...Deps) http.Handler {
	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/health", health)

	var resolved Deps
	if len(deps) > 0 {
		resolved = deps[0]
	}
	if resolved.Reader != nil {
		if resolved.Enforcer == nil {
			panic("server: Deps.Reader requires Deps.Enforcer; reads may not be exposed without a guardrail chokepoint")
		}
		mountClusterRoutes(serveMux, resolved.Reader, resolved.Enforcer, resolved.ClusterName, resolved.ClusterRegion)
	}
	if resolved.Ops != nil {
		if resolved.Enforcer == nil {
			panic("server: Deps.Ops requires Deps.Enforcer; mutations may not be exposed without a guardrail chokepoint")
		}
		mountOperationRoutes(serveMux, resolved.Ops, resolved.Enforcer)
	}
	return cors(settings.CORSOrigins)(serveMux)
}

func health(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func cors(allowed []string) func(http.Handler) http.Handler {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, allowedOrigin := range allowed {
		allowedSet[allowedOrigin] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			origin := request.Header.Get("Origin")
			if _, ok := allowedSet[origin]; ok {
				writer.Header().Set("Access-Control-Allow-Origin", origin)
				writer.Header().Set("Access-Control-Allow-Credentials", "true")
				writer.Header().Set("Vary", "Origin")
			}
			if request.Method == http.MethodOptions {
				writer.Header().Set("Access-Control-Allow-Methods", "*")
				writer.Header().Set("Access-Control-Allow-Headers", "*")
				writer.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(writer, request)
		})
	}
}

func writeJSON(writer http.ResponseWriter, status int, body any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(body)
}

func writeError(writer http.ResponseWriter, status int, msg string) {
	writeJSON(writer, status, map[string]string{"error": msg})
}
