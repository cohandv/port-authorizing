package authorization

import (
	"testing"

	"github.com/davidcohan/port-authorizing/internal/config"
)

func TestNewAuthorizer(t *testing.T) {
	cfg := &config.Config{
		Policies: []config.RolePolicy{
			{
				Name:  "admin-all",
				Roles: []string{"admin"},
				Tags:  []string{"env:production"},
			},
			{
				Name:  "dev-test",
				Roles: []string{"developer"},
				Tags:  []string{"env:test"},
			},
		},
		Connections: []config.ConnectionConfig{
			{
				Name: "postgres-test",
				Type: "postgres",
				Tags: []string{"env:test"},
			},
		},
	}

	authz := NewAuthorizer(cfg)

	if authz == nil {
		t.Fatal("NewAuthorizer() returned nil")
	}

	if len(authz.policies) != 2 {
		t.Errorf("NewAuthorizer() policies count = %d, want 2", len(authz.policies))
	}

	if len(authz.connections) != 1 {
		t.Errorf("NewAuthorizer() connections count = %d, want 1", len(authz.connections))
	}
}

func TestAuthorizer_CanAccessConnection(t *testing.T) {
	cfg := &config.Config{
		Policies: []config.RolePolicy{
			{
				Name:     "admin-all",
				Roles:    []string{"admin"},
				Tags:     []string{"env:production", "env:test"},
				TagMatch: "any",
			},
			{
				Name:  "dev-test",
				Roles: []string{"developer"},
				Tags:  []string{"env:test"},
			},
			{
				Name:     "dev-prod-readonly",
				Roles:    []string{"developer"},
				Tags:     []string{"env:production", "readonly"},
				TagMatch: "all",
			},
		},
		Connections: []config.ConnectionConfig{
			{
				Name: "postgres-test",
				Type: "postgres",
				Tags: []string{"env:test"},
			},
			{
				Name: "postgres-prod",
				Type: "postgres",
				Tags: []string{"env:production"},
			},
			{
				Name: "postgres-prod-readonly",
				Type: "postgres",
				Tags: []string{"env:production", "readonly"},
			},
			{
				Name: "no-tags",
				Type: "http",
			},
		},
	}

	authz := NewAuthorizer(cfg)

	tests := []struct {
		name           string
		roles          []string
		connectionName string
		want           bool
	}{
		{
			name:           "admin can access test",
			roles:          []string{"admin"},
			connectionName: "postgres-test",
			want:           true,
		},
		{
			name:           "admin can access prod",
			roles:          []string{"admin"},
			connectionName: "postgres-prod",
			want:           true,
		},
		{
			name:           "developer can access test",
			roles:          []string{"developer"},
			connectionName: "postgres-test",
			want:           true,
		},
		{
			name:           "developer cannot access prod (missing readonly tag)",
			roles:          []string{"developer"},
			connectionName: "postgres-prod",
			want:           false,
		},
		{
			name:           "developer can access prod-readonly (has both tags)",
			roles:          []string{"developer"},
			connectionName: "postgres-prod-readonly",
			want:           true,
		},
		{
			name:           "multiple roles - admin wins",
			roles:          []string{"developer", "admin"},
			connectionName: "postgres-prod",
			want:           true,
		},
		{
			name:           "non-existent connection",
			roles:          []string{"admin"},
			connectionName: "non-existent",
			want:           false,
		},
		{
			name:           "empty roles",
			roles:          []string{},
			connectionName: "postgres-test",
			want:           false,
		},
		{
			name:           "role with no policies",
			roles:          []string{"unknown-role"},
			connectionName: "postgres-test",
			want:           false,
		},
		{
			name:           "no-tags connection cannot be accessed",
			roles:          []string{"admin"},
			connectionName: "no-tags",
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := authz.CanAccessConnection(tt.roles, tt.connectionName)
			if got != tt.want {
				t.Errorf("CanAccessConnection() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthorizer_GetWhitelistForConnection(t *testing.T) {
	cfg := &config.Config{
		Policies: []config.RolePolicy{
			{
				Name:      "admin-all",
				Roles:     []string{"admin"},
				Tags:      []string{"env:production"},
				Whitelist: []string{".*"},
			},
			{
				Name:      "dev-test",
				Roles:     []string{"developer"},
				Tags:      []string{"env:test"},
				Whitelist: []string{"^SELECT.*", "^EXPLAIN.*"},
			},
			{
				Name:      "dev-prod",
				Roles:     []string{"developer"},
				Tags:      []string{"env:production"},
				Whitelist: []string{"^SELECT.*"},
			},
		},
		Connections: []config.ConnectionConfig{
			{
				Name: "postgres-test",
				Type: "postgres",
				Tags: []string{"env:test"},
			},
			{
				Name: "postgres-prod",
				Type: "postgres",
				Tags: []string{"env:production"},
			},
			{
				Name:      "legacy-conn",
				Type:      "postgres",
				Whitelist: []string{"^SELECT.*"},
			},
		},
	}

	authz := NewAuthorizer(cfg)

	tests := []struct {
		name           string
		roles          []string
		connectionName string
		wantPatterns   []string
	}{
		{
			name:           "admin gets full access",
			roles:          []string{"admin"},
			connectionName: "postgres-prod",
			wantPatterns:   []string{".*"},
		},
		{
			name:           "developer gets SELECT and EXPLAIN for test",
			roles:          []string{"developer"},
			connectionName: "postgres-test",
			wantPatterns:   []string{"^SELECT.*", "^EXPLAIN.*"},
		},
		{
			name:           "developer gets SELECT only for prod",
			roles:          []string{"developer"},
			connectionName: "postgres-prod",
			wantPatterns:   []string{"^SELECT.*"},
		},
		{
			name:           "legacy connection uses direct whitelist",
			roles:          []string{"admin"},
			connectionName: "legacy-conn",
			wantPatterns:   []string{"^SELECT.*"},
		},
		{
			name:           "non-existent connection returns nil",
			roles:          []string{"admin"},
			connectionName: "non-existent",
			wantPatterns:   nil,
		},
		{
			name:           "empty roles returns empty",
			roles:          []string{},
			connectionName: "postgres-test",
			wantPatterns:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := authz.GetWhitelistForConnection(tt.roles, tt.connectionName)

			if tt.wantPatterns == nil {
				if got != nil && len(got) > 0 {
					t.Errorf("GetWhitelistForConnection() = %v, want nil or empty", got)
				}
				return
			}

			if len(got) != len(tt.wantPatterns) {
				t.Errorf("GetWhitelistForConnection() returned %d patterns, want %d", len(got), len(tt.wantPatterns))
				t.Errorf("Got: %v, Want: %v", got, tt.wantPatterns)
				return
			}

			// Check if all wanted patterns are present (order doesn't matter due to map iteration)
			wantMap := make(map[string]bool)
			for _, pattern := range tt.wantPatterns {
				wantMap[pattern] = true
			}

			for _, pattern := range got {
				if !wantMap[pattern] {
					t.Errorf("GetWhitelistForConnection() contains unexpected pattern: %s", pattern)
				}
			}
		})
	}
}

func TestAuthorizer_ValidatePattern(t *testing.T) {
	authz := &Authorizer{}

	tests := []struct {
		name      string
		query     string
		whitelist []string
		wantErr   bool
	}{
		{
			name:      "valid SELECT query",
			query:     "SELECT * FROM users",
			whitelist: []string{"^SELECT.*"},
			wantErr:   false,
		},
		{
			name:      "invalid DELETE query",
			query:     "DELETE FROM users",
			whitelist: []string{"^SELECT.*"},
			wantErr:   true,
		},
		{
			name:      "empty whitelist allows all",
			query:     "DROP DATABASE production",
			whitelist: []string{},
			wantErr:   false,
		},
		{
			name:      "invalid regex pattern",
			query:     "SELECT * FROM users",
			whitelist: []string{"[invalid"},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := authz.ValidatePattern(tt.query, tt.whitelist)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePattern() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAuthorizer_ListAccessibleConnections(t *testing.T) {
	cfg := &config.Config{
		Policies: []config.RolePolicy{
			{
				Name:     "admin-all",
				Roles:    []string{"admin"},
				Tags:     []string{"env:production", "env:test"},
				TagMatch: "any",
			},
			{
				Name:  "dev-test",
				Roles: []string{"developer"},
				Tags:  []string{"env:test"},
			},
		},
		Connections: []config.ConnectionConfig{
			{Name: "postgres-test", Tags: []string{"env:test"}},
			{Name: "postgres-prod", Tags: []string{"env:production"}},
			{Name: "api-test", Tags: []string{"env:test"}},
		},
	}

	authz := NewAuthorizer(cfg)

	tests := []struct {
		name      string
		roles     []string
		wantCount int
		wantConns map[string]bool
	}{
		{
			name:      "admin sees all",
			roles:     []string{"admin"},
			wantCount: 3,
			wantConns: map[string]bool{
				"postgres-test": true,
				"postgres-prod": true,
				"api-test":      true,
			},
		},
		{
			name:      "developer sees test only",
			roles:     []string{"developer"},
			wantCount: 2,
			wantConns: map[string]bool{
				"postgres-test": true,
				"api-test":      true,
			},
		},
		{
			name:      "no roles sees nothing",
			roles:     []string{},
			wantCount: 0,
			wantConns: map[string]bool{},
		},
		{
			name:      "unknown role sees nothing",
			roles:     []string{"unknown"},
			wantCount: 0,
			wantConns: map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := authz.ListAccessibleConnections(tt.roles)

			if len(got) != tt.wantCount {
				t.Errorf("ListAccessibleConnections() returned %d connections, want %d", len(got), tt.wantCount)
			}

			for _, conn := range got {
				if !tt.wantConns[conn] {
					t.Errorf("ListAccessibleConnections() returned unexpected connection: %s", conn)
				}
			}
		})
	}
}

func TestAuthorizer_GetConnectionInfo(t *testing.T) {
	cfg := &config.Config{
		Connections: []config.ConnectionConfig{
			{
				Name:     "postgres-test",
				Type:     "postgres",
				Host:     "localhost",
				Port:     5432,
				Tags:     []string{"env:test"},
				Metadata: map[string]string{"description": "Test database"},
			},
		},
	}

	authz := NewAuthorizer(cfg)

	t.Run("existing connection", func(t *testing.T) {
		info := authz.GetConnectionInfo("postgres-test")

		if info == nil {
			t.Fatal("GetConnectionInfo() returned nil")
		}

		if info["name"] != "postgres-test" {
			t.Errorf("name = %v, want 'postgres-test'", info["name"])
		}

		if info["type"] != "postgres" {
			t.Errorf("type = %v, want 'postgres'", info["type"])
		}

		if info["host"] != "localhost" {
			t.Errorf("host = %v, want 'localhost'", info["host"])
		}

		if info["port"] != 5432 {
			t.Errorf("port = %v, want 5432", info["port"])
		}
	})

	t.Run("non-existent connection", func(t *testing.T) {
		info := authz.GetConnectionInfo("non-existent")

		if info != nil {
			t.Errorf("GetConnectionInfo() = %v, want nil", info)
		}
	})
}

func BenchmarkCanAccessConnection(b *testing.B) {
	cfg := &config.Config{
		Policies: []config.RolePolicy{
			{Name: "admin", Roles: []string{"admin"}, Tags: []string{"env:production"}},
			{Name: "dev", Roles: []string{"developer"}, Tags: []string{"env:test"}},
		},
		Connections: []config.ConnectionConfig{
			{Name: "postgres-test", Tags: []string{"env:test"}},
		},
	}

	authz := NewAuthorizer(cfg)
	roles := []string{"developer"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		authz.CanAccessConnection(roles, "postgres-test")
	}
}

func BenchmarkGetWhitelistForConnection(b *testing.B) {
	cfg := &config.Config{
		Policies: []config.RolePolicy{
			{Name: "dev", Roles: []string{"developer"}, Tags: []string{"env:test"}, Whitelist: []string{"^SELECT.*", "^EXPLAIN.*"}},
		},
		Connections: []config.ConnectionConfig{
			{Name: "postgres-test", Tags: []string{"env:test"}},
		},
	}

	authz := NewAuthorizer(cfg)
	roles := []string{"developer"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		authz.GetWhitelistForConnection(roles, "postgres-test")
	}
}
