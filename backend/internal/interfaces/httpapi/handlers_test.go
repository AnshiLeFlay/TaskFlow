package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/example/taskflow/backend/internal/application"
	"github.com/example/taskflow/backend/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tokenUserValidator resolves a fixed set of bearer tokens to users, so tests
// can authenticate as different actors against the same router.
type tokenUserValidator map[string]domain.User

func (v tokenUserValidator) Verify(_ context.Context, token string) (domain.User, error) {
	user, ok := v[token]
	if !ok {
		return domain.User{}, errors.New("unknown token")
	}
	return user, nil
}

// endpointFakeRepository implements only the domain.Repository methods
// exercised by the tests in this file; every other call panics via the nil
// embedded interface, mirroring the pattern in websocket/handler_test.go.
type endpointFakeRepository struct {
	domain.Repository
	projects map[string]domain.Project
	members  map[string]domain.Member
	boards   map[string]domain.Board
	statuses map[string]domain.Status
	tasks    map[string]domain.Task
}

func newEndpointFakeRepository() *endpointFakeRepository {
	return &endpointFakeRepository{
		projects: map[string]domain.Project{},
		members:  map[string]domain.Member{},
		boards:   map[string]domain.Board{},
		statuses: map[string]domain.Status{},
		tasks:    map[string]domain.Task{},
	}
}

func endpointMemberKey(projectID, userID string) string { return projectID + ":" + userID }

func (f *endpointFakeRepository) CreateProject(_ context.Context, project *domain.Project, owner domain.Member) error {
	f.projects[project.ID] = *project
	f.members[endpointMemberKey(owner.ProjectID, owner.UserID)] = owner
	return nil
}

func (f *endpointFakeRepository) GetProject(_ context.Context, id string) (domain.Project, error) {
	p, ok := f.projects[id]
	if !ok {
		return p, domain.ErrNotFound
	}
	return p, nil
}

func (f *endpointFakeRepository) GetMembership(_ context.Context, projectID, userID string) (domain.Member, error) {
	m, ok := f.members[endpointMemberKey(projectID, userID)]
	if !ok {
		return m, domain.ErrNotFound
	}
	return m, nil
}

func (f *endpointFakeRepository) ListMembers(_ context.Context, projectID string) ([]domain.Member, error) {
	var out []domain.Member
	for _, m := range f.members {
		if m.ProjectID == projectID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *endpointFakeRepository) GetBoard(_ context.Context, id string) (domain.Board, error) {
	b, ok := f.boards[id]
	if !ok {
		return b, domain.ErrNotFound
	}
	return b, nil
}

func (f *endpointFakeRepository) GetStatus(_ context.Context, id string) (domain.Status, error) {
	s, ok := f.statuses[id]
	if !ok {
		return s, domain.ErrNotFound
	}
	return s, nil
}

func (f *endpointFakeRepository) GetTask(_ context.Context, id string) (domain.Task, error) {
	t, ok := f.tasks[id]
	if !ok {
		return t, domain.ErrNotFound
	}
	return t, nil
}

func (f *endpointFakeRepository) FindRule(context.Context, string, string, string) (domain.TransitionRule, error) {
	return domain.TransitionRule{}, domain.ErrNotFound
}

func (f *endpointFakeRepository) CountComments(context.Context, string) (int, error) { return 0, nil }

func testRouter(t *testing.T, repo domain.Repository, validator application.TokenValidator) http.Handler {
	t.Helper()
	service := application.NewService(repo, nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(service, validator, http.NotFoundHandler(), "", nil, logger)
}

func TestProtectedRouteWithoutTokenReturnsUnauthorizedEnvelope(t *testing.T) {
	router := testRouter(t, newEndpointFakeRepository(), tokenUserValidator{})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	var body errorEnvelope
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	assert.Equal(t, "unauthorized", body.Error.Code)
}

func TestCreateProjectReturnsCreatedWithProjectBody(t *testing.T) {
	repo := newEndpointFakeRepository()
	validator := tokenUserValidator{"actor-token": domain.User{ID: "actor-1"}}
	router := testRouter(t, repo, validator)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewBufferString(`{"name":"New Project"}`))
	request.Header.Set("Authorization", "Bearer actor-token")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusCreated, recorder.Code)
	var project domain.Project
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &project))
	assert.Equal(t, "New Project", project.Name)
	assert.Equal(t, "actor-1", project.OwnerID)
	assert.Equal(t, domain.RoleAdmin, project.Role)
}

func TestListMembersReturnsOKForMemberAndForbiddenForNonMember(t *testing.T) {
	repo := newEndpointFakeRepository()
	repo.projects["project"] = domain.Project{ID: "project", OwnerID: "admin"}
	repo.members[endpointMemberKey("project", "admin")] = domain.Member{ProjectID: "project", UserID: "admin", Role: domain.RoleAdmin}
	repo.members[endpointMemberKey("project", "viewer")] = domain.Member{ProjectID: "project", UserID: "viewer", Role: domain.RoleViewer}
	validator := tokenUserValidator{
		"member-token":   domain.User{ID: "viewer"},
		"outsider-token": domain.User{ID: "outsider"},
	}
	router := testRouter(t, repo, validator)

	memberRequest := httptest.NewRequest(http.MethodGet, "/api/v1/projects/project/members", nil)
	memberRequest.Header.Set("Authorization", "Bearer member-token")
	memberRecorder := httptest.NewRecorder()
	router.ServeHTTP(memberRecorder, memberRequest)
	require.Equal(t, http.StatusOK, memberRecorder.Code)
	var body struct {
		Members []MemberSummary `json:"members"`
	}
	require.NoError(t, json.Unmarshal(memberRecorder.Body.Bytes(), &body))
	assert.Len(t, body.Members, 2)

	nonMemberRequest := httptest.NewRequest(http.MethodGet, "/api/v1/projects/project/members", nil)
	nonMemberRequest.Header.Set("Authorization", "Bearer outsider-token")
	nonMemberRecorder := httptest.NewRecorder()
	router.ServeHTTP(nonMemberRecorder, nonMemberRequest)
	assert.Equal(t, http.StatusForbidden, nonMemberRecorder.Code)
	var forbiddenBody errorEnvelope
	require.NoError(t, json.Unmarshal(nonMemberRecorder.Body.Bytes(), &forbiddenBody))
	assert.Equal(t, "forbidden", forbiddenBody.Error.Code)
}

func TestTransitionTaskViolatingRuleReturnsUnprocessableEntity(t *testing.T) {
	repo := newEndpointFakeRepository()
	repo.projects["project"] = domain.Project{ID: "project", OwnerID: "actor"}
	repo.members[endpointMemberKey("project", "actor")] = domain.Member{ProjectID: "project", UserID: "actor", Role: domain.RoleAdmin}
	repo.boards["board"] = domain.Board{ID: "board", ProjectID: "project"}
	repo.statuses["todo"] = domain.Status{ID: "todo", BoardID: "board"}
	repo.statuses["done"] = domain.Status{ID: "done", BoardID: "board"}
	repo.tasks["task"] = domain.Task{ID: "task", BoardID: "board", StatusID: "todo", AuthorID: "actor"}
	validator := tokenUserValidator{"actor-token": domain.User{ID: "actor"}}
	router := testRouter(t, repo, validator)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task/transition", bytes.NewBufferString(`{"target_status_id":"done"}`))
	request.Header.Set("Authorization", "Bearer actor-token")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
	var body errorEnvelope
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	assert.Equal(t, "transition_not_allowed", body.Error.Code)
}
