package auth

import (
	"fmt"

	"github.com/davidcohan/port-authorizing/internal/config"
)

// Provider defines the interface for authentication providers
type Provider interface {
	// Authenticate validates credentials and returns username and roles
	Authenticate(credentials map[string]string) (*UserInfo, error)
	// Name returns the provider name
	Name() string
	// Type returns the provider type
	Type() string
}

// UserInfo contains authenticated user information
type UserInfo struct {
	Username string
	Email    string
	Roles    []string
	Metadata map[string]string
}

// Manager manages multiple authentication providers
type Manager struct {
	providers []Provider
}

// NewManager creates a new auth provider manager
func NewManager(cfg *config.Config) (*Manager, error) {
	m := &Manager{
		providers: make([]Provider, 0),
	}

	// Add local provider if users are defined (backward compatibility)
	if len(cfg.Auth.Users) > 0 {
		m.providers = append(m.providers, NewLocalProvider(cfg.Auth.Users))
	}

	// Add configured providers
	for _, providerCfg := range cfg.Auth.Providers {
		if !providerCfg.Enabled {
			continue
		}

		var provider Provider
		var err error

		switch providerCfg.Type {
		case "local":
			// Local provider with external user source
			provider, err = NewLocalProviderFromConfig(providerCfg)
		case "oidc":
			provider, err = NewOIDCProvider(providerCfg)
		case "saml2":
			provider, err = NewSAML2Provider(providerCfg)
		case "ldap":
			provider, err = NewLDAPProvider(providerCfg)
		default:
			return nil, fmt.Errorf("unknown auth provider type: %s", providerCfg.Type)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to initialize %s provider: %w", providerCfg.Name, err)
		}

		m.providers = append(m.providers, provider)
	}

	if len(m.providers) == 0 {
		return nil, fmt.Errorf("no authentication providers configured")
	}

	return m, nil
}

// GetProviders returns all configured providers
func (m *Manager) GetProviders() []Provider {
	return m.providers
}

// Authenticate tries each provider in order
func (m *Manager) Authenticate(credentials map[string]string) (*UserInfo, error) {
	var lastErr error

	for _, provider := range m.providers {
		userInfo, err := provider.Authenticate(credentials)
		if err == nil {
			return userInfo, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, fmt.Errorf("authentication failed: %w", lastErr)
	}

	return nil, fmt.Errorf("authentication failed: no providers available")
}
