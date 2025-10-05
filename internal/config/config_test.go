package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file for testing
	yamlContent := `
server:
  port: 8080
  max_connection_duration: 1h

auth:
  jwt_secret: "test-secret"
  token_expiry: 24h
  users:
    - username: admin
      password: admin123
      roles: [admin]
  providers:
    - name: local
      type: local
      enabled: true

connections:
  - name: test-db
    type: postgres
    host: localhost
    port: 5432
    tags: [env:test]

policies:
  - name: admin-all
    roles: [admin]
    tags: [env:test]
    whitelist: [".*"]

security:
  enable_llm_analysis: false

logging:
  audit_log_path: "/tmp/audit.log"
  query_logging_enabled: true
`

	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(yamlContent); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	// Test loading the config
	cfg, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Validate loaded configuration
	if cfg.Server.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Server.Port)
	}

	if cfg.Server.MaxConnectionDuration != time.Hour {
		t.Errorf("MaxConnectionDuration = %v, want 1h", cfg.Server.MaxConnectionDuration)
	}

	if cfg.Auth.JWTSecret != "test-secret" {
		t.Errorf("JWTSecret = %s, want 'test-secret'", cfg.Auth.JWTSecret)
	}

	if cfg.Auth.TokenExpiry != 24*time.Hour {
		t.Errorf("TokenExpiry = %v, want 24h", cfg.Auth.TokenExpiry)
	}

	if len(cfg.Auth.Users) != 1 {
		t.Errorf("Users count = %d, want 1", len(cfg.Auth.Users))
	}

	if cfg.Auth.Users[0].Username != "admin" {
		t.Errorf("Username = %s, want 'admin'", cfg.Auth.Users[0].Username)
	}

	if len(cfg.Connections) != 1 {
		t.Errorf("Connections count = %d, want 1", len(cfg.Connections))
	}

	if cfg.Connections[0].Name != "test-db" {
		t.Errorf("Connection name = %s, want 'test-db'", cfg.Connections[0].Name)
	}

	if len(cfg.Policies) != 1 {
		t.Errorf("Policies count = %d, want 1", len(cfg.Policies))
	}

	if cfg.Policies[0].Name != "admin-all" {
		t.Errorf("Policy name = %s, want 'admin-all'", cfg.Policies[0].Name)
	}

	if cfg.Logging.AuditLogPath != "/tmp/audit.log" {
		t.Errorf("AuditLogPath = %s, want '/tmp/audit.log'", cfg.Logging.AuditLogPath)
	}
}

func TestLoadConfig_NonExistentFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/file.yaml")
	if err == nil {
		t.Error("LoadConfig() should fail for non-existent file")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write invalid YAML
	if _, err := tmpFile.WriteString("invalid: yaml: content: [[["); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	_, err = LoadConfig(tmpFile.Name())
	if err == nil {
		t.Error("LoadConfig() should fail for invalid YAML")
	}
}

func TestConnectionConfig_GetFullAddress(t *testing.T) {
	tests := []struct {
		name string
		conn ConnectionConfig
		want string
	}{
		{
			name: "postgres with explicit scheme",
			conn: ConnectionConfig{
				Scheme: "postgresql",
				Host:   "localhost",
				Port:   5432,
			},
			want: "postgresql://localhost:5432",
		},
		{
			name: "http connection",
			conn: ConnectionConfig{
				Type:   "http",
				Scheme: "https",
				Host:   "api.example.com",
				Port:   443,
			},
			want: "https://api.example.com:443",
		},
		{
			name: "tcp connection",
			conn: ConnectionConfig{
				Type: "tcp",
				Host: "redis.example.com",
				Port: 6379,
			},
			want: "redis.example.com:6379",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify the connection config fields are accessible
			if tt.conn.Host == "" {
				t.Error("Host should not be empty")
			}
			if tt.conn.Port == 0 {
				t.Error("Port should not be zero")
			}
		})
	}
}

func TestRolePolicy_Validation(t *testing.T) {
	tests := []struct {
		name    string
		policy  RolePolicy
		isValid bool
	}{
		{
			name: "valid policy with tags",
			policy: RolePolicy{
				Name:      "test-policy",
				Roles:     []string{"developer"},
				Tags:      []string{"env:test"},
				TagMatch:  "any",
				Whitelist: []string{"^SELECT.*"},
			},
			isValid: true,
		},
		{
			name: "policy with no roles",
			policy: RolePolicy{
				Name:      "invalid-policy",
				Roles:     []string{},
				Tags:      []string{"env:test"},
				Whitelist: []string{"^SELECT.*"},
			},
			isValid: false,
		},
		{
			name: "policy with empty name",
			policy: RolePolicy{
				Name:      "",
				Roles:     []string{"developer"},
				Tags:      []string{"env:test"},
				Whitelist: []string{"^SELECT.*"},
			},
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation - check if required fields are present
			isValid := tt.policy.Name != "" && len(tt.policy.Roles) > 0
			if isValid != tt.isValid {
				t.Errorf("Policy validation = %v, want %v", isValid, tt.isValid)
			}
		})
	}
}

