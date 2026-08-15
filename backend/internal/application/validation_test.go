package application

import (
	"context"
	"strings"
	"testing"

	"github.com/example/taskflow/backend/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandTextLimitsMatchOpenAPI(t *testing.T) {
	ctx := context.Background()
	actor := domain.User{ID: "admin"}
	repo := newFakeRepository()
	repo.projects["project"] = domain.Project{ID: "project", OwnerID: actor.ID}
	repo.members[memberKey("project", actor.ID)] = domain.Member{ProjectID: "project", UserID: actor.ID, Role: domain.RoleAdmin}
	repo.boards["board"] = domain.Board{ID: "board", ProjectID: "project"}
	repo.statuses["todo"] = domain.Status{ID: "todo", BoardID: "board"}
	repo.tasks["task"] = domain.Task{ID: "task", BoardID: "board", StatusID: "todo", AuthorID: actor.ID, Title: "Task"}
	service := NewService(repo, nil)

	tests := []struct {
		name  string
		field string
		call  func() error
	}{
		{
			name:  "project name",
			field: "name",
			call: func() error {
				_, err := service.CreateProject(ctx, actor, CreateProjectCommand{Name: strings.Repeat("x", maxProjectOrBoardNameLength+1)})
				return err
			},
		},
		{
			name:  "project description",
			field: "description",
			call: func() error {
				_, err := service.CreateProject(ctx, actor, CreateProjectCommand{Name: "Project", Description: strings.Repeat("x", maxProjectOrBoardDescLength+1)})
				return err
			},
		},
		{
			name:  "board name",
			field: "name",
			call: func() error {
				_, err := service.CreateBoard(ctx, actor, "project", CreateBoardCommand{Name: strings.Repeat("x", maxProjectOrBoardNameLength+1)})
				return err
			},
		},
		{
			name:  "status name",
			field: "name",
			call: func() error {
				_, err := service.CreateStatus(ctx, actor, "board", CreateStatusCommand{Name: strings.Repeat("x", maxStatusNameLength+1), Position: 1})
				return err
			},
		},
		{
			name:  "task title",
			field: "title",
			call: func() error {
				_, err := service.CreateTask(ctx, actor, "board", CreateTaskCommand{Title: strings.Repeat("x", maxTaskTitleLength+1), StatusID: "todo"})
				return err
			},
		},
		{
			name:  "task description",
			field: "description",
			call: func() error {
				_, err := service.UpdateTask(ctx, actor, "task", UpdateTaskCommand{Description: stringPointer(strings.Repeat("x", maxTaskDescriptionLength+1))})
				return err
			},
		},
		{
			name:  "comment body",
			field: "body",
			call: func() error {
				_, err := service.CreateComment(ctx, actor, "task", strings.Repeat("x", maxCommentLength+1))
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.call()
			require.Error(t, err)
			assert.ErrorIs(t, err, domain.ErrInvalid)
			var validation *domain.ValidationError
			require.ErrorAs(t, err, &validation)
			assert.Equal(t, test.field, validation.Field)
		})
	}
}

func stringPointer(value string) *string { return &value }
