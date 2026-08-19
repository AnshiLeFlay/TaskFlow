package mcpapi

import (
	"context"
	"errors"
	"time"

	"github.com/example/taskflow/backend/internal/application"
	"github.com/example/taskflow/backend/internal/domain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	serverName    = "taskflow"
	serverVersion = "1.0.0"
)

// Service is the application-layer surface exposed through MCP. The concrete
// *application.Service satisfies this interface; keeping the transport behind
// an interface also makes tool wiring independently testable.
type Service interface {
	CreateProject(context.Context, domain.User, application.CreateProjectCommand) (domain.Project, error)
	ListProjects(context.Context, domain.User) ([]domain.Project, error)
	ListUsers(context.Context) ([]domain.User, error)
	ListMembers(context.Context, domain.User, string) ([]domain.Member, error)
	AddMember(context.Context, domain.User, string, application.AddMemberCommand) (domain.Member, error)
	CreateBoard(context.Context, domain.User, string, application.CreateBoardCommand) (domain.BoardAggregate, error)
	ListBoards(context.Context, domain.User, string) ([]domain.Board, error)
	GetBoard(context.Context, domain.User, string) (domain.BoardAggregate, error)
	CreateStatus(context.Context, domain.User, string, application.CreateStatusCommand) (domain.Status, error)
	UpdateStatus(context.Context, domain.User, string, application.UpdateStatusCommand) (domain.Status, error)
	DeleteStatus(context.Context, domain.User, string) error
	CreateRule(context.Context, domain.User, string, application.CreateRuleCommand) (domain.TransitionRule, error)
	UpdateRule(context.Context, domain.User, string, application.UpdateRuleCommand) (domain.TransitionRule, error)
	DeleteRule(context.Context, domain.User, string) error
	ListTasks(context.Context, domain.User, string) ([]domain.Task, error)
	CreateTask(context.Context, domain.User, string, application.CreateTaskCommand) (domain.Task, error)
	UpdateTask(context.Context, domain.User, string, application.UpdateTaskCommand) (domain.Task, error)
	TransitionTask(context.Context, domain.User, string, string) (domain.Task, error)
	CreateComment(context.Context, domain.User, string, string) (domain.Comment, error)
}

var _ Service = (*application.Service)(nil)

// serverInstructions is returned to clients in the initialize response. Tool
// and field descriptions can say what one call does; they cannot convey how
// the pieces relate, which is where clients otherwise fail: they invent status
// names, skip the board layer, or assume a task can be dragged anywhere.
const serverInstructions = `TaskFlow manages projects, Kanban boards and tasks.

Structure: a project has members and one or more boards. A board owns ordered
statuses (columns) and transition rules. A task lives on one board and sits in
one status.

Start with list_projects, then get_board for the board you intend to work on.
get_board returns statuses, rules, tasks and comments in one call, and its IDs
are what every other tool expects. Never guess an ID or a status name.

Moving a task is the one operation with a rule attached. transition_task
succeeds only when a transition rule exists for that exact ordered status pair
and every condition on it passes; conditions can require the task's author,
its assignee, the project owner, at least one comment, or a given project role.
A missing rule is a configuration gap, not a permission problem: create it with
create_rule. Editing a task's other fields uses update_task and is unrelated.

Creating a project: create_project gives you an empty project, and a project
without a board holds nothing. Call create_board next. Omit its statuses to get
the standard preset - To Do, In Progress, Done, with rules linking neighbours
in both directions - which is a board that works immediately. Supply statuses
only when the workflow is genuinely different, and then add every rule with
create_rule yourself, because a board with statuses and no rules cannot move
any task.

Permissions come from project membership, not from the account itself: admin
manages members, statuses and rules, member works with tasks, viewer only
reads. Adding a user to a project is add_or_update_project_member. There are no
delete operations for tasks, projects, boards or members - the TaskFlow domain
has none.`

