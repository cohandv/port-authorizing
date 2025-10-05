package proxy

import (
	"testing"

	"github.com/davidcohan/port-authorizing/internal/config"
)

func TestNewPostgresAuthProxy(t *testing.T) {
	connConfig := &config.ConnectionConfig{
		Name:            "test-postgres",
		Type:            "postgres",
		Host:            "localhost",
		Port:            5432,
		BackendUsername: "testuser",
		BackendPassword: "testpass",
		BackendDatabase: "testdb",
	}

	globalConfig := &config.Config{
		Logging: config.LoggingConfig{
			AuditLogPath: "",
			LogLevel:     "info",
		},
	}

	whitelist := []string{"^SELECT.*", "^INSERT.*"}

	proxy := NewPostgresAuthProxy(connConfig, "", "user1", "conn-123", globalConfig, whitelist)

	if proxy == nil {
		t.Fatal("NewPostgresAuthProxy() returned nil")
	}

	if proxy.config != connConfig {
		t.Error("config not set correctly")
	}

	if proxy.username != "user1" {
		t.Errorf("username = %s, want 'user1'", proxy.username)
	}

	if proxy.connectionID != "conn-123" {
		t.Errorf("connectionID = %s, want 'conn-123'", proxy.connectionID)
	}

	if len(proxy.whitelist) != 2 {
		t.Errorf("whitelist length = %d, want 2", len(proxy.whitelist))
	}
}

func TestNewPostgresAuthProxy_EmptyWhitelist(t *testing.T) {
	connConfig := &config.ConnectionConfig{
		Name:            "test-postgres",
		Type:            "postgres",
		Host:            "localhost",
		Port:            5432,
		BackendUsername: "testuser",
		BackendPassword: "testpass",
		BackendDatabase: "testdb",
	}

	globalConfig := &config.Config{
		Logging: config.LoggingConfig{
			AuditLogPath: "",
			LogLevel:     "info",
		},
	}

	proxy := NewPostgresAuthProxy(connConfig, "", "user1", "conn-123", globalConfig, []string{})

	if proxy == nil {
		t.Fatal("NewPostgresAuthProxy() returned nil")
	}

	if len(proxy.whitelist) != 0 {
		t.Errorf("whitelist should be empty, got %d items", len(proxy.whitelist))
	}
}

func TestPostgresAuthProxy_IsQueryAllowed(t *testing.T) {
	connConfig := &config.ConnectionConfig{
		Name: "test-postgres",
		Type: "postgres",
		Host: "localhost",
		Port: 5432,
	}

	globalConfig := &config.Config{
		Logging: config.LoggingConfig{
			AuditLogPath: "",
			LogLevel:     "info",
		},
	}

	tests := []struct {
		name      string
		whitelist []string
		query     string
		want      bool
	}{
		{
			name:      "SELECT allowed",
			whitelist: []string{"^SELECT.*"},
			query:     "SELECT * FROM users",
			want:      true,
		},
		{
			name:      "DELETE blocked",
			whitelist: []string{"^SELECT.*"},
			query:     "DELETE FROM users",
			want:      false,
		},
		{
			name:      "case insensitive",
			whitelist: []string{"^SELECT.*"},
			query:     "select * from users",
			want:      true,
		},
		{
			name:      "empty whitelist allows all",
			whitelist: []string{},
			query:     "DELETE FROM users",
			want:      true,
		},
		{
			name:      "multiple patterns",
			whitelist: []string{"^SELECT.*", "^INSERT.*", "^UPDATE.*"},
			query:     "UPDATE users SET name='test'",
			want:      true,
		},
		{
			name:      "complex pattern",
			whitelist: []string{"^SELECT.*FROM users WHERE id.*"},
			query:     "SELECT * FROM users WHERE id=1",
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxy := NewPostgresAuthProxy(connConfig, "", "user1", "conn-123", globalConfig, tt.whitelist)
			got := proxy.isQueryAllowed(tt.query)

			if got != tt.want {
				t.Errorf("isQueryAllowed() = %v, want %v for query: %s", got, tt.want, tt.query)
			}
		})
	}
}

func TestPostgresAuthProxy_IsQueryAllowed_InvalidRegex(t *testing.T) {
	connConfig := &config.ConnectionConfig{
		Name: "test-postgres",
		Type: "postgres",
		Host: "localhost",
		Port: 5432,
	}

	globalConfig := &config.Config{
		Logging: config.LoggingConfig{
			AuditLogPath: "",
			LogLevel:     "info",
		},
	}

	// Invalid regex pattern
	whitelist := []string{"[invalid(regex"}
	proxy := NewPostgresAuthProxy(connConfig, "", "user1", "conn-123", globalConfig, whitelist)

	// Should not crash with invalid regex, should return false
	allowed := proxy.isQueryAllowed("SELECT * FROM users")
	if allowed {
		t.Error("isQueryAllowed() should return false for invalid regex pattern")
	}
}

func BenchmarkPostgresAuthProxy_IsQueryAllowed(b *testing.B) {
	connConfig := &config.ConnectionConfig{
		Name: "test-postgres",
		Type: "postgres",
		Host: "localhost",
		Port: 5432,
	}

	globalConfig := &config.Config{
		Logging: config.LoggingConfig{
			AuditLogPath: "",
			LogLevel:     "info",
		},
	}

	whitelist := []string{"^SELECT.*", "^INSERT.*", "^UPDATE.*"}
	proxy := NewPostgresAuthProxy(connConfig, "", "user1", "conn-123", globalConfig, whitelist)
	query := "SELECT * FROM users WHERE id=1"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proxy.isQueryAllowed(query)
	}
}
