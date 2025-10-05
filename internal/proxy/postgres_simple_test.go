package proxy

import (
	"testing"

	"github.com/davidcohan/port-authorizing/internal/config"
)

func TestNewSimplePostgresProxy(t *testing.T) {
	connConfig := &config.ConnectionConfig{
		Name:            "test-postgres",
		Type:            "postgres",
		Host:            "localhost",
		Port:            5432,
		BackendUsername: "testuser",
		BackendPassword: "testpass",
		BackendDatabase: "testdb",
	}

	proxy := NewSimplePostgresProxy(connConfig, "", "", "")

	if proxy == nil {
		t.Fatal("NewSimplePostgresProxy() returned nil")
	}

	if proxy.config != connConfig {
		t.Error("config not set correctly")
	}

	if proxy.username != "" {
		t.Errorf("username = %s, want empty", proxy.username)
	}

	if proxy.connectionID != "" {
		t.Errorf("connectionID = %s, want empty", proxy.connectionID)
	}
}

func BenchmarkNewSimplePostgresProxy(b *testing.B) {
	connConfig := &config.ConnectionConfig{
		Name:            "test-postgres",
		Type:            "postgres",
		Host:            "localhost",
		Port:            5432,
		BackendUsername: "testuser",
		BackendPassword: "testpass",
		BackendDatabase: "testdb",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewSimplePostgresProxy(connConfig, "", "", "")
	}
}
