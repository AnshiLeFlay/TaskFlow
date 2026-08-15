package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/example/taskflow/backend/internal/application"
	"github.com/example/taskflow/backend/internal/domain"
)

var ErrUnauthorized = errors.New("unauthorized")

// Compile-time assertion that KeycloakVerifier implements the
// application.TokenValidator port.
var _ application.TokenValidator = (*KeycloakVerifier)(nil)

type KeycloakVerifier struct {
	verifier *oidc.IDTokenVerifier
}

func NewKeycloakVerifier(ctx context.Context, issuerURL, jwksURL, clientID string, skipAudience bool) (*KeycloakVerifier, error) {
	issuerURL = strings.TrimRight(strings.TrimSpace(issuerURL), "/")
	jwksURL = strings.TrimSpace(jwksURL)
	if issuerURL == "" || jwksURL == "" {
		return nil, errors.New("Keycloak issuer and JWKS URLs are required")
	}
	keySet := oidc.NewRemoteKeySet(ctx, jwksURL)
	config := &oidc.Config{ClientID: clientID, SkipClientIDCheck: skipAudience || clientID == ""}
	return &KeycloakVerifier{verifier: oidc.NewVerifier(issuerURL, keySet, config)}, nil
}

func (v *KeycloakVerifier) Verify(ctx context.Context, rawToken string) (domain.User, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return domain.User{}, ErrUnauthorized
	}
	token, err := v.verifier.Verify(ctx, rawToken)
	if err != nil {
		return domain.User{}, fmt.Errorf("%w: invalid access token", ErrUnauthorized)
	}
	var claims struct {
		Subject           string `json:"sub"`
		PreferredUsername string `json:"preferred_username"`
		Email             string `json:"email"`
		Name              string `json:"name"`
		RealmAccess       struct {
			Roles []string `json:"roles"`
		} `json:"realm_access"`
	}
	if err := token.Claims(&claims); err != nil || claims.Subject == "" {
		return domain.User{}, fmt.Errorf("%w: malformed access token claims", ErrUnauthorized)
	}
	roles := append([]string(nil), claims.RealmAccess.Roles...)
	return domain.User{ID: claims.Subject, Username: claims.PreferredUsername, Email: claims.Email, Name: claims.Name, Roles: roles}, nil
}

func BearerToken(header string) (string, error) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", ErrUnauthorized
	}
	return parts[1], nil
}
