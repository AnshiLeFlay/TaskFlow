package application

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/example/taskflow/backend/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRepository struct {
	projects        map[string]domain.Project
	members         map[string]domain.Member
	boards          map[string]domain.Board
	statuses        map[string]domain.Status
	rules           map[string]domain.TransitionRule
	tasks           map[string]domain.Task
	commentCounts   map[string]int
	statusUpdates   int
	statusUpdateErr error
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{projects: map[string]domain.Project{}, members: map[string]domain.Member{}, boards: map[string]domain.Board{}, statuses: map[string]domain.Status{}, rules: map[string]domain.TransitionRule{}, tasks: map[string]domain.Task{}, commentCounts: map[string]int{}}
}

func memberKey(projectID, userID string) string   { return projectID + ":" + userID }
func ruleKey(boardID, fromID, toID string) string { return boardID + ":" + fromID + ":" + toID }

func (f *fakeRepository) Ping(context.Context) error { return nil }
func (f *fakeRepository) CreateProject(_ context.Context, project *domain.Project, owner domain.Member) error {
	f.projects[project.ID] = *project
	f.members[memberKey(owner.ProjectID, owner.UserID)] = owner
	return nil
}
func (f *fakeRepository) ListProjects(_ context.Context, userID string) ([]domain.Project, error) {
	var result []domain.Project
	for _, p := range f.projects {
		if m, ok := f.members[memberKey(p.ID, userID)]; ok {
			p.Role = m.Role
			result = append(result, p)
		}
	}
	return result, nil
}
func (f *fakeRepository) GetProject(_ context.Context, id string) (domain.Project, error) {
	p, ok := f.projects[id]
	if !ok {
		return p, domain.ErrNotFound
	}
	return p, nil
}
func (f *fakeRepository) GetMembership(_ context.Context, projectID, userID string) (domain.Member, error) {
	m, ok := f.members[memberKey(projectID, userID)]
	if !ok {
		return m, domain.ErrNotFound
	}
	return m, nil
}
func (f *fakeRepository) UpsertMember(_ context.Context, m domain.Member) error {
	f.members[memberKey(m.ProjectID, m.UserID)] = m
	return nil
}
func (f *fakeRepository) CreateBoard(_ context.Context, b *domain.Board, statuses []domain.Status) error {
	f.boards[b.ID] = *b
	for _, s := range statuses {
		f.statuses[s.ID] = s
	}
	return nil
}
func (f *fakeRepository) ListBoards(_ context.Context, projectID string) ([]domain.Board, error) {
	var out []domain.Board
	for _, b := range f.boards {
		if b.ProjectID == projectID {
			out = append(out, b)
		}
	}
	return out, nil
}
func (f *fakeRepository) GetBoard(_ context.Context, id string) (domain.Board, error) {
	b, ok := f.boards[id]
	if !ok {
		return b, domain.ErrNotFound
	}
	return b, nil
}
func (f *fakeRepository) GetBoardAggregate(_ context.Context, id string) (domain.BoardAggregate, error) {
	b, err := f.GetBoard(context.Background(), id)
	if err != nil {
		return domain.BoardAggregate{}, err
	}
	agg := domain.BoardAggregate{Board: b, Statuses: []domain.Status{}, Tasks: []domain.Task{}, Rules: []domain.TransitionRule{}}
	for _, v := range f.statuses {
		if v.BoardID == id {
			agg.Statuses = append(agg.Statuses, v)
		}
	}
	for _, v := range f.tasks {
		if v.BoardID == id {
			agg.Tasks = append(agg.Tasks, v)
		}
	}
	for _, v := range f.rules {
		if v.BoardID == id {
			agg.Rules = append(agg.Rules, v)
		}
	}
	return agg, nil
}
func (f *fakeRepository) CreateStatus(_ context.Context, s *domain.Status) error {
	f.statuses[s.ID] = *s
	return nil
}
func (f *fakeRepository) GetStatus(_ context.Context, id string) (domain.Status, error) {
	v, ok := f.statuses[id]
	if !ok {
		return v, domain.ErrNotFound
	}
	return v, nil
}
func (f *fakeRepository) UpdateStatus(_ context.Context, s domain.Status) error {
	f.statuses[s.ID] = s
	return nil
}
func (f *fakeRepository) DeleteStatus(_ context.Context, id string) error {
	delete(f.statuses, id)
	return nil
}
func (f *fakeRepository) CreateRule(_ context.Context, r *domain.TransitionRule) error {
	f.rules[r.ID] = *r
	f.rules[ruleKey(r.BoardID, r.FromStatusID, r.ToStatusID)] = *r
	return nil
}
func (f *fakeRepository) GetRule(_ context.Context, id string) (domain.TransitionRule, error) {
	v, ok := f.rules[id]
	if !ok {
		return v, domain.ErrNotFound
	}
	return v, nil
}
func (f *fakeRepository) FindRule(_ context.Context, boardID, fromID, toID string) (domain.TransitionRule, error) {
	v, ok := f.rules[ruleKey(boardID, fromID, toID)]
	if !ok {
		return v, domain.ErrNotFound
	}
	return v, nil
}
func (f *fakeRepository) UpdateRule(_ context.Context, r domain.TransitionRule) error {
	f.rules[r.ID] = r
	return nil
}
func (f *fakeRepository) DeleteRule(_ context.Context, id string) error {
	delete(f.rules, id)
	return nil
}
func (f *fakeRepository) ListTasks(_ context.Context, boardID string) ([]domain.Task, error) {
	var out []domain.Task
	for _, v := range f.tasks {
		if v.BoardID == boardID {
			out = append(out, v)
		}
	}
	return out, nil
}
func (f *fakeRepository) CreateTask(_ context.Context, t *domain.Task) error {
	f.tasks[t.ID] = *t
	return nil
}
func (f *fakeRepository) GetTask(_ context.Context, id string) (domain.Task, error) {
	v, ok := f.tasks[id]
	if !ok {
		return v, domain.ErrNotFound
	}
	return v, nil
}
func (f *fakeRepository) UpdateTask(_ context.Context, t domain.Task) error {
	f.tasks[t.ID] = t
	return nil
}
func (f *fakeRepository) UpdateTaskStatus(_ context.Context, id, expected string, expectedUpdatedAt time.Time, target string) (domain.Task, error) {
	if f.statusUpdateErr != nil {
		return domain.Task{}, f.statusUpdateErr
	}
	v, ok := f.tasks[id]
	if !ok {
		return v, domain.ErrNotFound
	}
	if v.StatusID != expected || !v.UpdatedAt.Equal(expectedUpdatedAt) {
		return v, domain.ErrConflict
	}
	v.StatusID = target
	f.tasks[id] = v
	f.statusUpdates++
	return v, nil
}
func (f *fakeRepository) CountComments(_ context.Context, id string) (int, error) {
	return f.commentCounts[id], nil
}
func (f *fakeRepository) CreateComment(_ context.Context, c *domain.Comment) error {
	f.commentCounts[c.TaskID]++
	return nil
}

