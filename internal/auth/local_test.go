package auth

import (
	"testing"

	"github.com/davidcohan/port-authorizing/internal/config"
)

func TestLocalProvider_Authenticate(t *testing.T) {
	users := []config.User{
		{
			Username: "admin",
			Password: "admin123",
			Roles:    []string{"admin", "developer"},
		},
		{
			Username: "developer",
			Password: "dev123",
			Roles:    []string{"developer"},
		},
	}

	provider := NewLocalProvider(users)

	tests := []struct {
		name        string
		credentials map[string]string
		wantUser    string
		wantRoles   []string
		wantErr     bool
	}{
		{
			name: "valid admin credentials",
			credentials: map[string]string{
				"username": "admin",
				"password": "admin123",
			},
			wantUser:  "admin",
			wantRoles: []string{"admin", "developer"},
			wantErr:   false,
		},
		{
			name: "valid developer credentials",
			credentials: map[string]string{
				"username": "developer",
				"password": "dev123",
			},
			wantUser:  "developer",
			wantRoles: []string{"developer"},
			wantErr:   false,
		},
		{
			name: "invalid password",
			credentials: map[string]string{
				"username": "admin",
				"password": "wrongpassword",
			},
			wantErr: true,
		},
		{
			name: "non-existent user",
			credentials: map[string]string{
				"username": "nonexistent",
				"password": "password",
			},
			wantErr: true,
		},
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
				"username": "admin",
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
			userInfo, err := provider.Authenticate(tt.credentials)

			if (err != nil) != tt.wantErr {
				t.Errorf("Authenticate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if userInfo.Username != tt.wantUser {
					t.Errorf("Authenticate() username = %v, want %v", userInfo.Username, tt.wantUser)
				}

				if len(userInfo.Roles) != len(tt.wantRoles) {
					t.Errorf("Authenticate() roles = %v, want %v", userInfo.Roles, tt.wantRoles)
				}

				for i, role := range tt.wantRoles {
					if userInfo.Roles[i] != role {
						t.Errorf("Authenticate() role[%d] = %v, want %v", i, userInfo.Roles[i], role)
					}
				}

				if userInfo.Metadata["provider"] != "local" {
					t.Errorf("Authenticate() provider metadata = %v, want 'local'", userInfo.Metadata["provider"])
				}
			}
		})
	}
}

func TestLocalProvider_Name(t *testing.T) {
	provider := NewLocalProvider([]config.User{})
	if provider.Name() != "local" {
		t.Errorf("Name() = %v, want 'local'", provider.Name())
	}
}

func TestLocalProvider_Type(t *testing.T) {
	provider := NewLocalProvider([]config.User{})
	if provider.Type() != "local" {
		t.Errorf("Type() = %v, want 'local'", provider.Type())
	}
}

func TestNewLocalProvider(t *testing.T) {
	users := []config.User{
		{Username: "user1", Password: "pass1", Roles: []string{"role1"}},
		{Username: "user2", Password: "pass2", Roles: []string{"role2"}},
	}

	provider := NewLocalProvider(users)

	if provider == nil {
		t.Fatal("NewLocalProvider() returned nil")
	}

	if len(provider.users) != 2 {
		t.Errorf("NewLocalProvider() users count = %d, want 2", len(provider.users))
	}

	if _, exists := provider.users["user1"]; !exists {
		t.Error("NewLocalProvider() user1 not found")
	}

	if _, exists := provider.users["user2"]; !exists {
		t.Error("NewLocalProvider() user2 not found")
	}
}
