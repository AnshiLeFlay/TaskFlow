//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/example/taskflow/backend/internal/application"
	"github.com/example/taskflow/backend/internal/domain"
	"github.com/example/taskflow/backend/internal/infrastructure/migrations"
	postgresrepo "github.com/example/taskflow/backend/internal/infrastructure/postgres"
	"github.com/example/taskflow/backend/internal/interfaces/mcpapi"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMCPStatelessHTTPTaskCycle exercises the actual MCP wire transport and
// application service against PostgreSQL. Keycloak is intentionally replaced
// by a deterministic TokenValidator here; Keycloak/OAuth discovery and PKCE
// have their own boundary tests.
func TestMCPStatelessHTTPTaskCycle(t *testing.T) {
	databaseURL := requireTestDatabaseURL(t)
	require.NoError(t, migrations.Up(databaseURL, testMigrationsURL()))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	repository, err := postgresrepo.New(ctx, databaseURL)
	require.NoError(t, err)
	t.Cleanup(repository.Close)
	cleanupPool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err)
	t.Cleanup(cleanupPool.Close)

	owner := domain.User{ID: "mcp-owner-" + uuid.NewString(), Username: "mcp-owner"}
	member := domain.User{ID: "mcp-member-" + uuid.NewString(), Username: "mcp-member"}
	viewer := domain.User{ID: "mcp-viewer-" + uuid.NewString(), Username: "mcp-viewer"}
	const (
		ownerToken  = "mcp-integration-owner-token"
		memberToken = "mcp-integration-member-token"
		viewerToken = "mcp-integration-viewer-token"
	)
	validator := staticTokenValidator{users: map[string]domain.User{
		ownerToken: owner, memberToken: member, viewerToken: viewer,
	}}
	events := &recordingEventPublisher{}
	service := application.NewService(repository, events)
	featureServer := mcpapi.NewServer(service)

	httpServer := httptest.NewUnstartedServer(nil)
	publicURL := "http://" + httpServer.Listener.Addr().String() + "/mcp"
	httpAuth, err := mcpapi.NewHTTPAuth(validator, mcpapi.HTTPAuthOptions{
		PublicURL:           publicURL,
		AuthorizationServer: "https://identity.example.test/realms/taskflow",
	})
	require.NoError(t, err)
	streamable := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return featureServer },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, PropagateRequestCancellation: true},
	)
	mux := http.NewServeMux()
	mux.Handle("/mcp", httpAuth.Protect(streamable))
	mux.Handle("/.well-known/oauth-protected-resource/mcp", httpAuth.MetadataHandler())
	httpServer.Config.Handler = mux
	httpServer.Start()
	t.Cleanup(httpServer.Close)

	ownerSession := connectMCPHTTPClient(t, ctx, httpServer.Client(), publicURL, ownerToken)
	memberSession := connectMCPHTTPClient(t, ctx, httpServer.Client(), publicURL, memberToken)
	viewerSession := connectMCPHTTPClient(t, ctx, httpServer.Client(), publicURL, viewerToken)

	tools, err := ownerSession.ListTools(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, tools.Tools, 20)

	var project domain.Project
	callMCPOK(t, ctx, ownerSession, "create_project", map[string]any{
		"name": "MCP integration " + uuid.NewString(), "description": "Stateless HTTP task cycle",
	}, &project)
	require.NotEmpty(t, project.ID)
	t.Cleanup(func() {
		_, cleanupErr := cleanupPool.Exec(context.Background(), `DELETE FROM projects WHERE id=$1`, project.ID)
		assert.NoError(t, cleanupErr)
	})
	assert.Equal(t, owner.ID, project.OwnerID)

	var addedMember domain.Member
	callMCPOK(t, ctx, ownerSession, "add_or_update_project_member", map[string]any{
		"project_id": project.ID, "user_id": member.ID, "role": "member",
	}, &addedMember)
	assert.Equal(t, domain.RoleMember, addedMember.Role)
	var addedViewer domain.Member
	callMCPOK(t, ctx, ownerSession, "add_or_update_project_member", map[string]any{
		"project_id": project.ID, "user_id": viewer.ID, "role": "viewer",
	}, &addedViewer)
	assert.Equal(t, domain.RoleViewer, addedViewer.Role)

	var board domain.BoardAggregate
	callMCPOK(t, ctx, ownerSession, "create_board", map[string]any{
		"project_id": project.ID,
		"name":       "Delivery",
		"statuses": []map[string]any{
			{"name": "To Do", "position": 0},
			{"name": "Done", "position": 1},
		},
	}, &board)
	require.Len(t, board.Statuses, 2)
	fromStatusID, targetStatusID := board.Statuses[0].ID, board.Statuses[1].ID

	viewerError := callMCPError(t, ctx, viewerSession, "create_task", map[string]any{
		"board_id": board.ID, "title": "Viewer must not create this", "status_id": fromStatusID,
	})
	assert.Equal(t, "forbidden", viewerError.Code)

	deadline := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	var task domain.Task
	callMCPOK(t, ctx, memberSession, "create_task", map[string]any{
		"board_id": board.ID, "title": "Implement MCP integration", "description": "Initial",
		"status_id": fromStatusID, "assignee_id": member.ID, "deadline": deadline.Format(time.RFC3339),
	}, &task)
	require.NotEmpty(t, task.ID)
	assert.Equal(t, member.ID, task.AuthorID)
	require.NotNil(t, task.AssigneeID)
	assert.Equal(t, member.ID, *task.AssigneeID)

	var updated domain.Task
	callMCPOK(t, ctx, memberSession, "update_task", map[string]any{
		"task_id": task.ID, "title": "Implement and verify MCP integration", "description": "Updated by member",
	}, &updated)
	assert.Equal(t, "Implement and verify MCP integration", updated.Title)
	assert.Equal(t, "Updated by member", updated.Description)

	var comment domain.Comment
	callMCPOK(t, ctx, memberSession, "add_task_comment", map[string]any{
		"task_id": task.ID, "body": "Implementation is ready for transition",
	}, &comment)
	assert.Equal(t, member.ID, comment.AuthorID)

	transitionError := callMCPError(t, ctx, memberSession, "transition_task", map[string]any{
		"task_id": task.ID, "target_status_id": targetStatusID,
	})
	assert.Equal(t, "transition_not_allowed", transitionError.Code)
	assert.Contains(t, transitionError.Message, "no workflow rule")

	var rule domain.TransitionRule
	callMCPOK(t, ctx, ownerSession, "create_workflow_rule", map[string]any{
		"board_id": board.ID, "from_status_id": fromStatusID, "to_status_id": targetStatusID,
		"conditions": map[string]any{"requires_comment": true, "allowed_roles": []string{"member"}},
	}, &rule)
	assert.True(t, rule.Conditions.RequiresComment)
	assert.Equal(t, []domain.ProjectRole{domain.RoleMember}, rule.Conditions.AllowedRoles)

	var transitioned domain.Task
	callMCPOK(t, ctx, memberSession, "transition_task", map[string]any{
		"task_id": task.ID, "target_status_id": targetStatusID,
	}, &transitioned)
	assert.Equal(t, targetStatusID, transitioned.StatusID)

	var persisted struct {
		Tasks []domain.Task `json:"tasks"`
	}
	callMCPOK(t, ctx, memberSession, "list_tasks", map[string]any{"board_id": board.ID}, &persisted)
	require.Len(t, persisted.Tasks, 1)
	assert.Equal(t, targetStatusID, persisted.Tasks[0].StatusID)
	assert.Equal(t, "Implement and verify MCP integration", persisted.Tasks[0].Title)
	require.Len(t, persisted.Tasks[0].Comments, 1)
	assert.Equal(t, comment.ID, persisted.Tasks[0].Comments[0].ID)

	transitionEvent, ok := events.find(domain.EventTaskTransitioned, task.ID)
	require.True(t, ok, "task.transitioned event was not published")
	assert.Equal(t, project.ID, transitionEvent.ProjectID)
	assert.Equal(t, board.ID, transitionEvent.BoardID)
	assert.Equal(t, member.ID, transitionEvent.ActorID)
	assert.Equal(t, fromStatusID, transitionEvent.FromStatusID)
	assert.Equal(t, targetStatusID, transitionEvent.ToStatusID)
}

