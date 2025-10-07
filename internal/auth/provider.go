package auth

import (
	"fmt"
	"log"

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
			// Log but don't fail for unknown provider types
			log.Printf("⚠️  Warning: unknown auth provider type '%s' (name: %s) - skipping", providerCfg.Type, providerCfg.Name)
			continue
		}

		if err != nil {
			// Log but don't fail - provider might be temporarily unavailable
			log.Printf("⚠️  Warning: failed to initialize %s provider '%s': %v - skipping", providerCfg.Type, providerCfg.Name, err)
			log.Printf("   The server will start without this provider. It will be unavailable until the server is restarted.")
			continue
		}

		m.providers = append(m.providers, provider)
		log.Printf("✅ Initialized %s provider: %s", providerCfg.Type, providerCfg.Name)
	}

	if len(m.providers) == 0 {
		log.Println("⚠️  Warning: no authentication providers successfully initialized!")
		log.Println("   This means authentication will NOT work until you:")
		log.Println("   1. Fix provider configurations (check OIDC/LDAP/SAML2 connectivity)")
		log.Println("   2. Or add local users to config.yaml")
		log.Println("   3. Then restart the server")
		log.Println("")
		log.Println("   Server will continue to start, but API authentication endpoints will fail.")
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
