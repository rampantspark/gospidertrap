package middleware

import (
	"log/slog"
	"net/http"
)

// RecoverPanic creates a middleware that recovers from panics in HTTP handlers.
//
// When a panic occurs, it logs the error and returns a 500 Internal Server Error
// to the client instead of crashing the entire server.
//
// Parameters:
//   - logger: structured logger instance for logging panic details
//
// Returns a middleware function that wraps an http.Handler.
func RecoverPanic(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Error("Panic recovered",
						"error", err,
						"path", r.URL.Path,
						"method", r.Method,
						"remote_addr", r.RemoteAddr,
					)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
