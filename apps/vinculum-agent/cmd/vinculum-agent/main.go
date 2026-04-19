package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/florian/vinculum/apps/vinculum-agent/internal/agent"
	"github.com/florian/vinculum/apps/vinculum-agent/internal/config"
	"github.com/florian/vinculum/apps/vinculum-agent/internal/httpapi"
	"github.com/florian/vinculum/apps/vinculum-agent/internal/kube"
	"github.com/florian/vinculum/apps/vinculum-agent/internal/tasks"
)

func main() {
	cfg := config.Load()
	logger := log.New(os.Stdout, "vinculum-agent ", log.LstdFlags|log.Lmsgprefix)

	if err := os.MkdirAll(cfg.WorkspaceRoot, 0o755); err != nil {
		logger.Fatalf("create workspace: %v", err)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		logger.Fatalf("create data dir: %v", err)
	}

	var kubeClient *kube.Client
	if cfg.Namespace != "" {
		kc, err := kube.NewInCluster(cfg.Namespace)
		if err != nil {
			logger.Printf("kube client unavailable: %v (status patches disabled)", err)
		} else {
			kubeClient = kc
		}
	} else {
		logger.Printf("AGENT_NAMESPACE empty; status patches disabled")
	}

	executor := agent.NewExecutor(cfg, logger)
	runner := tasks.NewRunner(cfg, logger, kubeClient, executor)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go runner.Run(ctx)

	server := &http.Server{
		Addr:              cfg.ServerAddr,
		Handler:           httpapi.New(cfg, runner).Mux(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Printf("agent=%s ns=%s listening on %s", cfg.AgentName, cfg.Namespace, cfg.ServerAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("server: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Printf("shutdown: %v", err)
	}
}
