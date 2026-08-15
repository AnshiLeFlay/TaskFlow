package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/example/taskflow/backend/internal/domain"
)

// KeycloakDirectory reads users through Keycloak's Admin REST API using a
// least-privilege service account.
type KeycloakDirectory struct {
	baseURL, realm, clientID, clientSecret string
	httpClient                             *http.Client
	mu                                     sync.Mutex
	token                                  string
	expiresAt                              time.Time
}

func NewKeycloakDirectory(baseURL, realm, clientID, clientSecret string) *KeycloakDirectory {
	return &KeycloakDirectory{
		baseURL: strings.TrimRight(baseURL, "/"), realm: realm,
		clientID: clientID, clientSecret: clientSecret,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type keycloakUser struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Enabled   bool   `json:"enabled"`
}

func (d *KeycloakDirectory) ListUsers(ctx context.Context) ([]domain.User, error) {
	const pageSize = 100
	users := make([]domain.User, 0)
	for first := 0; ; first += pageSize {
		var page []keycloakUser
		path := fmt.Sprintf("/admin/realms/%s/users?first=%d&max=%d", url.PathEscape(d.realm), first, pageSize)
		if err := d.get(ctx, path, &page); err != nil {
			return nil, err
		}
		for _, user := range page {
			if user.Enabled {
				users = append(users, mapKeycloakUser(user))
			}
		}
		if len(page) < pageSize {
			break
		}
	}
	return users, nil
}

func (d *KeycloakDirectory) GetUser(ctx context.Context, id string) (domain.User, error) {
	var raw keycloakUser
	if err := d.get(ctx, "/admin/realms/"+url.PathEscape(d.realm)+"/users/"+url.PathEscape(id), &raw); err != nil {
		return domain.User{}, err
	}
	if !raw.Enabled {
		return domain.User{}, fmt.Errorf("Keycloak user is disabled")
	}
	return mapKeycloakUser(raw), nil
}

func mapKeycloakUser(user keycloakUser) domain.User {
	return domain.User{ID: user.ID, Username: user.Username, Email: user.Email, Name: strings.TrimSpace(user.FirstName + " " + user.LastName), Roles: []string{}}
}

func (d *KeycloakDirectory) get(ctx context.Context, path string, target any) error {
	token, err := d.accessToken(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Keycloak directory request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return domain.ErrNotFound
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Keycloak directory returned %s", response.Status)
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return fmt.Errorf("decode Keycloak directory response: %w", err)
	}
	return nil
}

func (d *KeycloakDirectory) accessToken(ctx context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.token != "" && time.Now().Before(d.expiresAt) {
		return d.token, nil
	}
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {d.clientID}, "client_secret": {d.clientSecret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+"/realms/"+url.PathEscape(d.realm)+"/protocol/openid-connect/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := d.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request Keycloak service token: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Keycloak token endpoint returned %s", response.Status)
	}
	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("Keycloak returned an empty access token")
	}
	d.token = result.AccessToken
	lifetime := time.Duration(result.ExpiresIn) * time.Second
	if lifetime > 30*time.Second {
		lifetime -= 30 * time.Second
	}
	d.expiresAt = time.Now().Add(lifetime)
	return d.token, nil
}
