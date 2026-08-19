package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/example/taskflow/backend/internal/domain"
	"github.com/google/uuid"
)

type Service struct {
	repo      domain.Repository
	publisher domain.EventPublisher
	directory UserDirectory
	now       func() time.Time
	newID     func() string
}

const (
	maxProjectOrBoardNameLength = 160
	maxProjectOrBoardDescLength = 4000
	maxStatusNameLength         = 100
	maxTaskTitleLength          = 300
	maxTaskDescriptionLength    = 20000
	maxCommentLength            = 10000
)

type Option func(*Service)

func WithClock(now func() time.Time) Option   { return func(s *Service) { s.now = now } }
func WithIDGenerator(fn func() string) Option { return func(s *Service) { s.newID = fn } }
func WithUserDirectory(directory UserDirectory) Option {
	return func(s *Service) { s.directory = directory }
}

func NewService(repo domain.Repository, publisher domain.EventPublisher, opts ...Option) *Service {
	if publisher == nil {
		publisher = domain.NopPublisher{}
	}
	s := &Service{repo: repo, publisher: publisher, now: func() time.Time { return time.Now().UTC() }, newID: func() string { return uuid.NewString() }}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type CreateProjectCommand struct {
	Name        string
	Description string
}

func (s *Service) CreateProject(ctx context.Context, actor domain.User, cmd CreateProjectCommand) (domain.Project, error) {
	name := strings.TrimSpace(cmd.Name)
	if name == "" {
		return domain.Project{}, &domain.ValidationError{Field: "name", Message: "is required"}
	}
	if err := validateMaxLength("name", name, maxProjectOrBoardNameLength); err != nil {
		return domain.Project{}, err
	}
	description := strings.TrimSpace(cmd.Description)
	if err := validateMaxLength("description", description, maxProjectOrBoardDescLength); err != nil {
		return domain.Project{}, err
	}
	now := s.now()
	p := domain.Project{ID: s.newID(), Name: name, Description: description, OwnerID: actor.ID, Role: domain.RoleAdmin, CreatedAt: now, UpdatedAt: now}
	m := domain.Member{ProjectID: p.ID, UserID: actor.ID, Role: domain.RoleAdmin, CreatedAt: now}
	if err := s.repo.CreateProject(ctx, &p, m); err != nil {
		return domain.Project{}, err
	}
	return p, nil
}

func (s *Service) ListProjects(ctx context.Context, actor domain.User) ([]domain.Project, error) {
	if actor.IsSuperadmin() {
		return s.repo.ListAllProjects(ctx)
	}
	return s.repo.ListProjects(ctx, actor.ID)
}

type AddMemberCommand struct {
	UserID string
	Role   domain.ProjectRole
}

func (s *Service) AddMember(ctx context.Context, actor domain.User, projectID string, cmd AddMemberCommand) (domain.Member, error) {
	if _, err := s.requireRole(ctx, projectID, actor, func(r domain.ProjectRole) bool { return r.CanManageMembers() }); err != nil {
		return domain.Member{}, err
	}
	if strings.TrimSpace(cmd.UserID) == "" {
		return domain.Member{}, &domain.ValidationError{Field: "user_id", Message: "is required"}
	}
	if !cmd.Role.Valid() {
		return domain.Member{}, &domain.ValidationError{Field: "role", Message: "must be admin, member, or viewer"}
	}
	userID := strings.TrimSpace(cmd.UserID)
	if s.directory != nil {
		if _, err := s.directory.GetUser(ctx, userID); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return domain.Member{}, &domain.ValidationError{Field: "user_id", Message: "user does not exist"}
			}
			return domain.Member{}, err
		}
	}
	p, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return domain.Member{}, err
	}
	if userID == p.OwnerID {
		return domain.Member{}, fmt.Errorf("%w: project owner role cannot be changed", domain.ErrConflict)
	}
	m := domain.Member{ProjectID: projectID, UserID: userID, Role: cmd.Role, CreatedAt: s.now()}
	if err := s.repo.UpsertMember(ctx, m); err != nil {
		return domain.Member{}, err
	}
	return m, nil
}

// ListUsers returns the enabled identities available for project membership.
func (s *Service) ListUsers(ctx context.Context) ([]domain.User, error) {
	if s.directory == nil {
		return nil, errors.New("user directory is not configured")
	}
	return s.directory.ListUsers(ctx)
}

