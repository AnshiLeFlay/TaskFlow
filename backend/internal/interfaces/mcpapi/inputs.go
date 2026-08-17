package mcpapi

import "github.com/example/taskflow/backend/internal/domain"

// EmptyInput is used by tools that do not accept arguments. A named object
// type makes the SDK generate an object JSON schema instead of an untyped one.
type EmptyInput struct{}

type ProjectIDInput struct {
	ProjectID string `json:"project_id" jsonschema:"ID of the TaskFlow project"`
}

type BoardIDInput struct {
	BoardID string `json:"board_id" jsonschema:"ID of the TaskFlow board"`
}

type StatusIDInput struct {
	StatusID string `json:"status_id" jsonschema:"ID of the workflow status"`
}

type RuleIDInput struct {
	RuleID string `json:"rule_id" jsonschema:"ID of the workflow rule"`
}

type CreateProjectInput struct {
	Name        string `json:"name" jsonschema:"Project name"`
	Description string `json:"description,omitempty" jsonschema:"Optional project description"`
}

type AddOrUpdateProjectMemberInput struct {
	ProjectID string             `json:"project_id" jsonschema:"ID of the project whose membership is changed"`
	UserID    string             `json:"user_id" jsonschema:"Keycloak user ID to add or update"`
	Role      domain.ProjectRole `json:"role" jsonschema:"Project role: admin, member, or viewer"`
}

type BoardStatusInput struct {
	Name     string `json:"name" jsonschema:"Status name"`
	Position int    `json:"position" jsonschema:"Zero-based unique status position"`
}

type CreateBoardInput struct {
	ProjectID   string             `json:"project_id" jsonschema:"ID of the project that owns the board"`
	Name        string             `json:"name" jsonschema:"Board name"`
	Description string             `json:"description,omitempty" jsonschema:"Optional board description"`
	Statuses    []BoardStatusInput `json:"statuses,omitempty" jsonschema:"Optional initial statuses; defaults are created when omitted"`
}

type CreateStatusInput struct {
	BoardID  string `json:"board_id" jsonschema:"ID of the board that owns the status"`
	Name     string `json:"name" jsonschema:"Status name"`
	Position int    `json:"position" jsonschema:"Zero-based unique status position"`
}

type UpdateStatusInput struct {
	StatusID string  `json:"status_id" jsonschema:"ID of the status to update"`
	Name     *string `json:"name,omitempty" jsonschema:"New status name"`
	Position *int    `json:"position,omitempty" jsonschema:"New zero-based unique status position"`
}

type CreateWorkflowRuleInput struct {
	BoardID      string              `json:"board_id" jsonschema:"ID of the board that owns the rule"`
	FromStatusID string              `json:"from_status_id" jsonschema:"Source status ID"`
	ToStatusID   string              `json:"to_status_id" jsonschema:"Target status ID"`
	Conditions   RuleConditionsInput `json:"conditions,omitempty" jsonschema:"Conditions that must hold for the transition"`
}

type UpdateWorkflowRuleInput struct {
	RuleID       string               `json:"rule_id" jsonschema:"ID of the workflow rule to update"`
	FromStatusID *string              `json:"from_status_id,omitempty" jsonschema:"New source status ID"`
	ToStatusID   *string              `json:"to_status_id,omitempty" jsonschema:"New target status ID"`
	Conditions   *RuleConditionsInput `json:"conditions,omitempty" jsonschema:"New transition conditions"`
}

// RuleConditionsInput keeps boolean conditions optional in the generated JSON
// schema. Omitted flags naturally retain their false zero value.
type RuleConditionsInput struct {
	AuthorOnly       bool                 `json:"author_only,omitempty" jsonschema:"Only the task author may transition"`
	AssigneeOnly     bool                 `json:"assignee_only,omitempty" jsonschema:"Only the current assignee may transition"`
	RequiresComment  bool                 `json:"requires_comment,omitempty" jsonschema:"At least one task comment is required"`
	ProjectOwnerOnly bool                 `json:"project_owner_only,omitempty" jsonschema:"Only the project owner may transition"`
	AllowedRoles     []domain.ProjectRole `json:"allowed_roles,omitempty" jsonschema:"Optional project roles allowed to transition"`
}

func (input RuleConditionsInput) domainConditions() domain.RuleConditions {
	return domain.RuleConditions{
		AuthorOnly: input.AuthorOnly, AssigneeOnly: input.AssigneeOnly,
		RequiresComment: input.RequiresComment, ProjectOwnerOnly: input.ProjectOwnerOnly,
		AllowedRoles: input.AllowedRoles,
	}
}

type CreateTaskInput struct {
	BoardID     string  `json:"board_id" jsonschema:"ID of the board that owns the task"`
	Title       string  `json:"title" jsonschema:"Task title"`
	Description string  `json:"description,omitempty" jsonschema:"Optional task description"`
	StatusID    string  `json:"status_id,omitempty" jsonschema:"Initial status ID; the first board status is used when omitted"`
	AssigneeID  *string `json:"assignee_id,omitempty" jsonschema:"Optional ID of an admin or member in this project"`
	Deadline    *string `json:"deadline,omitempty" jsonschema:"Optional RFC 3339 deadline"`
}

type UpdateTaskInput struct {
	TaskID        string  `json:"task_id" jsonschema:"ID of the task to update"`
	Title         *string `json:"title,omitempty" jsonschema:"New task title"`
	Description   *string `json:"description,omitempty" jsonschema:"New task description"`
	AssigneeID    *string `json:"assignee_id,omitempty" jsonschema:"New assignee ID"`
	ClearAssignee bool    `json:"clear_assignee,omitempty" jsonschema:"Set true to make the task unassigned"`
	Deadline      *string `json:"deadline,omitempty" jsonschema:"New RFC 3339 deadline"`
	ClearDeadline bool    `json:"clear_deadline,omitempty" jsonschema:"Set true to remove the deadline"`
}

type TransitionTaskInput struct {
	TaskID         string `json:"task_id" jsonschema:"ID of the task to transition"`
	TargetStatusID string `json:"target_status_id" jsonschema:"ID of the target workflow status"`
}

type AddTaskCommentInput struct {
	TaskID string `json:"task_id" jsonschema:"ID of the task to comment on"`
	Body   string `json:"body" jsonschema:"Comment text"`
}
