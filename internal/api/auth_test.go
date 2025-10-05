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

func TestRespondError(t *testing.T) {
	w := httptest.NewRecorder()
	respondError(w, http.StatusBadRequest, "test error message")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["error"] != "test error message" {
		t.Errorf("error message = %s, want 'test error message'", response["error"])
	}
}

func TestRespondJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]interface{}{
		"key1": "value1",
		"key2": 123,
		"key3": true,
	}

	respondJSON(w, http.StatusOK, data)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["key1"] != "value1" {
		t.Errorf("key1 = %v, want 'value1'", response["key1"])
	}

	if response["key2"].(float64) != 123 {
		t.Errorf("key2 = %v, want 123", response["key2"])
	}

	if response["key3"] != true {
		t.Errorf("key3 = %v, want true", response["key3"])
	}
}

func TestHandleLogin_FullFlow(t *testing.T) {
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
		Logging: config.LoggingConfig{
			AuditLogPath: "",
			LogLevel:     "info",
		},
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	tests := []struct {
		name         string
		username     string
		password     string
		wantStatus   int
		wantToken    bool
		wantUsername string
	}{
		{
			name:         "admin login success",
			username:     "admin",
			password:     "admin123",
			wantStatus:   http.StatusOK,
			wantToken:    true,
			wantUsername: "admin",
		},
		{
			name:         "developer login success",
			username:     "developer",
			password:     "dev123",
			wantStatus:   http.StatusOK,
			wantToken:    true,
			wantUsername: "developer",
		},
		{
			name:       "invalid password",
			username:   "admin",
			password:   "wrongpassword",
			wantStatus: http.StatusUnauthorized,
			wantToken:  false,
		},
		{
			name:       "invalid username",
			username:   "nonexistent",
			password:   "password",
			wantStatus: http.StatusUnauthorized,
			wantToken:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loginReq := map[string]string{
				"username": tt.username,
				"password": tt.password,
			}
			body, _ := json.Marshal(loginReq)

			req := httptest.NewRequest("POST", "/api/login", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.handleLogin(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantToken {
				var response map[string]interface{}
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				if response["token"] == nil {
					t.Error("response should contain token")
				}

				if response["username"] != nil && response["username"].(string) != tt.wantUsername {
					t.Errorf("username = %v, want %s", response["username"], tt.wantUsername)
				}

				if response["expires_at"] == nil {
					t.Error("response should contain expires_at")
				}
			}
		})
	}
}

func TestHandleLogin_MissingFields(t *testing.T) {
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

	tests := []struct {
		name       string
		body       map[string]string
		wantStatus int
	}{
		{
			name: "missing username",
			body: map[string]string{
				"password": "admin123",
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "missing password",
			body: map[string]string{
				"username": "admin",
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "empty body",
			body:       map[string]string{},
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)

			req := httptest.NewRequest("POST", "/api/login", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.handleLogin(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func BenchmarkRespondJSON(b *testing.B) {
	data := map[string]interface{}{
		"key1": "value1",
		"key2": 123,
		"key3": true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		respondJSON(w, http.StatusOK, data)
	}
}

func BenchmarkHandleLogin(b *testing.B) {
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

	loginReq := map[string]string{
		"username": "admin",
		"password": "admin123",
	}
	body, _ := json.Marshal(loginReq)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/api/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		server.handleLogin(w, req)
	}
}