// ListMembers returns a project's members ordered by role then user ID. Any
// project member, regardless of role, may list membership.
func (s *Service) ListMembers(ctx context.Context, actor domain.User, projectID string) ([]domain.Member, error) {
	if _, err := s.repo.GetProject(ctx, projectID); err != nil {
		return nil, err
	}
	if _, err := s.requireMember(ctx, projectID, actor); err != nil {
		return nil, err
	}
	return s.repo.ListMembers(ctx, projectID)
}

type StatusInput struct {
	Name     string
	Position int
}

type CreateBoardCommand struct {
	Name        string
	Description string
	Statuses    []StatusInput
}

func (s *Service) CreateBoard(ctx context.Context, actor domain.User, projectID string, cmd CreateBoardCommand) (domain.BoardAggregate, error) {
	if _, err := s.requireRole(ctx, projectID, actor, func(r domain.ProjectRole) bool { return r.CanManageWorkflow() }); err != nil {
		return domain.BoardAggregate{}, err
	}
	name := strings.TrimSpace(cmd.Name)
	if name == "" {
		return domain.BoardAggregate{}, &domain.ValidationError{Field: "name", Message: "is required"}
	}
	if err := validateMaxLength("name", name, maxProjectOrBoardNameLength); err != nil {
		return domain.BoardAggregate{}, err
	}
	description := strings.TrimSpace(cmd.Description)
	if err := validateMaxLength("description", description, maxProjectOrBoardDescLength); err != nil {
		return domain.BoardAggregate{}, err
	}
	now := s.now()
	b := domain.Board{ID: s.newID(), ProjectID: projectID, Name: name, Description: description, CreatedAt: now, UpdatedAt: now}
	inputs := cmd.Statuses
	// A board with statuses but no transition rules cannot move a single task,
	// because every transition needs a rule for its exact status pair. The
	// default preset therefore ships both; a caller that names its own
	// statuses is designing a workflow and wires the rules itself.
	preset := len(inputs) == 0
	if preset {
		inputs = defaultStatuses()
	}
	statuses := make([]domain.Status, 0, len(inputs))
	seenPositions := make(map[int]struct{}, len(inputs))
	for i, in := range inputs {
		statusName := strings.TrimSpace(in.Name)
		if statusName == "" {
			return domain.BoardAggregate{}, &domain.ValidationError{Field: fmt.Sprintf("statuses[%d].name", i), Message: "is required"}
		}
		if err := validateMaxLength(fmt.Sprintf("statuses[%d].name", i), statusName, maxStatusNameLength); err != nil {
			return domain.BoardAggregate{}, err
		}
		position := in.Position
		if position < 0 {
			return domain.BoardAggregate{}, &domain.ValidationError{Field: fmt.Sprintf("statuses[%d].position", i), Message: "must be non-negative"}
		}
		if _, exists := seenPositions[position]; exists {
			return domain.BoardAggregate{}, &domain.ValidationError{Field: "statuses", Message: "positions must be unique"}
		}
		seenPositions[position] = struct{}{}
		statuses = append(statuses, domain.Status{ID: s.newID(), BoardID: b.ID, Name: statusName, Position: position, CreatedAt: now, UpdatedAt: now})
	}
	rules := make([]domain.TransitionRule, 0)
	if preset {
		rules = s.presetRules(statuses, now)
	}
	if err := s.repo.CreateBoard(ctx, &b, statuses, rules); err != nil {
		return domain.BoardAggregate{}, err
	}
	return domain.BoardAggregate{Board: b, Statuses: statuses, Tasks: []domain.Task{}, Rules: rules}, nil
}

// defaultStatuses is the preset column set. Backlog and Canceled are not
// decoration: work arriving automatically has to land somewhere before anyone
// commits to it, and work that turns out to be wrong needs an ending that is
// not Done, or Done stops meaning anything. Separating Backlog from To Do also
// records the moment work was accepted, which is what tells waiting time apart
// from working time.
func defaultStatuses() []StatusInput {
	return []StatusInput{
		{Name: "Backlog", Position: 0},
		{Name: "To Do", Position: 1},
		{Name: "In Progress", Position: 2},
		{Name: "Done", Position: 3},
		{Name: "Canceled", Position: 4},
	}
}

