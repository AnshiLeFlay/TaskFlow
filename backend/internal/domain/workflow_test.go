package domain

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTransitionRejectsMissingRule(t *testing.T) {
	task := Task{StatusID: "todo", AuthorID: "author"}
	err := ValidateTransition(task, nil, "done", TransitionContext{ActorID: "author", ProjectRole: RoleMember})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTransitionNotAllowed)
}

func TestValidateTransitionRejectsEmptyTargetStatusID(t *testing.T) {
	task := Task{StatusID: "todo", AuthorID: "author"}
	err := ValidateTransition(task, nil, "", TransitionContext{ActorID: "author", ProjectRole: RoleMember})
	require.Error(t, err)
	var validation *ValidationError
	require.ErrorAs(t, err, &validation)
	assert.Equal(t, "target_status_id", validation.Field)
}

func TestValidateTransitionRejectsTargetSameAsCurrentStatus(t *testing.T) {
	task := Task{StatusID: "todo", AuthorID: "author"}
	err := ValidateTransition(task, nil, "todo", TransitionContext{ActorID: "author", ProjectRole: RoleMember})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTransitionNotAllowed)
	assert.Contains(t, err.Error(), "already in target status")
}

func TestValidateTransitionChecksEveryConfiguredCondition(t *testing.T) {
	assignee := "assignee"
	task := Task{StatusID: "review", AuthorID: "author", AssigneeID: &assignee}
	rule := &TransitionRule{
		FromStatusID: "review",
		ToStatusID:   "done",
		Conditions: RuleConditions{
			AuthorOnly: true, AssigneeOnly: true, RequiresComment: true,
			ProjectOwnerOnly: true, AllowedRoles: []ProjectRole{RoleAdmin},
		},
	}
	base := TransitionContext{ActorID: "someone", ProjectOwner: "someone", ProjectRole: RoleAdmin, CommentCount: 1}
	tests := []struct {
		name   string
		mutate func(*Task, *TransitionContext)
		want   string
	}{
		{name: "author", mutate: func(_ *Task, _ *TransitionContext) {}, want: "only the task author"},
		{name: "assignee", mutate: func(task *Task, ctx *TransitionContext) { ctx.ActorID = "author" }, want: "only the task assignee"},
		{name: "comment", mutate: func(task *Task, ctx *TransitionContext) {
			task.AuthorID = "assignee"
			ctx.ActorID = "assignee"
			ctx.ProjectOwner = "assignee"
			ctx.CommentCount = 0
		}, want: "comment"},
		{name: "owner", mutate: func(task *Task, ctx *TransitionContext) {
			task.AuthorID = "assignee"
			ctx.ActorID = "assignee"
			ctx.ProjectOwner = "owner"
		}, want: "project owner"},
		{name: "role", mutate: func(task *Task, ctx *TransitionContext) {
			task.AuthorID = "assignee"
			ctx.ActorID = "assignee"
			ctx.ProjectOwner = "assignee"
			ctx.ProjectRole = RoleMember
		}, want: "not allowed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidateTask := task
			candidateCtx := base
			tt.mutate(&candidateTask, &candidateCtx)
			err := ValidateTransition(candidateTask, rule, "done", candidateCtx)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrTransitionNotAllowed)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestValidateTransitionAllowsMatchingActorAndRole(t *testing.T) {
	actor := "user-1"
	task := Task{StatusID: "review", AuthorID: actor, AssigneeID: &actor}
	rule := &TransitionRule{FromStatusID: "review", ToStatusID: "done", Conditions: RuleConditions{
		AuthorOnly: true, AssigneeOnly: true, RequiresComment: true, ProjectOwnerOnly: true,
		AllowedRoles: []ProjectRole{RoleAdmin},
	}}
	err := ValidateTransition(task, rule, "done", TransitionContext{ActorID: actor, ProjectOwner: actor, ProjectRole: RoleAdmin, CommentCount: 1})
	require.NoError(t, err)
}

func TestValidationErrorUnwrapsInvalid(t *testing.T) {
	err := &ValidationError{Field: "name", Message: "required"}
	assert.True(t, errors.Is(err, ErrInvalid))
}
