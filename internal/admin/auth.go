package admin

import (
	cryptorand "crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
)

// Authenticator handles admin authentication with secure token generation.
type Authenticator struct {
	token    string
	path     string
	useHTTPS bool
}

// Authentication constants
const (
	// Length of random admin endpoint path
	adminPathLength = 32
	// Length of admin authentication token
	adminTokenLength = 32
	// Cookie settings
	cookieName   = "gospidertrap_admin_token"
	cookieMaxAge = 86400 // 24 hours in seconds
)

// NewAuthenticator creates a new authenticator with secure random token and path.
//
// Parameters:
//   - useHTTPS: whether HTTPS is being used (affects cookie Secure flag)
//
// Returns a new Authenticator instance or an error if random generation fails.
func NewAuthenticator(useHTTPS bool) (*Authenticator, error) {
	token, err := generateSecureRandomString(adminTokenLength)
	if err != nil {
		return nil, fmt.Errorf("failed to generate admin token: %w", err)
	}

	path, err := generateAdminPath()
	if err != nil {
		return nil, fmt.Errorf("failed to generate admin path: %w", err)
	}

	return &Authenticator{
		token:    token,
		path:     path,
		useHTTPS: useHTTPS,
	}, nil
}

// generateSecureRandomString generates a cryptographically secure random string
// of the specified length using crypto/rand.
//
// The generated string uses URL-safe base64 encoding and is suitable for
// security-sensitive purposes such as authentication tokens and secret paths.
//
// Parameters:
//   - length: the desired length of the generated string
//
// Returns a cryptographically secure random string, or an error if random
// generation fails.
func generateSecureRandomString(length int) (string, error) {
	// Generate enough random bytes to ensure we get the desired length after encoding
	// Base64 encoding produces 4 characters for every 3 bytes, so we need (length * 3 / 4) bytes
	numBytes := (length * 3) / 4
	if numBytes < length {
		numBytes = length
	}

	b := make([]byte, numBytes)
	if _, err := cryptorand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate secure random string: %w", err)
	}

	// Use URL-safe base64 encoding without padding
	encoded := base64.RawURLEncoding.EncodeToString(b)

	// Trim to exact length if needed
	if len(encoded) > length {
		encoded = encoded[:length]
	}

	return encoded, nil
}

// generateAdminPath generates a cryptographically secure random admin endpoint path.
//
// Uses crypto/rand for secure random generation to prevent path prediction attacks.
//
// Returns a random path string starting with "/" for the admin UI, or an error if
// random generation fails.
func generateAdminPath() (string, error) {
	randomStr, err := generateSecureRandomString(adminPathLength)
	if err != nil {
		return "", fmt.Errorf("failed to generate admin path: %w", err)
	}
	return "/" + randomStr, nil
}

// GetToken returns the authentication token.
func (a *Authenticator) GetToken() string {
	return a.token
}

// GetPath returns the admin endpoint path.
func (a *Authenticator) GetPath() string {
	return a.path
}

// ValidateToken checks if the provided token matches the admin token using
// constant-time comparison to prevent timing attacks.
//
// Parameters:
//   - token: the token to validate
//
// Returns true if the token is valid, false otherwise.
func (a *Authenticator) ValidateToken(token string) bool {
	// Use constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare([]byte(token), []byte(a.token)) == 1
}

// GetTokenFromRequest extracts the admin token from either cookie or query parameter.
//
// For backward compatibility, it checks cookies first (preferred), then query parameters.
//
// Parameters:
//   - r: the HTTP request
//
// Returns the token if found, empty string otherwise.
func (a *Authenticator) GetTokenFromRequest(r *http.Request) string {
	// Check cookie first (preferred method)
	if cookie, err := r.Cookie(cookieName); err == nil {
		return cookie.Value
	}
	// Fallback to query parameter for backward compatibility
	return r.URL.Query().Get("token")
}

// SetCookie sets the admin authentication cookie.
//
// The Secure flag is set based on the useHTTPS configuration to ensure
// the cookie is only transmitted over secure connections when HTTPS is enabled.
//
// Parameters:
//   - w: the HTTP response writer
func (a *Authenticator) SetCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    a.token,
		Path:     "/",
		MaxAge:   cookieMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   a.useHTTPS, // Only send cookie over HTTPS when enabled
	})
}

// IsAuthenticated checks if the request is authenticated with a valid admin token.
//
// Parameters:
//   - r: the HTTP request
//
// Returns true if authenticated, false otherwise.
func (a *Authenticator) IsAuthenticated(r *http.Request) bool {
	token := a.GetTokenFromRequest(r)
	return a.ValidateToken(token)
}

// GetLoginURL returns the login URL with token parameter.
//
// Parameters:
//   - host: the host to use in the URL (e.g., "localhost:8000")
//
// Returns the complete login URL.
func (a *Authenticator) GetLoginURL(host string) string {
	scheme := "http"
	if a.useHTTPS {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s%s/login?token=%s", scheme, host, a.path, a.token)
}

// GetAdminURL returns the admin UI URL (without token).
//
// Parameters:
//   - host: the host to use in the URL (e.g., "localhost:8000")
//
// Returns the admin UI URL.
func (a *Authenticator) GetAdminURL(host string) string {
	scheme := "http"
	if a.useHTTPS {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s%s", scheme, host, a.path)
}