// presetTransitions is the default workflow written as status names rather
// than derived from column order, because the shape is not a straight line:
// Canceled is a terminal side state reachable from any working column, not the
// column that follows Done.
//
// Two absences are deliberate. There is no Done to Canceled: finished work
// cannot be un-finished, and deciding it was unnecessary is a new task rather
// than an undo. And Canceled returns only to Backlog, never straight into the
// working columns, so anything revived re-enters through the same intake and
// its timings stay comparable with everything else.
var presetTransitions = [][2]string{
	{"Backlog", "To Do"},
	{"To Do", "In Progress"},
	{"In Progress", "Done"},

	{"To Do", "Backlog"},
	{"In Progress", "To Do"},
	{"Done", "In Progress"},

	{"Backlog", "Canceled"},
	{"To Do", "Canceled"},
	{"In Progress", "Canceled"},

	{"Canceled", "Backlog"},
}

// presetRules resolves presetTransitions against the statuses just created. No
// conditions are attached: the preset stays permissive and a project tightens
// it afterwards.
func (s *Service) presetRules(statuses []domain.Status, now time.Time) []domain.TransitionRule {
	byName := make(map[string]domain.Status, len(statuses))
	for _, status := range statuses {
		byName[status.Name] = status
	}
	rules := make([]domain.TransitionRule, 0, len(presetTransitions))
	for _, transition := range presetTransitions {
		from, fromExists := byName[transition[0]]
		to, toExists := byName[transition[1]]
		if !fromExists || !toExists {
			continue
		}
		rules = append(rules, domain.TransitionRule{
			ID: s.newID(), BoardID: from.BoardID,
			FromStatusID: from.ID, ToStatusID: to.ID,
			CreatedAt: now, UpdatedAt: now,
		})
	}
	return rules
}

func (s *Service) ListBoards(ctx context.Context, actor domain.User, projectID string) ([]domain.Board, error) {
	if _, err := s.requireMember(ctx, projectID, actor); err != nil {
		return nil, err
	}
	return s.repo.ListBoards(ctx, projectID)
}

func (s *Service) GetBoard(ctx context.Context, actor domain.User, boardID string) (domain.BoardAggregate, error) {
	b, _, err := s.authorizeBoard(ctx, actor, boardID, nil)
	if err != nil {
		return domain.BoardAggregate{}, err
	}
	return s.repo.GetBoardAggregate(ctx, b.ID)
}

type CreateStatusCommand struct {
	Name     string
	Position int
}