// NewServer constructs the MCP feature server. HTTP transport and OAuth are
// deliberately composed outside this function; every handler reads the actor
// installed in the individual request context by HTTPAuth.Protect.
func NewServer(service Service) *mcp.Server {
	if service == nil {
		panic("mcpapi: nil application service")
	}
	server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: serverVersion}, &mcp.ServerOptions{Instructions: serverInstructions})
	registerNavigationTools(server, service)
	registerProjectTools(server, service)
	registerWorkflowTools(server, service)
	registerTaskTools(server, service)
	return server
}

type toolHandler[In any] func(context.Context, domain.User, In) (any, error)

func addTool[In any](server *mcp.Server, tool *mcp.Tool, handler toolHandler[In]) {
	mcp.AddTool(server, tool, func(ctx context.Context, _ *mcp.CallToolRequest, input In) (*mcp.CallToolResult, any, error) {
		actor, ok := ActorFromContext(ctx)
		if !ok {
			return toolResponse(nil, errMissingActor)
		}
		value, err := handler(ctx, actor, input)
		return toolResponse(value, err)
	})
}

func registerNavigationTools(server *mcp.Server, service Service) {
	addTool(server, readTool("get_current_user", "Return the authenticated TaskFlow user."), func(_ context.Context, actor domain.User, _ EmptyInput) (any, error) {
		return actor, nil
	})
	addTool(server, readTool("list_users", "List enabled users available for project membership."), func(ctx context.Context, _ domain.User, _ EmptyInput) (any, error) {
		users, err := service.ListUsers(ctx)
		if users == nil {
			users = []domain.User{}
		}
		return map[string]any{"users": users}, err
	})
	addTool(server, readTool("list_projects", "List projects visible to the authenticated user."), func(ctx context.Context, actor domain.User, _ EmptyInput) (any, error) {
		projects, err := service.ListProjects(ctx, actor)
		if projects == nil {
			projects = []domain.Project{}
		}
		return map[string]any{"projects": projects}, err
	})
	addTool(server, readTool("list_boards", "List boards in a project visible to the authenticated user."), func(ctx context.Context, actor domain.User, input ProjectIDInput) (any, error) {
		boards, err := service.ListBoards(ctx, actor, input.ProjectID)
		if boards == nil {
			boards = []domain.Board{}
		}
		return map[string]any{"boards": boards}, err
	})
	addTool(server, readTool("get_board", "Return a board with its statuses, workflow rules, tasks, and comments."), func(ctx context.Context, actor domain.User, input BoardIDInput) (any, error) {
		return service.GetBoard(ctx, actor, input.BoardID)
	})
	addTool(server, readTool("list_tasks", "List tasks on a board."), func(ctx context.Context, actor domain.User, input BoardIDInput) (any, error) {
		tasks, err := service.ListTasks(ctx, actor, input.BoardID)
		if tasks == nil {
			tasks = []domain.Task{}
		}
		return map[string]any{"tasks": tasks}, err
	})
}

func registerProjectTools(server *mcp.Server, service Service) {
	addTool(server, writeTool("create_project", "Create a project owned by the authenticated user.", false, false), func(ctx context.Context, actor domain.User, input CreateProjectInput) (any, error) {
		return service.CreateProject(ctx, actor, application.CreateProjectCommand{Name: input.Name, Description: input.Description})
	})
	addTool(server, readTool("list_project_members", "List the members and roles of a project."), func(ctx context.Context, actor domain.User, input ProjectIDInput) (any, error) {
		members, err := service.ListMembers(ctx, actor, input.ProjectID)
		if members == nil {
			members = []domain.Member{}
		}
		return map[string]any{"members": members}, err
	})
	addTool(server, writeTool("add_or_update_project_member", "Add a user to a project or update the user's project role.", false, true), func(ctx context.Context, actor domain.User, input AddOrUpdateProjectMemberInput) (any, error) {
		return service.AddMember(ctx, actor, input.ProjectID, application.AddMemberCommand{UserID: input.UserID, Role: input.Role})
	})
	addTool(server, writeTool("create_board", "Create a board in a project. Omit statuses to get the standard preset (To Do, In Progress, Done) with transition rules already linking neighbouring columns both ways. Supplying statuses creates them without any rules, so add each rule with create_rule.", false, false), func(ctx context.Context, actor domain.User, input CreateBoardInput) (any, error) {
		statuses := make([]application.StatusInput, 0, len(input.Statuses))
		for _, status := range input.Statuses {
			statuses = append(statuses, application.StatusInput{Name: status.Name, Position: status.Position})
		}
		return service.CreateBoard(ctx, actor, input.ProjectID, application.CreateBoardCommand{Name: input.Name, Description: input.Description, Statuses: statuses})
	})
}

