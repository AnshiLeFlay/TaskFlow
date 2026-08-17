package config

import (
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadKeycloakSkipAudienceDefaultsToFalseWhenMissing(t *testing.T) {
	setRequiredTestEnvironment(t)
	unsetEnvironment(t, "KEYCLOAK_SKIP_AUDIENCE")

	cfg, err := Load()
	require.NoError(t, err)
	assert.False(t, cfg.KeycloakSkipAudience)
}

func TestLoadKeycloakSkipAudienceAcceptsValidBooleans(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
		want  bool
	}{
		{name: "true", value: "true", want: true},
		{name: "false", value: "false", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			setRequiredTestEnvironment(t)
			t.Setenv("KEYCLOAK_SKIP_AUDIENCE", test.value)

			cfg, err := Load()
			require.NoError(t, err)
			assert.Equal(t, test.want, cfg.KeycloakSkipAudience)
		})
	}
}

func TestLoadKeycloakSkipAudienceRejectsInvalidBoolean(t *testing.T) {
	setRequiredTestEnvironment(t)
	t.Setenv("KEYCLOAK_SKIP_AUDIENCE", "sometimes")

	_, err := Load()
	require.Error(t, err)
	assert.ErrorContains(t, err, "KEYCLOAK_SKIP_AUDIENCE must be a boolean")
}

func TestLoadLogLevelDefaultsToInfoWhenMissing(t *testing.T) {
	setRequiredTestEnvironment(t)
	unsetEnvironment(t, "LOG_LEVEL")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, slog.LevelInfo, cfg.LogLevel)
}

func TestLoadLogLevelAcceptsKnownValuesCaseInsensitively(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
		want  slog.Level
	}{
		{name: "debug", value: "debug", want: slog.LevelDebug},
		{name: "info", value: "info", want: slog.LevelInfo},
		{name: "warn", value: "warn", want: slog.LevelWarn},
		{name: "error", value: "error", want: slog.LevelError},
		{name: "uppercase", value: "DEBUG", want: slog.LevelDebug},
	} {
		t.Run(test.name, func(t *testing.T) {
			setRequiredTestEnvironment(t)
			t.Setenv("LOG_LEVEL", test.value)

			cfg, err := Load()
			require.NoError(t, err)
			assert.Equal(t, test.want, cfg.LogLevel)
		})
	}
}

func TestLoadLogLevelRejectsUnknownValue(t *testing.T) {
	setRequiredTestEnvironment(t)
	t.Setenv("LOG_LEVEL", "verbose")

	_, err := Load()
	require.Error(t, err)
	assert.ErrorContains(t, err, "LOG_LEVEL must be one of debug|info|warn|error")
}

func TestLoadMCPDefaultsToDisabledWithSeparateSettings(t *testing.T) {
	setRequiredTestEnvironment(t)
	unsetEnvironment(t, "MCP_ENABLED")
	unsetEnvironment(t, "MCP_PUBLIC_URL")
	unsetEnvironment(t, "MCP_AUDIENCE")
	unsetEnvironment(t, "MCP_ALLOWED_ORIGINS")
	unsetEnvironment(t, "CORS_ALLOWED_ORIGINS")

	cfg, err := Load()
	require.NoError(t, err)
	assert.False(t, cfg.MCPEnabled)
	assert.Equal(t, "http://localhost:8080/mcp", cfg.MCPPublicURL)
	assert.Equal(t, cfg.MCPPublicURL, cfg.MCPAudience)
	assert.Equal(t, []string{"http://localhost:5173", "http://localhost:8081"}, cfg.MCPAllowedOrigins)
}

