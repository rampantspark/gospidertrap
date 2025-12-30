package admin

import (
	"encoding/json"
	"html"
	"io"
	"log/slog"
	"net/http"

	"github.com/rampantspark/gospidertrap/internal/stats"
)

// Handler handles admin UI HTTP requests.
type Handler struct {
	auth         *Authenticator
	statsManager *stats.Manager
	renderer     *Renderer
	logger       *slog.Logger
}

// NewHandler creates a new admin handler.
//
// Parameters:
//   - auth: authenticator instance
//   - statsManager: stats manager for retrieving data
//   - logger: structured logger instance
//
// Returns a new Handler instance.
func NewHandler(auth *Authenticator, statsManager *stats.Manager, logger *slog.Logger) *Handler {
	return &Handler{
		auth:         auth,
		statsManager: statsManager,
		renderer:     NewRenderer(auth.GetPath()),
		logger:       logger,
	}
}

// HandleLogin handles login requests for the admin UI.
//
// It accepts a token via query parameter, validates it, and sets an HTTP cookie
// if valid. For backward compatibility, it also accepts the token in the URL.
//
// Parameters:
//   - w: the HTTP response writer
//   - r: the HTTP request
func (h *Handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check if context is cancelled
	if ctx.Err() != nil {
		return
	}

	token := r.URL.Query().Get("token")
	if !h.auth.ValidateToken(token) {
		// Log failed login attempt for security monitoring
		ip := r.RemoteAddr
		h.logger.Warn("Failed admin login attempt",
			"ip", ip,
			"path", r.URL.Path,
			"user_agent", r.Header.Get("User-Agent"))

		// Set security headers
		h.setSecurityHeaders(w)
		w.WriteHeader(http.StatusForbidden)
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<!DOCTYPE html>\n<html>\n<head><title>Access Denied</title></head>\n<body>\n<h1>403 Forbidden</h1>\n<p>Invalid or missing authentication token.</p>\n</body>\n</html>")
		return
	}

	// Log successful login
	ip := r.RemoteAddr
	h.logger.Info("Successful admin login",
		"ip", ip,
		"user_agent", r.Header.Get("User-Agent"))

	// Set authentication cookie
	h.auth.SetCookie(w)

	// Redirect to admin UI (without token in URL)
	http.Redirect(w, r, h.auth.GetPath(), http.StatusSeeOther)
}

// HandleChartData handles requests for chart data in JSON format.
//
// It validates the authentication token and returns chart data as JSON.
//
// Parameters:
//   - w: the HTTP response writer
//   - r: the HTTP request
func (h *Handler) HandleChartData(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Validate authentication
	if !h.auth.IsAuthenticated(r) {
		// Set security headers
		h.setSecurityHeaders(w)
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid or missing authentication token"})
		return
	}

	// Check if context is cancelled
	if ctx.Err() != nil {
		return
	}

	// Get chart data from stats manager
	data := h.statsManager.GetChartData(10, 50) // top 10 items, max 50 char user agents

	// Set security headers
	h.setSecurityHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// HandleUI handles requests to the admin UI endpoint.
//
// It validates authentication via cookie or query parameter (for backward compatibility).
// If authentication fails, it returns a 403 Forbidden response.
// Otherwise, it displays connection statistics including total requests, IP counts,
// user agent counts, and recent request history in a formatted HTML page.
//
// Parameters:
//   - w: the HTTP response writer
//   - r: the HTTP request
func (h *Handler) HandleUI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check if context is cancelled
	if ctx.Err() != nil {
		return
	}

	// Validate authentication (supports both cookie and query param for backward compatibility)
	if !h.auth.IsAuthenticated(r) {
		// If token is in query param but not in cookie, set cookie for future requests
		if token := r.URL.Query().Get("token"); token != "" && h.auth.ValidateToken(token) {
			h.auth.SetCookie(w)
		} else {
			w.WriteHeader(http.StatusForbidden)
			w.Header().Set("Content-Type", "text/html")
			io.WriteString(w, "<!DOCTYPE html>\n<html>\n<head><title>Access Denied</title></head>\n<body>\n<h1>403 Forbidden</h1>\n<p>Invalid or missing authentication token.</p>\n<p>Use: <a href=\""+html.EscapeString(h.auth.GetPath())+"/login?token=YOUR_TOKEN\">Login</a></p>\n</body>\n</html>")
			return
		}
	}

	// Check context again before processing
	if ctx.Err() != nil {
		return
	}

	// Get chart data
	chartData := h.statsManager.GetChartData(10, 50) // top 10 items, max 50 char user agents

	// Get stats and recent requests
	uptime, totalRequests, uniqueIPs, uniqueUAs := h.statsManager.GetStats()
	recentRequests := h.statsManager.GetRecentRequests(50) // max 50 recent requests

	// Render HTML
	html := h.renderer.RenderAdminUI(
		chartData,
		uptime,
		totalRequests,
		uniqueIPs,
		uniqueUAs,
		recentRequests,
		50, // max display
	)

	// Set security headers before sending response
	h.setSecurityHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, html)
}

// setSecurityHeaders sets security headers for admin UI responses.
//
// This includes CSP headers to prevent XSS attacks and other security headers
// to harden the admin interface.
func (h *Handler) setSecurityHeaders(w http.ResponseWriter) {
	// Content Security Policy - restrict resource loading
	w.Header().Set("Content-Security-Policy",
		"default-src 'self'; "+
			"script-src 'self' https://cdn.jsdelivr.net; "+
			"style-src 'self' 'unsafe-inline'; "+
			"img-src 'self' data:; "+
			"connect-src 'self'; "+
			"frame-ancestors 'none'")

	// Prevent page from being displayed in iframe (clickjacking protection)
	w.Header().Set("X-Frame-Options", "DENY")

	// Prevent MIME type sniffing
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// Referrer policy - don't leak URLs to external sites
	w.Header().Set("Referrer-Policy", "no-referrer")

	// XSS protection (legacy header but still useful)
	w.Header().Set("X-XSS-Protection", "1; mode=block")
}

// GetPath returns the admin endpoint path.
func (h *Handler) GetPath() string {
	return h.auth.GetPath()
}

// GetLoginURL returns the login URL.
//
// Parameters:
//   - host: the host to use in the URL (e.g., "localhost:8000")
//
// Returns the complete login URL with token.
func (h *Handler) GetLoginURL(host string) string {
	return h.auth.GetLoginURL(host)
}

// GetAdminURL returns the admin UI URL (without token).
//
// Parameters:
//   - host: the host to use in the URL (e.g., "localhost:8000")
//
// Returns the admin UI URL.
func (h *Handler) GetAdminURL(host string) string {
	return h.auth.GetAdminURL(host)
}
