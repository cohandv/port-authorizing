package auth

import (
	"testing"

	"github.com/davidcohan/port-authorizing/internal/config"
)

func TestNewSAML2Provider(t *testing.T) {
	tests := []struct {
		name    string
		config  config.AuthProviderConfig
		wantErr bool
	}{
		{
			name: "valid SAML2 config",
			config: config.AuthProviderConfig{
				Name: "test-saml",
				Type: "saml2",
				Config: map[string]string{
					"idp_metadata_url": "http://localhost:8080/metadata",
					"sp_entity_id":     "port-auth",
					"sp_acs_url":       "http://localhost:8080/callback",
				},
			},
			wantErr: false,
		},
		{
			name: "missing idp_metadata_url",
			config: config.AuthProviderConfig{
				Name: "test-saml",
				Type: "saml2",
				Config: map[string]string{
					"sp_entity_id": "port-auth",
					"sp_acs_url":   "http://localhost:8080/callback",
				},
			},
			wantErr: true,
		},
		{
			name: "missing sp_entity_id",
			config: config.AuthProviderConfig{
				Name: "test-saml",
				Type: "saml2",
				Config: map[string]string{
					"idp_metadata_url": "http://localhost:8080/metadata",
					"sp_acs_url":       "http://localhost:8080/callback",
				},
			},
			wantErr: true,
		},
		{
			name: "missing sp_acs_url",
			config: config.AuthProviderConfig{
				Name: "test-saml",
				Type: "saml2",
				Config: map[string]string{
					"idp_metadata_url": "http://localhost:8080/metadata",
					"sp_entity_id":     "port-auth",
				},
			},
			wantErr: true,
		},
		{
			name: "empty config",
			config: config.AuthProviderConfig{
				Name:   "test-saml",
				Type:   "saml2",
				Config: map[string]string{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewSAML2Provider(tt.config)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewSAML2Provider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if provider == nil {
					t.Fatal("NewSAML2Provider() returned nil")
				}

				if provider.Name() != tt.config.Name {
					t.Errorf("Name() = %s, want %s", provider.Name(), tt.config.Name)
				}

				if provider.Type() != "saml2" {
					t.Errorf("Type() = %s, want 'saml2'", provider.Type())
				}
			}
		})
	}
}

func TestSAML2Provider_Authenticate(t *testing.T) {
	provider, _ := NewSAML2Provider(config.AuthProviderConfig{
		Name: "test-saml",
		Type: "saml2",
		Config: map[string]string{
			"idp_metadata_url": "http://localhost:8080/metadata",
			"sp_entity_id":     "port-auth",
			"sp_acs_url":       "http://localhost:8080/callback",
		},
	})

	tests := []struct {
		name        string
		credentials map[string]string
		wantErr     bool
	}{
		{
			name: "missing saml_response",
			credentials: map[string]string{
				"other": "value",
			},
			wantErr: true,
		},
		{
			name: "empty saml_response",
			credentials: map[string]string{
				"saml_response": "",
			},
			wantErr: true,
		},
		{
			name: "valid saml_response (not implemented)",
			credentials: map[string]string{
				"saml_response": "base64encodedsamlresponse",
			},
			wantErr: true, // Implementation not complete
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userInfo, err := provider.Authenticate(tt.credentials)

			if (err != nil) != tt.wantErr {
				t.Errorf("Authenticate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && userInfo == nil {
				t.Error("Authenticate() returned nil userInfo")
			}
		})
	}
}

func TestSAML2Provider_Methods(t *testing.T) {
	provider, _ := NewSAML2Provider(config.AuthProviderConfig{
		Name: "test-saml",
		Type: "saml2",
		Config: map[string]string{
			"idp_metadata_url": "http://localhost:8080/metadata",
			"sp_entity_id":     "port-auth",
			"sp_acs_url":       "http://localhost:8080/callback",
		},
	})

	if provider.Name() != "test-saml" {
		t.Errorf("Name() = %s, want 'test-saml'", provider.Name())
	}

	if provider.Type() != "saml2" {
		t.Errorf("Type() = %s, want 'saml2'", provider.Type())
	}
}
