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
