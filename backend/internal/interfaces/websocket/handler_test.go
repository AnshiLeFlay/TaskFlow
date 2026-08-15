package websocket

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/example/taskflow/backend/internal/application"
	"github.com/example/taskflow/backend/internal/domain"
	"github.com/example/taskflow/backend/internal/infrastructure/realtime"
	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type staticValidator struct{ user domain.User }

func (v staticValidator) Verify(context.Context, string) (domain.User, error) { return v.user, nil }

type authorizationCheck struct {
	projectID string
	allowed   bool
}

type membershipRepository struct {
	domain.Repository
	mu          sync.Mutex
	memberships map[string]domain.Member
	checks      chan authorizationCheck
}

func newMembershipRepository() *membershipRepository {
	return &membershipRepository{
		memberships: make(map[string]domain.Member),
		checks:      make(chan authorizationCheck, 16),
	}
}

func (r *membershipRepository) ListProjects(_ context.Context, userID string) ([]domain.Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	projects := make([]domain.Project, 0, len(r.memberships))
	for _, membership := range r.memberships {
		if membership.UserID == userID {
			projects = append(projects, domain.Project{ID: membership.ProjectID, Role: membership.Role})
		}
	}
	return projects, nil
}

func (r *membershipRepository) GetMembership(_ context.Context, projectID, userID string) (domain.Member, error) {
	r.mu.Lock()
	membership, allowed := r.memberships[membershipTestKey(projectID, userID)]
	r.mu.Unlock()
	r.checks <- authorizationCheck{projectID: projectID, allowed: allowed}
	if !allowed {
		return domain.Member{}, domain.ErrNotFound
	}
	return membership, nil
}

func (r *membershipRepository) setMembership(projectID, userID string, allowed bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := membershipTestKey(projectID, userID)
	if !allowed {
		delete(r.memberships, key)
		return
	}
	r.memberships[key] = domain.Member{ProjectID: projectID, UserID: userID, Role: domain.RoleMember}
}

func membershipTestKey(projectID, userID string) string { return projectID + "\x00" + userID }

func TestHandlerRejectsUnauthorizedScopedProjectBeforeUpgrade(t *testing.T) {
	repo := newMembershipRepository()
	server, broker := newWebSocketTestServer(t, repo)
	defer server.Close()
	defer broker.Close()

	connection, response, err := gorillawebsocket.DefaultDialer.Dial(webSocketURL(server.URL)+"?token=valid&project_id=private", nil)
	if connection != nil {
		defer connection.Close()
	}
	require.Error(t, err)
	require.NotNil(t, response)
	defer response.Body.Close()
	assert.Equal(t, http.StatusForbidden, response.StatusCode)
}

func TestGlobalHandlerReauthorizesEveryEventAsMembershipChanges(t *testing.T) {
	repo := newMembershipRepository()
	server, broker := newWebSocketTestServer(t, repo)
	defer server.Close()
	defer broker.Close()

	connection, _, err := gorillawebsocket.DefaultDialer.Dial(webSocketURL(server.URL)+"?token=valid", nil)
	require.NoError(t, err)
	defer connection.Close()
	require.NoError(t, connection.SetReadDeadline(time.Now().Add(5*time.Second)))

	var connected map[string]any
	require.NoError(t, connection.ReadJSON(&connected))
	assert.Equal(t, "connected", connected["type"])

	broker.Publish(domain.TaskEvent{ID: "denied-before-add", ProjectID: "project"})
	assert.Equal(t, authorizationCheck{projectID: "project", allowed: false}, receiveAuthorizationCheck(t, repo.checks))

	repo.setMembership("project", "user", true)
	broker.Publish(domain.TaskEvent{ID: "allowed-after-add", ProjectID: "project"})
	assert.Equal(t, authorizationCheck{projectID: "project", allowed: true}, receiveAuthorizationCheck(t, repo.checks))
	var allowed domain.TaskEvent
	require.NoError(t, connection.ReadJSON(&allowed))
	assert.Equal(t, "allowed-after-add", allowed.ID)

	repo.setMembership("project", "user", false)
	broker.Publish(domain.TaskEvent{ID: "denied-after-removal", ProjectID: "project"})
	assert.Equal(t, authorizationCheck{projectID: "project", allowed: false}, receiveAuthorizationCheck(t, repo.checks))

	repo.setMembership("project", "user", true)
	broker.Publish(domain.TaskEvent{ID: "allowed-after-readd", ProjectID: "project"})
	assert.Equal(t, authorizationCheck{projectID: "project", allowed: true}, receiveAuthorizationCheck(t, repo.checks))
	require.NoError(t, connection.ReadJSON(&allowed))
	assert.Equal(t, "allowed-after-readd", allowed.ID)

	broker.Close()
	_, _, err = connection.ReadMessage()
	require.Error(t, err)
}

func newWebSocketTestServer(t *testing.T, repo *membershipRepository) (*httptest.Server, *realtime.Broker) {
	t.Helper()
	broker := realtime.NewBroker()
	service := application.NewService(repo, broker)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewHandler(service, staticValidator{user: domain.User{ID: "user"}}, broker, nil, logger)
	return httptest.NewServer(handler), broker
}

func webSocketURL(httpURL string) string { return "ws" + strings.TrimPrefix(httpURL, "http") }

func receiveAuthorizationCheck(t *testing.T, checks <-chan authorizationCheck) authorizationCheck {
	t.Helper()
	select {
	case check := <-checks:
		return check
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for project authorization")
		return authorizationCheck{}
	}
}
