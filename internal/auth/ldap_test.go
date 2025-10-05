package auth

import (
	"testing"

	"github.com/davidcohan/port-authorizing/internal/config"
)

func TestNewLDAPProvider(t *testing.T) {
	tests := []struct {
		name    string
		config  config.AuthProviderConfig
		wantErr bool
	}{
		{
			name: "valid LDAP config",
			config: config.AuthProviderConfig{
				Name: "test-ldap",
				Type: "ldap",
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
			wantErr: false,
		},
		{
			name: "missing url",
			config: config.AuthProviderConfig{
				Name: "test-ldap",
				Type: "ldap",
				Config: map[string]string{
					"bind_dn":       "cn=admin,dc=test,dc=local",
					"bind_password": "password",
					"user_base_dn":  "ou=users,dc=test,dc=local",
					"user_filter":   "(uid=%s)",
				},
			},
			wantErr: true,
		},
		{
			name: "missing bind_dn",
			config: config.AuthProviderConfig{
				Name: "test-ldap",
				Type: "ldap",
				Config: map[string]string{
					"url":           "localhost:389",
					"bind_password": "password",
					"user_base_dn":  "ou=users,dc=test,dc=local",
					"user_filter":   "(uid=%s)",
				},
			},
			wantErr: true,
		},
		{
			name: "missing user_base_dn",
			config: config.AuthProviderConfig{
				Name: "test-ldap",
				Type: "ldap",
				Config: map[string]string{
					"url":           "localhost:389",
					"bind_dn":       "cn=admin,dc=test,dc=local",
					"bind_password": "password",
					"user_filter":   "(uid=%s)",
				},
			},
			wantErr: true,
		},
		{
			name: "missing user_filter (uses default)",
			config: config.AuthProviderConfig{
				Name: "test-ldap",
				Type: "ldap",
				Config: map[string]string{
					"url":           "localhost:389",
					"bind_dn":       "cn=admin,dc=test,dc=local",
					"bind_password": "password",
					"user_base_dn":  "ou=users,dc=test,dc=local",
				},
			},
			wantErr: false, // Has default value (uid=%s)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewLDAPProvider(tt.config)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewLDAPProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if provider == nil {
					t.Fatal("NewLDAPProvider() returned nil")
				}

				if provider.Name() != tt.config.Name {
					t.Errorf("Name() = %s, want %s", provider.Name(), tt.config.Name)
				}

				if provider.Type() != "ldap" {
					t.Errorf("Type() = %s, want 'ldap'", provider.Type())
				}
			}
		})
	}
}

func TestLDAPProvider_Authenticate(t *testing.T) {
	provider, _ := NewLDAPProvider(config.AuthProviderConfig{
		Name: "test-ldap",
		Type: "ldap",
		Config: map[string]string{
			"url":           "localhost:389",
			"bind_dn":       "cn=admin,dc=test,dc=local",
			"bind_password": "password",
			"user_base_dn":  "ou=users,dc=test,dc=local",
			"user_filter":   "(uid=%s)",
		},
	})

	tests := []struct {
		name        string
		credentials map[string]string
		wantErr     bool
	}{
		{
			name: "missing username",
			credentials: map[string]string{
				"password": "password",
			},
			wantErr: true,
		},
		{
			name: "missing password",
			credentials: map[string]string{
				"username": "testuser",
			},
			wantErr: true,
		},
		{
			name: "valid credentials (server not running)",
			credentials: map[string]string{
				"username": "testuser",
				"password": "password",
			},
			wantErr: true, // LDAP server not running in unit tests
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

func TestLDAPProvider_Methods(t *testing.T) {
	provider, _ := NewLDAPProvider(config.AuthProviderConfig{
		Name: "test-ldap",
		Type: "ldap",
		Config: map[string]string{
			"url":           "localhost:389",
			"bind_dn":       "cn=admin,dc=test,dc=local",
			"bind_password": "password",
			"user_base_dn":  "ou=users,dc=test,dc=local",
			"user_filter":   "(uid=%s)",
		},
	})

	if provider.Name() != "test-ldap" {
		t.Errorf("Name() = %s, want 'test-ldap'", provider.Name())
	}

	if provider.Type() != "ldap" {
		t.Errorf("Type() = %s, want 'ldap'", provider.Type())
	}
}
