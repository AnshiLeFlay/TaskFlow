//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/example/taskflow/backend/internal/domain"
	"github.com/example/taskflow/backend/internal/infrastructure/migrations"
	postgresrepo "github.com/example/taskflow/backend/internal/infrastructure/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresRepositoryRoundTripAndAtomicTransition(t *testing.T) {
	databaseURL := requireTestDatabaseURL(t)
	require.NoError(t, migrations.Up(databaseURL, testMigrationsURL()))
	ctx := context.Background()
	repo, err := postgresrepo.New(ctx, databaseURL)
	require.NoError(t, err)
	t.Cleanup(repo.Close)
	cleanupPool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err)
	t.Cleanup(cleanupPool.Close)

	now := time.Now().UTC().Truncate(time.Microsecond)
	projectID, boardID := uuid.NewString(), uuid.NewString()
	fromID, toID, taskID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	project := domain.Project{ID: projectID, Name: "Integration", OwnerID: "owner-sub", CreatedAt: now, UpdatedAt: now}
	owner := domain.Member{ProjectID: projectID, UserID: "owner-sub", Role: domain.RoleAdmin, CreatedAt: now}
	require.NoError(t, repo.CreateProject(ctx, &project, owner))
	t.Cleanup(func() { _, _ = cleanupPool.Exec(context.Background(), `DELETE FROM projects WHERE id=$1`, projectID) })

	projects, err := repo.ListProjects(ctx, "owner-sub")
	require.NoError(t, err)
	require.Len(t, projects, 1)
	assert.Equal(t, domain.RoleAdmin, projects[0].Role)

	board := domain.Board{ID: boardID, ProjectID: projectID, Name: "Main", CreatedAt: now, UpdatedAt: now}
	statuses := []domain.Status{
		{ID: fromID, BoardID: boardID, Name: "To Do", Position: 0, CreatedAt: now, UpdatedAt: now},
		{ID: toID, BoardID: boardID, Name: "Done", Position: 1, CreatedAt: now, UpdatedAt: now},
	}
	require.NoError(t, repo.CreateBoard(ctx, &board, statuses))
	rule := domain.TransitionRule{ID: uuid.NewString(), BoardID: boardID, FromStatusID: fromID, ToStatusID: toID, Conditions: domain.RuleConditions{AuthorOnly: true, AllowedRoles: []domain.ProjectRole{domain.RoleAdmin}}, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateRule(ctx, &rule))
	task := domain.Task{ID: taskID, BoardID: boardID, StatusID: fromID, Title: "Exercise repository", AuthorID: "owner-sub", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateTask(ctx, &task))
	comment := domain.Comment{ID: uuid.NewString(), TaskID: taskID, AuthorID: "owner-sub", Body: "Persisted activity", CreatedAt: now.Add(time.Second)}
	require.NoError(t, repo.CreateComment(ctx, &comment))

	aggregate, err := repo.GetBoardAggregate(ctx, boardID)
	require.NoError(t, err)
	assert.Len(t, aggregate.Statuses, 2)
	assert.Len(t, aggregate.Tasks, 1)
	require.Len(t, aggregate.Tasks[0].Comments, 1)
	assert.Equal(t, comment.ID, aggregate.Tasks[0].Comments[0].ID)
	assert.Equal(t, "owner-sub", aggregate.Tasks[0].Comments[0].AuthorID)
	assert.Equal(t, "Persisted activity", aggregate.Tasks[0].Comments[0].Body)
	assert.Len(t, aggregate.Rules, 1)
	found, err := repo.FindRule(ctx, boardID, fromID, toID)
	require.NoError(t, err)
	assert.True(t, found.Conditions.AuthorOnly)
	assert.Equal(t, []domain.ProjectRole{domain.RoleAdmin}, found.Conditions.AllowedRoles)

	updated, err := repo.UpdateTaskStatus(ctx, taskID, fromID, task.UpdatedAt, toID)
	require.NoError(t, err)
	assert.Equal(t, toID, updated.StatusID)
	_, err = repo.UpdateTaskStatus(ctx, taskID, fromID, task.UpdatedAt, toID)
	assert.ErrorIs(t, err, domain.ErrConflict)
}
