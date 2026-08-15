package config

import (
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
