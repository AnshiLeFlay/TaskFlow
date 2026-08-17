package mcpapi

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ObserveHTTP adds the same request correlation, access logging, and panic
// boundary used by the REST transport without coupling MCP to REST routing or
// CORS policy.
func ObserveHTTP(logger *slog.Logger, next http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if requestID == "" {
			requestID = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", requestID)
		recorder := &observedResponseWriter{ResponseWriter: w, status: http.StatusOK}
		started := time.Now()
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("panic in MCP HTTP handler", "error", recovered, "method", r.Method, "path", r.URL.Path, "request_id", requestID)
				writeOAuthError(recorder, http.StatusInternalServerError, "internal_error")
			}
			logger.Info("MCP HTTP request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", recorder.status,
				"duration", time.Since(started),
				"request_id", requestID,
			)
		}()
		next.ServeHTTP(recorder, r)
	})
}

type observedResponseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *observedResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *observedResponseWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.status = http.StatusOK
		w.wroteHeader = true
	}
	return w.ResponseWriter.Write(body)
}

func (w *observedResponseWriter) Flush() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *observedResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
