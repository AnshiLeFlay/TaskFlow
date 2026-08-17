package mcpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/example/taskflow/backend/internal/application"
	"github.com/example/taskflow/backend/internal/domain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testActor = domain.User{ID: "11111111-1111-4111-8111-111111111111", Username: "alice"}

func TestServerRegistersAndForwardsEveryTool(t *testing.T) {
	service := &recordingService{}
	session, ctx := connectTestClient(t, service, true)

	listed, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	require.Len(t, listed.Tools, 20)

	wantReadOnly := map[string]bool{
		"get_current_user": true, "list_users": true, "list_projects": true,
		"list_boards": true, "get_board": true, "list_tasks": true,
		"list_project_members": true,
	}
	for _, tool := range listed.Tools {
		require.NotNil(t, tool.InputSchema, tool.Name)
		require.NotNil(t, tool.Annotations, tool.Name)
		assertSchemaUsesSnakeCase(t, tool.Name, tool.InputSchema)
		assert.Equal(t, wantReadOnly[tool.Name], tool.Annotations.ReadOnlyHint, tool.Name)
		if tool.Name == "delete_status" || tool.Name == "delete_workflow_rule" {
			require.NotNil(t, tool.Annotations.DestructiveHint)
			assert.True(t, *tool.Annotations.DestructiveHint, tool.Name)
		}
	}

	cases := []struct {
		name        string
		arguments   map[string]any
		serviceCall string
	}{
		{"get_current_user", map[string]any{}, ""},
		{"list_users", map[string]any{}, "ListUsers"},
		{"list_projects", map[string]any{}, "ListProjects"},
		{"list_boards", map[string]any{"project_id": "p1"}, "ListBoards"},
		{"get_board", map[string]any{"board_id": "b1"}, "GetBoard"},
		{"list_tasks", map[string]any{"board_id": "b1"}, "ListTasks"},
		{"create_project", map[string]any{"name": "Project", "description": "Description"}, "CreateProject"},
		{"list_project_members", map[string]any{"project_id": "p1"}, "ListMembers"},
		{"add_or_update_project_member", map[string]any{"project_id": "p1", "user_id": "u2", "role": "member"}, "AddMember"},
		{"create_board", map[string]any{"project_id": "p1", "name": "Board", "statuses": []map[string]any{{"name": "Todo", "position": 0}}}, "CreateBoard"},
		{"create_status", map[string]any{"board_id": "b1", "name": "Done", "position": 1}, "CreateStatus"},
		{"update_status", map[string]any{"status_id": "s1", "name": "Ready"}, "UpdateStatus"},
		{"delete_status", map[string]any{"status_id": "s1"}, "DeleteStatus"},
		{"create_workflow_rule", map[string]any{"board_id": "b1", "from_status_id": "s1", "to_status_id": "s2", "conditions": map[string]any{}}, "CreateRule"},
		{"update_workflow_rule", map[string]any{"rule_id": "r1", "to_status_id": "s3"}, "UpdateRule"},
		{"delete_workflow_rule", map[string]any{"rule_id": "r1"}, "DeleteRule"},
		{"create_task", map[string]any{"board_id": "b1", "title": "Task", "deadline": "2030-01-02T03:04:05+04:00"}, "CreateTask"},
		{"update_task", map[string]any{"task_id": "t1", "clear_assignee": true, "clear_deadline": true}, "UpdateTask"},
		{"transition_task", map[string]any{"task_id": "t1", "target_status_id": "s2"}, "TransitionTask"},
		{"add_task_comment", map[string]any{"task_id": "t1", "body": "Comment"}, "CreateComment"},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			before := service.callCount()
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: test.name, Arguments: test.arguments})
			require.NoError(t, err)
			require.False(t, result.IsError, "content: %#v", result.Content)
			require.NotNil(t, result.StructuredContent)
			if test.serviceCall == "" {
				assert.Equal(t, before, service.callCount())
			} else {
				require.Equal(t, before+1, service.callCount())
				assert.Equal(t, test.serviceCall, service.lastCall())
				assert.Equal(t, testActor.ID, service.lastActorID())
			}
		})
	}

	require.NotNil(t, service.lastCreateTask.Deadline)
	assert.Equal(t, time.Date(2030, 1, 1, 23, 4, 5, 0, time.UTC), *service.lastCreateTask.Deadline)
	assert.True(t, service.lastUpdateTask.SetAssignee)
	assert.Nil(t, service.lastUpdateTask.AssigneeID)
	assert.True(t, service.lastUpdateTask.SetDeadline)
	assert.Nil(t, service.lastUpdateTask.Deadline)
}

