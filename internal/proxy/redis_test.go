package proxy

import (
	"net"
	"testing"

	"github.com/davidcohan/port-authorizing/internal/config"
)

func TestRedisProxy_ValidateCommand(t *testing.T) {
	tests := []struct {
		name      string
		whitelist []string
		cmd       *RedisCommand
		wantErr   bool
	}{
		{
			name:      "no whitelist allows all",
			whitelist: []string{},
			cmd:       &RedisCommand{Command: "GET", Args: []string{"key"}},
			wantErr:   false,
		},
		{
			name:      "GET allowed by wildcard",
			whitelist: []string{"GET *"},
			cmd:       &RedisCommand{Command: "GET", Args: []string{"anykey"}},
			wantErr:   false,
		},
		{
			name:      "GET blocked by prefix",
			whitelist: []string{"GET myapp:*"},
			cmd:       &RedisCommand{Command: "GET", Args: []string{"otherapp:key"}},
			wantErr:   true,
		},
		{
			name:      "SET allowed by prefix",
			whitelist: []string{"SET myapp:*"},
			cmd:       &RedisCommand{Command: "SET", Args: []string{"myapp:session:123", "value"}},
			wantErr:   false,
		},
		{
			name:      "DEL blocked - not in whitelist",
			whitelist: []string{"GET *", "SET *"},
			cmd:       &RedisCommand{Command: "DEL", Args: []string{"key"}},
			wantErr:   true,
		},
		{
			name:      "HGET allowed with hash pattern",
			whitelist: []string{"HGET users:*"},
			cmd:       &RedisCommand{Command: "HGET", Args: []string{"users:123", "name"}},
			wantErr:   false,
		},
		{
			name:      "multiple patterns - first matches",
			whitelist: []string{"GET myapp:*", "SET myapp:*", "DEL myapp:*"},
			cmd:       &RedisCommand{Command: "GET", Args: []string{"myapp:key"}},
			wantErr:   false,
		},
		{
			name:      "multiple patterns - second matches",
			whitelist: []string{"GET myapp:*", "SET myapp:*", "DEL myapp:*"},
			cmd:       &RedisCommand{Command: "SET", Args: []string{"myapp:key", "value"}},
			wantErr:   false,
		},
		{
			name:      "command only pattern",
			whitelist: []string{"PING", "INFO"},
			cmd:       &RedisCommand{Command: "PING", Args: []string{}},
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxy := &RedisProxy{
				config: &config.ConnectionConfig{
					Name: "test-redis",
					Type: "redis",
					Host: "localhost",
					Port: 6379,
				},
				auditLogPath: "",
				username:     "testuser",
				connectionID: "test-conn-id",
				whitelist:    tt.whitelist,
			}

			err := proxy.validateCommand(tt.cmd)

			if tt.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}

			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestNewRedisProxy(t *testing.T) {
	cfg := &config.ConnectionConfig{
		Name:            "test-redis",
		Type:            "redis",
		Host:            "localhost",
		Port:            6379,
		BackendPassword: "secret",
	}

	proxy := NewRedisProxy(cfg, "/tmp/audit.log", "testuser", "conn-123", []string{"GET *"})

	if proxy == nil {
		t.Fatal("expected non-nil proxy")
	}

	if proxy.config != cfg {
		t.Errorf("config not set correctly")
	}

	if proxy.username != "testuser" {
		t.Errorf("username = %q, want %q", proxy.username, "testuser")
	}

	if proxy.connectionID != "conn-123" {
		t.Errorf("connectionID = %q, want %q", proxy.connectionID, "conn-123")
	}

	if len(proxy.whitelist) != 1 || proxy.whitelist[0] != "GET *" {
		t.Errorf("whitelist not set correctly: %v", proxy.whitelist)
	}
}

func TestRedisProxy_SetApprovalManager(t *testing.T) {
	proxy := NewRedisProxy(
		&config.ConnectionConfig{Name: "test", Type: "redis", Host: "localhost", Port: 6379},
		"",
		"testuser",
		"conn-id",
		nil,
	)

	if proxy.approvalMgr != nil {
		t.Error("expected nil approval manager initially")
	}

	// Note: We can't easily test with a real approval.Manager without circular dependencies
	// This test mainly ensures the method exists and doesn't panic
	proxy.SetApprovalManager(nil)

	if proxy.approvalMgr != nil {
		t.Error("expected approval manager to remain nil")
	}
}

func TestRedisClusterProxy_ValidateCommand(t *testing.T) {
	tests := []struct {
		name      string
		whitelist []string
		cmd       *RedisCommand
		wantErr   bool
	}{
		{
			name:      "no whitelist allows all",
			whitelist: []string{},
			cmd:       &RedisCommand{Command: "GET", Args: []string{"key"}},
			wantErr:   false,
		},
		{
			name:      "GET allowed",
			whitelist: []string{"GET *"},
			cmd:       &RedisCommand{Command: "GET", Args: []string{"key"}},
			wantErr:   false,
		},
		{
			name:      "SET blocked",
			whitelist: []string{"GET *"},
			cmd:       &RedisCommand{Command: "SET", Args: []string{"key", "value"}},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxy := &RedisClusterProxy{
				config: &config.ConnectionConfig{
					Name:         "test-redis-cluster",
					Type:         "redis",
					Host:         "localhost",
					Port:         7000,
					RedisCluster: true,
				},
				auditLogPath: "",
				username:     "testuser",
				connectionID: "test-conn-id",
				whitelist:    tt.whitelist,
				nodeConns:    make(map[string]net.Conn),
			}

			err := proxy.validateCommand(tt.cmd)

			if tt.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}

			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestNewRedisClusterProxy(t *testing.T) {
	cfg := &config.ConnectionConfig{
		Name:            "test-redis-cluster",
		Type:            "redis",
		Host:            "localhost",
		Port:            7000,
		BackendPassword: "secret",
		RedisCluster:    true,
	}

	proxy := NewRedisClusterProxy(cfg, "/tmp/audit.log", "testuser", "conn-123", []string{"GET *"})

	if proxy == nil {
		t.Fatal("expected non-nil proxy")
	}

	if proxy.config != cfg {
		t.Errorf("config not set correctly")
	}

	if proxy.username != "testuser" {
		t.Errorf("username = %q, want %q", proxy.username, "testuser")
	}

	if proxy.connectionID != "conn-123" {
		t.Errorf("connectionID = %q, want %q", proxy.connectionID, "conn-123")
	}

	if len(proxy.whitelist) != 1 || proxy.whitelist[0] != "GET *" {
		t.Errorf("whitelist not set correctly: %v", proxy.whitelist)
	}

	if proxy.nodeConns == nil {
		t.Error("expected nodeConns map to be initialized")
	}
}

func TestRedisClusterProxy_ParseRedirectAddress(t *testing.T) {
	proxy := &RedisClusterProxy{}

	tests := []struct {
		name    string
		msg     string
		want    string
		wantErr bool
	}{
		{
			name:    "MOVED redirect",
			msg:     "MOVED 3999 127.0.0.1:6381",
			want:    "127.0.0.1:6381",
			wantErr: false,
		},
		{
			name:    "ASK redirect",
			msg:     "ASK 3999 127.0.0.1:6382",
			want:    "127.0.0.1:6382",
			wantErr: false,
		},
		{
			name:    "invalid format - too few parts",
			msg:     "MOVED 3999",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid format - empty",
			msg:     "",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := proxy.parseRedirectAddress(tt.msg)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Errorf("parseRedirectAddress(%q) = %q, want %q", tt.msg, got, tt.want)
			}
		})
	}
}

func BenchmarkRedisProxy_ValidateCommand(b *testing.B) {
	proxy := &RedisProxy{
		config: &config.ConnectionConfig{
			Name: "test-redis",
			Type: "redis",
			Host: "localhost",
			Port: 6379,
		},
		whitelist: []string{"GET myapp:*", "SET myapp:*", "HGET users:*"},
	}

	cmd := &RedisCommand{
		Command: "GET",
		Args:    []string{"myapp:session:12345"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = proxy.validateCommand(cmd)
	}
}
