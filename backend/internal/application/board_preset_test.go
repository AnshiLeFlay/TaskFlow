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

// presetBoard returns the created board and its statuses keyed by name, since
// the preset's shape is expressed in names rather than column order.
func presetBoard(t *testing.T) (*Service, domain.User, domain.BoardAggregate, map[string]string) {
	t.Helper()
	_, service, actor := presetService()
	board, err := service.CreateBoard(context.Background(), actor, "project", CreateBoardCommand{Name: "Board"})
	require.NoError(t, err)
	byName := make(map[string]string, len(board.Statuses))
	for _, status := range board.Statuses {
		byName[status.Name] = status.ID
	}
	return service, actor, board, byName
}

func transitionSet(board domain.BoardAggregate) map[[2]string]bool {
	pairs := make(map[[2]string]bool, len(board.Rules))
	for _, rule := range board.Rules {
		pairs[[2]string{rule.FromStatusID, rule.ToStatusID}] = true
	}
	return pairs
}

func TestDefaultBoardColumns(t *testing.T) {
	_, _, board, byName := presetBoard(t)

	require.Len(t, board.Statuses, 5)
	for _, name := range []string{"Backlog", "To Do", "In Progress", "Done", "Canceled"} {
		assert.Contains(t, byName, name)
	}
	// Column order is what the board renders and what "next column" means.
	positions := map[string]int{}
	for _, status := range board.Statuses {
		positions[status.Name] = status.Position
	}
	assert.Equal(t, 0, positions["Backlog"])
	assert.Equal(t, 4, positions["Canceled"])
}

func TestDefaultBoardTransitions(t *testing.T) {
	_, _, board, byName := presetBoard(t)
	pairs := transitionSet(board)
	has := func(from, to string) bool { return pairs[[2]string{byName[from], byName[to]}] }

	require.Len(t, board.Rules, 10)
	for _, rule := range board.Rules {
		assert.Equal(t, board.Board.ID, rule.BoardID)
		assert.Equal(t, domain.RuleConditions{}, rule.Conditions, "the preset stays permissive")
	}

	assert.True(t, has("Backlog", "To Do"))
	assert.True(t, has("To Do", "In Progress"))
	assert.True(t, has("In Progress", "Done"))

	assert.True(t, has("To Do", "Backlog"))
	assert.True(t, has("In Progress", "To Do"))
	assert.True(t, has("Done", "In Progress"))

	assert.True(t, has("Backlog", "Canceled"))
	assert.True(t, has("To Do", "Canceled"))
	assert.True(t, has("In Progress", "Canceled"))

	assert.True(t, has("Canceled", "Backlog"))

	// Skipping a column is a workflow decision, not a sensible default.
	assert.False(t, has("Backlog", "In Progress"))
	assert.False(t, has("To Do", "Done"))
}

// Finished work cannot be un-finished; deciding it was unnecessary is a new
// task rather than an undo.
func TestPresetCannotCancelFinishedWork(t *testing.T) {
	service, actor, board, byName := presetBoard(t)
	ctx := context.Background()
	pairs := transitionSet(board)
	assert.False(t, pairs[[2]string{byName["Done"], byName["Canceled"]}])

	task, err := service.CreateTask(ctx, actor, board.Board.ID, CreateTaskCommand{Title: "Shipped", StatusID: byName["Backlog"]})
	require.NoError(t, err)
	for _, step := range []string{"To Do", "In Progress", "Done"} {
		_, err = service.TransitionTask(ctx, actor, task.ID, byName[step])
		require.NoError(t, err, "advancing to %s", step)
	}

	_, err = service.TransitionTask(ctx, actor, task.ID, byName["Canceled"])
	assert.ErrorIs(t, err, domain.ErrTransitionNotAllowed)
}

// A revived task must re-enter through intake, or its timings stop being
// comparable with everything that came through Backlog.
func TestPresetRevivesOnlyThroughBacklog(t *testing.T) {
	service, actor, board, byName := presetBoard(t)
	ctx := context.Background()
	pairs := transitionSet(board)
	assert.False(t, pairs[[2]string{byName["Canceled"], byName["To Do"]}])
	assert.False(t, pairs[[2]string{byName["Canceled"], byName["In Progress"]}])

	task, err := service.CreateTask(ctx, actor, board.Board.ID, CreateTaskCommand{Title: "Duplicate", StatusID: byName["Backlog"]})
	require.NoError(t, err)
	_, err = service.TransitionTask(ctx, actor, task.ID, byName["Canceled"])
	require.NoError(t, err)

	_, err = service.TransitionTask(ctx, actor, task.ID, byName["To Do"])
	assert.ErrorIs(t, err, domain.ErrTransitionNotAllowed)

	revived, err := service.TransitionTask(ctx, actor, task.ID, byName["Backlog"])
	require.NoError(t, err)
	assert.Equal(t, byName["Backlog"], revived.StatusID)
}

// The preset exists so a board is usable the moment it is created: before it,
// a default board had columns and no way to move a task between them.
func TestDefaultBoardRunsATaskEndToEnd(t *testing.T) {
	service, actor, board, byName := presetBoard(t)
	ctx := context.Background()

	task, err := service.CreateTask(ctx, actor, board.Board.ID, CreateTaskCommand{Title: "Task", StatusID: byName["Backlog"]})
	require.NoError(t, err)

	for _, step := range []string{"To Do", "In Progress", "Done"} {
		moved, err := service.TransitionTask(ctx, actor, task.ID, byName[step])
		require.NoError(t, err, "advancing to %s", step)
		assert.Equal(t, byName[step], moved.StatusID)
	}
}

func TestPresetRulesArePersisted(t *testing.T) {
	repo, service, actor := presetService()

	_, err := service.CreateBoard(context.Background(), actor, "project", CreateBoardCommand{Name: "Board"})
	require.NoError(t, err)

	assert.Len(t, repo.rules, 10, "rules are stored, not only returned")
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