type recordingPublisher struct{ events []domain.TaskEvent }

func (p *recordingPublisher) Publish(event domain.TaskEvent) { p.events = append(p.events, event) }

func TestCreateProjectMakesCreatorAdminAndOwner(t *testing.T) {
	repo := newFakeRepository()
	clock := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	ids := []string{"project-id"}
	service := NewService(repo, nil, WithClock(func() time.Time { return clock }), WithIDGenerator(func() string { id := ids[0]; ids = ids[1:]; return id }))
	project, err := service.CreateProject(context.Background(), domain.User{ID: "user-1"}, CreateProjectCommand{Name: " TaskFlow "})
	require.NoError(t, err)
	assert.Equal(t, "TaskFlow", project.Name)
	assert.Equal(t, "user-1", project.OwnerID)
	assert.Equal(t, domain.RoleAdmin, project.Role)
	membership, err := repo.GetMembership(context.Background(), project.ID, "user-1")
	require.NoError(t, err)
	assert.Equal(t, domain.RoleAdmin, membership.Role)
}

func TestTransitionTaskRejectsMissingRuleWithoutMutation(t *testing.T) {
	repo, service, publisher := transitionFixture()
	delete(repo.rules, ruleKey("board", "todo", "done"))
	_, err := service.TransitionTask(context.Background(), domain.User{ID: "actor"}, "task", "done")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrTransitionNotAllowed))
	assert.Equal(t, 0, repo.statusUpdates)
	assert.Empty(t, publisher.events)
}

