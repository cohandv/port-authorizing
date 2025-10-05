package proxy

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/davidcohan/port-authorizing/internal/config"
)

func TestNewHTTPProxyWithWhitelist(t *testing.T) {
	cfg := &config.ConnectionConfig{
		Name:   "test-api",
		Type:   "http",
		Host:   "localhost",
		Port:   8080,
		Scheme: "http",
	}

	whitelist := []string{"^GET /api/.*", "^POST /api/users"}
	tmpFile, _ := os.CreateTemp("", "audit-*.log")
	defer os.Remove(tmpFile.Name())

	proxy := NewHTTPProxyWithWhitelist(cfg, whitelist, tmpFile.Name(), "testuser", "conn-123")

	if proxy == nil {
		t.Fatal("NewHTTPProxyWithWhitelist() returned nil")
	}

	if len(proxy.whitelist) != 2 {
		t.Errorf("whitelist length = %d, want 2", len(proxy.whitelist))
	}

	if proxy.username != "testuser" {
		t.Errorf("username = %s, want 'testuser'", proxy.username)
	}

	if proxy.connectionID != "conn-123" {
		t.Errorf("connectionID = %s, want 'conn-123'", proxy.connectionID)
	}
}

func TestHTTPProxy_isRequestAllowed(t *testing.T) {
	tmpFile, _ := os.CreateTemp("", "audit-*.log")
	defer os.Remove(tmpFile.Name())

	tests := []struct {
		name      string
		whitelist []string
		request   string
		want      bool
	}{
		{
			name:      "GET request matches GET pattern",
			whitelist: []string{"^GET /api/.*"},
			request:   "GET /api/users",
			want:      true,
		},
		{
			name:      "GET request matches multiple patterns",
			whitelist: []string{"^GET /api/users", "^GET /api/.*"},
			request:   "GET /api/items",
			want:      true,
		},
		{
			name:      "POST request blocked by GET-only whitelist",
			whitelist: []string{"^GET /api/.*"},
			request:   "POST /api/users",
			want:      false,
		},
		{
			name:      "DELETE request blocked",
			whitelist: []string{"^GET /api/.*", "^POST /api/users"},
			request:   "DELETE /api/users/1",
			want:      false,
		},
		{
			name:      "PUT request blocked",
			whitelist: []string{"^GET /api/.*"},
			request:   "PUT /api/users/1",
			want:      false,
		},
		{
			name:      "PATCH request blocked",
			whitelist: []string{"^GET /api/.*"},
			request:   "PATCH /api/users/1",
			want:      false,
		},
		{
			name:      "specific endpoint pattern",
			whitelist: []string{"^GET /api/users/[0-9]+$"},
			request:   "GET /api/users/123",
			want:      true,
		},
		{
			name:      "specific endpoint pattern - wrong endpoint",
			whitelist: []string{"^GET /api/users/[0-9]+$"},
			request:   "GET /api/users/abc",
			want:      false,
		},
		{
			name:      "case insensitive HTTP method",
			whitelist: []string{"^GET /api/.*"},
			request:   "get /api/users",
			want:      true,
		},
		{
			name:      "case insensitive HTTP method - mixed case",
			whitelist: []string{"^GET /api/.*"},
			request:   "gEt /api/users",
			want:      true,
		},
		{
			name:      "empty whitelist allows all",
			whitelist: []string{},
			request:   "DELETE /api/everything",
			want:      true,
		},
		{
			name:      "nil whitelist allows all",
			whitelist: nil,
			request:   "DELETE /api/everything",
			want:      true,
		},
		{
			name:      "wildcard pattern allows all",
			whitelist: []string{".*"},
			request:   "DELETE /api/users/1",
			want:      true,
		},
		{
			name:      "multiple verbs for same endpoint",
			whitelist: []string{"^(GET|POST|PUT) /api/users.*"},
			request:   "POST /api/users",
			want:      true,
		},
		{
			name:      "query parameters in path",
			whitelist: []string{"^GET /api/users\\?.*"},
			request:   "GET /api/users?page=1",
			want:      true,
		},
		{
			name:      "complex path pattern",
			whitelist: []string{"^GET /api/v[0-9]+/users.*"},
			request:   "GET /api/v1/users/123",
			want:      true,
		},
		{
			name:      "HEAD request",
			whitelist: []string{"^(GET|HEAD) /api/.*"},
			request:   "HEAD /api/users",
			want:      true,
		},
		{
			name:      "OPTIONS request",
			whitelist: []string{"^OPTIONS /api/.*"},
			request:   "OPTIONS /api/users",
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxy := &HTTPProxy{
				config: &config.ConnectionConfig{
					Name: "test-api",
					Type: "http",
				},
				whitelist:    tt.whitelist,
				auditLogPath: tmpFile.Name(),
				username:     "testuser",
				connectionID: "conn-123",
			}

			got := proxy.isRequestAllowed(tt.request)
			if got != tt.want {
				t.Errorf("isRequestAllowed() = %v, want %v\nRequest: %s\nWhitelist: %v",
					got, tt.want, tt.request, tt.whitelist)
			}
		})
	}
}

