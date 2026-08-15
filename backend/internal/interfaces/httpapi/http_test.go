package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/taskflow/backend/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCORSPreflightBypassesMuxMethodAndAuthenticationMatching(t *testing.T) {
	router := NewRouter(nil, nil, http.NotFoundHandler(), "", []string{"http://localhost:8081"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/projects", nil)
	request.Header.Set("Origin", "http://localhost:8081")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusNoContent, recorder.Code)
	assert.Equal(t, "http://localhost:8081", recorder.Header().Get("Access-Control-Allow-Origin"))
	assert.True(t, strings.Contains(recorder.Header().Get("Access-Control-Allow-Headers"), "Authorization"))
}

func TestWriteErrorMapsOptimisticConflictAndWorkflowDenial(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"optimistic conflict", fmt.Errorf("%w: task changed concurrently", domain.ErrConflict), http.StatusConflict, "conflict"},
		{"workflow denial", fmt.Errorf("%w: no rule", domain.ErrTransitionNotAllowed), http.StatusUnprocessableEntity, "transition_not_allowed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			writeError(recorder, test.err)
			assert.Equal(t, test.status, recorder.Code)
			var body errorEnvelope
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
			assert.Equal(t, test.code, body.Error.Code)
		})
	}
}
