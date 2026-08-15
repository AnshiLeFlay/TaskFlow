package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeycloakDirectoryListsEnabledUsersWithIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/realms/taskflow/protocol/openid-connect/token":
			require.NoError(t, r.ParseForm())
			assert.Equal(t, "client_credentials", r.Form.Get("grant_type"))
			assert.Equal(t, "taskflow-backend", r.Form.Get("client_id"))
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "directory-token", "expires_in": 60})
		case "/admin/realms/taskflow/users":
			assert.Equal(t, "Bearer directory-token", r.Header.Get("Authorization"))
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": "1", "username": "alice", "email": "alice@example.test", "firstName": "Alice", "lastName": "Admin", "enabled": true},
				{"id": "2", "username": "disabled", "enabled": false},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	directory := NewKeycloakDirectory(server.URL, "taskflow", "taskflow-backend", "secret")
	users, err := directory.ListUsers(context.Background())

	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.Equal(t, "alice", users[0].Username)
	assert.Equal(t, "Alice Admin", users[0].Name)
}
