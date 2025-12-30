package middleware

import (
	"net/http"
)

// LimitRequestBody creates a middleware that enforces request body size limits.
//
// This prevents memory exhaustion attacks by limiting the maximum size of request bodies.
//
// Parameters:
//   - maxBytes: maximum allowed request body size in bytes
//
// Returns a middleware function that wraps an http.Handler.
func LimitRequestBody(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Wrap the request body with a size limit
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
