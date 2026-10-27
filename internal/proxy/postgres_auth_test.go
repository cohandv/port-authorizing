package proxy

import (
	"io"
	"net"
	"testing"

	"github.com/davidcohan/port-authorizing/internal/config"
)

func TestNewPostgresAuthProxy(t *testing.T) {
	connConfig := &config.ConnectionConfig{
		Name:            "test-postgres",
		Type:            "postgres",
		Host:            "localhost",
		Port:            5432,
		BackendUsername: config.ConfigSource{Type: config.ConfigSourceTypePlain, Value: "testuser"},
		BackendPassword: config.ConfigSource{Type: config.ConfigSourceTypePlain, Value: "testpass"},
		BackendDatabase: "testdb",
	}

	globalConfig := &config.Config{
		Logging: config.LoggingConfig{
			AuditLogPath: "",
			LogLevel:     "info",
		},
	}

	whitelist := []string{"^SELECT.*", "^INSERT.*"}

	roles := []string{"admin", "developer"}
	proxy := NewPostgresAuthProxy(connConfig, "", "user1", "conn-123", roles, globalConfig, whitelist, nil)

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

	if len(proxy.roles) != 2 {
		t.Errorf("roles length = %d, want 2", len(proxy.roles))
	}
}

func TestNewPostgresAuthProxy_EmptyWhitelist(t *testing.T) {
	connConfig := &config.ConnectionConfig{
		Name:            "test-postgres",
		Type:            "postgres",
		Host:            "localhost",
		Port:            5432,
		BackendUsername: config.ConfigSource{Type: config.ConfigSourceTypePlain, Value: "testuser"},
		BackendPassword: config.ConfigSource{Type: config.ConfigSourceTypePlain, Value: "testpass"},
		BackendDatabase: "testdb",
	}

	globalConfig := &config.Config{
		Logging: config.LoggingConfig{
			AuditLogPath: "",
			LogLevel:     "info",
		},
	}

	proxy := NewPostgresAuthProxy(connConfig, "", "user1", "conn-123", nil, globalConfig, []string{}, nil)

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
			name:      "second statement checked too",
			whitelist: []string{"^SELECT.*"},
			query:     "SELECT 1; TRUNCATE users",
			want:      false,
		},
		{
			name:      "semicolons inside literals are not statements",
			whitelist: []string{"^SELECT.*", "^WITH.*"},
			query:     "with s as (select 1) select 'id=' || x || '; a=' || y || '; b=' from s;",
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
			proxy := NewPostgresAuthProxy(connConfig, "", "user1", "conn-123", nil, globalConfig, tt.whitelist, nil)
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
	proxy := NewPostgresAuthProxy(connConfig, "", "user1", "conn-123", nil, globalConfig, whitelist, nil)

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
	proxy := NewPostgresAuthProxy(connConfig, "", "user1", "conn-123", nil, globalConfig, whitelist, nil)
	query := "SELECT * FROM users WHERE id=1"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proxy.isQueryAllowed(query)
	}
}

func TestSendBackendStartup_BackendParams(t *testing.T) {
	cfg := &config.ConnectionConfig{BackendParams: map[string]string{
		"statement_timeout": "30s",
		"application_name":  "pa",
		"user":              "evil",
	}}
	p := NewPostgresAuthProxy(cfg, "", "u", "id", nil, &config.Config{}, nil, nil)

	client, server := net.Pipe()
	go func() { _ = p.sendBackendStartup(client, "backend", "db"); _ = client.Close() }()
	msg, _ := io.ReadAll(server)

	want := "user\x00backend\x00database\x00db\x00application_name\x00pa\x00statement_timeout\x0030s\x00\x00"
	if got := string(msg[8:]); got != want {
		t.Errorf("startup params = %q, want %q", got, want)
	}
}

func TestSendBackendStartup_RejectsInvalidParams(t *testing.T) {
	for _, params := range []map[string]string{{"": "x"}, {"a\x00b": "x"}, {"search_path": "public\x00evil"}} {
		p := NewPostgresAuthProxy(&config.ConnectionConfig{BackendParams: params}, "", "u", "id", nil, &config.Config{}, nil, nil)
		client, server := net.Pipe()
		go func() { _, _ = io.Copy(io.Discard, server) }()
		if err := p.sendBackendStartup(client, "backend", "db"); err == nil {
			t.Errorf("%q: want error", params)
		}
		_ = client.Close()
	}
}
