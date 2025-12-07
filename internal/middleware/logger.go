package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Use a response wrapper to capture the status code if needed,
		// but for simplicity in a boilerplate, we might just log the request start/end.
		// A proper implementation would wrap ResponseWriter.

		wrapper := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(wrapper, r)

		slog.Info("Request processed",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapper.status,
			"duration", time.Since(start),
			"remote_addr", r.RemoteAddr,
		)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
