package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewAuthenticator(t *testing.T) {
	tests := []struct {
		name     string
		useHTTPS bool
	}{
		{"with HTTPS", true},
		{"without HTTPS", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := NewAuthenticator(tt.useHTTPS)
			if err != nil {
				t.Fatalf("NewAuthenticator() error = %v", err)
			}
			if auth == nil {
				t.Fatal("NewAuthenticator() returned nil")
			}
			if auth.token == "" {
				t.Error("token is empty")
			}
			if auth.path == "" {
				t.Error("path is empty")
			}
			if auth.useHTTPS != tt.useHTTPS {
				t.Errorf("useHTTPS = %v, want %v", auth.useHTTPS, tt.useHTTPS)
			}
		})
	}
}

func TestNewAuthenticator_Uniqueness(t *testing.T) {
	// Create multiple authenticators and verify tokens and paths are unique
	tokens := make(map[string]bool)
	paths := make(map[string]bool)

	for i := 0; i < 10; i++ {
		auth, err := NewAuthenticator(false)
		if err != nil {
			t.Fatalf("NewAuthenticator() error = %v", err)
		}

		if tokens[auth.token] {
			t.Error("NewAuthenticator() produced duplicate token")
		}
		tokens[auth.token] = true

		if paths[auth.path] {
			t.Error("NewAuthenticator() produced duplicate path")
		}
		paths[auth.path] = true

		// Verify token and path properties
		if len(auth.token) == 0 {
			t.Error("token is empty")
		}
		if !strings.HasPrefix(auth.path, "/") {
			t.Errorf("path = %q, should start with /", auth.path)
		}
	}
}

func TestValidateToken(t *testing.T) {
	auth, _ := NewAuthenticator(false)

	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{
			name:  "valid token",
			token: auth.token,
			want:  true,
		},
		{
			name:  "invalid token",
			token: "wrongtoken",
			want:  false,
		},
		{
			name:  "empty token",
			token: "",
			want:  false,
		},
		{
			name:  "token with different case",
			token: strings.ToUpper(auth.token),
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := auth.ValidateToken(tt.token); got != tt.want {
				t.Errorf("ValidateToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetTokenFromRequest(t *testing.T) {
	auth, _ := NewAuthenticator(false)

	tests := []struct {
		name       string
		setupReq   func(*http.Request)
		wantToken  string
	}{
		{
			name: "token in query param",
			setupReq: func(r *http.Request) {
				q := r.URL.Query()
				q.Set("token", "test123")
				r.URL.RawQuery = q.Encode()
			},
			wantToken: "test123",
		},
		{
			name: "token in cookie",
			setupReq: func(r *http.Request) {
				r.AddCookie(&http.Cookie{
					Name:  cookieName,
					Value: "cookietoken",
				})
			},
			wantToken: "cookietoken",
		},
		{
			name: "cookie takes precedence over query param",
			setupReq: func(r *http.Request) {
				q := r.URL.Query()
				q.Set("token", "queryparam")
				r.URL.RawQuery = q.Encode()
				r.AddCookie(&http.Cookie{
					Name:  cookieName,
					Value: "cookietoken",
				})
			},
			wantToken: "cookietoken",
		},
		{
			name:      "no token",
			setupReq:  func(r *http.Request) {},
			wantToken: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			tt.setupReq(req)

			got := auth.GetTokenFromRequest(req)
			if got != tt.wantToken {
				t.Errorf("GetTokenFromRequest() = %q, want %q", got, tt.wantToken)
			}
		})
	}
}

func TestSetCookie(t *testing.T) {
	tests := []struct {
		name     string
		useHTTPS bool
	}{
		{"with HTTPS", true},
		{"without HTTPS", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, _ := NewAuthenticator(tt.useHTTPS)
			w := httptest.NewRecorder()

			auth.SetCookie(w)

			cookies := w.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatalf("got %d cookies, want 1", len(cookies))
			}

			cookie := cookies[0]
			if cookie.Name != cookieName {
				t.Errorf("cookie name = %q, want %q", cookie.Name, cookieName)
			}
			if cookie.Value != auth.token {
				t.Errorf("cookie value = %q, want %q", cookie.Value, auth.token)
			}
			if cookie.HttpOnly != true {
				t.Error("cookie should be HttpOnly")
			}
			if cookie.SameSite != http.SameSiteStrictMode {
				t.Error("cookie should have SameSite=Strict")
			}
			if cookie.Secure != tt.useHTTPS {
				t.Errorf("cookie Secure = %v, want %v", cookie.Secure, tt.useHTTPS)
			}
			if cookie.MaxAge != cookieMaxAge {
				t.Errorf("cookie MaxAge = %d, want %d", cookie.MaxAge, cookieMaxAge)
			}
		})
	}
}