func (s *Service) CreateStatus(ctx context.Context, actor domain.User, boardID string, cmd CreateStatusCommand) (domain.Status, error) {
	b, _, err := s.authorizeBoard(ctx, actor, boardID, func(r domain.ProjectRole) bool { return r.CanManageWorkflow() })
	if err != nil {
		return domain.Status{}, err
	}
	name := strings.TrimSpace(cmd.Name)
	if name == "" {
		return domain.Status{}, &domain.ValidationError{Field: "name", Message: "is required"}
	}
	if err := validateMaxLength("name", name, maxStatusNameLength); err != nil {
		return domain.Status{}, err
	}
	if cmd.Position < 0 {
		return domain.Status{}, &domain.ValidationError{Field: "position", Message: "must be non-negative"}
	}
	if err := s.ensurePositionAvailable(ctx, b.ID, cmd.Position, ""); err != nil {
		return domain.Status{}, err
	}
	now := s.now()
	status := domain.Status{ID: s.newID(), BoardID: b.ID, Name: name, Position: cmd.Position, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.CreateStatus(ctx, &status); err != nil {
		return domain.Status{}, err
	}
	return status, nil
}

type UpdateStatusCommand struct {
	Name     *string
	Position *int
}

func (s *Service) UpdateStatus(ctx context.Context, actor domain.User, statusID string, cmd UpdateStatusCommand) (domain.Status, error) {
	if cmd.Name == nil && cmd.Position == nil {
		return domain.Status{}, &domain.ValidationError{Message: "at least one status field is required"}
	}
	status, err := s.repo.GetStatus(ctx, statusID)
	if err != nil {
		return domain.Status{}, err
	}
	if _, _, err := s.authorizeBoard(ctx, actor, status.BoardID, func(r domain.ProjectRole) bool { return r.CanManageWorkflow() }); err != nil {
		return domain.Status{}, err
	}
	if cmd.Name != nil {
		status.Name = strings.TrimSpace(*cmd.Name)
		if status.Name == "" {
			return domain.Status{}, &domain.ValidationError{Field: "name", Message: "cannot be empty"}
		}
		if err := validateMaxLength("name", status.Name, maxStatusNameLength); err != nil {
			return domain.Status{}, err
		}
	}
	if cmd.Position != nil {
		if *cmd.Position < 0 {
			return domain.Status{}, &domain.ValidationError{Field: "position", Message: "must be non-negative"}
		}
		if err := s.ensurePositionAvailable(ctx, status.BoardID, *cmd.Position, status.ID); err != nil {
			return domain.Status{}, err
		}
		status.Position = *cmd.Position
	}
	status.UpdatedAt = s.now()
	if err := s.repo.UpdateStatus(ctx, status); err != nil {
		return domain.Status{}, err
	}
	return status, nil
}

func (s *Service) DeleteStatus(ctx context.Context, actor domain.User, statusID string) error {
	status, err := s.repo.GetStatus(ctx, statusID)
	if err != nil {
		return err
	}
	if _, _, err := s.authorizeBoard(ctx, actor, status.BoardID, func(r domain.ProjectRole) bool { return r.CanManageWorkflow() }); err != nil {
		return err
	}
	return s.repo.DeleteStatus(ctx, statusID)
}

type CreateRuleCommand struct {
	FromStatusID string
	ToStatusID   string
	Conditions   domain.RuleConditions
}

func (s *Service) CreateRule(ctx context.Context, actor domain.User, boardID string, cmd CreateRuleCommand) (domain.TransitionRule, error) {
	b, _, err := s.authorizeBoard(ctx, actor, boardID, func(r domain.ProjectRole) bool { return r.CanManageWorkflow() })
	if err != nil {
		return domain.TransitionRule{}, err
	}
	if err := s.validateRule(ctx, b.ID, cmd.FromStatusID, cmd.ToStatusID, cmd.Conditions); err != nil {
		return domain.TransitionRule{}, err
	}
	now := s.now()
	rule := domain.TransitionRule{ID: s.newID(), BoardID: b.ID, FromStatusID: cmd.FromStatusID, ToStatusID: cmd.ToStatusID, Conditions: cmd.Conditions, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.CreateRule(ctx, &rule); err != nil {
		return domain.TransitionRule{}, err
	}
	return rule, nil
}

type UpdateRuleCommand struct {
	FromStatusID *string
	ToStatusID   *string
	Conditions   *domain.RuleConditions
}

func (s *Service) UpdateRule(ctx context.Context, actor domain.User, ruleID string, cmd UpdateRuleCommand) (domain.TransitionRule, error) {
	if cmd.FromStatusID == nil && cmd.ToStatusID == nil && cmd.Conditions == nil {
		return domain.TransitionRule{}, &domain.ValidationError{Message: "at least one rule field is required"}
	}
	rule, err := s.repo.GetRule(ctx, ruleID)
	if err != nil {
		return domain.TransitionRule{}, err
	}
	if _, _, err := s.authorizeBoard(ctx, actor, rule.BoardID, func(r domain.ProjectRole) bool { return r.CanManageWorkflow() }); err != nil {
		return domain.TransitionRule{}, err
	}
	if cmd.FromStatusID != nil {
		rule.FromStatusID = *cmd.FromStatusID
	}
	if cmd.ToStatusID != nil {
		rule.ToStatusID = *cmd.ToStatusID
	}
	if cmd.Conditions != nil {
		rule.Conditions = *cmd.Conditions
	}
	if err := s.validateRule(ctx, rule.BoardID, rule.FromStatusID, rule.ToStatusID, rule.Conditions); err != nil {
		return domain.TransitionRule{}, err
	}
	rule.UpdatedAt = s.now()
	if err := s.repo.UpdateRule(ctx, rule); err != nil {
		return domain.TransitionRule{}, err
	}
	return rule, nil
}

func (s *Service) DeleteRule(ctx context.Context, actor domain.User, ruleID string) error {
	rule, err := s.repo.GetRule(ctx, ruleID)
	if err != nil {
		return err
	}
	if _, _, err := s.authorizeBoard(ctx, actor, rule.BoardID, func(r domain.ProjectRole) bool { return r.CanManageWorkflow() }); err != nil {
		return err
	}
	return s.repo.DeleteRule(ctx, ruleID)
}

func (s *Service) ListTasks(ctx context.Context, actor domain.User, boardID string) ([]domain.Task, error) {
	if _, _, err := s.authorizeBoard(ctx, actor, boardID, nil); err != nil {
		return nil, err
	}
	return s.repo.ListTasks(ctx, boardID)
}

type CreateTaskCommand struct {
	Title       string
	Description string
	StatusID    string
	AssigneeID  *string
	Deadline    *time.Time
}

func (s *Service) CreateTask(ctx context.Context, actor domain.User, boardID string, cmd CreateTaskCommand) (domain.Task, error) {
	b, _, err := s.authorizeBoard(ctx, actor, boardID, func(r domain.ProjectRole) bool { return r.CanManageTasks() })
	if err != nil {
		return domain.Task{}, err
	}
	title := strings.TrimSpace(cmd.Title)
	if title == "" {
		return domain.Task{}, &domain.ValidationError{Field: "title", Message: "is required"}
	}
	if err := validateMaxLength("title", title, maxTaskTitleLength); err != nil {
		return domain.Task{}, err
	}
	description := strings.TrimSpace(cmd.Description)
	if err := validateMaxLength("description", description, maxTaskDescriptionLength); err != nil {
		return domain.Task{}, err
	}
	statusID := cmd.StatusID
	if statusID == "" {
		agg, err := s.repo.GetBoardAggregate(ctx, boardID)
		if err != nil {
			return domain.Task{}, err
		}
		if len(agg.Statuses) == 0 {
			return domain.Task{}, fmt.Errorf("%w: board has no statuses", domain.ErrConflict)
		}
		statusID = agg.Statuses[0].ID
	}
	status, err := s.repo.GetStatus(ctx, statusID)
	if err != nil {
		return domain.Task{}, err
	}
	if status.BoardID != b.ID {
		return domain.Task{}, &domain.ValidationError{Field: "status_id", Message: "status belongs to another board"}
	}
	assignee, err := s.normalizedAssignee(ctx, b.ProjectID, cmd.AssigneeID)
	if err != nil {
		return domain.Task{}, err
	}
	now := s.now()
	task := domain.Task{ID: s.newID(), BoardID: b.ID, StatusID: statusID, Title: title, Description: description, AuthorID: actor.ID, AssigneeID: assignee, Deadline: cmd.Deadline, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.CreateTask(ctx, &task); err != nil {
		return domain.Task{}, err
	}
	return task, nil
}

type UpdateTaskCommand struct {
	Title       *string
	Description *string
	AssigneeID  *string
	SetAssignee bool
	Deadline    *time.Time
	SetDeadline bool
}

func (s *Service) UpdateTask(ctx context.Context, actor domain.User, taskID string, cmd UpdateTaskCommand) (domain.Task, error) {
	if cmd.Title == nil && cmd.Description == nil && !cmd.SetAssignee && !cmd.SetDeadline {
		return domain.Task{}, &domain.ValidationError{Message: "at least one task field is required"}
	}
	task, b, _, err := s.authorizeTask(ctx, actor, taskID, func(r domain.ProjectRole) bool { return r.CanManageTasks() })
	if err != nil {
		return domain.Task{}, err
	}
	if cmd.Title != nil {
		task.Title = strings.TrimSpace(*cmd.Title)
		if task.Title == "" {
			return domain.Task{}, &domain.ValidationError{Field: "title", Message: "cannot be empty"}
		}
		if err := validateMaxLength("title", task.Title, maxTaskTitleLength); err != nil {
			return domain.Task{}, err
		}
	}
	if cmd.Description != nil {
		task.Description = strings.TrimSpace(*cmd.Description)
		if err := validateMaxLength("description", task.Description, maxTaskDescriptionLength); err != nil {
			return domain.Task{}, err
		}
	}
	if cmd.SetAssignee {
		task.AssigneeID, err = s.normalizedAssignee(ctx, b.ProjectID, cmd.AssigneeID)
		if err != nil {
			return domain.Task{}, err
		}
	}
	if cmd.SetDeadline {
		task.Deadline = cmd.Deadline
	}
	task.UpdatedAt = s.now()
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return domain.Task{}, err
	}
	s.publisher.Publish(domain.TaskEvent{ID: s.newID(), Type: domain.EventTaskUpdated, ProjectID: b.ProjectID, BoardID: b.ID, TaskID: task.ID, ActorID: actor.ID, OccurredAt: s.now(), Payload: map[string]any{"task": task}})
	return task, nil
}

func (s *Service) CreateComment(ctx context.Context, actor domain.User, taskID, body string) (domain.Comment, error) {
	task, b, _, err := s.authorizeTask(ctx, actor, taskID, func(r domain.ProjectRole) bool { return r.CanManageTasks() })
	if err != nil {
		return domain.Comment{}, err
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return domain.Comment{}, &domain.ValidationError{Field: "body", Message: "is required"}
	}
	if err := validateMaxLength("body", body, maxCommentLength); err != nil {
		return domain.Comment{}, err
	}
	comment := domain.Comment{ID: s.newID(), TaskID: task.ID, AuthorID: actor.ID, Body: body, CreatedAt: s.now()}
	if err := s.repo.CreateComment(ctx, &comment); err != nil {
		return domain.Comment{}, err
	}
	s.publisher.Publish(domain.TaskEvent{ID: s.newID(), Type: domain.EventCommentCreated, ProjectID: b.ProjectID, BoardID: b.ID, TaskID: task.ID, ActorID: actor.ID, OccurredAt: s.now(), Payload: map[string]any{"comment": comment}})
	return comment, nil
}

func (s *Service) TransitionTask(ctx context.Context, actor domain.User, taskID, targetStatusID string) (domain.Task, error) {
	task, b, role, err := s.authorizeTask(ctx, actor, taskID, func(r domain.ProjectRole) bool { return r.CanManageTasks() })
	if err != nil {
		return domain.Task{}, err
	}
	target, err := s.repo.GetStatus(ctx, targetStatusID)
	if err != nil {
		return domain.Task{}, err
	}
	if target.BoardID != b.ID {
		return domain.Task{}, &domain.ValidationError{Field: "target_status_id", Message: "status belongs to another board"}
	}
	rule, err := s.repo.FindRule(ctx, b.ID, task.StatusID, targetStatusID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return domain.Task{}, err
	}
	var rulePtr *domain.TransitionRule
	if err == nil {
		rulePtr = &rule
	}
	commentCount, err := s.repo.CountComments(ctx, task.ID)
	if err != nil {
		return domain.Task{}, err
	}
	project, err := s.repo.GetProject(ctx, b.ProjectID)
	if err != nil {
		return domain.Task{}, err
	}
	if err := domain.ValidateTransition(task, rulePtr, targetStatusID, domain.TransitionContext{ActorID: actor.ID, ProjectOwner: project.OwnerID, ProjectRole: role, CommentCount: commentCount}); err != nil {
		return domain.Task{}, err
	}
	updated, err := s.repo.UpdateTaskStatus(ctx, task.ID, task.StatusID, task.UpdatedAt, targetStatusID)
	if err != nil {
		return domain.Task{}, err
	}
	s.publisher.Publish(domain.TaskEvent{ID: s.newID(), Type: domain.EventTaskTransitioned, ProjectID: b.ProjectID, BoardID: b.ID, TaskID: task.ID, ActorID: actor.ID, FromStatusID: task.StatusID, ToStatusID: targetStatusID, OccurredAt: s.now(), Payload: map[string]any{"task": updated}})
	return updated, nil
}

func (s *Service) AuthorizeProject(ctx context.Context, actor domain.User, projectID string) error {
	_, err := s.requireMember(ctx, projectID, actor)
	return err
}

// ProjectIDs lists the projects whose realtime events the actor may receive.
// It mirrors ListProjects so WebSocket and gRPC subscriptions never diverge
// from what the REST API would show.
func (s *Service) ProjectIDs(ctx context.Context, actor domain.User) ([]string, error) {
	projects, err := s.ListProjects(ctx, actor)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(projects))
	for _, project := range projects {
		ids = append(ids, project.ID)
	}
	return ids, nil
}

func (s *Service) Ready(ctx context.Context) error { return s.repo.Ping(ctx) }

// requireMember is the single gate every project-scoped operation passes
// through. It takes the whole actor rather than an ID because authorization
// has two sources: project_members, and the superadmin realm role carried by
// the access token.
func (s *Service) requireMember(ctx context.Context, projectID string, actor domain.User) (domain.ProjectRole, error) {
	if actor.IsSuperadmin() {
		// A superadmin has no project_members row, so the project's existence
		// is checked explicitly: an unknown ID must still be ErrNotFound
		// rather than silently authorized.
		if _, err := s.repo.GetProject(ctx, projectID); err != nil {
			return "", err
		}
		return domain.RoleAdmin, nil
	}
	m, err := s.repo.GetMembership(ctx, projectID, actor.ID)
	if errors.Is(err, domain.ErrNotFound) {
		return "", domain.ErrForbidden
	}
	if err != nil {
		return "", err
	}
	return m.Role, nil
}

func (s *Service) requireRole(ctx context.Context, projectID string, actor domain.User, allowed func(domain.ProjectRole) bool) (domain.ProjectRole, error) {
	role, err := s.requireMember(ctx, projectID, actor)
	if err != nil {
		return "", err
	}
	if !allowed(role) {
		return "", domain.ErrForbidden
	}
	return role, nil
}

func (s *Service) authorizeBoard(ctx context.Context, actor domain.User, boardID string, allowed func(domain.ProjectRole) bool) (domain.Board, domain.ProjectRole, error) {
	b, err := s.repo.GetBoard(ctx, boardID)
	if err != nil {
		return domain.Board{}, "", err
	}
	role, err := s.requireMember(ctx, b.ProjectID, actor)
	if err != nil {
		return domain.Board{}, "", err
	}
	if allowed != nil && !allowed(role) {
		return domain.Board{}, "", domain.ErrForbidden
	}
	return b, role, nil
}

func (s *Service) authorizeTask(ctx context.Context, actor domain.User, taskID string, allowed func(domain.ProjectRole) bool) (domain.Task, domain.Board, domain.ProjectRole, error) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return domain.Task{}, domain.Board{}, "", err
	}
	b, role, err := s.authorizeBoard(ctx, actor, task.BoardID, allowed)
	return task, b, role, err
}

