package mcpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/example/taskflow/backend/internal/domain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamableHTTPConnectsAndCallsToolWithOAuthActor(t *testing.T) {
	service := &recordingService{}
	server := NewServer(service)
	transport := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, PropagateRequestCancellation: true},
	)
	httpAuth := newTestHTTPAuth(t, func(_ context.Context, token string) (domain.User, error) {
		if token != "valid-user-token" {
			return domain.User{}, assert.AnError
		}
		return testActor, nil
	})
	httpServer := httptest.NewServer(httpAuth.Protect(transport))
	defer httpServer.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "taskflow-http-test", Version: "1.0.0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: httpServer.URL,
		HTTPClient: &http.Client{Transport: bearerTransport{
			base:  http.DefaultTransport,
			token: "valid-user-token",
		}},
	}, nil)
	require.NoError(t, err)
	defer session.Close()

	tools, err := session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	assert.Len(t, tools.Tools, 20)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_projects", Arguments: map[string]any{}})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Equal(t, "ListProjects", service.lastCall())
	assert.Equal(t, testActor.ID, service.lastActorID())
}

type bearerTransport struct {
	base  http.RoundTripper
	token string
}

func (t bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(clone)
}