func registerWorkflowTools(server *mcp.Server, service Service) {
	addTool(server, writeTool("create_status", "Create a status on a board.", false, false), func(ctx context.Context, actor domain.User, input CreateStatusInput) (any, error) {
		return service.CreateStatus(ctx, actor, input.BoardID, application.CreateStatusCommand{Name: input.Name, Position: input.Position})
	})
	addTool(server, writeTool("update_status", "Update a workflow status name or position.", false, false), func(ctx context.Context, actor domain.User, input UpdateStatusInput) (any, error) {
		return service.UpdateStatus(ctx, actor, input.StatusID, application.UpdateStatusCommand{Name: input.Name, Position: input.Position})
	})
	addTool(server, writeTool("delete_status", "Delete an unused workflow status.", true, true), func(ctx context.Context, actor domain.User, input StatusIDInput) (any, error) {
		if err := service.DeleteStatus(ctx, actor, input.StatusID); err != nil {
			return nil, err
		}
		return map[string]any{"deleted": true, "status_id": input.StatusID}, nil
	})
	addTool(server, writeTool("create_workflow_rule", "Create a directed workflow transition rule.", false, false), func(ctx context.Context, actor domain.User, input CreateWorkflowRuleInput) (any, error) {
		return service.CreateRule(ctx, actor, input.BoardID, application.CreateRuleCommand{FromStatusID: input.FromStatusID, ToStatusID: input.ToStatusID, Conditions: input.Conditions.domainConditions()})
	})
	addTool(server, writeTool("update_workflow_rule", "Update a workflow transition rule.", false, false), func(ctx context.Context, actor domain.User, input UpdateWorkflowRuleInput) (any, error) {
		var conditions *domain.RuleConditions
		if input.Conditions != nil {
			converted := input.Conditions.domainConditions()
			conditions = &converted
		}
		return service.UpdateRule(ctx, actor, input.RuleID, application.UpdateRuleCommand{FromStatusID: input.FromStatusID, ToStatusID: input.ToStatusID, Conditions: conditions})
	})
	addTool(server, writeTool("delete_workflow_rule", "Delete a workflow transition rule.", true, true), func(ctx context.Context, actor domain.User, input RuleIDInput) (any, error) {
		if err := service.DeleteRule(ctx, actor, input.RuleID); err != nil {
			return nil, err
		}
		return map[string]any{"deleted": true, "rule_id": input.RuleID}, nil
	})
}

