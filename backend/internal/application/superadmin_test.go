package application

import (
	"context"
	"testing"

	"github.com/example/taskflow/backend/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// superadminFixture builds a project owned by someone else, with a board, two
// statuses, a rule between them and one task. The superadmin is deliberately
// absent from project_members: that is the whole point of the role.
func superadminFixture() (*fakeRepository, *Service) {
	repo := newFakeRepository()
	repo.projects["project"] = domain.Project{ID: "project", Name: "Someone else's", OwnerID: "owner"}
	repo.members[memberKey("project", "owner")] = domain.Member{ProjectID: "project", UserID: "owner", Role: domain.RoleAdmin}
	repo.boards["board"] = domain.Board{ID: "board", ProjectID: "project"}
	repo.statuses["todo"] = domain.Status{ID: "todo", BoardID: "board", Name: "To Do", Position: 0}
	repo.statuses["done"] = domain.Status{ID: "done", BoardID: "board", Name: "Done", Position: 1}
	repo.rules[ruleKey("board", "todo", "done")] = domain.TransitionRule{ID: "rule", BoardID: "board", FromStatusID: "todo", ToStatusID: "done"}
	repo.tasks["task"] = domain.Task{ID: "task", BoardID: "board", StatusID: "todo", Title: "Task", AuthorID: "owner"}
	return repo, NewService(repo, nil)
}

func superadmin() domain.User {
	return domain.User{ID: "athena", Username: "athena", Roles: []string{domain.RealmRoleSuperadmin}}
}

func outsider() domain.User {
	return domain.User{ID: "outsider", Username: "outsider", Roles: []string{"admin", "member"}}
}

func TestSuperadminReachesProjectWithoutMembership(t *testing.T) {
	ctx := context.Background()
	_, service := superadminFixture()

	board, err := service.GetBoard(ctx, superadmin(), "board")
	require.NoError(t, err)
	assert.Equal(t, "board", board.Board.ID)

	members, err := service.ListMembers(ctx, superadmin(), "project")
	require.NoError(t, err)
	assert.Len(t, members, 1)
}

// The realm role "admin" is what every ordinary user carries; only
// "superadmin" may cross project boundaries.
func TestNonSuperadminStillForbidden(t *testing.T) {
	ctx := context.Background()
	_, service := superadminFixture()

	_, err := service.GetBoard(ctx, outsider(), "board")
	assert.ErrorIs(t, err, domain.ErrForbidden)

	_, err = service.ListMembers(ctx, outsider(), "project")
	assert.ErrorIs(t, err, domain.ErrForbidden)

	projects, err := service.ListProjects(ctx, outsider())
	require.NoError(t, err)
	assert.Empty(t, projects)
}

func TestSuperadminListsEveryProjectAsAdmin(t *testing.T) {
	ctx := context.Background()
	repo, service := superadminFixture()
	repo.projects["other"] = domain.Project{ID: "other", Name: "Another", OwnerID: "someone"}

	projects, err := service.ListProjects(ctx, superadmin())
	require.NoError(t, err)
	require.Len(t, projects, 2)
	for _, project := range projects {
		assert.Equal(t, domain.RoleAdmin, project.Role, "superadmin acts as admin in %s", project.ID)
	}
}

// Realtime subscriptions must not diverge from the REST project list, or the
// agent would see a project it cannot receive events for.
func TestSuperadminRealtimeMatchesProjectList(t *testing.T) {
	ctx := context.Background()
	repo, service := superadminFixture()
	repo.projects["other"] = domain.Project{ID: "other", OwnerID: "someone"}

	ids, err := service.ProjectIDs(ctx, superadmin())
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"project", "other"}, ids)

	assert.NoError(t, service.AuthorizeProject(ctx, superadmin(), "project"))
	assert.ErrorIs(t, service.AuthorizeProject(ctx, outsider(), "project"), domain.ErrForbidden)
}

func TestSuperadminGetsNotFoundForUnknownProject(t *testing.T) {
	ctx := context.Background()
	_, service := superadminFixture()

	err := service.AuthorizeProject(ctx, superadmin(), "no-such-project")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestSuperadminMayManageWorkflowAndTasks(t *testing.T) {
	ctx := context.Background()
	_, service := superadminFixture()

	_, err := service.CreateStatus(ctx, superadmin(), "board", CreateStatusCommand{Name: "Review", Position: 2})
	require.NoError(t, err)

	_, err = service.CreateTask(ctx, superadmin(), "board", CreateTaskCommand{Title: "From the agent", StatusID: "todo"})
	require.NoError(t, err)

	_, err = service.TransitionTask(ctx, superadmin(), "task", "done")
	require.NoError(t, err)
}

// Workflow conditions encode the project's own process, not who may reach it.
// A superadmin is not the project owner and must still be refused here.
func TestSuperadminDoesNotBypassWorkflowConditions(t *testing.T) {
	ctx := context.Background()
	repo, service := superadminFixture()
	key := ruleKey("board", "todo", "done")
	rule := repo.rules[key]
	rule.Conditions = domain.RuleConditions{ProjectOwnerOnly: true}
	repo.rules[key] = rule

	_, err := service.TransitionTask(ctx, superadmin(), "task", "done")
	assert.ErrorIs(t, err, domain.ErrTransitionNotAllowed)
}

// A superadmin has no project_members row, so making them assignable would
// write an assignee the project does not actually contain.
func TestSuperadminIsNotAssignableWithoutMembership(t *testing.T) {
	ctx := context.Background()
	_, service := superadminFixture()
	agent := superadmin().ID

	_, err := service.CreateTask(ctx, superadmin(), "board", CreateTaskCommand{Title: "Assigned", StatusID: "todo", AssigneeID: &agent})

	var validation *domain.ValidationError
	require.ErrorAs(t, err, &validation)
	assert.Equal(t, "assignee_id", validation.Field)
}
