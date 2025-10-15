package api

import (
	"testing"
	"time"

	"github.com/davidcohan/port-authorizing/internal/config"
)

func TestNewServer_WithVariousConfigs(t *testing.T) {
	tests := []struct {
		name    string
		config  *config.Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &config.Config{
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
			},
			wantErr: false,
		},
		{
			name: "config with connections",
			config: &config.Config{
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
						Name:      "admin-policy",
						Roles:     []string{"admin"},
						Tags:      []string{"env:test"},
						Whitelist: []string{".*"},
					},
				},
				Logging: config.LoggingConfig{
					AuditLogPath: "",
					LogLevel:     "info",
				},
			},
			wantErr: false,
		},
		{
			name: "config with OIDC provider",
			config: &config.Config{
				Server: config.ServerConfig{
					Port: 8080,
				},
				Auth: config.AuthConfig{
					JWTSecret:   "test-secret",
					TokenExpiry: 24 * time.Hour,
					Users: []config.User{
						{Username: "admin", Password: "admin123", Roles: []string{"admin"}},
					},
					Providers: []config.AuthProviderConfig{
						{
							Name:    "test-oidc",
							Type:    "oidc",
							Enabled: false, // Disabled so it doesn't try to connect
							Config: map[string]string{
								"issuer":         "http://localhost:8180/realms/test",
								"client_id":      "test",
								"client_secret":  "secret",
								"redirect_url":   "http://localhost:8080/callback",
								"roles_claim":    "roles",
								"username_claim": "preferred_username",
							},
						},
					},
				},
				Logging: config.LoggingConfig{
					AuditLogPath: "",
					LogLevel:     "info",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(tt.config)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewServer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if server == nil {
					t.Fatal("NewServer() returned nil")
				}

				if server.config != tt.config {
					t.Error("config not set correctly")
				}

				if server.router == nil {
					t.Error("router should be initialized")
				}

				if server.connMgr == nil {
					t.Error("connection manager should be initialized")
				}

				if server.authz == nil {
					t.Error("authorizer should be initialized")
				}
			}
		})
	}
}

func TestServer_SetupRoutes(t *testing.T) {
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

	// Routes should be set up during server creation
	if server.router == nil {
		t.Fatal("Router not initialized")
	}

	// Verify router was configured (has routes)
	// We can't easily inspect gorilla/mux routes, so just verify router exists
	if server.router == nil {
		t.Error("setupRoutes() should initialize router")
	}
}

func BenchmarkNewServer(b *testing.B) {
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = NewServer(cfg)
	}
}
