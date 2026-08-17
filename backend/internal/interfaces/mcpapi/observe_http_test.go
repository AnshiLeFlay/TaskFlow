package mcpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObserveHTTPAddsRequestIDAndAccessLog(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := ObserveHTTP(logger, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/mcp", nil))

	assert.Equal(t, http.StatusAccepted, recorder.Code)
	require.NotEmpty(t, recorder.Header().Get("X-Request-ID"))
	assert.Contains(t, logs.String(), `"msg":"MCP HTTP request"`)
	assert.Contains(t, logs.String(), `"status":202`)
}

func TestObserveHTTPRecoversPanicsWithoutExposingValue(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := ObserveHTTP(logger, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("database-password-must-not-be-returned")
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/mcp", nil))

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.JSONEq(t, `{"error":"internal_error"}`, strings.TrimSpace(recorder.Body.String()))
	assert.NotContains(t, recorder.Body.String(), "database-password")
	assert.Contains(t, logs.String(), `"msg":"panic in MCP HTTP handler"`)
}