func TestLoadMCPAcceptsHTTPSAndExplicitAudience(t *testing.T) {
	setRequiredTestEnvironment(t)
	t.Setenv("MCP_ENABLED", "true")
	t.Setenv("MCP_PUBLIC_URL", "https://taskflow.example.com/mcp")
	t.Setenv("MCP_AUDIENCE", "https://taskflow.example.com/mcp")
	t.Setenv("MCP_ALLOWED_ORIGINS", "https://agent.example.com, https://admin.example.com")

	cfg, err := Load()
	require.NoError(t, err)
	assert.True(t, cfg.MCPEnabled)
	assert.Equal(t, cfg.MCPPublicURL, cfg.MCPAudience)
	assert.Equal(t, []string{"https://agent.example.com", "https://admin.example.com"}, cfg.MCPAllowedOrigins)
}

func TestLoadMCPRejectsAudienceDifferentFromResource(t *testing.T) {
	setRequiredTestEnvironment(t)
	t.Setenv("MCP_ENABLED", "true")
	t.Setenv("MCP_PUBLIC_URL", "https://taskflow.example.com/mcp")
	t.Setenv("MCP_AUDIENCE", "taskflow-mcp")

	_, err := Load()
	require.Error(t, err)
	assert.ErrorContains(t, err, "must equal MCP_PUBLIC_URL")
}

func TestLoadMCPAllowsHTTPOnlyForLoopback(t *testing.T) {
	for _, publicURL := range []string{
		"http://localhost:8080/mcp",
		"http://127.0.0.1:8080/mcp",
		"http://[::1]:8080/mcp",
	} {
		t.Run(publicURL, func(t *testing.T) {
			setRequiredTestEnvironment(t)
			t.Setenv("MCP_ENABLED", "true")
			t.Setenv("MCP_PUBLIC_URL", publicURL)
			_, err := Load()
			require.NoError(t, err)
		})
	}
}

func TestLoadMCPAllowsEmptyBrowserOriginList(t *testing.T) {
	setRequiredTestEnvironment(t)
	t.Setenv("MCP_ENABLED", "true")
	t.Setenv("MCP_ALLOWED_ORIGINS", "")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Empty(t, cfg.MCPAllowedOrigins)
}

func TestLoadMCPRejectsInsecureNonLocalPublicURL(t *testing.T) {
	setRequiredTestEnvironment(t)
	t.Setenv("MCP_ENABLED", "true")
	t.Setenv("MCP_PUBLIC_URL", "http://taskflow.example.com/mcp")

	_, err := Load()
	require.Error(t, err)
	assert.ErrorContains(t, err, "must use HTTPS")
}

func TestLoadMCPRejectsInvalidEnabledConfiguration(t *testing.T) {
	for _, test := range []struct {
		name      string
		envName   string
		envValue  string
		wantError string
	}{
		{name: "invalid enabled flag", envName: "MCP_ENABLED", envValue: "sometimes", wantError: "MCP_ENABLED must be a boolean"},
		{name: "unsupported URL scheme", envName: "MCP_PUBLIC_URL", envValue: "ftp://localhost/mcp", wantError: "must use HTTP or HTTPS"},
		{name: "wrong URL path", envName: "MCP_PUBLIC_URL", envValue: "http://localhost:8080/rpc", wantError: "path must be /mcp"},
		{name: "URL query", envName: "MCP_PUBLIC_URL", envValue: "http://localhost:8080/mcp?tenant=a", wantError: "must not contain"},
	} {
		t.Run(test.name, func(t *testing.T) {
			setRequiredTestEnvironment(t)
			t.Setenv("MCP_ENABLED", "true")
			t.Setenv(test.envName, test.envValue)
			_, err := Load()
			require.Error(t, err)
			assert.ErrorContains(t, err, test.wantError)
		})
	}
}

// These tests deliberately do not call t.Parallel: process environment is shared.
func setRequiredTestEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://taskflow:test@localhost:5432/taskflow")
}

func unsetEnvironment(t *testing.T, name string) {
	t.Helper()
	value, existed := os.LookupEnv(name)
	require.NoError(t, os.Unsetenv(name))
	t.Cleanup(func() {
		if existed {
			require.NoError(t, os.Setenv(name, value))
			return
		}
		require.NoError(t, os.Unsetenv(name))
	})
}
