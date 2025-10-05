package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/davidcohan/port-authorizing/internal/config"
)

func TestAuthMiddleware(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		Auth: config.AuthConfig{
			JWTSecret:   "test-secret",
			TokenExpiry: 24 * time.Hour,
			Users: []config.User{
				{Username: "admin", Password: "admin123", Roles: []string{"admin"}},
			},
		},
		Logging: config.LoggingConfig{
			AuditLogPath: "",
			LogLevel:     "info",
		},
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Create a test handler that checks if authentication info is in context
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username := r.Context().Value("username")
		roles := r.Context().Value("roles")

		if username == nil {
			t.Error("username not found in context")
		}
		if roles == nil {
			t.Error("roles not found in context")
		}

		w.WriteHeader(http.StatusOK)
	})

	// Wrap test handler with auth middleware
	handler := server.authMiddleware(testHandler)

	tests := []struct {
		name       string
		token      string
		wantStatus int
		wantAuth   bool
	}{
		{
			name:       "no authorization header",
			token:      "",
			wantStatus: http.StatusUnauthorized,
			wantAuth:   false,
		},
		{
			name:       "invalid authorization format",
			token:      "InvalidFormat",
			wantStatus: http.StatusUnauthorized,
			wantAuth:   false,
		},
		{
			name:       "wrong bearer prefix",
			token:      "Basic sometoken",
			wantStatus: http.StatusUnauthorized,
			wantAuth:   false,
		},
		{
			name:       "invalid token",
			token:      "Bearer invalid.token.here",
			wantStatus: http.StatusUnauthorized,
			wantAuth:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			if tt.token != "" {
				req.Header.Set("Authorization", tt.token)
			}
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

// TestAuthMiddleware_ValidToken is tested in handlers_test.go with full integration

func TestCORSMiddleware_Headers(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		Auth: config.AuthConfig{
			JWTSecret:   "test-secret",
			TokenExpiry: 24 * time.Hour,
			Users: []config.User{
				{Username: "admin", Password: "admin123", Roles: []string{"admin"}},
			},
		},
		Logging: config.LoggingConfig{
			AuditLogPath: "",
			LogLevel:     "info",
		},
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := server.corsMiddleware(testHandler)

	tests := []struct {
		name         string
		method       string
		wantStatus   int
		checkHeaders bool
	}{
		{
			name:         "OPTIONS request",
			method:       "OPTIONS",
			wantStatus:   http.StatusOK,
			checkHeaders: true,
		},
		{
			name:         "GET request",
			method:       "GET",
			wantStatus:   http.StatusOK,
			checkHeaders: true,
		},
		{
			name:         "POST request",
			method:       "POST",
			wantStatus:   http.StatusOK,
			checkHeaders: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/test", nil)
			req.Header.Set("Origin", "http://localhost:3000")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.checkHeaders {
				if w.Header().Get("Access-Control-Allow-Origin") != "*" {
					t.Error("Access-Control-Allow-Origin header not set correctly")
				}

				if w.Header().Get("Access-Control-Allow-Methods") == "" {
					t.Error("Access-Control-Allow-Methods header not set")
				}

				if w.Header().Get("Access-Control-Allow-Headers") == "" {
					t.Error("Access-Control-Allow-Headers header not set")
				}
			}
		})
	}
}

func TestContextHelpers(t *testing.T) {
	t.Run("username in context", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), "username", "testuser")
		username := ctx.Value("username")
		if username == nil {
			t.Fatal("username not found in context")
		}
		if username.(string) != "testuser" {
			t.Errorf("username = %s, want 'testuser'", username.(string))
		}
	})

	t.Run("roles in context", func(t *testing.T) {
		roles := []string{"admin", "developer"}
		ctx := context.WithValue(context.Background(), "roles", roles)
		gotRoles := ctx.Value("roles")
		if gotRoles == nil {
			t.Fatal("roles not found in context")
		}
		if len(gotRoles.([]string)) != 2 {
			t.Errorf("roles count = %d, want 2", len(gotRoles.([]string)))
		}
	})
}

// Benchmark tested via handlers_test.go

func BenchmarkCORSMiddleware(b *testing.B) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		Auth: config.AuthConfig{
			JWTSecret:   "test-secret",
			TokenExpiry: 24 * time.Hour,
		},
		Logging: config.LoggingConfig{
			LogLevel: "info",
		},
	}

	server, _ := NewServer(cfg)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := server.corsMiddleware(testHandler)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("OPTIONS", "/test", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}