func TestTransitionTaskAppliesRuleAndPublishesDomainEvent(t *testing.T) {
	repo, service, publisher := transitionFixture()
	updated, err := service.TransitionTask(context.Background(), domain.User{ID: "actor"}, "task", "done")
	require.NoError(t, err)
	assert.Equal(t, "done", updated.StatusID)
	assert.Equal(t, 1, repo.statusUpdates)
	require.Len(t, publisher.events, 1)
	event := publisher.events[0]
	assert.Equal(t, domain.EventTaskTransitioned, event.Type)
	assert.Equal(t, "todo", event.FromStatusID)
	assert.Equal(t, "done", event.ToStatusID)
	assert.Equal(t, "project", event.ProjectID)
}

func TestViewerCannotCreateTask(t *testing.T) {
	repo := newFakeRepository()
	repo.projects["project"] = domain.Project{ID: "project", OwnerID: "owner"}
	repo.members[memberKey("project", "viewer")] = domain.Member{ProjectID: "project", UserID: "viewer", Role: domain.RoleViewer}
	repo.boards["board"] = domain.Board{ID: "board", ProjectID: "project"}
	service := NewService(repo, nil)
	_, err := service.CreateTask(context.Background(), domain.User{ID: "viewer"}, "board", CreateTaskCommand{Title: "Nope"})
	assert.ErrorIs(t, err, domain.ErrForbidden)
}

func TestAddMemberEnforcesAdminAndPublicRoles(t *testing.T) {
	repo := newFakeRepository()
	repo.projects["project"] = domain.Project{ID: "project", OwnerID: "admin"}
	repo.members[memberKey("project", "admin")] = domain.Member{ProjectID: "project", UserID: "admin", Role: domain.RoleAdmin}
	repo.members[memberKey("project", "member")] = domain.Member{ProjectID: "project", UserID: "member", Role: domain.RoleMember}
	service := NewService(repo, nil)

	created, err := service.AddMember(context.Background(), domain.User{ID: "admin"}, "project", AddMemberCommand{UserID: "new-user", Role: domain.RoleViewer})
	require.NoError(t, err)
	assert.Equal(t, domain.RoleViewer, created.Role)
	_, err = service.AddMember(context.Background(), domain.User{ID: "member"}, "project", AddMemberCommand{UserID: "other", Role: domain.RoleMember})
	assert.ErrorIs(t, err, domain.ErrForbidden)
	_, err = service.AddMember(context.Background(), domain.User{ID: "admin"}, "project", AddMemberCommand{UserID: "other", Role: domain.ProjectRole("owner")})
	assert.ErrorIs(t, err, domain.ErrInvalid)
	_, err = service.AddMember(context.Background(), domain.User{ID: "admin"}, "project", AddMemberCommand{UserID: "admin", Role: domain.RoleViewer})
	assert.ErrorIs(t, err, domain.ErrConflict)
}

func TestCreateBoardProvidesDefaultsAndRejectsAmbiguousPositions(t *testing.T) {
	repo := newFakeRepository()
	repo.projects["project"] = domain.Project{ID: "project", OwnerID: "admin"}
	repo.members[memberKey("project", "admin")] = domain.Member{ProjectID: "project", UserID: "admin", Role: domain.RoleAdmin}
	next := 0
	service := NewService(repo, nil, WithIDGenerator(func() string { next++; return fmt.Sprintf("id-%d", next) }))

	board, err := service.CreateBoard(context.Background(), domain.User{ID: "admin"}, "project", CreateBoardCommand{Name: "Main"})
	require.NoError(t, err)
	require.Len(t, board.Statuses, 3)
	assert.Equal(t, []string{"To Do", "In Progress", "Done"}, []string{board.Statuses[0].Name, board.Statuses[1].Name, board.Statuses[2].Name})
	_, err = service.CreateBoard(context.Background(), domain.User{ID: "admin"}, "project", CreateBoardCommand{Name: "Bad", Statuses: []StatusInput{{Name: "A", Position: 0}, {Name: "B", Position: 0}}})
	assert.ErrorIs(t, err, domain.ErrInvalid)
}

