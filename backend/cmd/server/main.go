package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"eks-control-plane/backend/internal/config"
	applog "eks-control-plane/backend/internal/logging"
	"eks-control-plane/backend/internal/server"
)

func main() {
	cfg := config.Load()
	applog.Configure(cfg.LogLevel)

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8000"
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           server.New(cfg),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
