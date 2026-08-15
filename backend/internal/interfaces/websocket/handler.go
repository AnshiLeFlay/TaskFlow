package websocket

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/example/taskflow/backend/internal/application"
	"github.com/example/taskflow/backend/internal/domain"
	"github.com/example/taskflow/backend/internal/infrastructure/auth"
	"github.com/example/taskflow/backend/internal/infrastructure/realtime"
	gorillawebsocket "github.com/gorilla/websocket"
)

type Handler struct {
	service   *application.Service
	validator auth.TokenValidator
	broker    *realtime.Broker
	upgrader  gorillawebsocket.Upgrader
	logger    *slog.Logger
}

func NewHandler(service *application.Service, validator auth.TokenValidator, broker *realtime.Broker, allowedOrigins []string, logger *slog.Logger) *Handler {
	origins := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		origins[strings.TrimRight(origin, "/")] = struct{}{}
	}
	return &Handler{
		service: service, validator: validator, broker: broker, logger: logger,
		upgrader: gorillawebsocket.Upgrader{
			ReadBufferSize: 1024, WriteBufferSize: 4096,
			CheckOrigin: func(r *http.Request) bool {
				origin := strings.TrimRight(r.Header.Get("Origin"), "/")
				if origin == "" {
					return true
				}
				_, ok := origins[origin]
				return ok
			},
		},
	}
}

// ServeHTTP godoc
// @Summary Open an authenticated task-event WebSocket stream
// @Tags realtime
// @Param token query string true "Keycloak JWT access token"
// @Param project_id query string false "Optional project scope"
// @Success 101 {string} string "WebSocket protocol selected"
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Router /ws [get]
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	user, err := h.validator.Verify(r.Context(), token)
	if err != nil {
		writeWSError(w, http.StatusUnauthorized, "unauthorized", "valid token query parameter required")
		return
	}
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	var projectIDs []string
	if projectID != "" {
		if err := h.service.AuthorizeProject(r.Context(), user.ID, projectID); err != nil {
			if errors.Is(err, domain.ErrForbidden) || errors.Is(err, domain.ErrNotFound) {
				writeWSError(w, http.StatusForbidden, "forbidden", "project membership is required")
				return
			}
			h.logger.Error("authorize scoped WebSocket subscription", "error", err, "user_id", user.ID, "project_id", projectID)
			writeWSError(w, http.StatusInternalServerError, "internal_error", "could not authorize subscription")
			return
		}
		projectIDs = []string{projectID}
	} else {
		projectIDs, err = h.service.ProjectIDs(r.Context(), user.ID)
		if err != nil {
			h.logger.Error("list WebSocket projects", "error", err, "user_id", user.ID)
			writeWSError(w, http.StatusInternalServerError, "internal_error", "could not prepare subscription")
			return
		}
	}
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Debug("WebSocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()
	var events <-chan domain.TaskEvent
	var unsubscribe func()
	if projectID == "" {
		events, unsubscribe = h.broker.SubscribeAll()
	} else {
		events, unsubscribe = h.broker.Subscribe(projectIDs)
	}
	defer unsubscribe()

	conn.SetReadLimit(4096)
	_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(90 * time.Second)) })
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	if err := writeWSJSON(conn, map[string]any{"type": "connected", "project_ids": projectIDs}); err != nil {
		return
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-closed:
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if projectID != "" && event.ProjectID != projectID {
				continue
			}
			if err := h.service.AuthorizeProject(r.Context(), user.ID, event.ProjectID); err != nil {
				if errors.Is(err, domain.ErrForbidden) || errors.Is(err, domain.ErrNotFound) {
					if projectID != "" {
						return
					}
					continue
				}
				h.logger.Error("reauthorize WebSocket event", "error", err, "user_id", user.ID, "project_id", event.ProjectID)
				if projectID != "" {
					return
				}
				continue
			}
			if err := writeWSJSON(conn, event); err != nil {
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(gorillawebsocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func writeWSJSON(conn *gorillawebsocket.Conn, value any) error {
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return conn.WriteJSON(value)
}

func writeWSError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}