func (s *Service) validateRule(ctx context.Context, boardID, fromID, toID string, conditions domain.RuleConditions) error {
	if fromID == "" || toID == "" {
		return &domain.ValidationError{Field: "status_id", Message: "from_status_id and to_status_id are required"}
	}
	if fromID == toID {
		return &domain.ValidationError{Field: "to_status_id", Message: "must differ from from_status_id"}
	}
	from, err := s.repo.GetStatus(ctx, fromID)
	if err != nil {
		return err
	}
	to, err := s.repo.GetStatus(ctx, toID)
	if err != nil {
		return err
	}
	if from.BoardID != boardID || to.BoardID != boardID {
		return &domain.ValidationError{Field: "status_id", Message: "both statuses must belong to the board"}
	}
	for _, role := range conditions.AllowedRoles {
		if !role.Valid() {
			return &domain.ValidationError{Field: "conditions.allowed_roles", Message: fmt.Sprintf("unknown role %q", role)}
		}
	}
	return nil
}

func (s *Service) ensurePositionAvailable(ctx context.Context, boardID string, position int, excludeStatusID string) error {
	agg, err := s.repo.GetBoardAggregate(ctx, boardID)
	if err != nil {
		return err
	}
	for _, status := range agg.Statuses {
		if status.ID == excludeStatusID {
			continue
		}
		if status.Position == position {
			return &domain.ValidationError{Field: "position", Message: "already used by another status on this board"}
		}
	}
	return nil
}

func (s *Service) normalizedAssignee(ctx context.Context, projectID string, candidate *string) (*string, error) {
	if candidate == nil || strings.TrimSpace(*candidate) == "" {
		return nil, nil
	}
	id := strings.TrimSpace(*candidate)
	// This asks whether the assignee is a project member, not whether the
	// caller may act, so it reads project_members directly: the superadmin
	// role must never make an outsider assignable.
	membership, err := s.repo.GetMembership(ctx, projectID, id)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, &domain.ValidationError{Field: "assignee_id", Message: "assignee is not a project member"}
	}
	if err != nil {
		return nil, err
	}
	if membership.Role == domain.RoleViewer {
		return nil, &domain.ValidationError{Field: "assignee_id", Message: "viewer cannot be assigned to tasks"}
	}
	return &id, nil
}

func validateMaxLength(field, value string, maximum int) error {
	if utf8.RuneCountInString(value) > maximum {
		return &domain.ValidationError{Field: field, Message: fmt.Sprintf("must be at most %d characters", maximum)}
	}
	return nil
}
