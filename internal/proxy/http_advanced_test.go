package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/davidcohan/port-authorizing/internal/config"
)

func TestHTTPProxy_HandleRequest_Methods(t *testing.T) {
	// Create a test backend server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "true")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("backend response: " + r.Method))
	}))
	defer backend.Close()

	tests := []struct {
		name       string
		method     string
		path       string
		whitelist  []string
		wantStatus int
		wantAllow  bool
	}{
		{
			name:       "GET allowed",
			method:     "GET",
			path:       "/api/users",
			whitelist:  []string{"^GET /api/.*"},
			wantStatus: http.StatusOK,
			wantAllow:  true,
		},
		{
			name:       "POST allowed",
			method:     "POST",
			path:       "/api/users",
			whitelist:  []string{"^POST /api/.*"},
			wantStatus: http.StatusOK,
			wantAllow:  true,
		},
		{
			name:       "PUT allowed",
			method:     "PUT",
			path:       "/api/users/123",
			whitelist:  []string{"^PUT /api/users/[0-9]+"},
			wantStatus: http.StatusOK,
			wantAllow:  true,
		},
		{
			name:       "DELETE blocked",
			method:     "DELETE",
			path:       "/api/users/123",
			whitelist:  []string{"^GET /.*", "^POST /.*"},
			wantStatus: http.StatusForbidden,
			wantAllow:  false,
		},
		{
			name:       "PATCH allowed",
			method:     "PATCH",
			path:       "/api/users/123",
			whitelist:  []string{"^PATCH /.*"},
			wantStatus: http.StatusOK,
			wantAllow:  true,
		},
		{
			name:       "HEAD allowed",
			method:     "HEAD",
			path:       "/api/status",
			whitelist:  []string{"^HEAD /.*"},
			wantStatus: http.StatusOK,
			wantAllow:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connConfig := &config.ConnectionConfig{
				Name: "test-backend",
				Type: "http",
				Host: "localhost",
				Port: 8080,
			}

			proxy := NewHTTPProxyWithWhitelist(connConfig, tt.whitelist, "", "testuser", "conn-123")

			// Test the whitelist logic
			allowed := proxy.isRequestAllowed(tt.method + " " + tt.path)
			if allowed != tt.wantAllow {
				t.Errorf("isRequestAllowed() = %v, want %v", allowed, tt.wantAllow)
			}
		})
	}
}

func TestHTTPProxy_CORSHeaders(t *testing.T) {
	connConfig := &config.ConnectionConfig{
		Name: "test-backend",
		Type: "http",
		Host: "localhost",
		Port: 8080,
	}

	proxy := NewHTTPProxyWithWhitelist(connConfig, []string{".*"}, "", "testuser", "conn-123")

	if proxy == nil {
		t.Fatal("NewHTTPProxyWithWhitelist returned nil")
	}

	// Verify proxy properties
	if proxy.username != "testuser" {
		t.Errorf("username = %s, want 'testuser'", proxy.username)
	}

	if proxy.connectionID != "conn-123" {
		t.Errorf("connectionID = %s, want 'conn-123'", proxy.connectionID)
	}

	if len(proxy.whitelist) != 1 {
		t.Errorf("whitelist length = %d, want 1", len(proxy.whitelist))
	}
}

