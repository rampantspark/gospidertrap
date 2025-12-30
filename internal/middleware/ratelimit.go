package middleware

import (
	"net/http"

	"github.com/rampantspark/gospidertrap/internal/ratelimit"
)

// RateLimit creates a middleware that enforces rate limiting per IP address.
//
// Parameters:
//   - limiter: the rate limiter instance
//   - getIP: function to extract IP from request
//
// Returns a middleware function that wraps an http.Handler.
func RateLimit(limiter *ratelimit.Limiter, getIP func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := getIP(r)
			if !limiter.Allow(ip) {
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
