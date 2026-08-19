package application

import (
	"context"
	"testing"

	"github.com/example/taskflow/backend/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func presetService() (*fakeRepository, *Service, domain.User) {
	repo := newFakeRepository()
	actor := domain.User{ID: "owner"}
	repo.projects["project"] = domain.Project{ID: "project", OwnerID: actor.ID}
	repo.members[memberKey("project", actor.ID)] = domain.Member{ProjectID: "project", UserID: actor.ID, Role: domain.RoleAdmin}
	return repo, NewService(repo, nil), actor
}

// The preset exists so that a board is usable the moment it is created: before
// it, a default board had three columns and no way to move a task between them.
func TestDefaultBoardShipsWithWorkingRules(t *testing.T) {
	repo, service, actor := presetService()

	board, err := service.CreateBoard(context.Background(), actor, "project", CreateBoardCommand{Name: "Board"})
	require.NoError(t, err)

	require.Len(t, board.Statuses, 3)
	// Neighbours linked both ways: 2 gaps x 2 directions.
	require.Len(t, board.Rules, 4)

	byName := map[string]string{}
	for _, status := range board.Statuses {
		byName[status.Name] = status.ID
	}
	pairs := map[[2]string]bool{}
	for _, rule := range board.Rules {
		pairs[[2]string{rule.FromStatusID, rule.ToStatusID}] = true
		assert.Equal(t, board.Board.ID, rule.BoardID)
		assert.Equal(t, domain.RuleConditions{}, rule.Conditions, "the preset stays permissive")
	}
	assert.True(t, pairs[[2]string{byName["To Do"], byName["In Progress"]}])
	assert.True(t, pairs[[2]string{byName["In Progress"], byName["Done"]}])
	assert.True(t, pairs[[2]string{byName["In Progress"], byName["To Do"]}])
	assert.True(t, pairs[[2]string{byName["Done"], byName["In Progress"]}])
	// Skipping a column is not part of the preset.
	assert.False(t, pairs[[2]string{byName["To Do"], byName["Done"]}])

	assert.Len(t, repo.rules, 4, "rules are persisted, not only returned")
}

func TestDefaultBoardCanMoveATaskImmediately(t *testing.T) {
	_, service, actor := presetService()
	ctx := context.Background()

	board, err := service.CreateBoard(ctx, actor, "project", CreateBoardCommand{Name: "Board"})
	require.NoError(t, err)
	todo, inProgress := board.Statuses[0].ID, board.Statuses[1].ID

	task, err := service.CreateTask(ctx, actor, board.Board.ID, CreateTaskCommand{Title: "Task", StatusID: todo})
	require.NoError(t, err)

	moved, err := service.TransitionTask(ctx, actor, task.ID, inProgress)
	require.NoError(t, err)
	assert.Equal(t, inProgress, moved.StatusID)
}

// A caller that names its own statuses is designing a workflow; inventing
// rules for it would be guesswork.
func TestCustomStatusesGetNoRules(t *testing.T) {
	_, service, actor := presetService()

	board, err := service.CreateBoard(context.Background(), actor, "project", CreateBoardCommand{
		Name:     "Board",
		Statuses: []StatusInput{{Name: "Backlog", Position: 0}, {Name: "Shipped", Position: 1}},
	})
	require.NoError(t, err)

	assert.Len(t, board.Statuses, 2)
	assert.Empty(t, board.Rules)
}
