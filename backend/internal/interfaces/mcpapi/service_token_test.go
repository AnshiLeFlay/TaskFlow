package mcpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/taskflow/backend/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testServiceToken = "0123456789abcdef0123456789abcdef" // 32 chars, the minimum

// rejectingValidator stands in for Keycloak and refuses everything, so a
// request that succeeds can only have succeeded through the service token.
type rejectingValidator struct{ calls int }

func (v *rejectingValidator) Verify(context.Context, string) (domain.User, error) {
	v.calls++
	return domain.User{}, assertErr
}

var assertErr = errRejected{}

type errRejected struct{}

func (errRejected) Error() string { return "rejected" }

func serviceAuth(t *testing.T, token string) (*HTTPAuth, *rejectingValidator) {
	t.Helper()
	validator := &rejectingValidator{}
	auth, err := NewHTTPAuth(validator, HTTPAuthOptions{
		PublicURL:           "http://localhost:8080/mcp",
		AuthorizationServer: "http://localhost:8082/realms/taskflow",
		ServiceToken:        token,
		ServiceActor: domain.User{
			ID:       "athena-service",
			Username: "athena",
			Roles:    []string{domain.RealmRoleSuperadmin},
		},
	})
	require.NoError(t, err)
	return auth, validator
}

func post(auth *HTTPAuth, header string) (*httptest.ResponseRecorder, *domain.User) {
	var seen *domain.User
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if actor, ok := ActorFromContext(r.Context()); ok {
			seen = &actor
		}
	})
	req := httptest.NewRequest(http.MethodPost, "http://localhost:8080/mcp", strings.NewReader("{}"))
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	rec := httptest.NewRecorder()
	auth.Protect(next).ServeHTTP(rec, req)
	return rec, seen
}

func TestServiceTokenAuthenticatesConfiguredActor(t *testing.T) {
	auth, validator := serviceAuth(t, testServiceToken)

	rec, actor := post(auth, "Bearer "+testServiceToken)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, actor)
	assert.Equal(t, "athena-service", actor.ID)
	assert.True(t, actor.IsSuperadmin())
	assert.Zero(t, validator.calls, "a matching service token must not reach the JWT verifier")
}

// A wrong token must fall through to JWT verification rather than being
// treated as a near-miss, and must not authenticate on its own.
func TestWrongServiceTokenFallsThroughToJWT(t *testing.T) {
	auth, validator := serviceAuth(t, testServiceToken)

	rec, actor := post(auth, "Bearer "+strings.Repeat("f", len(testServiceToken)))

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Nil(t, actor)
	assert.Equal(t, 1, validator.calls)
}

func TestServiceTokenDisabledWhenUnset(t *testing.T) {
	auth, validator := serviceAuth(t, "")

	rec, actor := post(auth, "Bearer "+testServiceToken)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Nil(t, actor)
	assert.Equal(t, 1, validator.calls, "with no service token every bearer goes to the verifier")
}

func TestMissingAuthorizationHeaderIsRejected(t *testing.T) {
	auth, _ := serviceAuth(t, testServiceToken)

	rec, actor := post(auth, "")

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Nil(t, actor)
	assert.Contains(t, rec.Header().Get("WWW-Authenticate"), "resource_metadata=")
}

// A short token is the one failure mode a static credential cannot survive,
// so it is refused at construction rather than at request time.
func TestShortServiceTokenIsRefused(t *testing.T) {
	_, err := NewHTTPAuth(&rejectingValidator{}, HTTPAuthOptions{
		PublicURL:           "http://localhost:8080/mcp",
		AuthorizationServer: "http://localhost:8082/realms/taskflow",
		ServiceToken:        "too-short",
		ServiceActor:        domain.User{ID: "athena-service"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least")
}

func TestServiceTokenWithoutSubjectIsRefused(t *testing.T) {
	_, err := NewHTTPAuth(&rejectingValidator{}, HTTPAuthOptions{
		PublicURL:           "http://localhost:8080/mcp",
		AuthorizationServer: "http://localhost:8082/realms/taskflow",
		ServiceToken:        testServiceToken,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subject")
}
