package stats

import (
	"net"
	"net/http"
	"strings"
)

// IPResolver handles IP address extraction from HTTP requests.
type IPResolver struct {
	trustProxy bool
}

// NewIPResolver creates a new IP resolver.
//
// Parameters:
//   - trustProxy: whether to trust X-Forwarded-For and X-Real-IP headers
//
// Returns a new IPResolver instance.
func NewIPResolver(trustProxy bool) *IPResolver {
	return &IPResolver{
		trustProxy: trustProxy,
	}
}

// GetClientIP extracts the client IP address from the request.
//
// If trustProxy is true, it checks X-Forwarded-For and X-Real-IP headers for
// proxied requests and validates that they contain valid IP addresses.
// Invalid IPs or untrusted headers are ignored.
// Falls back to RemoteAddr if proxy headers are not present or not trusted.
//
// Parameters:
//   - r: the HTTP request
//
// Returns the client IP address as a string.
func (resolver *IPResolver) GetClientIP(r *http.Request) string {
	if resolver.trustProxy {
		// Check X-Forwarded-For header (first IP in comma-separated list)
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ips := strings.Split(xff, ",")
			if len(ips) > 0 {
				ip := strings.TrimSpace(ips[0])
				// Validate IP format
				if parsedIP := net.ParseIP(ip); parsedIP != nil {
					return ip
				}
			}
		}
		// Check X-Real-IP header
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			ip := strings.TrimSpace(xri)
			// Validate IP format
			if parsedIP := net.ParseIP(ip); parsedIP != nil {
				return ip
			}
		}
	}
	// Fall back to RemoteAddr, removing port if present
	ip := r.RemoteAddr
	if colonIdx := strings.LastIndex(ip, ":"); colonIdx != -1 {
		ip = ip[:colonIdx]
	}
	return ip
}