func TestIsAuthenticated(t *testing.T) {
	auth, _ := NewAuthenticator(false)

	tests := []struct {
		name     string
		setupReq func(*http.Request)
		want     bool
	}{
		{
			name: "authenticated with valid token in query",
			setupReq: func(r *http.Request) {
				q := r.URL.Query()
				q.Set("token", auth.token)
				r.URL.RawQuery = q.Encode()
			},
			want: true,
		},
		{
			name: "authenticated with valid token in cookie",
			setupReq: func(r *http.Request) {
				r.AddCookie(&http.Cookie{
					Name:  cookieName,
					Value: auth.token,
				})
			},
			want: true,
		},
		{
			name: "not authenticated with invalid token",
			setupReq: func(r *http.Request) {
				q := r.URL.Query()
				q.Set("token", "wrongtoken")
				r.URL.RawQuery = q.Encode()
			},
			want: false,
		},
		{
			name:     "not authenticated with no token",
			setupReq: func(r *http.Request) {},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			tt.setupReq(req)

			if got := auth.IsAuthenticated(req); got != tt.want {
				t.Errorf("IsAuthenticated() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetPath(t *testing.T) {
	auth, _ := NewAuthenticator(false)

	path := auth.GetPath()
	if path == "" {
		t.Error("GetPath() returned empty string")
	}
	if !strings.HasPrefix(path, "/") {
		t.Errorf("GetPath() = %q, should start with /", path)
	}
	if path != auth.path {
		t.Errorf("GetPath() = %q, want %q", path, auth.path)
	}
}

func TestGetLoginURL(t *testing.T) {
	auth, _ := NewAuthenticator(false)

	tests := []struct {
		name string
		host string
	}{
		{"localhost", "localhost:8080"},
		{"domain", "example.com"},
		{"IP address", "192.168.1.1:3000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := auth.GetLoginURL(tt.host)
			if url == "" {
				t.Error("GetLoginURL() returned empty string")
			}
			if !strings.Contains(url, tt.host) {
				t.Errorf("GetLoginURL() = %q, should contain host %q", url, tt.host)
			}
			if !strings.Contains(url, "token=") {
				t.Error("GetLoginURL() missing token parameter")
			}
			if !strings.Contains(url, auth.token) {
				t.Error("GetLoginURL() missing actual token value")
			}
		})
	}
}

func TestGetAdminURL(t *testing.T) {
	auth, _ := NewAuthenticator(false)

	tests := []struct {
		name string
		host string
	}{
		{"localhost", "localhost:8080"},
		{"domain", "example.com"},
		{"IP address", "192.168.1.1:3000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := auth.GetAdminURL(tt.host)
			if url == "" {
				t.Error("GetAdminURL() returned empty string")
			}
			if !strings.Contains(url, tt.host) {
				t.Errorf("GetAdminURL() = %q, should contain host %q", url, tt.host)
			}
			if !strings.Contains(url, auth.path) {
				t.Error("GetAdminURL() missing admin path")
			}
		})
	}
}

func BenchmarkValidateToken(b *testing.B) {
	auth, _ := NewAuthenticator(false)
	token := auth.token

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		auth.ValidateToken(token)
	}
}

func BenchmarkNewAuthenticator(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewAuthenticator(false)
	}
}