func TestToolBusinessErrorIsStructured(t *testing.T) {
	service := &recordingService{createProjectErr: &domain.ValidationError{Field: "name", Message: "is required"}}
	session, ctx := connectTestClient(t, service, true)

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "create_project", Arguments: map[string]any{"name": ""}})
	require.NoError(t, err)
	require.True(t, result.IsError)

	structured, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok, "%T", result.StructuredContent)
	errorValue, ok := structured["error"].(map[string]any)
	require.True(t, ok, "%#v", structured)
	assert.Equal(t, "invalid_input", errorValue["code"])
	assert.Equal(t, "name: is required", errorValue["message"])
	assert.Equal(t, map[string]any{"field": "name"}, errorValue["details"])
}

func TestToolRejectsMissingActor(t *testing.T) {
	service := &recordingService{}
	session, ctx := connectTestClient(t, service, false)

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_projects", Arguments: map[string]any{}})
	require.NoError(t, err)
	require.True(t, result.IsError)
	structured := result.StructuredContent.(map[string]any)
	errorValue := structured["error"].(map[string]any)
	assert.Equal(t, "internal_error", errorValue["code"])
	assert.Zero(t, service.callCount())
}

func assertSchemaUsesSnakeCase(t *testing.T, toolName string, schema any) {
	t.Helper()
	encoded, err := json.Marshal(schema)
	require.NoError(t, err)
	var decoded any
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	propertyName := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	var walk func(any)
	walk = func(value any) {
		switch current := value.(type) {
		case map[string]any:
			if properties, ok := current["properties"].(map[string]any); ok {
				for name := range properties {
					assert.Regexp(t, propertyName, name, "%s has a non-snake_case property", toolName)
				}
			}
			for _, child := range current {
				walk(child)
			}
		case []any:
			for _, child := range current {
				walk(child)
			}
		}
	}
	walk(decoded)
}

func TestToolErrorMapping(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		code    string
		message string
	}{
		{"forbidden", domain.ErrForbidden, "forbidden", "you do not have permission to perform this action"},
		{"not found", domain.ErrNotFound, "not_found", "resource not found"},
		{"conflict", fmt.Errorf("wrapped: %w", domain.ErrConflict), "conflict", "wrapped: conflict"},
		{"transition", errors.Join(domain.ErrTransitionNotAllowed, errors.New("comment required")), "transition_not_allowed", "transition not allowed\ncomment required"},
		{"internal", errors.New("database credentials must stay private"), "internal_error", "internal server error"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := mapToolError(test.err)
			assert.Equal(t, test.code, got.Code)
			assert.Equal(t, test.message, got.Message)
		})
	}
}

func TestUpdateTaskRejectsContradictoryClearFlags(t *testing.T) {
	service := &recordingService{}
	session, ctx := connectTestClient(t, service, true)

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "update_task", Arguments: map[string]any{
		"task_id": "t1", "assignee_id": "u2", "clear_assignee": true,
	}})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Zero(t, service.callCount())
}

func connectTestClient(t *testing.T, service Service, authenticated bool) (*mcp.ClientSession, context.Context) {
	t.Helper()
	server := NewServer(service)
	ctx := context.Background()
	if authenticated {
		ctx = WithActor(ctx, testActor)
	}
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	client := mcp.NewClient(&mcp.Implementation{Name: "taskflow-test", Version: "1.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = clientSession.Close()
		_ = serverSession.Wait()
	})
	return clientSession, ctx
}

type recordingService struct {
	mu               sync.Mutex
	calls            []string
	actorIDs         []string
	createProjectErr error
	lastCreateTask   application.CreateTaskCommand
	lastUpdateTask   application.UpdateTaskCommand
}

func (s *recordingService) record(call string, actor ...domain.User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, call)
	if len(actor) > 0 {
		s.actorIDs = append(s.actorIDs, actor[0].ID)
	} else {
		s.actorIDs = append(s.actorIDs, testActor.ID)
	}
}

func (s *recordingService) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func (s *recordingService) lastCall() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[len(s.calls)-1]
}

