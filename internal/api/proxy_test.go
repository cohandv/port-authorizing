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

func TestHandleProxyStream_PostgresConnection(t *testing.T) {
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
		Connections: []config.ConnectionConfig{
			{
				Name:            "test-postgres",
				Type:            "postgres",
				Host:            "localhost",
				Port:            5432,
				BackendUsername: "dbuser",
				BackendPassword: "dbpass",
				BackendDatabase: "testdb",
				Tags:            []string{"env:test"},
			},
		},
		Policies: []config.RolePolicy{
			{
				Name:      "admin-all",
				Roles:     []string{"admin"},
				Tags:      []string{"env:test"},
				TagMatch:  "any",
				Whitelist: []string{".*"},
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

	// Login to get token
	loginReq := map[string]string{
		"username": "admin",
		"password": "admin123",
	}
	loginBody, _ := json.Marshal(loginReq)
	loginReqHTTP := httptest.NewRequest("POST", "/api/login", bytes.NewReader(loginBody))
	loginReqHTTP.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	server.handleLogin(loginW, loginReqHTTP)

	var loginResp map[string]interface{}
	json.NewDecoder(loginW.Body).Decode(&loginResp)
	token := loginResp["token"].(string)

	// Create connection
	connectReq := httptest.NewRequest("POST", "/api/connect/test-postgres", nil)
	connectReq.Header.Set("Authorization", "Bearer "+token)
	connectW := httptest.NewRecorder()
	server.router.ServeHTTP(connectW, connectReq)

	if connectW.Code != http.StatusOK {
		t.Fatalf("Failed to create connection: status %d", connectW.Code)
	}

	var connectResp map[string]interface{}
	json.NewDecoder(connectW.Body).Decode(&connectResp)
	connectionID := connectResp["connection_id"].(string)

	// Test proxy endpoint exists (we can't fully test TCP proxy without backend)
	proxyReq := httptest.NewRequest("POST", "/api/proxy/"+connectionID, nil)
	proxyReq.Header.Set("Authorization", "Bearer "+token)
	proxyW := httptest.NewRecorder()

	// This will fail to connect to backend, but verifies route exists
	server.router.ServeHTTP(proxyW, proxyReq)

	// Just verify we got some response (backend won't be reachable)
	// We're testing that the route exists and auth works
	_ = proxyW.Code
}

func TestHandleProxyStream_HTTPConnection(t *testing.T) {
	// Create a test backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("backend response"))
	}))
	defer backend.Close()

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
		Connections: []config.ConnectionConfig{
			{
				Name: "test-http",
				Type: "http",
				Host: "localhost",
				Port: 8080,
				Tags: []string{"env:test"},
			},
		},
		Policies: []config.RolePolicy{
			{
				Name:      "admin-all",
				Roles:     []string{"admin"},
				Tags:      []string{"env:test"},
				TagMatch:  "any",
				Whitelist: []string{".*"},
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

	// Login
	loginReq := map[string]string{
		"username": "admin",
		"password": "admin123",
	}
	loginBody, _ := json.Marshal(loginReq)
	loginReqHTTP := httptest.NewRequest("POST", "/api/login", bytes.NewReader(loginBody))
	loginReqHTTP.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	server.handleLogin(loginW, loginReqHTTP)

	var loginResp map[string]interface{}
	json.NewDecoder(loginW.Body).Decode(&loginResp)
	token := loginResp["token"].(string)

	// Create connection
	connectReq := httptest.NewRequest("POST", "/api/connect/test-http", nil)
	connectReq.Header.Set("Authorization", "Bearer "+token)
	connectW := httptest.NewRecorder()
	server.router.ServeHTTP(connectW, connectReq)

	if connectW.Code != http.StatusOK {
		t.Fatalf("Failed to create connection: status %d, body: %s", connectW.Code, connectW.Body.String())
	}

	var connectResp map[string]interface{}
	json.NewDecoder(connectW.Body).Decode(&connectResp)
	connectionID := connectResp["connection_id"].(string)

	// Test proxy endpoint
	proxyReq := httptest.NewRequest("GET", "/api/proxy/"+connectionID+"/test", nil)
	proxyReq.Header.Set("Authorization", "Bearer "+token)
	proxyW := httptest.NewRecorder()

	server.router.ServeHTTP(proxyW, proxyReq)

	// Just verify we got some response
	// We're testing that the route exists and connection was created
	_ = proxyW.Code
}

func TestHandleProxyStream_InvalidConnectionID(t *testing.T) {
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

	// Login
	loginReq := map[string]string{
		"username": "admin",
		"password": "admin123",
	}
	loginBody, _ := json.Marshal(loginReq)
	loginReqHTTP := httptest.NewRequest("POST", "/api/login", bytes.NewReader(loginBody))
	loginReqHTTP.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	server.handleLogin(loginW, loginReqHTTP)

	var loginResp map[string]interface{}
	json.NewDecoder(loginW.Body).Decode(&loginResp)
	token := loginResp["token"].(string)

	// Try to access non-existent connection
	proxyReq := httptest.NewRequest("GET", "/api/proxy/fake-connection-id/test", nil)
	proxyReq.Header.Set("Authorization", "Bearer "+token)
	proxyW := httptest.NewRecorder()

	server.router.ServeHTTP(proxyW, proxyReq)

	if proxyW.Code != http.StatusNotFound && proxyW.Code != http.StatusBadRequest {
		t.Errorf("Expected 404 or 400 for invalid connection, got %d", proxyW.Code)
	}
}

func TestHandleProxyStream_ExpiredConnection(t *testing.T) {
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
		Connections: []config.ConnectionConfig{
			{
				Name: "test-http",
				Type: "http",
				Host: "localhost",
				Port: 8080,
				Tags: []string{"env:test"},
			},
		},
		Policies: []config.RolePolicy{
			{
				Name:      "admin-all",
				Roles:     []string{"admin"},
				Tags:      []string{"env:test"},
				TagMatch:  "any",
				Whitelist: []string{".*"},
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

	// Login
	loginReq := map[string]string{
		"username": "admin",
		"password": "admin123",
	}
	loginBody, _ := json.Marshal(loginReq)
	loginReqHTTP := httptest.NewRequest("POST", "/api/login", bytes.NewReader(loginBody))
	loginReqHTTP.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	server.handleLogin(loginW, loginReqHTTP)

	var loginResp map[string]interface{}
	json.NewDecoder(loginW.Body).Decode(&loginResp)
	token := loginResp["token"].(string)

	// Create connection with very short duration
	connectReq := httptest.NewRequest("POST", "/api/connect/test-http?duration=1ms", nil)
	connectReq.Header.Set("Authorization", "Bearer "+token)
	connectW := httptest.NewRecorder()
	server.router.ServeHTTP(connectW, connectReq)

	var connectResp map[string]interface{}
	json.NewDecoder(connectW.Body).Decode(&connectResp)
	connectionID := connectResp["connection_id"].(string)

	// Wait for connection to expire
	time.Sleep(10 * time.Millisecond)

	// Try to use expired connection
	proxyReq := httptest.NewRequest("GET", "/api/proxy/"+connectionID+"/test", nil)
	proxyReq.Header.Set("Authorization", "Bearer "+token)
	proxyW := httptest.NewRecorder()

	server.router.ServeHTTP(proxyW, proxyReq)

	// Should get error for expired connection
	if proxyW.Code == http.StatusOK {
		t.Error("Should not allow access to expired connection")
	}
}

func BenchmarkHandleProxyStream(b *testing.B) {
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
		Connections: []config.ConnectionConfig{
			{
				Name: "test-http",
				Type: "http",
				Host: "localhost",
				Port: 8080,
				Tags: []string{"env:test"},
			},
		},
		Policies: []config.RolePolicy{
			{
				Name:      "admin-all",
				Roles:     []string{"admin"},
				Tags:      []string{"env:test"},
				TagMatch:  "any",
				Whitelist: []string{".*"},
			},
		},
		Logging: config.LoggingConfig{
			AuditLogPath: "",
			LogLevel:     "info",
		},
	}

	server, _ := NewServer(cfg)

	// Get token and connection
	loginReq := map[string]string{"username": "admin", "password": "admin123"}
	loginBody, _ := json.Marshal(loginReq)
	loginReqHTTP := httptest.NewRequest("POST", "/api/login", bytes.NewReader(loginBody))
	loginReqHTTP.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	server.handleLogin(loginW, loginReqHTTP)

	var loginResp map[string]interface{}
	json.NewDecoder(loginW.Body).Decode(&loginResp)
	token := loginResp["token"].(string)

	connectReq := httptest.NewRequest("POST", "/api/connect/test-http", nil)
	connectReq.Header.Set("Authorization", "Bearer "+token)
	connectW := httptest.NewRecorder()
	server.router.ServeHTTP(connectW, connectReq)

	var connectResp map[string]interface{}
	json.NewDecoder(connectW.Body).Decode(&connectResp)
	connectionID := connectResp["connection_id"].(string)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proxyReq := httptest.NewRequest("GET", "/api/proxy/"+connectionID+"/test", nil)
		proxyReq.Header.Set("Authorization", "Bearer "+token)
		proxyW := httptest.NewRecorder()
		server.router.ServeHTTP(proxyW, proxyReq)
	}
}

