package mcpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/example/taskflow/backend/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testTokenValidator func(context.Context, string) (domain.User, error)

func (v testTokenValidator) Verify(ctx context.Context, token string) (domain.User, error) {
	return v(ctx, token)
}

func newTestHTTPAuth(t *testing.T, validator testTokenValidator, origins ...string) *HTTPAuth {
	t.Helper()
	httpAuth, err := NewHTTPAuth(validator, HTTPAuthOptions{
		PublicURL:           "https://taskflow.example.com/mcp",
		AuthorizationServer: "https://id.example.com/realms/taskflow/",
		AllowedOrigins:      origins,
	})
	require.NoError(t, err)
	return httpAuth
}

func TestProtectedResourceMetadata(t *testing.T) {
	httpAuth := newTestHTTPAuth(t, func(context.Context, string) (domain.User, error) {
		return domain.User{}, nil
	})

	request := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource/mcp", nil)
	recorder := httptest.NewRecorder()
	httpAuth.MetadataHandler().ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "*", recorder.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "https://taskflow.example.com/.well-known/oauth-protected-resource/mcp", httpAuth.MetadataURL())
	var metadata ProtectedResourceMetadata
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &metadata))
	assert.Equal(t, "https://taskflow.example.com/mcp", metadata.Resource)
	assert.Equal(t, []string{"https://id.example.com/realms/taskflow"}, metadata.AuthorizationServers)
	assert.Equal(t, []string{DefaultOAuthScope}, metadata.ScopesSupported)
	assert.Equal(t, []string{"header"}, metadata.BearerMethodsSupported)
}

func TestProtectedResourceMetadataSupportsHeadAndRejectsMutation(t *testing.T) {
	httpAuth := newTestHTTPAuth(t, func(context.Context, string) (domain.User, error) {
		return domain.User{}, nil
	})

	head := httptest.NewRecorder()
	httpAuth.MetadataHandler().ServeHTTP(head, httptest.NewRequest(http.MethodHead, "/", nil))
	assert.Equal(t, http.StatusOK, head.Code)
	assert.Empty(t, head.Body.String())

	post := httptest.NewRecorder()
	httpAuth.MetadataHandler().ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, post.Code)
	assert.Equal(t, "GET, HEAD", post.Header().Get("Allow"))
}

func TestMCPRequiresBearerTokenAndAdvertisesMetadata(t *testing.T) {
	httpAuth := newTestHTTPAuth(t, func(context.Context, string) (domain.User, error) {
		return domain.User{}, errors.New("invalid")
	})
	handler := httpAuth.Protect(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("unauthenticated request reached MCP handler")
	}))

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodPost, "/mcp", nil))
	require.Equal(t, http.StatusUnauthorized, missing.Code)
	challenge := missing.Header().Get("WWW-Authenticate")
	assert.Contains(t, challenge, `Bearer realm="taskflow-mcp"`)
	assert.Contains(t, challenge, `resource_metadata="https://taskflow.example.com/.well-known/oauth-protected-resource/mcp"`)
	assert.NotContains(t, challenge, "invalid_token")

	invalidRequest := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	invalidRequest.Header.Set("Authorization", "Bearer invalid")
	invalid := httptest.NewRecorder()
	handler.ServeHTTP(invalid, invalidRequest)
	assert.Equal(t, http.StatusUnauthorized, invalid.Code)
	assert.Contains(t, invalid.Header().Get("WWW-Authenticate"), `error="invalid_token"`)
}

func TestMCPPassesAuthenticatedActorInRequestContext(t *testing.T) {
	want := domain.User{ID: "user-1", Username: "alice", Email: "alice@example.com"}
	httpAuth := newTestHTTPAuth(t, func(_ context.Context, token string) (domain.User, error) {
		assert.Equal(t, "valid-token", token)
		return want, nil
	})
	handler := httpAuth.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, ok := ActorFromContext(r.Context())
		require.True(t, ok)
		assert.Equal(t, want, actor)
		w.WriteHeader(http.StatusAccepted)
	}))

	request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	request.Header.Set("Authorization", "Bearer valid-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusAccepted, recorder.Code)
}

