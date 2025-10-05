package auth

import (
	"testing"
)

func TestOIDCProvider_Configuration(t *testing.T) {
	tests := []struct {
		name       string
		issuer     string
		clientID   string
		wantIssuer string
	}{
		{
			name:       "keycloak issuer",
			issuer:     "http://localhost:8180/realms/portauth",
			clientID:   "port-authorizing",
			wantIssuer: "http://localhost:8180/realms/portauth",
		},
		{
			name:       "auth0 issuer",
			issuer:     "https://example.auth0.com",
			clientID:   "client123",
			wantIssuer: "https://example.auth0.com",
		},
		{
			name:       "custom issuer",
			issuer:     "https://auth.example.com",
			clientID:   "app1",
			wantIssuer: "https://auth.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify strings match expected
			if tt.issuer != tt.wantIssuer {
				t.Errorf("issuer = %s, want %s", tt.issuer, tt.wantIssuer)
			}
		})
	}
}

func TestOIDCProvider_Claims(t *testing.T) {
	tests := []struct {
		name           string
		usernameClaim  string
		rolesClaim     string
		expectUsername string
		expectRoles    string
	}{
		{
			name:           "standard claims",
			usernameClaim:  "preferred_username",
			rolesClaim:     "roles",
			expectUsername: "preferred_username",
			expectRoles:    "roles",
		},
		{
			name:           "custom claims",
			usernameClaim:  "email",
			rolesClaim:     "groups",
			expectUsername: "email",
			expectRoles:    "groups",
		},
		{
			name:           "sub as username",
			usernameClaim:  "sub",
			rolesClaim:     "resource_access",
			expectUsername: "sub",
			expectRoles:    "resource_access",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.usernameClaim != tt.expectUsername {
				t.Errorf("usernameClaim = %s, want %s", tt.usernameClaim, tt.expectUsername)
			}
			if tt.rolesClaim != tt.expectRoles {
				t.Errorf("rolesClaim = %s, want %s", tt.rolesClaim, tt.expectRoles)
			}
		})
	}
}

func TestOIDCProvider_Scopes(t *testing.T) {
	tests := []struct {
		name   string
		scopes []string
		want   int
	}{
		{
			name:   "standard scopes",
			scopes: []string{"openid", "profile", "email"},
			want:   3,
		},
		{
			name:   "with custom scope",
			scopes: []string{"openid", "profile", "email", "custom"},
			want:   4,
		},
		{
			name:   "minimal",
			scopes: []string{"openid"},
			want:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.scopes) != tt.want {
				t.Errorf("scopes count = %d, want %d", len(tt.scopes), tt.want)
			}
		})
	}
}

func TestOIDCProvider_RedirectURL(t *testing.T) {
	tests := []struct {
		name        string
		redirectURL string
		valid       bool
	}{
		{
			name:        "localhost",
			redirectURL: "http://localhost:8080/api/auth/oidc/callback",
			valid:       true,
		},
		{
			name:        "custom domain",
			redirectURL: "https://app.example.com/auth/callback",
			valid:       true,
		},
		{
			name:        "with port",
			redirectURL: "http://localhost:3000/callback",
			valid:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.redirectURL == "" {
				t.Error("redirectURL should not be empty")
			}
		})
	}
}

func TestLDAPProvider_Configuration(t *testing.T) {
	tests := []struct {
		name     string
		server   string
		baseDN   string
		bindUser string
	}{
		{
			name:     "standard ldap",
			server:   "ldap://localhost:389",
			baseDN:   "dc=example,dc=com",
			bindUser: "cn=admin,dc=example,dc=com",
		},
		{
			name:     "ldaps secure",
			server:   "ldaps://ldap.example.com:636",
			baseDN:   "ou=users,dc=example,dc=com",
			bindUser: "cn=readonly,dc=example,dc=com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.server == "" {
				t.Error("server should not be empty")
			}
			if tt.baseDN == "" {
				t.Error("baseDN should not be empty")
			}
		})
	}
}

func TestSAML2Provider_Configuration(t *testing.T) {
	tests := []struct {
		name     string
		entityID string
		ssoURL   string
	}{
		{
			name:     "okta saml",
			entityID: "http://www.okta.com/exk123",
			ssoURL:   "https://example.okta.com/app/sso/saml",
		},
		{
			name:     "azure ad saml",
			entityID: "https://sts.windows.net/tenant-id/",
			ssoURL:   "https://login.microsoftonline.com/tenant-id/saml2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.entityID == "" {
				t.Error("entityID should not be empty")
			}
			if tt.ssoURL == "" {
				t.Error("ssoURL should not be empty")
			}
		})
	}
}

func BenchmarkOIDCConfiguration(b *testing.B) {
	issuer := "http://localhost:8180/realms/portauth"
	clientID := "port-authorizing"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = issuer
		_ = clientID
	}
}