func TestCreateRuleRejectsStatusFromAnotherBoard(t *testing.T) {
	repo := newFakeRepository()
	repo.projects["project"] = domain.Project{ID: "project", OwnerID: "admin"}
	repo.members[memberKey("project", "admin")] = domain.Member{ProjectID: "project", UserID: "admin", Role: domain.RoleAdmin}
	repo.boards["board-a"] = domain.Board{ID: "board-a", ProjectID: "project"}
	repo.boards["board-b"] = domain.Board{ID: "board-b", ProjectID: "project"}
	repo.statuses["a"] = domain.Status{ID: "a", BoardID: "board-a"}
	repo.statuses["b"] = domain.Status{ID: "b", BoardID: "board-b"}
	service := NewService(repo, nil)
	_, err := service.CreateRule(context.Background(), domain.User{ID: "admin"}, "board-a", CreateRuleCommand{FromStatusID: "a", ToStatusID: "b"})
	assert.ErrorIs(t, err, domain.ErrInvalid)
}

func TestTransitionRequiresCommentBeforeRepositoryMutation(t *testing.T) {
	repo, service, publisher := transitionFixture()
	repo.commentCounts["task"] = 0
	_, err := service.TransitionTask(context.Background(), domain.User{ID: "actor"}, "task", "done")
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrTransitionNotAllowed)
	assert.Equal(t, 0, repo.statusUpdates)
	assert.Empty(t, publisher.events)
	repo.commentCounts["task"] = 1
	_, err = service.TransitionTask(context.Background(), domain.User{ID: "actor"}, "task", "done")
	require.NoError(t, err)
	assert.Equal(t, 1, repo.statusUpdates)
}

func TestCreateTaskRequiresAssigneeMembership(t *testing.T) {
	repo := newFakeRepository()
	repo.projects["project"] = domain.Project{ID: "project", OwnerID: "admin"}
	repo.members[memberKey("project", "admin")] = domain.Member{ProjectID: "project", UserID: "admin", Role: domain.RoleAdmin}
	repo.members[memberKey("project", "assignee")] = domain.Member{ProjectID: "project", UserID: "assignee", Role: domain.RoleMember}
	repo.boards["board"] = domain.Board{ID: "board", ProjectID: "project"}
	repo.statuses["todo"] = domain.Status{ID: "todo", BoardID: "board", Position: 0}
	service := NewService(repo, nil, WithIDGenerator(func() string { return "task-id" }))
	outsider := "outsider"
	_, err := service.CreateTask(context.Background(), domain.User{ID: "admin"}, "board", CreateTaskCommand{Title: "Task", StatusID: "todo", AssigneeID: &outsider})
	assert.ErrorIs(t, err, domain.ErrInvalid)
	assignee := "assignee"
	task, err := service.CreateTask(context.Background(), domain.User{ID: "admin"}, "board", CreateTaskCommand{Title: "Task", StatusID: "todo", AssigneeID: &assignee})
	require.NoError(t, err)
	require.NotNil(t, task.AssigneeID)
	assert.Equal(t, assignee, *task.AssigneeID)
}

func TestUpdateTaskPublishesTaskUpdatedEvent(t *testing.T) {
	repo := eventFixtureRepository()
	publisher := &recordingPublisher{}
	now := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	service := NewService(repo, publisher,
		WithClock(func() time.Time { return now }),
		WithIDGenerator(func() string { return "update-event" }),
	)
	title := "Updated title"

	updated, err := service.UpdateTask(context.Background(), domain.User{ID: "actor"}, "task", UpdateTaskCommand{Title: &title})

	require.NoError(t, err)
	assert.Equal(t, title, updated.Title)
	require.Len(t, publisher.events, 1)
	event := publisher.events[0]
	assert.Equal(t, "update-event", event.ID)
	assert.Equal(t, domain.EventTaskUpdated, event.Type)
	assert.Equal(t, "project", event.ProjectID)
	assert.Equal(t, "board", event.BoardID)
	assert.Equal(t, "task", event.TaskID)
	assert.Equal(t, "actor", event.ActorID)
	assert.Equal(t, now, event.OccurredAt)
	payloadTask, ok := event.Payload["task"].(domain.Task)
	require.True(t, ok)
	assert.Equal(t, title, payloadTask.Title)
}