func TestMCPRejectsEmptyActorReturnedByValidator(t *testing.T) {
	httpAuth := newTestHTTPAuth(t, func(context.Context, string) (domain.User, error) {
		return domain.User{}, nil
	})
	handler := httpAuth.Protect(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("empty actor reached MCP handler")
	}))
	request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	request.Header.Set("Authorization", "Bearer valid-looking-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestMCPOriginAllowlistAndPreflight(t *testing.T) {
	httpAuth := newTestHTTPAuth(t, func(context.Context, string) (domain.User, error) {
		return domain.User{ID: "user"}, nil
	}, "https://agent.example.com")
	handler := httpAuth.Protect(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	preflightRequest := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
	preflightRequest.Header.Set("Origin", "https://agent.example.com")
	preflight := httptest.NewRecorder()
	handler.ServeHTTP(preflight, preflightRequest)
	assert.Equal(t, http.StatusNoContent, preflight.Code)
	assert.Equal(t, "https://agent.example.com", preflight.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, preflight.Header().Get("Access-Control-Allow-Headers"), "MCP-Protocol-Version")

	forbiddenRequest := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	forbiddenRequest.Header.Set("Origin", "https://evil.example.com")
	forbiddenRequest.Header.Set("Authorization", "Bearer token")
	forbidden := httptest.NewRecorder()
	handler.ServeHTTP(forbidden, forbiddenRequest)
	assert.Equal(t, http.StatusForbidden, forbidden.Code)
	assert.Empty(t, forbidden.Header().Get("WWW-Authenticate"))

	// Native MCP clients normally omit Origin and remain supported even when
	// no browser origin has been allowlisted.
	nativeRequest := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	nativeRequest.Header.Set("Authorization", "Bearer token")
	native := httptest.NewRecorder()
	handler.ServeHTTP(native, nativeRequest)
	assert.Equal(t, http.StatusOK, native.Code)
}

func TestMCPOnlyAcceptsPost(t *testing.T) {
	httpAuth := newTestHTTPAuth(t, func(context.Context, string) (domain.User, error) {
		return domain.User{ID: "user"}, nil
	})
	recorder := httptest.NewRecorder()
	httpAuth.Protect(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("GET reached MCP handler")
	})).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/mcp", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, recorder.Code)
	assert.Equal(t, "POST, OPTIONS", recorder.Header().Get("Allow"))
}

func TestMCPConcurrentRequestsDoNotLeakIdentity(t *testing.T) {
	httpAuth := newTestHTTPAuth(t, func(_ context.Context, token string) (domain.User, error) {
		return domain.User{ID: token}, nil
	})
	handler := httpAuth.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, ok := ActorFromContext(r.Context())
		if !ok {
			http.Error(w, "actor missing", http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprint(w, actor.ID)
	}))

	const clients = 50
	var wait sync.WaitGroup
	errorsCh := make(chan error, clients)
	for index := 0; index < clients; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			token := fmt.Sprintf("user-%d", index)
			request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			request.Header.Set("Authorization", "Bearer "+token)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK || recorder.Body.String() != token {
				errorsCh <- fmt.Errorf("%s received status %d and actor %q", token, recorder.Code, recorder.Body.String())
			}
		}(index)
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		require.NoError(t, err)
	}
}

func TestNewHTTPAuthValidatesDiscoveryConfiguration(t *testing.T) {
	validator := testTokenValidator(func(context.Context, string) (domain.User, error) {
		return domain.User{}, nil
	})
	for _, test := range []struct {
		name    string
		options HTTPAuthOptions
	}{
		{name: "relative resource", options: HTTPAuthOptions{PublicURL: "/mcp", AuthorizationServer: "https://id.example.com"}},
		{name: "insecure public resource", options: HTTPAuthOptions{PublicURL: "http://api.example.com/mcp", AuthorizationServer: "https://id.example.com"}},
		{name: "wrong resource path", options: HTTPAuthOptions{PublicURL: "https://api.example.com/rpc", AuthorizationServer: "https://id.example.com"}},
		{name: "relative authorization server", options: HTTPAuthOptions{PublicURL: "https://api.example.com/mcp", AuthorizationServer: "/realms/taskflow"}},
		{name: "insecure authorization server", options: HTTPAuthOptions{PublicURL: "https://api.example.com/mcp", AuthorizationServer: "http://id.example.com"}},
		{name: "origin with path", options: HTTPAuthOptions{PublicURL: "https://api.example.com/mcp", AuthorizationServer: "https://id.example.com", AllowedOrigins: []string{"https://agent.example.com/path"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewHTTPAuth(validator, test.options)
			require.Error(t, err)
		})
	}
}