func TestHTTPProxy_isRequestAllowed_InvalidRegex(t *testing.T) {
	tmpFile, _ := os.CreateTemp("", "audit-*.log")
	defer os.Remove(tmpFile.Name())

	proxy := &HTTPProxy{
		config: &config.ConnectionConfig{
			Name: "test-api",
			Type: "http",
		},
		whitelist:    []string{"[invalid regex"},
		auditLogPath: tmpFile.Name(),
		username:     "testuser",
		connectionID: "conn-123",
	}

	// Should not panic and should return false for invalid regex
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("isRequestAllowed() panicked with invalid regex: %v", r)
		}
	}()

	got := proxy.isRequestAllowed("GET /api/users")
	if got {
		t.Error("isRequestAllowed() with invalid regex should return false")
	}

	// Verify error was logged
	time.Sleep(100 * time.Millisecond)
	content, _ := os.ReadFile(tmpFile.Name())
	if len(content) == 0 {
		t.Error("Expected error to be logged for invalid regex")
	}
}

func TestHTTPProxy_HandleRequest_WithWhitelist(t *testing.T) {
	// Create a test HTTP server to act as backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message":"success"}`))
	}))
	defer backend.Close()

	tmpFile, _ := os.CreateTemp("", "audit-*.log")
	defer os.Remove(tmpFile.Name())

	// Parse backend URL
	backendHost := "localhost"
	backendPort := 8080 // The actual backend test server will override this

	cfg := &config.ConnectionConfig{
		Name:   "test-api",
		Type:   "http",
		Host:   backendHost,
		Port:   backendPort,
		Scheme: "http",
	}

	tests := []struct {
		name           string
		whitelist      []string
		requestMethod  string
		requestPath    string
		expectedStatus int
		shouldBlock    bool
	}{
		{
			name:           "allowed GET request",
			whitelist:      []string{"^GET /api/.*"},
			requestMethod:  "GET",
			requestPath:    "/api/users",
			expectedStatus: http.StatusOK,
			shouldBlock:    false,
		},
		{
			name:           "blocked POST request",
			whitelist:      []string{"^GET /api/.*"},
			requestMethod:  "POST",
			requestPath:    "/api/users",
			expectedStatus: http.StatusForbidden,
			shouldBlock:    true,
		},
		{
			name:           "blocked DELETE request",
			whitelist:      []string{"^GET /api/.*", "^POST /api/users"},
			requestMethod:  "DELETE",
			requestPath:    "/api/users/1",
			expectedStatus: http.StatusForbidden,
			shouldBlock:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxy := NewHTTPProxyWithWhitelist(cfg, tt.whitelist, tmpFile.Name(), "testuser", "conn-123")

			// Create a request body in HTTP format
			requestBody := bytes.NewBuffer([]byte(tt.requestMethod + " " + tt.requestPath + " HTTP/1.1\r\n\r\n"))

			req := httptest.NewRequest("POST", "/proxy/conn-123", requestBody)
			w := httptest.NewRecorder()

			err := proxy.HandleRequest(w, req)

			if tt.shouldBlock {
				// For blocked requests, we expect an error and 403 status
				if err == nil {
					t.Error("Expected error for blocked request, got nil")
				}
				if w.Code != http.StatusForbidden {
					t.Errorf("Expected status %d for blocked request, got %d", http.StatusForbidden, w.Code)
				}
			} else {
				// For allowed requests, behavior depends on if backend is reachable
				// Since we're using a mock backend that might not be properly connected,
				// we mainly check that the request wasn't blocked
				if w.Code == http.StatusForbidden {
					t.Error("Request was blocked when it should have been allowed")
				}
			}
		})
	}
}

func BenchmarkHTTPProxy_isRequestAllowed(b *testing.B) {
	tmpFile, _ := os.CreateTemp("", "audit-*.log")
	defer os.Remove(tmpFile.Name())

	proxy := &HTTPProxy{
		config: &config.ConnectionConfig{
			Name: "test-api",
			Type: "http",
		},
		whitelist:    []string{"^GET /api/.*", "^POST /api/users", "^PUT /api/users/[0-9]+"},
		auditLogPath: tmpFile.Name(),
		username:     "testuser",
		connectionID: "conn-123",
	}

	request := "GET /api/users/123"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proxy.isRequestAllowed(request)
	}
}

func BenchmarkHTTPProxy_isRequestAllowed_NoMatch(b *testing.B) {
	tmpFile, _ := os.CreateTemp("", "audit-*.log")
	defer os.Remove(tmpFile.Name())

	proxy := &HTTPProxy{
		config: &config.ConnectionConfig{
			Name: "test-api",
			Type: "http",
		},
		whitelist:    []string{"^GET /api/.*", "^POST /api/users"},
		auditLogPath: tmpFile.Name(),
		username:     "testuser",
		connectionID: "conn-123",
	}

	request := "DELETE /api/users/123"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proxy.isRequestAllowed(request)
	}
}

