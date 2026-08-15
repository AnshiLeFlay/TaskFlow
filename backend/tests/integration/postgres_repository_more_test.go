//go:build integration

package integration_test

import (
	"context"
	"os"
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

// requireTestDatabaseURL fails the test with instructions instead of silently
// skipping: a run tagged `-tags=integration` states intent to exercise
// PostgreSQL, so a missing TEST_DATABASE_URL is a misconfiguration, not
// something to shrug off as "not applicable here".
func requireTestDatabaseURL(t *testing.T) string {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required to run PostgreSQL integration tests. " +
			"Run `make test-integration` (starts postgres via docker compose and sets the URL), " +
			"or see the command documented in tests/integration for running it manually against a golang:1.23 container.")
	}
	return databaseURL
}

func testMigrationsURL() string {
	if url := os.Getenv("TEST_MIGRATIONS_URL"); url != "" {
		return url
	}
	return "file://../../migrations"
}

// TestPostgresRepositoryCoversRemainingMethods exercises the repository
// methods not already covered by TestPostgresRepositoryRoundTripAndAtomicTransition:
// UpdateStatus, DeleteStatus, UpdateRule, DeleteRule, UpsertMember +
// GetMembership, UpdateTask, ListTasks, ListBoards, CountComments, and
// ListMembers.
func TestPostgresRepositoryCoversRemainingMethods(t *testing.T) {
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
	projectID := uuid.NewString()
	project := domain.Project{ID: projectID, Name: "Integration Coverage", OwnerID: "owner-sub", CreatedAt: now, UpdatedAt: now}
	owner := domain.Member{ProjectID: projectID, UserID: "owner-sub", Role: domain.RoleAdmin, CreatedAt: now}
	require.NoError(t, repo.CreateProject(ctx, &project, owner))
	t.Cleanup(func() { _, _ = cleanupPool.Exec(context.Background(), `DELETE FROM projects WHERE id=$1`, projectID) })

	// UpsertMember + GetMembership + ListMembers.
	memberUserID := "member-sub"
	require.NoError(t, repo.UpsertMember(ctx, domain.Member{ProjectID: projectID, UserID: memberUserID, Role: domain.RoleMember, CreatedAt: now}))
	membership, err := repo.GetMembership(ctx, projectID, memberUserID)
	require.NoError(t, err)
	assert.Equal(t, domain.RoleMember, membership.Role)
	// UpsertMember again with a new role must update in place, not duplicate.
	require.NoError(t, repo.UpsertMember(ctx, domain.Member{ProjectID: projectID, UserID: memberUserID, Role: domain.RoleViewer, CreatedAt: now}))
	membership, err = repo.GetMembership(ctx, projectID, memberUserID)
	require.NoError(t, err)
	assert.Equal(t, domain.RoleViewer, membership.Role)

	members, err := repo.ListMembers(ctx, projectID)
	require.NoError(t, err)
	require.Len(t, members, 2)
	assert.Equal(t, domain.RoleAdmin, members[0].Role) // "admin" sorts before "viewer"
	assert.Equal(t, "owner-sub", members[0].UserID)
	assert.Equal(t, domain.RoleViewer, members[1].Role)
	assert.Equal(t, memberUserID, members[1].UserID)

	// ListBoards.
	boardID := uuid.NewString()
	fromID, toID, extraStatusID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	board := domain.Board{ID: boardID, ProjectID: projectID, Name: "Coverage Board", CreatedAt: now, UpdatedAt: now}
	statuses := []domain.Status{
		{ID: fromID, BoardID: boardID, Name: "To Do", Position: 0, CreatedAt: now, UpdatedAt: now},
		{ID: toID, BoardID: boardID, Name: "Done", Position: 1, CreatedAt: now, UpdatedAt: now},
	}
	require.NoError(t, repo.CreateBoard(ctx, &board, statuses))

	boards, err := repo.ListBoards(ctx, projectID)
	require.NoError(t, err)
	require.Len(t, boards, 1)
	assert.Equal(t, boardID, boards[0].ID)

	// CreateStatus + UpdateStatus + DeleteStatus, on a status unreferenced by
	// tasks or rules so it can be safely deleted.
	extraStatus := domain.Status{ID: extraStatusID, BoardID: boardID, Name: "Backlog", Position: 2, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateStatus(ctx, &extraStatus))
	extraStatus.Name = "Icebox"
	extraStatus.Position = 3
	extraStatus.UpdatedAt = now.Add(time.Second)
	require.NoError(t, repo.UpdateStatus(ctx, extraStatus))
	reloaded, err := repo.GetStatus(ctx, extraStatusID)
	require.NoError(t, err)
	assert.Equal(t, "Icebox", reloaded.Name)
	assert.Equal(t, 3, reloaded.Position)
	require.NoError(t, repo.DeleteStatus(ctx, extraStatusID))
	_, err = repo.GetStatus(ctx, extraStatusID)
	assert.ErrorIs(t, err, domain.ErrNotFound)

	// CreateRule + UpdateRule + DeleteRule.
	ruleID := uuid.NewString()
	rule := domain.TransitionRule{ID: ruleID, BoardID: boardID, FromStatusID: fromID, ToStatusID: toID, Conditions: domain.RuleConditions{RequiresComment: false}, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateRule(ctx, &rule))
	rule.Conditions = domain.RuleConditions{RequiresComment: true, AllowedRoles: []domain.ProjectRole{domain.RoleAdmin}}
	rule.UpdatedAt = now.Add(time.Second)
	require.NoError(t, repo.UpdateRule(ctx, rule))
	reloadedRule, err := repo.GetRule(ctx, ruleID)
	require.NoError(t, err)
	assert.True(t, reloadedRule.Conditions.RequiresComment)
	assert.Equal(t, []domain.ProjectRole{domain.RoleAdmin}, reloadedRule.Conditions.AllowedRoles)
	require.NoError(t, repo.DeleteRule(ctx, ruleID))
	_, err = repo.GetRule(ctx, ruleID)
	assert.ErrorIs(t, err, domain.ErrNotFound)

	// CreateTask + UpdateTask + ListTasks + CountComments.
	taskID := uuid.NewString()
	task := domain.Task{ID: taskID, BoardID: boardID, StatusID: fromID, Title: "Cover repository", AuthorID: "owner-sub", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateTask(ctx, &task))

	count, err := repo.CountComments(ctx, taskID)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	comment := domain.Comment{ID: uuid.NewString(), TaskID: taskID, AuthorID: "owner-sub", Body: "First pass", CreatedAt: now.Add(time.Second)}
	require.NoError(t, repo.CreateComment(ctx, &comment))
	count, err = repo.CountComments(ctx, taskID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	task.Title = "Cover repository (updated)"
	task.AssigneeID = &memberUserID
	task.UpdatedAt = now.Add(2 * time.Second)
	require.NoError(t, repo.UpdateTask(ctx, task))

	tasks, err := repo.ListTasks(ctx, boardID)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "Cover repository (updated)", tasks[0].Title)
	require.NotNil(t, tasks[0].AssigneeID)
	assert.Equal(t, memberUserID, *tasks[0].AssigneeID)
	require.Len(t, tasks[0].Comments, 1)
	assert.Equal(t, comment.ID, tasks[0].Comments[0].ID)
}
