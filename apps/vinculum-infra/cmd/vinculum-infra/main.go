package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/florian/vinculum/apps/vinculum-infra/internal/bootstrap"
	"github.com/florian/vinculum/apps/vinculum-infra/internal/config"
	"github.com/florian/vinculum/apps/vinculum-infra/internal/forgejo"
	"github.com/florian/vinculum/apps/vinculum-infra/internal/keycloak"
)

func main() {
	cfg := config.Load()
	logger := log.New(os.Stdout, "vinculum-infra ", log.LstdFlags|log.Lmsgprefix)

	service := bootstrap.NewService(
		logger,
		keycloak.NewClient(cfg.Keycloak),
		forgejo.NewClient(cfg.Forgejo),
		cfg,
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		result, err := service.Bootstrap(ctx)
		if err != nil {
			logger.Printf("bootstrap failed: %v", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, result)
	})

	server := &http.Server{
		Addr:              cfg.ServerAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	if cfg.AutoBootstrap {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			result, err := retryBootstrap(ctx, logger, service)
			if err != nil {
				logger.Printf("startup bootstrap failed: %v", err)
				return
			}

			logger.Printf("startup bootstrap completed: realm_created=%t client_created=%t org_created=%t",
				result.Keycloak.RealmCreated,
				result.Keycloak.ClientCreated,
				result.Forgejo.OrganizationCreated,
			)
		}()
	}

	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Printf("listening on %s", cfg.ServerAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("server failed: %v", err)
		}
	}()

	<-shutdownCtx.Done()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Printf("shutdown failed: %v", err)
	}
}

func retryBootstrap(ctx context.Context, logger *log.Logger, service *bootstrap.Service) (bootstrap.Result, error) {
	backoff := 2 * time.Second
	for attempt := 1; ; attempt++ {
		result, err := service.Bootstrap(ctx)
		if err == nil {
			if attempt > 1 {
				logger.Printf("startup bootstrap succeeded after %d attempts", attempt)
			}
			return result, nil
		}

		if ctx.Err() != nil {
			return bootstrap.Result{}, ctx.Err()
		}

		logger.Printf("startup bootstrap attempt %d failed: %v", attempt, err)

		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return bootstrap.Result{}, ctx.Err()
		case <-timer.C:
		}

		if backoff < 15*time.Second {
			backoff *= 2
			if backoff > 15*time.Second {
				backoff = 15 * time.Second
			}
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
