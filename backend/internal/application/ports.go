package application

import (
	"context"

	"github.com/example/taskflow/backend/internal/domain"
)

// TokenValidator verifies a raw bearer token and returns the authenticated
// user it represents. It is declared here, in the application layer, so that
// interfaces packages (httpapi, websocket, grpcapi) depend only on
// application and domain, never on infrastructure, to reach the port.
type TokenValidator interface {
	Verify(context.Context, string) (domain.User, error)
}

// UserDirectory exposes the small, read-only part of the identity provider
// needed by TaskFlow. Project membership remains owned by TaskFlow itself.
type UserDirectory interface {
	ListUsers(context.Context) ([]domain.User, error)
	GetUser(context.Context, string) (domain.User, error)
}
