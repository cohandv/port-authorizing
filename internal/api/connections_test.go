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

func TestHandleConnect_FullFlow(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		Auth: config.AuthConfig{
			JWTSecret:   "test-secret",
			TokenExpiry: 24 * time.Hour,
			Users: []config.User{
				{Username: "admin", Password: "admin123", Roles: []string{"admin"}},
				{Username: "developer", Password: "dev123", Roles: []string{"developer"}},
			},
		},
		Connections: []config.ConnectionConfig{
			{
				Name:            "test-db",
				Type:            "postgres",
				Host:            "localhost",
				Port:            5432,
				BackendUsername: "dbuser",
				BackendPassword: "dbpass",
				BackendDatabase: "testdb",
				Tags:            []string{"env:test"},
			},
			{
				Name: "prod-db",
				Type: "postgres",
				Host: "prod.example.com",
				Port: 5432,
				Tags: []string{"env:prod"},
			},
		},
		Policies: []config.RolePolicy{
			{
				Name:      "admin-all",
				Roles:     []string{"admin"},
				Tags:      []string{"env:test", "env:prod"},
				TagMatch:  "any",
				Whitelist: []string{".*"},
			},
			{
				Name:      "dev-test-only",
				Roles:     []string{"developer"},
				Tags:      []string{"env:test"},
				TagMatch:  "any",
				Whitelist: []string{"^SELECT.*"},
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

	// First login to get token
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
	_ = json.NewDecoder(loginW.Body).Decode(&loginResp)
	token := loginResp["token"].(string)

	tests := []struct {
		name          string
		connection    string
		wantStatus    int
		checkResponse bool
	}{
		{
			name:          "connect to test-db",
			connection:    "test-db",
			wantStatus:    http.StatusOK,
			checkResponse: true,
		},
		{
			name:       "connect to non-existent db",
			connection: "fake-db",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/connect/"+tt.connection, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			// Add username and roles to context (auth middleware would do this)
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d, body: %s", w.Code, tt.wantStatus, w.Body.String())
			}

			if tt.checkResponse && w.Code == http.StatusOK {
				var response map[string]interface{}
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				if response["connection_id"] == nil {
					t.Error("response should contain connection_id")
				}

				if response["expires_at"] == nil {
					t.Error("response should contain expires_at")
				}
			}
		})
	}
}

func TestHandleListConnections_WithFiltering(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		Auth: config.AuthConfig{
			JWTSecret:   "test-secret",
			TokenExpiry: 24 * time.Hour,
			Users: []config.User{
				{Username: "developer", Password: "dev123", Roles: []string{"developer"}},
			},
		},
		Connections: []config.ConnectionConfig{
			{Name: "test-db-1", Type: "postgres", Host: "localhost", Port: 5432, Tags: []string{"env:test"}},
			{Name: "test-db-2", Type: "postgres", Host: "localhost", Port: 5433, Tags: []string{"env:test"}},
			{Name: "prod-db", Type: "postgres", Host: "prod.example.com", Port: 5432, Tags: []string{"env:prod"}},
		},
		Policies: []config.RolePolicy{
			{
				Name:      "dev-test-only",
				Roles:     []string{"developer"},
				Tags:      []string{"env:test"},
				TagMatch:  "any",
				Whitelist: []string{"^SELECT.*"},
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
		"username": "developer",
		"password": "dev123",
	}
	loginBody, _ := json.Marshal(loginReq)
	loginReqHTTP := httptest.NewRequest("POST", "/api/login", bytes.NewReader(loginBody))
	loginReqHTTP.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	server.handleLogin(loginW, loginReqHTTP)

	var loginResp map[string]interface{}
	_ = json.NewDecoder(loginW.Body).Decode(&loginResp)
	token := loginResp["token"].(string)

	// List connections
	req := httptest.NewRequest("GET", "/api/connections", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var connections []interface{}
	if err := json.NewDecoder(w.Body).Decode(&connections); err != nil {
		t.Fatalf("Failed to decode response: %v, body: %s", err, w.Body.String())
	}

	// Developer should only see 2 test connections, not prod
	if len(connections) != 2 {
		t.Errorf("connections count = %d, want 2", len(connections))
	}
}

func BenchmarkHandleConnect(b *testing.B) {
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
				Name: "test-db",
				Type: "postgres",
				Host: "localhost",
				Port: 5432,
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

	// Get token
	loginReq := map[string]string{"username": "admin", "password": "admin123"}
	loginBody, _ := json.Marshal(loginReq)
	loginReqHTTP := httptest.NewRequest("POST", "/api/login", bytes.NewReader(loginBody))
	loginReqHTTP.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	server.handleLogin(loginW, loginReqHTTP)

	var loginResp map[string]interface{}
	_ = json.NewDecoder(loginW.Body).Decode(&loginResp)
	token := loginResp["token"].(string)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/api/connect/test-db", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)
	}
}