func registerTaskTools(server *mcp.Server, service Service) {
	addTool(server, writeTool("create_task", "Create a task on a board.", false, false), func(ctx context.Context, actor domain.User, input CreateTaskInput) (any, error) {
		deadline, err := parseDeadline(input.Deadline)
		if err != nil {
			return nil, err
		}
		return service.CreateTask(ctx, actor, input.BoardID, application.CreateTaskCommand{
			Title: input.Title, Description: input.Description, StatusID: input.StatusID,
			AssigneeID: input.AssigneeID, Deadline: deadline,
		})
	})
	addTool(server, writeTool("update_task", "Update task fields without changing workflow status.", false, false), func(ctx context.Context, actor domain.User, input UpdateTaskInput) (any, error) {
		if input.ClearAssignee && input.AssigneeID != nil {
			return nil, &domain.ValidationError{Field: "assignee_id", Message: "cannot be combined with clear_assignee"}
		}
		if input.ClearDeadline && input.Deadline != nil {
			return nil, &domain.ValidationError{Field: "deadline", Message: "cannot be combined with clear_deadline"}
		}
		deadline, err := parseDeadline(input.Deadline)
		if err != nil {
			return nil, err
		}
		return service.UpdateTask(ctx, actor, input.TaskID, application.UpdateTaskCommand{
			Title: input.Title, Description: input.Description,
			AssigneeID: input.AssigneeID, SetAssignee: input.AssigneeID != nil || input.ClearAssignee,
			Deadline: deadline, SetDeadline: input.Deadline != nil || input.ClearDeadline,
		})
	})
	addTool(server, writeTool("transition_task", "Move a task to another status after evaluating its workflow rule.", false, false), func(ctx context.Context, actor domain.User, input TransitionTaskInput) (any, error) {
		return service.TransitionTask(ctx, actor, input.TaskID, input.TargetStatusID)
	})
	addTool(server, writeTool("add_task_comment", "Add a comment to a task.", false, false), func(ctx context.Context, actor domain.User, input AddTaskCommentInput) (any, error) {
		return service.CreateComment(ctx, actor, input.TaskID, input.Body)
	})
}

func parseDeadline(raw *string) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, *raw)
	if err != nil {
		return nil, &domain.ValidationError{Field: "deadline", Message: "must be an RFC 3339 timestamp"}
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func readTool(name, description string) *mcp.Tool {
	return &mcp.Tool{
		Name: name, Description: description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, DestructiveHint: boolPointer(false), OpenWorldHint: boolPointer(false)},
	}
}

func writeTool(name, description string, destructive, idempotent bool) *mcp.Tool {
	return &mcp.Tool{
		Name: name, Description: description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, IdempotentHint: idempotent, DestructiveHint: boolPointer(destructive), OpenWorldHint: boolPointer(false)},
	}
}

func boolPointer(value bool) *bool { return &value }

var errMissingActor = errors.New("authenticated user is missing from MCP request context")

type ToolError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

type ToolErrorEnvelope struct {
	Error ToolError `json:"error"`
}

func toolResponse(value any, err error) (*mcp.CallToolResult, any, error) {
	if err == nil {
		return nil, value, nil
	}
	return &mcp.CallToolResult{IsError: true}, ToolErrorEnvelope{Error: mapToolError(err)}, nil
}

func mapToolError(err error) ToolError {
	result := ToolError{Code: "internal_error", Message: "internal server error", Details: map[string]any{}}
	switch {
	case errors.Is(err, errMissingActor):
		// Authentication failures are rejected by HTTPAuth before an MCP call is
		// dispatched. A missing actor here therefore means server misconfiguration,
		// not a tool-level OAuth error.
		result.Code, result.Message = "internal_error", "internal server error"
	case errors.Is(err, domain.ErrInvalid):
		result.Code, result.Message = "invalid_input", err.Error()
		var validation *domain.ValidationError
		if errors.As(err, &validation) && validation.Field != "" {
			result.Details = map[string]any{"field": validation.Field}
		}
	case errors.Is(err, domain.ErrForbidden):
		result.Code, result.Message = "forbidden", "you do not have permission to perform this action"
	case errors.Is(err, domain.ErrNotFound):
		result.Code, result.Message = "not_found", "resource not found"
	case errors.Is(err, domain.ErrConflict):
		result.Code, result.Message = "conflict", err.Error()
	case errors.Is(err, domain.ErrTransitionNotAllowed):
		result.Code, result.Message = "transition_not_allowed", err.Error()
	}
	return result
}
