package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr                string
	GRPCAddr                string
	DatabaseURL             string
	MigrationsURL           string
	KeycloakIssuerURL       string
	KeycloakJWKSURL         string
	KeycloakClientID        string
	KeycloakSkipAudience    bool
	KeycloakAdminURL        string
	KeycloakRealm           string
	KeycloakDirectoryID     string
	KeycloakDirectorySecret string
	AllowedOrigins          []string
	MCPEnabled              bool
	MCPPublicURL            string
	MCPAudience             string
	MCPAllowedOrigins       []string
	MCPInsecureHTTPHosts    []string
	MCPServiceToken         string
	MCPServiceSubject       string
	MCPServiceUsername      string
	MCPServiceRoles         []string
	ShutdownTimeout         time.Duration
	LogLevel                slog.Level
}

func Load() (Config, error) {
	issuer := env("KEYCLOAK_ISSUER_URL", "http://localhost:8082/realms/taskflow")
	skipAudience, err := envBool("KEYCLOAK_SKIP_AUDIENCE", false)
	if err != nil {
		return Config{}, err
	}
	mcpEnabled, err := envBool("MCP_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	logLevel, err := envLogLevel("LOG_LEVEL", slog.LevelInfo)
	if err != nil {
		return Config{}, err
	}
	corsOrigins := env("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:8081")
	mcpPublicURL := env("MCP_PUBLIC_URL", "http://localhost:8080/mcp")
	cfg := Config{
		HTTPAddr:                env("HTTP_ADDR", ":8080"),
		GRPCAddr:                env("GRPC_ADDR", ":50051"),
		DatabaseURL:             strings.TrimSpace(os.Getenv("DATABASE_URL")),
		MigrationsURL:           env("MIGRATIONS_URL", "file://migrations"),
		KeycloakIssuerURL:       issuer,
		KeycloakJWKSURL:         env("KEYCLOAK_JWKS_URL", strings.TrimRight(issuer, "/")+"/protocol/openid-connect/certs"),
		KeycloakClientID:        env("KEYCLOAK_CLIENT_ID", "taskflow-web"),
		KeycloakSkipAudience:    skipAudience,
		KeycloakAdminURL:        env("KEYCLOAK_ADMIN_URL", "http://keycloak:8080"),
		KeycloakRealm:           env("KEYCLOAK_REALM", "taskflow"),
		KeycloakDirectoryID:     env("KEYCLOAK_DIRECTORY_CLIENT_ID", "taskflow-backend"),
		KeycloakDirectorySecret: env("KEYCLOAK_DIRECTORY_CLIENT_SECRET", "taskflow-backend-secret"),
		AllowedOrigins:          splitCSV(corsOrigins),
		MCPEnabled:              mcpEnabled,
		MCPPublicURL:            mcpPublicURL,
		MCPAudience:             env("MCP_AUDIENCE", mcpPublicURL),
		MCPAllowedOrigins:       envCSV("MCP_ALLOWED_ORIGINS", corsOrigins),
		MCPInsecureHTTPHosts:    envCSV("MCP_INSECURE_HTTP_HOSTS", ""),
		MCPServiceToken:         strings.TrimSpace(os.Getenv("MCP_SERVICE_TOKEN")),
		MCPServiceSubject:       strings.TrimSpace(os.Getenv("MCP_SERVICE_SUBJECT")),
		MCPServiceUsername:      env("MCP_SERVICE_USERNAME", "mcp-service"),
		MCPServiceRoles:         envCSV("MCP_SERVICE_ROLES", ""),
		ShutdownTimeout:         envDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
		LogLevel:                logLevel,
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.MCPEnabled {
		if err := validateMCPPublicURL(cfg.MCPPublicURL, cfg.MCPInsecureHTTPHosts); err != nil {
			return Config{}, err
		}
		if strings.TrimSpace(cfg.MCPAudience) == "" {
			return Config{}, fmt.Errorf("MCP_AUDIENCE is required when MCP is enabled")
		}
		if cfg.MCPAudience != cfg.MCPPublicURL {
			return Config{}, fmt.Errorf("MCP_AUDIENCE must equal MCP_PUBLIC_URL")
		}
		// The static service token is an escape hatch for MCP clients that
		// cannot run the OAuth flow. It is off unless set, and refuses to
		// start half-configured: a token without a subject would authenticate
		// a caller TaskFlow cannot attribute any action to.
		if cfg.MCPServiceToken != "" && cfg.MCPServiceSubject == "" {
			return Config{}, fmt.Errorf("MCP_SERVICE_SUBJECT is required when MCP_SERVICE_TOKEN is set")
		}
		if cfg.MCPServiceToken == "" && cfg.MCPServiceSubject != "" {
			return Config{}, fmt.Errorf("MCP_SERVICE_TOKEN is required when MCP_SERVICE_SUBJECT is set")
		}
	}
	return cfg, nil
}

func validateMCPPublicURL(rawURL string, insecureHTTPHosts []string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("MCP_PUBLIC_URL must be an absolute HTTP(S) URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("MCP_PUBLIC_URL must not contain user info, a query, or a fragment")
	}
	if parsed.Path != "/mcp" {
		return fmt.Errorf("MCP_PUBLIC_URL path must be /mcp")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		return nil
	case "http":
		host := parsed.Hostname()
		ip := net.ParseIP(host)
		if strings.EqualFold(host, "localhost") || (ip != nil && ip.IsLoopback()) || hostAllowed(host, insecureHTTPHosts) {
			return nil
		}
		return fmt.Errorf("MCP_PUBLIC_URL must use HTTPS unless its host is localhost, a loopback address, or explicitly listed in MCP_INSECURE_HTTP_HOSTS")
	default:
		return fmt.Errorf("MCP_PUBLIC_URL must use HTTP or HTTPS")
	}
}

func hostAllowed(host string, allowed []string) bool {
	for _, candidate := range allowed {
		if strings.EqualFold(strings.TrimSpace(candidate), host) {
			return true
		}
	}
	return false
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envCSV(name, fallback string) []string {
	value, exists := os.LookupEnv(name)
	if !exists {
		value = fallback
	}
	return splitCSV(value)
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

func envLogLevel(name string, fallback slog.Level) (slog.Level, error) {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	switch value {
	case "":
		return fallback, nil
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return fallback, fmt.Errorf("%s must be one of debug|info|warn|error", name)
	}
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
