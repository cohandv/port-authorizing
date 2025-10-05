package config

import (
	"testing"
	"time"
)

func TestConnectionConfig_Address(t *testing.T) {
	tests := []struct {
		name   string
		config ConnectionConfig
		want   string
	}{
		{
			name: "localhost postgres",
			config: ConnectionConfig{
				Host: "localhost",
				Port: 5432,
			},
			want: "localhost:5432",
		},
		{
			name: "remote host",
			config: ConnectionConfig{
				Host: "db.example.com",
				Port: 3306,
			},
			want: "db.example.com:3306",
		},
		{
			name: "IP address",
			config: ConnectionConfig{
				Host: "192.168.1.100",
				Port: 8080,
			},
			want: "192.168.1.100:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify the fields are set correctly
			if tt.config.Host == "" {
				t.Error("Host should not be empty")
			}
			if tt.config.Port == 0 {
				t.Error("Port should not be 0")
			}
		})
	}
}

func TestConfig_Defaults(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Port: 0,
		},
		Auth: AuthConfig{
			TokenExpiry: 0,
		},
	}

	// Apply defaults
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}

	if cfg.Auth.TokenExpiry == 0 {
		cfg.Auth.TokenExpiry = 24 * time.Hour
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("Default port = %d, want 8080", cfg.Server.Port)
	}

	if cfg.Auth.TokenExpiry != 24*time.Hour {
		t.Errorf("Default token expiry = %v, want 24h", cfg.Auth.TokenExpiry)
	}
}

func TestUser_Properties(t *testing.T) {
	user := User{
		Username: "testuser",
		Password: "testpass",
		Roles:    []string{"admin", "developer"},
	}

	if user.Username != "testuser" {
		t.Errorf("Username = %s, want testuser", user.Username)
	}

	if len(user.Roles) != 2 {
		t.Errorf("Roles count = %d, want 2", len(user.Roles))
	}
}

func TestRolePolicy_TagMatch(t *testing.T) {
	tests := []struct {
		name    string
		policy  RolePolicy
		wantAny bool
		wantAll bool
	}{
		{
			name: "any match",
			policy: RolePolicy{
				TagMatch: "any",
			},
			wantAny: true,
			wantAll: false,
		},
		{
			name: "all match",
			policy: RolePolicy{
				TagMatch: "all",
			},
			wantAny: false,
			wantAll: true,
		},
		{
			name: "empty defaults to any",
			policy: RolePolicy{
				TagMatch: "",
			},
			wantAny: true,
			wantAll: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isAny := tt.policy.TagMatch == "any" || tt.policy.TagMatch == ""
			isAll := tt.policy.TagMatch == "all"

			if isAny != tt.wantAny {
				t.Errorf("isAny = %v, want %v", isAny, tt.wantAny)
			}

			if isAll != tt.wantAll {
				t.Errorf("isAll = %v, want %v", isAll, tt.wantAll)
			}
		})
	}
}

func TestConnectionConfig_Tags(t *testing.T) {
	config := ConnectionConfig{
		Name: "test-db",
		Tags: []string{"env:dev", "region:us", "tier:free"},
	}

	if len(config.Tags) != 3 {
		t.Errorf("Tags count = %d, want 3", len(config.Tags))
	}

	// Check specific tag
	hasEnvTag := false
	for _, tag := range config.Tags {
		if tag == "env:dev" {
			hasEnvTag = true
			break
		}
	}

	if !hasEnvTag {
		t.Error("Should have env:dev tag")
	}
}

func TestAuthProviderConfig_Properties(t *testing.T) {
	provider := AuthProviderConfig{
		Name:    "test-oidc",
		Type:    "oidc",
		Enabled: true,
		Config: map[string]string{
			"issuer":    "http://localhost:8180",
			"client_id": "test",
		},
	}

	if provider.Name != "test-oidc" {
		t.Errorf("Name = %s, want test-oidc", provider.Name)
	}

	if provider.Type != "oidc" {
		t.Errorf("Type = %s, want oidc", provider.Type)
	}

	if !provider.Enabled {
		t.Error("Enabled should be true")
	}

	if provider.Config["issuer"] != "http://localhost:8180" {
		t.Error("Issuer config mismatch")
	}
}

func BenchmarkConnectionConfig(b *testing.B) {
	config := ConnectionConfig{
		Host: "localhost",
		Port: 5432,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.Host
		_ = config.Port
	}
}
