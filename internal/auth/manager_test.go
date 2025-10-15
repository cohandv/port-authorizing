package auth

import (
	"testing"

	"github.com/davidcohan/port-authorizing/internal/config"
)

func TestNewManager(t *testing.T) {
	tests := []struct {
		name    string
		config  *config.Config
		wantErr bool
		wantLen int
	}{
		{
			name: "create manager with local users",
			config: &config.Config{
				Auth: config.AuthConfig{
					Users: []config.User{
						{Username: "admin", Password: "admin123", Roles: []string{"admin"}},
					},
				},
			},
			wantErr: false,
			wantLen: 1,
		},
		{
			name: "create manager with no providers",
			config: &config.Config{
				Auth: config.AuthConfig{
					Users:     []config.User{},
					Providers: []config.AuthProviderConfig{},
				},
			},
			wantErr: false, // Now server starts even with no providers
			wantLen: 0,
		},
		{
			name: "create manager with OIDC provider (gracefully handles unreachable server)",
			config: &config.Config{
				Auth: config.AuthConfig{
					Providers: []config.AuthProviderConfig{
						{
							Name:    "test-oidc",
							Type:    "oidc",
							Enabled: true,
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
			},
			wantErr: false, // Now succeeds - provider is skipped if unreachable
			wantLen: 0,     // No providers initialized (OIDC server not running)
		},
		{
			name: "create manager with LDAP provider",
			config: &config.Config{
				Auth: config.AuthConfig{
					Providers: []config.AuthProviderConfig{
						{
							Name:    "test-ldap",
							Type:    "ldap",
							Enabled: true,
							Config: map[string]string{
								"url":             "localhost:389",
								"bind_dn":         "cn=admin,dc=test,dc=local",
								"bind_password":   "password",
								"user_base_dn":    "ou=users,dc=test,dc=local",
								"user_filter":     "(uid=%s)",
								"group_base_dn":   "ou=groups,dc=test,dc=local",
								"group_filter":    "(member=%s)",
								"use_tls":         "false",
								"skip_tls_verify": "true",
							},
						},
					},
				},
			},
			wantErr: false,
			wantLen: 1,
		},
		{
			name: "create manager with SAML2 provider",
			config: &config.Config{
				Auth: config.AuthConfig{
					Providers: []config.AuthProviderConfig{
						{
							Name:    "test-saml",
							Type:    "saml2",
							Enabled: true,
							Config: map[string]string{
								"idp_metadata_url": "http://localhost:8080/metadata",
								"sp_entity_id":     "port-auth",
								"sp_acs_url":       "http://localhost:8080/callback",
							},
						},
					},
				},
			},
			wantErr: false,
			wantLen: 1,
		},
		{
			name: "disabled provider should be skipped",
			config: &config.Config{
				Auth: config.AuthConfig{
					Users: []config.User{
						{Username: "admin", Password: "admin123", Roles: []string{"admin"}},
					},
					Providers: []config.AuthProviderConfig{
						{
							Name:    "test-oidc",
							Type:    "oidc",
							Enabled: false,
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
			},
			wantErr: false,
			wantLen: 1, // Only local provider
		},
		{
			name: "multiple providers (local only when OIDC server not running)",
			config: &config.Config{
				Auth: config.AuthConfig{
					Users: []config.User{
						{Username: "admin", Password: "admin123", Roles: []string{"admin"}},
					},
					Providers: []config.AuthProviderConfig{
						{
							Name:    "test-oidc",
							Type:    "oidc",
							Enabled: true,
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
			},
			wantErr: false, // Now succeeds - OIDC skipped, local provider works
			wantLen: 1,     // Only local provider (OIDC unreachable)
		},
		{
			name: "unsupported provider type",
			config: &config.Config{
				Auth: config.AuthConfig{
					Providers: []config.AuthProviderConfig{
						{
							Name:    "test-unknown",
							Type:    "unknown",
							Enabled: true,
							Config:  map[string]string{},
						},
					},
				},
			},
			wantErr: false, // Now succeeds - unknown provider skipped with warning
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewManager(tt.config)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if manager == nil {
					t.Fatal("NewManager() returned nil")
				}

				providers := manager.GetProviders()
				if len(providers) != tt.wantLen {
					t.Errorf("NewManager() providers count = %d, want %d", len(providers), tt.wantLen)
				}
			}
		})
	}
}

func TestManager_GetProviders(t *testing.T) {
	config := &config.Config{
		Auth: config.AuthConfig{
			Users: []config.User{
				{Username: "admin", Password: "admin123", Roles: []string{"admin"}},
				{Username: "user", Password: "user123", Roles: []string{"user"}},
			},
		},
	}

	manager, err := NewManager(config)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	providers := manager.GetProviders()

	if providers == nil {
		t.Fatal("GetProviders() returned nil")
	}

	if len(providers) != 1 {
		t.Errorf("GetProviders() count = %d, want 1", len(providers))
	}

	if providers[0].Type() != "local" {
		t.Errorf("GetProviders()[0].Type() = %s, want 'local'", providers[0].Type())
	}
}

func TestNewLocalProviderFromConfig(t *testing.T) {
	cfg := config.AuthProviderConfig{
		Name:    "test-local",
		Type:    "local",
		Enabled: true,
		Config:  map[string]string{},
	}

	provider, err := NewLocalProviderFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewLocalProviderFromConfig() error = %v", err)
	}

	if provider == nil {
		t.Fatal("NewLocalProviderFromConfig() returned nil")
	}

	if provider.Name() != "test-local" {
		t.Errorf("Name() = %s, want 'test-local'", provider.Name())
	}

	if provider.Type() != "local" {
		t.Errorf("Type() = %s, want 'local'", provider.Type())
	}
}

func BenchmarkNewManager(b *testing.B) {
	config := &config.Config{
		Auth: config.AuthConfig{
			Users: []config.User{
				{Username: "admin", Password: "admin123", Roles: []string{"admin"}},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = NewManager(config)
	}
}
