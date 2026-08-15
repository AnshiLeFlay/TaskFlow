// Package main starts the TaskFlow REST, WebSocket, and gRPC interfaces.
//
// @title TaskFlow API
// @version 1.0
// @description Configurable project boards and workflow transitions.
// @BasePath /
// @schemes http https
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Keycloak access token in the form: Bearer {token}
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/example/taskflow/backend/internal/application"
	"github.com/example/taskflow/backend/internal/infrastructure/auth"
	"github.com/example/taskflow/backend/internal/infrastructure/migrations"
	"github.com/example/taskflow/backend/internal/infrastructure/postgres"
	"github.com/example/taskflow/backend/internal/infrastructure/realtime"
	"github.com/example/taskflow/backend/internal/interfaces/grpcapi"
	"github.com/example/taskflow/backend/internal/interfaces/httpapi"
	wsapi "github.com/example/taskflow/backend/internal/interfaces/websocket"
	"github.com/example/taskflow/backend/pkg/config"
	taskflowv1 "github.com/example/taskflow/backend/proto/taskflowv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := healthcheck(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("TaskFlow stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := migrateWithRetry(cfg.DatabaseURL, cfg.MigrationsURL, 30*time.Second); err != nil {
		return err
	}
	ctx := context.Background()
	repository, err := postgres.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer repository.Close()
	if err := repository.Ping(ctx); err != nil {
		return fmt.Errorf("ping PostgreSQL: %w", err)
	}
	validator, err := auth.NewKeycloakVerifier(ctx, cfg.KeycloakIssuerURL, cfg.KeycloakJWKSURL, cfg.KeycloakClientID, cfg.KeycloakSkipAudience)
	if err != nil {
		return err
	}
	broker := realtime.NewBroker()
	service := application.NewService(repository, broker)
	websocketHandler := wsapi.NewHandler(service, validator, broker, cfg.AllowedOrigins, logger)

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewRouter(service, validator, websocketHandler, "swagger", cfg.AllowedOrigins, logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	grpcListener, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return fmt.Errorf("listen gRPC: %w", err)
	}
	grpcServer := grpc.NewServer()
	taskflowv1.RegisterTaskEventsServer(grpcServer, grpcapi.NewServer(service, validator, broker))
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	reflection.Register(grpcServer)

	errCh := make(chan error, 2)
	go func() {
		logger.Info("HTTP server listening", "address", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("HTTP server: %w", err)
		}
	}()
	go func() {
		logger.Info("gRPC server listening", "address", cfg.GRPCAddr)
		if err := grpcServer.Serve(grpcListener); err != nil {
			errCh <- fmt.Errorf("gRPC server: %w", err)
		}
	}()

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var serveErr error
	select {
	case <-signalCtx.Done():
		logger.Info("shutdown signal received")
	case serveErr = <-errCh:
		logger.Error("server failed; shutting down", "error", serveErr)
	}
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	// WebSocket connections are hijacked from net/http and are not closed by
	// http.Server.Shutdown. Closing the broker first ends both WS and gRPC streams.
	broker.Close()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Warn("HTTP graceful shutdown timed out", "error", err)
	}
	grpcStopped := make(chan struct{})
	go func() { grpcServer.GracefulStop(); close(grpcStopped) }()
	select {
	case <-grpcStopped:
	case <-shutdownCtx.Done():
		grpcServer.Stop()
	}
	return serveErr
}

func migrateWithRetry(databaseURL, sourceURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		if err := migrations.Up(databaseURL, sourceURL); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			return lastErr
		}
		time.Sleep(time.Second)
	}
}

func healthcheck() error {
	address := os.Getenv("HEALTHCHECK_URL")
	if address == "" {
		address = "http://127.0.0.1:8080/healthz"
	}
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(address)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("health endpoint returned %s", response.Status)
	}
	return nil
}