type staticTokenValidator struct {
	users map[string]domain.User
}

func (validator staticTokenValidator) Verify(_ context.Context, token string) (domain.User, error) {
	user, ok := validator.users[token]
	if !ok {
		return domain.User{}, errors.New("invalid test token")
	}
	return user, nil
}

type bearerRoundTripper struct {
	base  http.RoundTripper
	token string
}

func (transport bearerRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+transport.token)
	return transport.base.RoundTrip(clone)
}

func connectMCPHTTPClient(t *testing.T, ctx context.Context, baseClient *http.Client, endpoint, token string) *mcp.ClientSession {
	t.Helper()
	clientTransport := baseClient.Transport
	if clientTransport == nil {
		clientTransport = http.DefaultTransport
	}
	httpClient := &http.Client{
		Transport: bearerRoundTripper{base: clientTransport, token: token},
		Timeout:   10 * time.Second,
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "taskflow-integration-test", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: endpoint, HTTPClient: httpClient, MaxRetries: -1, DisableStandaloneSSE: true,
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, session.Close()) })
	return session
}

func callMCPOK(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, arguments map[string]any, output any) {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	require.NoError(t, err, name)
	require.False(t, result.IsError, "%s returned tool error: %#v", name, result.Content)
	require.NotNil(t, result.StructuredContent, name)
	if output == nil {
		return
	}
	encoded, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(encoded, output), "%s structured output: %s", name, encoded)
}

type mcpToolError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func callMCPError(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, arguments map[string]any) mcpToolError {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	require.NoError(t, err, name)
	require.True(t, result.IsError, "%s unexpectedly succeeded", name)
	var envelope struct {
		Error mcpToolError `json:"error"`
	}
	encoded, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(encoded, &envelope), "%s structured error: %s", name, encoded)
	require.NotEmpty(t, envelope.Error.Code, fmt.Sprintf("%s structured error: %s", name, encoded))
	return envelope.Error
}

type recordingEventPublisher struct {
	mu     sync.Mutex
	events []domain.TaskEvent
}

func (publisher *recordingEventPublisher) Publish(event domain.TaskEvent) {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	publisher.events = append(publisher.events, event)
}

func (publisher *recordingEventPublisher) find(eventType domain.EventType, taskID string) (domain.TaskEvent, bool) {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	for _, event := range publisher.events {
		if event.Type == eventType && event.TaskID == taskID {
			return event, true
		}
	}
	return domain.TaskEvent{}, false
}
