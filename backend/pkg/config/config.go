package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr             string
	GRPCAddr             string
	DatabaseURL          string
	MigrationsURL        string
	KeycloakIssuerURL    string
	KeycloakJWKSURL      string
	KeycloakClientID     string
	KeycloakSkipAudience bool
	AllowedOrigins       []string
	ShutdownTimeout      time.Duration
}

func Load() (Config, error) {
	issuer := env("KEYCLOAK_ISSUER_URL", "http://localhost:8082/realms/taskflow")
	skipAudience, err := envBool("KEYCLOAK_SKIP_AUDIENCE", false)
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		HTTPAddr:             env("HTTP_ADDR", ":8080"),
		GRPCAddr:             env("GRPC_ADDR", ":50051"),
		DatabaseURL:          strings.TrimSpace(os.Getenv("DATABASE_URL")),
		MigrationsURL:        env("MIGRATIONS_URL", "file://migrations"),
		KeycloakIssuerURL:    issuer,
		KeycloakJWKSURL:      env("KEYCLOAK_JWKS_URL", strings.TrimRight(issuer, "/")+"/protocol/openid-connect/certs"),
		KeycloakClientID:     env("KEYCLOAK_CLIENT_ID", "taskflow-web"),
		KeycloakSkipAudience: skipAudience,
		AllowedOrigins:       splitCSV(env("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:8081")),
		ShutdownTimeout:      envDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	return cfg, nil
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envBool(name string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", name, err)
	}
	return parsed, nil
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}
