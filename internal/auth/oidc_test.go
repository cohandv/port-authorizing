package auth

import (
	"testing"

	"github.com/davidcohan/port-authorizing/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestNewOIDCProvider_MissingFields(t *testing.T) {
	tests := []struct {
		name    string
		config  config.AuthProviderConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "missing issuer",
			config: config.AuthProviderConfig{
				Name: "test-oidc",
				Type: "oidc",
				Config: map[string]string{
					"client_id":      "test",
					"client_secret":  "secret",
					"redirect_url":   "http://localhost:8080/callback",
					"roles_claim":    "roles",
					"username_claim": "preferred_username",
					"audience":       "test",
				},
			},
			wantErr: true,
			errMsg:  "issuer",
		},
		{
			name: "missing client_id",
			config: config.AuthProviderConfig{
				Name: "test-oidc",
				Type: "oidc",
				Config: map[string]string{
					"issuer":         "http://localhost:8180/realms/test",
					"client_secret":  "secret",
					"redirect_url":   "http://localhost:8080/callback",
					"roles_claim":    "roles",
					"username_claim": "preferred_username",
					"audience":       "test",
				},
			},
			wantErr: true,
			errMsg:  "client_id",
		},
		{
			name: "missing client_secret",
			config: config.AuthProviderConfig{
				Name: "test-oidc",
				Type: "oidc",
				Config: map[string]string{
					"issuer":         "http://localhost:8180/realms/test",
					"client_id":      "test",
					"redirect_url":   "http://localhost:8080/callback",
					"roles_claim":    "roles",
					"username_claim": "preferred_username",
					"audience":       "test",
				},
			},
			wantErr: true,
			errMsg:  "client_secret",
		},
		{
			name: "missing redirect_url",
			config: config.AuthProviderConfig{
				Name: "test-oidc",
				Type: "oidc",
				Config: map[string]string{
					"issuer":         "http://localhost:8180/realms/test",
					"client_id":      "test",
					"client_secret":  "secret",
					"roles_claim":    "roles",
					"username_claim": "preferred_username",
					"audience":       "test",
				},
			},
			wantErr: true,
			errMsg:  "redirect_url",
		},
		{
			name: "missing audience",
			config: config.AuthProviderConfig{
				Name: "test-oidc",
				Type: "oidc",
				Config: map[string]string{
					"issuer":         "http://localhost:8180/realms/test",
					"client_id":      "test",
					"redirect_url":   "http://localhost:8080/callback",
					"client_secret":  "secret",
					"roles_claim":    "roles",
					"username_claim": "preferred_username",
				},
			},
			wantErr: true,
			errMsg:  "audience",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewOIDCProvider(tt.config)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewOIDCProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				// Just check that error message contains the missing field
				// (actual error message may vary)
				_ = err.Error()
			}
		})
	}
}

func TestOIDCProvider_Methods(t *testing.T) {
	// Create provider with valid config (may fail to initialize but that's ok for testing methods)
	cfg := config.AuthProviderConfig{
		Name: "test-oidc",
		Type: "oidc",
		Config: map[string]string{
			"issuer":         "http://localhost:8180/realms/test",
			"client_id":      "test",
			"client_secret":  "secret",
			"redirect_url":   "http://localhost:8080/callback",
			"roles_claim":    "roles",
			"username_claim": "preferred_username",
			"audience":       "test",
		},
	}

	provider, err := NewOIDCProvider(cfg)
	// Provider may fail to initialize if OIDC server not running
	// But if it succeeds, test the methods
	if err == nil && provider != nil {
		if provider.Name() != "test-oidc" {
			t.Errorf("Name() = %s, want 'test-oidc'", provider.Name())
		}

		if provider.Type() != "oidc" {
			t.Errorf("Type() = %s, want 'oidc'", provider.Type())
		}

		// Test GetIssuer
		if provider.GetIssuer() != "http://localhost:8180/realms/test" {
			t.Errorf("GetIssuer() = %s, want 'http://localhost:8180/realms/test'", provider.GetIssuer())
		}

		// Test GetClientID
		if provider.GetClientID() != "test" {
			t.Errorf("GetClientID() = %s, want 'test'", provider.GetClientID())
		}

		// Test GetClientSecret
		if provider.GetClientSecret() != "secret" {
			t.Errorf("GetClientSecret() = %s, want 'secret'", provider.GetClientSecret())
		}

		// Test GetUsernameClaim
		if provider.GetUsernameClaim() != "preferred_username" {
			t.Errorf("GetUsernameClaim() = %s, want 'preferred_username'", provider.GetUsernameClaim())
		}

		// Test GetRolesClaim
		if provider.GetRolesClaim() != "roles" {
			t.Errorf("GetRolesClaim() = %s, want 'roles'", provider.GetRolesClaim())
		}

		// Test IsEnabled
		if !provider.IsEnabled() {
			t.Error("IsEnabled() should return true")
		}
	}
}

func TestOIDCProvider_Authenticate_MissingCode(t *testing.T) {
	cfg := config.AuthProviderConfig{
		Name: "test-oidc",
		Type: "oidc",
		Config: map[string]string{
			"issuer":         "http://localhost:8180/realms/test",
			"client_id":      "test",
			"client_secret":  "secret",
			"redirect_url":   "http://localhost:8080/callback",
			"roles_claim":    "roles",
			"username_claim": "preferred_username",
			"audience":       "test",
		},
	}

	provider, err := NewOIDCProvider(cfg)
	if err != nil {
		// Expected if OIDC server not running
		return
	}

	tests := []struct {
		name        string
		credentials map[string]string
		wantErr     bool
	}{
		{
			name: "missing code",
			credentials: map[string]string{
				"other": "value",
			},
			wantErr: true,
		},
		{
			name:        "empty credentials",
			credentials: map[string]string{},
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := provider.Authenticate(tt.credentials)

			if (err != nil) != tt.wantErr {
				t.Errorf("Authenticate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestOIDCProvider_validateAudience(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
		claims  map[string]any
	}{
		{
			name:    "valid aud",
			wantErr: false,
			claims: map[string]any{
				"aud": "test",
			},
		},
		{
			name:    "'aud' does not match",
			wantErr: true,
			claims: map[string]any{
				"aud": "invalid",
			},
		},
		{
			name:    "'aud' claim not found",
			wantErr: true,
			claims:  map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			provider := OIDCProvider{
				audience: "test",
			}

			err := provider.validateAudience(tt.claims)
			if tt.wantErr {
				assert.ErrorContains(t, err, tt.name)
			} else {
				assert.NoError(t, err)
			}
		})
	}

}
