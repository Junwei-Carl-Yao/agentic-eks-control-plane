package main

import (
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

	deps := server.Deps{
		ClusterName:   settings.ClusterName,
		ClusterRegion: settings.AWSRegion,
	}
	if kubeClient, err := kubernetes.NewClient(settings); err != nil {
		// In local-dev with no KUBECONFIG, the cluster routes simply stay
		// unmounted. Log and continue rather than refusing to start - /health
		// remains useful.
		logger.Warn("kubernetes client unavailable; cluster routes disabled", "err", err)
	} else {
		deps.Reader = kubeClient
		deps.Ops = kubeClient
		deps.Enforcer = guardrails.New(guardrails.DefaultPolicy(), logger)
	}

	httpServer := &http.Server{
		Addr:              address,
		Handler:           server.New(settings, deps),
		ReadHeaderTimeout: 10 * time.Second,
	}
	logger.Info("listening", "addr", address)
	log.Fatal(httpServer.ListenAndServe())
}
