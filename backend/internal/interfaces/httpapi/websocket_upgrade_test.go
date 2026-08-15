package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/taskflow/backend/internal/application"
	"github.com/example/taskflow/backend/internal/domain"
	"github.com/example/taskflow/backend/internal/infrastructure/realtime"
	wsapi "github.com/example/taskflow/backend/internal/interfaces/websocket"
	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// TestWebSocketUpgradeSucceedsThroughFullRouterMiddlewareStack is a regression
// test for a bug where accessLog's statusRecorder wrapper did not implement
// http.Hijacker: gorilla/websocket upgrades via a direct w.(http.Hijacker)
// type assertion, so every /ws connection routed through the real router
// (recoverPanic -> accessLog -> mux) returned 500 instead of 101, even though
// calling the WebSocket handler directly (bypassing the router) still
// "passed". This dials a real client against a real router-backed
// httptest.Server, the way the bug actually manifested in production.
func TestWebSocketUpgradeSucceedsThroughFullRouterMiddlewareStack(t *testing.T) {
	repo := newEndpointFakeRepository()
	broker := realtime.NewBroker()
	defer broker.Close()
	service := application.NewService(repo, broker)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	validator := tokenUserValidator{"valid-token": domain.User{ID: "user-1"}}
	wsHandler := wsapi.NewHandler(service, validator, broker, nil, logger)

	router := NewRouter(service, validator, wsHandler, "", nil, logger)
	server := httptest.NewServer(router)
	defer server.Close()

	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=valid-token"
	conn, resp, err := gorillawebsocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err, "WebSocket upgrade through the full router must succeed")
	if resp != nil {
		defer resp.Body.Close()
		require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	}
	defer conn.Close()

	var connected map[string]any
	require.NoError(t, conn.ReadJSON(&connected))
	require.Equal(t, "connected", connected["type"])
}
