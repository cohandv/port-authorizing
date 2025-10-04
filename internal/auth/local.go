package auth

import (
	"fmt"

	"github.com/davidcohan/port-authorizing/internal/config"
)

// LocalProvider implements local username/password authentication
type LocalProvider struct {
	name  string
	users map[string]*localUser
}

type localUser struct {
	username string
	password string
	roles    []string
}

// NewLocalProvider creates a local provider from user list
func NewLocalProvider(users []config.User) *LocalProvider {
	userMap := make(map[string]*localUser)
	for _, u := range users {
		userMap[u.Username] = &localUser{
			username: u.Username,
			password: u.Password,
			roles:    u.Roles,
		}
	}

	return &LocalProvider{
		name:  "local",
		users: userMap,
	}
}

// NewLocalProviderFromConfig creates a local provider from config
func NewLocalProviderFromConfig(cfg config.AuthProviderConfig) (*LocalProvider, error) {
	// This would load users from a file or database
	// For now, it's a placeholder
	return &LocalProvider{
		name:  cfg.Name,
		users: make(map[string]*localUser),
	}, nil
}

// Authenticate validates username and password
func (p *LocalProvider) Authenticate(credentials map[string]string) (*UserInfo, error) {
	username, ok := credentials["username"]
	if !ok {
		return nil, fmt.Errorf("username not provided")
	}

	password, ok := credentials["password"]
	if !ok {
		return nil, fmt.Errorf("password not provided")
	}

	user, exists := p.users[username]
	if !exists {
		return nil, fmt.Errorf("invalid credentials")
	}

	// TODO: Use proper password hashing (bcrypt, argon2, etc.)
	if user.password != password {
		return nil, fmt.Errorf("invalid credentials")
	}

	return &UserInfo{
		Username: user.username,
		Roles:    user.roles,
		Metadata: map[string]string{
			"provider": p.name,
		},
	}, nil
}

// Name returns the provider name
func (p *LocalProvider) Name() string {
	return p.name
}

// Type returns the provider type
func (p *LocalProvider) Type() string {
	return "local"
}
