package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"eks-control-plane/backend/internal/config"
	"eks-control-plane/backend/internal/guardrails"
	"eks-control-plane/backend/internal/kubernetes"
	applog "eks-control-plane/backend/internal/logging"
	"eks-control-plane/backend/internal/server"
)

func main() {
	settings := config.Load()
	logger := applog.Configure()

	address := os.Getenv("ADDR")
	if address == "" {
		address = ":8000"
	}

	deps := server.Deps{}
	if kubeClient, err := kubernetes.NewClient(settings); err != nil {
		// In local-dev with no KUBECONFIG, the cluster routes simply stay
		// unmounted. Log and continue rather than refusing to start - /health
		// remains useful.
		logger.Warn("kubernetes client unavailable; cluster routes disabled", "err", err)
	} else {
		policy := guardrails.DefaultPolicy()
		deps.Reader = kubeClient
		deps.Ops = kubeClient
		deps.Enforcer = guardrails.New(policy, featureFlagsFromConfigMap(kubeClient, policy.AllowedNamespaces[0]), logger)
	}

	httpServer := &http.Server{
		Addr:              address,
		Handler:           server.New(settings, deps),
		ReadHeaderTimeout: 10 * time.Second,
	}
	logger.Info("listening", "addr", address)
	log.Fatal(httpServer.ListenAndServe())
}

// featureFlagsFromConfigMap returns a loader that reads the feature-flag
// ConfigMap on every call, so operators can adjust flags by editing the
// ConfigMap rather than restarting the binary. Adding a new flag means
// adding a parser on the Enforcer that consumes the returned map; the fetch
// path stays put. The namespace is supplied by the caller so the loader
// doesn't reach back into guardrails for a global.
func featureFlagsFromConfigMap(client *kubernetes.Client, namespace string) func() (map[string]string, error) {
	return func() (map[string]string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return client.GetFeatureFlags(ctx, namespace, guardrails.FeatureFlagConfigMap)
	}
}
