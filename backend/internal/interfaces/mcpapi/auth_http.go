package mcpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/example/taskflow/backend/internal/application"
	"github.com/example/taskflow/backend/internal/domain"
	"github.com/example/taskflow/backend/internal/infrastructure/auth"
)

const DefaultOAuthScope = "taskflow:mcp"

type actorContextKey struct{}

// WithActor attaches the independently authenticated user to one MCP request.
// The value belongs to the request context and is never retained by the MCP
// server, which keeps stateless transports safe for concurrent users.
func WithActor(ctx context.Context, actor domain.User) context.Context {
	return context.WithValue(ctx, actorContextKey{}, actor)
}

// ActorFromContext returns the user authenticated for the current MCP request.
func ActorFromContext(ctx context.Context) (domain.User, bool) {
	actor, ok := ctx.Value(actorContextKey{}).(domain.User)
	return actor, ok && actor.ID != ""
}

type HTTPAuthOptions struct {
	PublicURL           string
	AuthorizationServer string
	AllowedOrigins      []string
	Scope               string
}

type ProtectedResourceMetadata struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	ScopesSupported        []string `json:"scopes_supported,omitempty"`
	BearerMethodsSupported []string `json:"bearer_methods_supported"`
	ResourceName           string   `json:"resource_name,omitempty"`
}

// HTTPAuth owns the HTTP-only portion of MCP authorization. JWT verification
// remains behind the TokenValidator port, so main can use a verifier configured
// with the MCP audience instead of the web application's audience.
type HTTPAuth struct {
	validator   application.TokenValidator
	metadata    ProtectedResourceMetadata
	metadataURL string
	origins     map[string]struct{}
	scope       string
}

func NewHTTPAuth(validator application.TokenValidator, options HTTPAuthOptions) (*HTTPAuth, error) {
	if validator == nil {
		return nil, errors.New("MCP token validator is required")
	}
	resource, metadataURL, err := validateResourceURL(options.PublicURL)
	if err != nil {
		return nil, err
	}
	authorizationServer, err := validateAuthorizationServer(options.AuthorizationServer)
	if err != nil {
		return nil, err
	}
	scope := strings.TrimSpace(options.Scope)
	if scope == "" {
		scope = DefaultOAuthScope
	}
	origins, err := validateOrigins(options.AllowedOrigins)
	if err != nil {
		return nil, err
	}
	return &HTTPAuth{
		validator: validator,
		metadata: ProtectedResourceMetadata{
			Resource:               resource,
			AuthorizationServers:   []string{authorizationServer},
			ScopesSupported:        []string{scope},
			BearerMethodsSupported: []string{"header"},
			ResourceName:           "TaskFlow MCP",
		},
		metadataURL: metadataURL,
		origins:     origins,
		scope:       scope,
	}, nil
}

func (a *HTTPAuth) MetadataURL() string { return a.metadataURL }

// MetadataHandler serves RFC 9728 protected resource metadata. It is public
// discovery data and therefore deliberately does not require a bearer token.
func (a *HTTPAuth) MetadataHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeOAuthError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(a.metadata)
	})
}

// Protect authenticates each stateless Streamable HTTP request, enforces the
// browser origin allowlist, and places the resulting domain.User in context.
// The wrapped SDK handler receives only POST requests.
func (a *HTTPAuth) Protect(next http.Handler) http.Handler {
	if next == nil {
		next = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "MCP handler is not configured", http.StatusInternalServerError)
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.allowOrigin(w, r) {
			writeOAuthError(w, http.StatusForbidden, "origin_not_allowed")
			return
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST, OPTIONS")
			writeOAuthError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}

		rawToken, err := auth.BearerToken(r.Header.Get("Authorization"))
		if err != nil {
			a.writeUnauthorized(w, false)
			return
		}
		actor, err := a.validator.Verify(r.Context(), rawToken)
		if err != nil || actor.ID == "" {
			a.writeUnauthorized(w, true)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithActor(r.Context(), actor)))
	})
}

func (a *HTTPAuth) allowOrigin(w http.ResponseWriter, r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	w.Header().Add("Vary", "Origin")
	if _, ok := a.origins[origin]; !ok {
		return false
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, MCP-Protocol-Version, MCP-Session-Id, Last-Event-ID")
	w.Header().Set("Access-Control-Expose-Headers", "MCP-Session-Id")
	return true
}

func (a *HTTPAuth) writeUnauthorized(w http.ResponseWriter, invalidToken bool) {
	challenge := fmt.Sprintf(`Bearer realm="taskflow-mcp", resource_metadata="%s", scope="%s"`, a.metadataURL, a.scope)
	if invalidToken {
		challenge += `, error="invalid_token"`
	}
	w.Header().Set("WWW-Authenticate", challenge)
	writeOAuthError(w, http.StatusUnauthorized, "unauthorized")
}

func writeOAuthError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}

func validateResourceURL(raw string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "", errors.New("MCP public URL must be an absolute URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", errors.New("MCP public URL must not contain user info, a query, or a fragment")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "", errors.New("MCP public URL must use HTTP or HTTPS")
	}
	if parsed.Scheme == "http" {
		host := parsed.Hostname()
		ip := net.ParseIP(host)
		if !strings.EqualFold(host, "localhost") && (ip == nil || !ip.IsLoopback()) {
			return "", "", errors.New("MCP public URL must use HTTPS unless its host is localhost or a loopback address")
		}
	}
	if parsed.Path != "/mcp" {
		return "", "", errors.New("MCP public URL path must be /mcp")
	}
	parsed.RawPath = ""
	resource := parsed.String()
	parsed.Path = "/.well-known/oauth-protected-resource/mcp"
	return resource, parsed.String(), nil
}

func validateAuthorizationServer(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("MCP authorization server must be an absolute URL")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("MCP authorization server must use HTTP or HTTPS")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("MCP authorization server must not contain user info, a query, or a fragment")
	}
	if parsed.Scheme == "http" {
		host := parsed.Hostname()
		ip := net.ParseIP(host)
		if !strings.EqualFold(host, "localhost") && (ip == nil || !ip.IsLoopback()) {
			return "", errors.New("MCP authorization server must use HTTPS unless its host is localhost or a loopback address")
		}
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func validateOrigins(values []string) (map[string]struct{}, error) {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		origin := strings.TrimSpace(value)
		if origin == "" {
			continue
		}
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, fmt.Errorf("invalid MCP allowed origin %q", origin)
		}
		parsed.Scheme = strings.ToLower(parsed.Scheme)
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return nil, fmt.Errorf("invalid MCP allowed origin %q", origin)
		}
		result[origin] = struct{}{}
	}
	return result, nil
}
