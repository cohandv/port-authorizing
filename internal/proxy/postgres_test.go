package proxy

import (
	"testing"

	"github.com/davidcohan/port-authorizing/internal/config"
)

func TestNewPostgresProxy(t *testing.T) {
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

	proxy := NewPostgresProxy(connConfig, "", "user1", "conn-123", globalConfig)

	if proxy == nil {
		t.Fatal("NewPostgresProxy() returned nil")
	}

	if proxy.config != connConfig {
		t.Error("config not set correctly")
	}
}

func TestPostgresProxy_Properties(t *testing.T) {
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

	proxy := NewPostgresProxy(connConfig, "/tmp/audit.log", "user1", "conn-123", globalConfig)

	if proxy.username != "user1" {
		t.Errorf("username = %s, want 'user1'", proxy.username)
	}

	if proxy.connectionID != "conn-123" {
		t.Errorf("connectionID = %s, want 'conn-123'", proxy.connectionID)
	}

	if proxy.auditLogPath != "/tmp/audit.log" {
		t.Errorf("auditLogPath = %s, want '/tmp/audit.log'", proxy.auditLogPath)
	}
}

func TestNewSimpleChunkReader(t *testing.T) {
	reader := newSimpleChunkReader(nil)

	if reader == nil {
		t.Fatal("newSimpleChunkReader() returned nil")
	}
}

func BenchmarkNewPostgresProxy(b *testing.B) {
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewPostgresProxy(connConfig, "", "user1", "conn-123", globalConfig)
	}
}