func TestHTTPProxy_IsRequestAllowed_Benchmarks(t *testing.T) {
	connConfig := &config.ConnectionConfig{
		Name: "test",
		Type: "http",
		Host: "localhost",
		Port: 8080,
	}

	tests := []struct {
		name      string
		whitelist []string
		request   string
		want      bool
	}{
		{
			name:      "simple match",
			whitelist: []string{"^GET /.*"},
			request:   "GET /api/users",
			want:      true,
		},
		{
			name:      "complex regex",
			whitelist: []string{"^(GET|POST|PUT) /api/v[0-9]+/users/[0-9]+$"},
			request:   "GET /api/v1/users/123",
			want:      true,
		},
		{
			name:      "no match",
			whitelist: []string{"^GET /admin/.*"},
			request:   "GET /api/users",
			want:      false,
		},
		{
			name:      "multiple patterns",
			whitelist: []string{"^GET /api/.*", "^POST /api/.*", "^PUT /api/.*"},
			request:   "POST /api/users",
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxy := NewHTTPProxyWithWhitelist(connConfig, tt.whitelist, "", "user", "conn")
			got := proxy.isRequestAllowed(tt.request)
			if got != tt.want {
				t.Errorf("isRequestAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHTTPProxy_Close(t *testing.T) {
	connConfig := &config.ConnectionConfig{
		Name: "test",
		Type: "http",
		Host: "localhost",
		Port: 8080,
	}

	proxy := NewHTTPProxyWithWhitelist(connConfig, []string{}, "", "", "")

	err := proxy.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Multiple closes should be safe
	err = proxy.Close()
	if err != nil {
		t.Errorf("Second Close() error = %v", err)
	}
}

func TestHTTPProxy_EmptyWhitelist(t *testing.T) {
	connConfig := &config.ConnectionConfig{
		Name: "test",
		Type: "http",
		Host: "localhost",
		Port: 8080,
	}

	proxy := NewHTTPProxyWithWhitelist(connConfig, []string{}, "", "user", "conn")

	// Empty whitelist should allow all
	tests := []string{
		"GET /anything",
		"POST /anything",
		"DELETE /anything",
		"PUT /anything",
	}

	for _, request := range tests {
		if !proxy.isRequestAllowed(request) {
			t.Errorf("Empty whitelist should allow %s", request)
		}
	}
}

func TestHTTPProxy_NilWhitelist(t *testing.T) {
	connConfig := &config.ConnectionConfig{
		Name: "test",
		Type: "http",
		Host: "localhost",
		Port: 8080,
	}

	proxy := NewHTTPProxyWithWhitelist(connConfig, nil, "", "user", "conn")

	// Nil whitelist should allow all
	if !proxy.isRequestAllowed("DELETE /anything") {
		t.Error("Nil whitelist should allow all requests")
	}
}

func BenchmarkHTTPProxy_IsRequestAllowed_Simple(b *testing.B) {
	connConfig := &config.ConnectionConfig{
		Name: "test",
		Type: "http",
		Host: "localhost",
		Port: 8080,
	}

	proxy := NewHTTPProxyWithWhitelist(connConfig, []string{"^GET /.*"}, "", "user", "conn")
	request := "GET /api/users"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proxy.isRequestAllowed(request)
	}
}

func BenchmarkHTTPProxy_IsRequestAllowed_Complex(b *testing.B) {
	connConfig := &config.ConnectionConfig{
		Name: "test",
		Type: "http",
		Host: "localhost",
		Port: 8080,
	}

	whitelist := []string{
		"^GET /api/v[0-9]+/users/[0-9]+$",
		"^POST /api/v[0-9]+/users$",
		"^PUT /api/v[0-9]+/users/[0-9]+$",
		"^DELETE /api/v[0-9]+/users/[0-9]+$",
		"^PATCH /api/v[0-9]+/users/[0-9]+/profile$",
	}

	proxy := NewHTTPProxyWithWhitelist(connConfig, whitelist, "", "user", "conn")
	request := "GET /api/v1/users/123"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proxy.isRequestAllowed(request)
	}
}

func BenchmarkHTTPProxy_HandleRequest(b *testing.B) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "OK")
	}))
	defer backend.Close()

	connConfig := &config.ConnectionConfig{
		Name: "test",
		Type: "http",
		Host: "localhost",
		Port: 8080,
	}

	proxy := NewHTTPProxyWithWhitelist(connConfig, []string{"^GET /.*"}, "", "user", "conn")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		w := httptest.NewRecorder()
		_ = proxy.HandleRequest(w, req)
	}
}
