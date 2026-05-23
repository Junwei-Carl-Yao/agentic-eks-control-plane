package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"eks-control-plane/backend/internal/config"
	"eks-control-plane/backend/internal/guardrails"
	"eks-control-plane/backend/internal/kubernetes"
	applog "eks-control-plane/backend/internal/logging"
	"eks-control-plane/backend/internal/server"
)

// shutdownTimeout caps how long graceful shutdown waits for in-flight
// requests after SIGTERM. Picked to fit inside the default Kubernetes
// terminationGracePeriodSeconds (30s) so the kubelet's SIGKILL never wins
// the race when nothing is genuinely stuck.
const shutdownTimeout = 25 * time.Second

func main() {
	settings := config.Load()
	logger := applog.Configure()

	address := os.Getenv("ADDR")
	if address == "" {
		address = ":8000"
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)

	if err := run(settings, address, logger, signals); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

// run wires the HTTP server, starts it, and blocks until either the listener
// fails or a shutdown signal arrives on signals. Extracted from main so the
// shutdown path is testable end-to-end without spawning a process or
// wrestling with os.Exit.
func run(settings config.Settings, address string, logger *slog.Logger, signals <-chan os.Signal) error {
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

	return serve(httpServer, logger, signals)
}

// serve runs httpServer and returns when either the listener errors or a
// shutdown signal arrives on signals. Splitting this off lets tests inject a
// pre-built *http.Server (with a httptest listener) and observe the full
// shutdown handshake without going through run's wiring.
func serve(httpServer *http.Server, logger *slog.Logger, signals <-chan os.Signal) error {
	listenErrors := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			listenErrors <- err
		}
		close(listenErrors)
	}()

	select {
	case err := <-listenErrors:
		return err
	case received := <-signals:
		logger.Info("shutdown signal received", "signal", received.String())
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownContext); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
		return err
	}
	return nil
}