func (s *recordingService) lastActorID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.actorIDs[len(s.actorIDs)-1]
}

func (s *recordingService) CreateProject(_ context.Context, actor domain.User, _ application.CreateProjectCommand) (domain.Project, error) {
	s.record("CreateProject", actor)
	return domain.Project{ID: "p1"}, s.createProjectErr
}
func (s *recordingService) ListProjects(_ context.Context, actor domain.User) ([]domain.Project, error) {
	s.record("ListProjects", actor)
	return []domain.Project{}, nil
}
func (s *recordingService) ListUsers(context.Context) ([]domain.User, error) {
	s.record("ListUsers")
	return []domain.User{}, nil
}
func (s *recordingService) ListMembers(_ context.Context, actor domain.User, _ string) ([]domain.Member, error) {
	s.record("ListMembers", actor)
	return []domain.Member{}, nil
}
func (s *recordingService) AddMember(_ context.Context, actor domain.User, _ string, _ application.AddMemberCommand) (domain.Member, error) {
	s.record("AddMember", actor)
	return domain.Member{ProjectID: "p1", UserID: "u2"}, nil
}
func (s *recordingService) CreateBoard(_ context.Context, actor domain.User, _ string, _ application.CreateBoardCommand) (domain.BoardAggregate, error) {
	s.record("CreateBoard", actor)
	return domain.BoardAggregate{Board: domain.Board{ID: "b1"}}, nil
}
func (s *recordingService) ListBoards(_ context.Context, actor domain.User, _ string) ([]domain.Board, error) {
	s.record("ListBoards", actor)
	return []domain.Board{}, nil
}
func (s *recordingService) GetBoard(_ context.Context, actor domain.User, _ string) (domain.BoardAggregate, error) {
	s.record("GetBoard", actor)
	return domain.BoardAggregate{Board: domain.Board{ID: "b1"}}, nil
}
func (s *recordingService) CreateStatus(_ context.Context, actor domain.User, _ string, _ application.CreateStatusCommand) (domain.Status, error) {
	s.record("CreateStatus", actor)
	return domain.Status{ID: "s1"}, nil
}
func (s *recordingService) UpdateStatus(_ context.Context, actor domain.User, _ string, _ application.UpdateStatusCommand) (domain.Status, error) {
	s.record("UpdateStatus", actor)
	return domain.Status{ID: "s1"}, nil
}
func (s *recordingService) DeleteStatus(_ context.Context, actor domain.User, _ string) error {
	s.record("DeleteStatus", actor)
	return nil
}
func (s *recordingService) CreateRule(_ context.Context, actor domain.User, _ string, _ application.CreateRuleCommand) (domain.TransitionRule, error) {
	s.record("CreateRule", actor)
	return domain.TransitionRule{ID: "r1"}, nil
}
func (s *recordingService) UpdateRule(_ context.Context, actor domain.User, _ string, _ application.UpdateRuleCommand) (domain.TransitionRule, error) {
	s.record("UpdateRule", actor)
	return domain.TransitionRule{ID: "r1"}, nil
}
func (s *recordingService) DeleteRule(_ context.Context, actor domain.User, _ string) error {
	s.record("DeleteRule", actor)
	return nil
}
func (s *recordingService) ListTasks(_ context.Context, actor domain.User, _ string) ([]domain.Task, error) {
	s.record("ListTasks", actor)
	return []domain.Task{}, nil
}
func (s *recordingService) CreateTask(_ context.Context, actor domain.User, _ string, command application.CreateTaskCommand) (domain.Task, error) {
	s.record("CreateTask", actor)
	s.lastCreateTask = command
	return domain.Task{ID: "t1"}, nil
}
func (s *recordingService) UpdateTask(_ context.Context, actor domain.User, _ string, command application.UpdateTaskCommand) (domain.Task, error) {
	s.record("UpdateTask", actor)
	s.lastUpdateTask = command
	return domain.Task{ID: "t1"}, nil
}
func (s *recordingService) TransitionTask(_ context.Context, actor domain.User, _, _ string) (domain.Task, error) {
	s.record("TransitionTask", actor)
	return domain.Task{ID: "t1"}, nil
}
func (s *recordingService) CreateComment(_ context.Context, actor domain.User, _, _ string) (domain.Comment, error) {
	s.record("CreateComment", actor)
	return domain.Comment{ID: "c1"}, nil
}
