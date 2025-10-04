package auth

import (
	"fmt"

	"github.com/davidcohan/port-authorizing/internal/config"
)

// SAML2Provider implements SAML 2.0 authentication
type SAML2Provider struct {
	name string
	// TODO: Add actual SAML implementation using crewjam/saml or similar
	// For now, this is a stub
}

// NewSAML2Provider creates a new SAML2 provider
func NewSAML2Provider(cfg config.AuthProviderConfig) (*SAML2Provider, error) {
	// Validate required configuration
	requiredFields := []string{"idp_metadata_url", "sp_entity_id", "sp_acs_url"}
	for _, field := range requiredFields {
		if _, ok := cfg.Config[field]; !ok {
			return nil, fmt.Errorf("missing required SAML2 config: %s", field)
		}
	}

	return &SAML2Provider{
		name: cfg.Name,
	}, nil
}

// Authenticate validates SAML assertion
func (p *SAML2Provider) Authenticate(credentials map[string]string) (*UserInfo, error) {
	// TODO: Implement SAML validation
	// Expected credentials:
	// - saml_response: base64-encoded SAML response from IdP

	samlResponse, ok := credentials["saml_response"]
	if !ok {
		return nil, fmt.Errorf("saml_response not provided")
	}

	// Placeholder validation
	if samlResponse == "" {
		return nil, fmt.Errorf("invalid SAML response")
	}

	// TODO: Parse and validate SAML assertion
	// TODO: Extract attributes (username, email, roles/groups)

	return nil, fmt.Errorf("SAML2 authentication not yet implemented")
}

// Name returns the provider name
func (p *SAML2Provider) Name() string {
	return p.name
}

// Type returns the provider type
func (p *SAML2Provider) Type() string {
	return "saml2"
}