func TestCreateCommentPublishesCommentCreatedEvent(t *testing.T) {
	repo := eventFixtureRepository()
	publisher := &recordingPublisher{}
	now := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	ids := []string{"comment-id", "comment-event"}
	service := NewService(repo, publisher,
		WithClock(func() time.Time { return now }),
		WithIDGenerator(func() string {
			id := ids[0]
			ids = ids[1:]
			return id
		}),
	)

	comment, err := service.CreateComment(context.Background(), domain.User{ID: "actor"}, "task", " useful context ")

	require.NoError(t, err)
	assert.Equal(t, "comment-id", comment.ID)
	assert.Equal(t, "useful context", comment.Body)
	require.Len(t, publisher.events, 1)
	event := publisher.events[0]
	assert.Equal(t, "comment-event", event.ID)
	assert.Equal(t, domain.EventCommentCreated, event.Type)
	assert.Equal(t, "project", event.ProjectID)
	assert.Equal(t, "board", event.BoardID)
	assert.Equal(t, "task", event.TaskID)
	assert.Equal(t, "actor", event.ActorID)
	assert.Equal(t, now, event.OccurredAt)
	payloadComment, ok := event.Payload["comment"].(domain.Comment)
	require.True(t, ok)
	assert.Equal(t, comment, payloadComment)
}

func TestTransitionTaskPropagatesRepositoryConflictWithoutPublishing(t *testing.T) {
	repo, service, publisher := transitionFixture()
	repo.statusUpdateErr = domain.ErrConflict

	_, err := service.TransitionTask(context.Background(), domain.User{ID: "actor"}, "task", "done")

	assert.ErrorIs(t, err, domain.ErrConflict)
	assert.Equal(t, 0, repo.statusUpdates)
	assert.Empty(t, publisher.events)
}

func eventFixtureRepository() *fakeRepository {
	repo := newFakeRepository()
	repo.projects["project"] = domain.Project{ID: "project", OwnerID: "actor"}
	repo.members[memberKey("project", "actor")] = domain.Member{ProjectID: "project", UserID: "actor", Role: domain.RoleMember}
	repo.boards["board"] = domain.Board{ID: "board", ProjectID: "project"}
	repo.tasks["task"] = domain.Task{ID: "task", BoardID: "board", StatusID: "todo", Title: "Original", AuthorID: "actor"}
	return repo
}

func transitionFixture() (*fakeRepository, *Service, *recordingPublisher) {
	repo := newFakeRepository()
	repo.projects["project"] = domain.Project{ID: "project", OwnerID: "actor"}
	repo.members[memberKey("project", "actor")] = domain.Member{ProjectID: "project", UserID: "actor", Role: domain.RoleAdmin}
	repo.boards["board"] = domain.Board{ID: "board", ProjectID: "project"}
	repo.statuses["todo"] = domain.Status{ID: "todo", BoardID: "board"}
	repo.statuses["done"] = domain.Status{ID: "done", BoardID: "board"}
	assignee := "actor"
	repo.tasks["task"] = domain.Task{ID: "task", BoardID: "board", StatusID: "todo", AuthorID: "actor", AssigneeID: &assignee, UpdatedAt: time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)}
	rule := domain.TransitionRule{ID: "rule", BoardID: "board", FromStatusID: "todo", ToStatusID: "done", Conditions: domain.RuleConditions{AuthorOnly: true, AssigneeOnly: true, RequiresComment: true, ProjectOwnerOnly: true, AllowedRoles: []domain.ProjectRole{domain.RoleAdmin}}}
	repo.rules[ruleKey("board", "todo", "done")] = rule
	repo.commentCounts["task"] = 1
	publisher := &recordingPublisher{}
	ids := 0
	service := NewService(repo, publisher, WithIDGenerator(func() string { ids++; return "event-id" }), WithClock(func() time.Time { return time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC) }))
	return repo, service, publisher
}
