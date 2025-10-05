package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/davidcohan/port-authorizing/internal/config"
)

func TestHandleHealth(t *testing.T) {
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

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleHealth() status = %d, want %d", w.Code, http.StatusOK)
	}

	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["status"] != "healthy" {
		t.Errorf("status = %s, want 'healthy'", response["status"])
	}
}

func TestHandleLogin_InvalidJSON(t *testing.T) {
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

	req := httptest.NewRequest("POST", "/api/login", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleLogin(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("handleLogin() status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAuthMiddleware_NoToken(t *testing.T) {
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

	req := httptest.NewRequest("GET", "/api/connections", nil)
	w := httptest.NewRecorder()

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := server.authMiddleware(testHandler)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("authMiddleware() status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
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

	req := httptest.NewRequest("GET", "/api/connections", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	w := httptest.NewRecorder()

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := server.authMiddleware(testHandler)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("authMiddleware() status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// CORS functionality is tested through integration tests (handleLogin, handleListConnections, etc.)
// which all use the router that has CORS middleware applied

func TestNewServer(t *testing.T) {
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
		t.Fatalf("NewServer() error = %v", err)
	}

	if server == nil {
		t.Fatal("NewServer() returned nil")
	}

	if server.config != cfg {
		t.Error("Server config not set correctly")
	}

	if server.router == nil {
		t.Error("Router should be initialized")
	}

	if server.connMgr == nil {
		t.Error("Connection manager should be initialized")
	}
}

func BenchmarkHandleHealth(b *testing.B) {
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

	server, _ := NewServer(cfg)
	req := httptest.NewRequest("GET", "/api/health", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		server.handleHealth(w, req)
	}
}
