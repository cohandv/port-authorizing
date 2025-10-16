package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/davidcohan/port-authorizing/internal/audit"
	"github.com/davidcohan/port-authorizing/internal/config"
	"golang.org/x/oauth2"
)

// OIDCProvider implements OpenID Connect authentication
type OIDCProvider struct {
	name          string
	provider      *oidc.Provider
	oauth2Config  oauth2.Config
	verifier      *oidc.IDTokenVerifier
	rolesClaim    string
	usernameClaim string
	auditLogPath  string
}

// NewOIDCProvider creates a new OIDC provider
func NewOIDCProvider(cfg config.AuthProviderConfig) (*OIDCProvider, error) {
	issuer, ok := cfg.Config["issuer"]
	if !ok {
		return nil, fmt.Errorf("issuer not configured")
	}

	clientID, ok := cfg.Config["client_id"]
	if !ok {
		return nil, fmt.Errorf("client_id not configured")
	}

	clientSecret, ok := cfg.Config["client_secret"]
	if !ok {
		return nil, fmt.Errorf("client_secret not configured")
	}

	redirectURL, ok := cfg.Config["redirect_url"]
	if !ok {
		return nil, fmt.Errorf("redirect_url not configured")
	}

	ctx := context.Background()
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC provider: %w", err)
	}

	oauth2Config := oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email", "roles"},
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: clientID})

	rolesClaim := cfg.Config["roles_claim"]
	if rolesClaim == "" {
		rolesClaim = "roles"
	}

	usernameClaim := cfg.Config["username_claim"]
	if usernameClaim == "" {
		usernameClaim = "preferred_username"
	}

	return &OIDCProvider{
		name:          cfg.Name,
		provider:      provider,
		oauth2Config:  oauth2Config,
		verifier:      verifier,
		rolesClaim:    rolesClaim,
		usernameClaim: usernameClaim,
	}, nil
}

// Authenticate validates OIDC token
func (p *OIDCProvider) Authenticate(credentials map[string]string) (*UserInfo, error) {
	// For API authentication, we expect either:
	// 1. id_token (for token validation)
	// 2. code (for authorization code flow)
	// 3. username+password (for resource owner password credentials flow, if supported)

	idToken, hasToken := credentials["id_token"]
	code, hasCode := credentials["code"]

	ctx := context.Background()

	var rawIDToken string

	if hasToken {
		// Direct token validation
		rawIDToken = idToken
	} else if hasCode {
		// Exchange authorization code for tokens
		token, err := p.oauth2Config.Exchange(ctx, code)
		if err != nil {
			return nil, fmt.Errorf("failed to exchange code: %w", err)
		}

		rawIDToken, _ = token.Extra("id_token").(string)
		if rawIDToken == "" {
			return nil, fmt.Errorf("no id_token in response")
		}
	} else {
		// Check for username/password (ROPC flow)
		username, hasUsername := credentials["username"]
		password, hasPassword := credentials["password"]

		if hasUsername && hasPassword {
			// Try Resource Owner Password Credentials flow
			token, err := p.oauth2Config.PasswordCredentialsToken(ctx, username, password)
			if err != nil {
				return nil, fmt.Errorf("password credentials flow failed: %w", err)
			}

			rawIDToken, _ = token.Extra("id_token").(string)
			if rawIDToken == "" {
				return nil, fmt.Errorf("no id_token in response")
			}
		} else {
			return nil, fmt.Errorf("no valid OIDC credentials provided (need id_token, code, or username+password)")
		}
	}

	// Verify ID token
	_ = audit.Log("stdout", "system", "oidc_verify_start", "oidc", map[string]interface{}{
		"has_raw_token": rawIDToken != "",
	})

	idTokenParsed, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		_ = audit.Log("stdout", "system", "oidc_verify_failed", "oidc", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to verify ID token: %w", err)
	}

	_ = audit.Log("stdout", "system", "oidc_verify_success", "oidc", nil)

	// Extract claims
	var claims map[string]interface{}
	if err := idTokenParsed.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	// Debug: log all claims to see what we received
	_ = audit.Log("stdout", "system", "oidc_debug_claims", "oidc", claims)
	_ = audit.Log("stdout", "system", "oidc_debug_roles_claim", "oidc", map[string]interface{}{
		"roles_claim": p.rolesClaim,
	})

	// Extract username
	username, _ := claims[p.usernameClaim].(string)
	if username == "" {
		username, _ = claims["sub"].(string)
	}

	// Extract email
	email, _ := claims["email"].(string)

	// Extract roles
	roles := []string{}
	if rolesInterface, ok := claims[p.rolesClaim]; ok {
		_ = audit.Log("stdout", "system", "oidc_debug_roles_found", "oidc", map[string]interface{}{
			"value": rolesInterface,
			"type":  fmt.Sprintf("%T", rolesInterface),
		})
		switch v := rolesInterface.(type) {
		case []interface{}:
			for _, role := range v {
				if roleStr, ok := role.(string); ok {
					roles = append(roles, roleStr)
				}
			}
		case []string:
			roles = v
		case string:
			roles = []string{v}
		}
	} else {
		availableKeys := []string{}
		for k := range claims {
			availableKeys = append(availableKeys, k)
		}
		_ = audit.Log("stdout", "system", "oidc_debug_roles_not_found", "oidc", map[string]interface{}{
			"roles_claim":    p.rolesClaim,
			"available_keys": availableKeys,
		})
	}
	_ = audit.Log("stdout", "system", "oidc_debug_extracted_roles", "oidc", map[string]interface{}{
		"roles": roles,
	})

	return &UserInfo{
		Username: username,
		Email:    email,
		Roles:    roles,
		Metadata: map[string]string{
			"provider": p.name,
			"subject":  claims["sub"].(string),
		},
	}, nil
}

// Name returns the provider name
func (p *OIDCProvider) Name() string {
	return p.name
}

// Type returns the provider type
func (p *OIDCProvider) Type() string {
	return "oidc"
}

// GetAuthURL returns the OAuth2 authorization URL
func (p *OIDCProvider) GetAuthURL(state string) string {
	return p.oauth2Config.AuthCodeURL(state)
}

// GetIssuer returns the OIDC issuer URL
func (p *OIDCProvider) GetIssuer() string {
	return p.provider.Endpoint().AuthURL[:strings.LastIndex(p.provider.Endpoint().AuthURL, "/protocol")]
}

// GetClientID returns the OAuth2 client ID
func (p *OIDCProvider) GetClientID() string {
	return p.oauth2Config.ClientID
}

// GetClientSecret returns the OAuth2 client secret
func (p *OIDCProvider) GetClientSecret() string {
	return p.oauth2Config.ClientSecret
}

// GetUsernameClaim returns the username claim name
func (p *OIDCProvider) GetUsernameClaim() string {
	return p.usernameClaim
}

// GetRolesClaim returns the roles claim name
func (p *OIDCProvider) GetRolesClaim() string {
	return p.rolesClaim
}

// IsEnabled returns whether the provider is enabled
func (p *OIDCProvider) IsEnabled() bool {
	return true // Providers are only created if enabled
}

// GetAuthorizationURL builds the OIDC authorization URL
func (p *OIDCProvider) GetAuthorizationURL(state, redirectURL string) (string, error) {
	return p.oauth2Config.AuthCodeURL(state), nil
}

// ExchangeCodeForToken exchanges authorization code for access token and user info
func (p *OIDCProvider) ExchangeCodeForToken(code, redirectURL string) (*UserInfo, error) {
	ctx := context.Background()

	// DEBUG: Log function entry
	_ = audit.Log("stdout", "system", "oidc_exchange_function_start", "oidc", map[string]interface{}{
		"provider":     p.name,
		"redirect_url": redirectURL,
		"has_code":     code != "",
	})

	// Exchange authorization code for tokens
	token, err := p.oauth2Config.Exchange(ctx, code)
	if err != nil {
		_ = audit.Log("stdout", "system", "oidc_exchange_failed", "oidc", map[string]interface{}{
			"error":       err.Error(),
			"error_type":  fmt.Sprintf("%T", err),
			"scopes_requested": p.oauth2Config.Scopes,
		})
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	
	// Log detailed token information
	tokenExtras := map[string]interface{}{}
	if token.Extra("error") != nil {
		tokenExtras["error"] = token.Extra("error")
	}
	if token.Extra("error_description") != nil {
		tokenExtras["error_description"] = token.Extra("error_description")
	}
	
	_ = audit.Log("stdout", "system", "oidc_exchange_success", "oidc", map[string]interface{}{
		"has_access_token": token.AccessToken != "",
		"token_type":       token.TokenType,
		"expires_in":       token.Expiry.Sub(time.Now()).Seconds(),
		"scopes_requested": p.oauth2Config.Scopes,
		"token_extras":     tokenExtras,
	})

	// Extract ID token
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		_ = audit.Log("stdout", "system", "oidc_no_id_token", "oidc", nil)
		return nil, fmt.Errorf("no id_token in response")
	}

	// Verify ID token
	_ = audit.Log("stdout", "system", "oidc_verify_id_token_start", "oidc", nil)
	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		_ = audit.Log("stdout", "system", "oidc_verify_id_token_failed", "oidc", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to verify ID token: %w", err)
	}

	// Extract claims
	var claims map[string]interface{}
	if err := idToken.Claims(&claims); err != nil {
		_ = audit.Log("stdout", "system", "oidc_claims_parse_failed", "oidc", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	// Debug: log all claims to see what we received
	_ = audit.Log("stdout", "system", "oidc_debug_claims", "oidc", claims)
	_ = audit.Log("stdout", "system", "oidc_debug_roles_claim", "oidc", map[string]interface{}{
		"roles_claim": p.rolesClaim,
	})

	// Extract username
	username, _ := claims[p.usernameClaim].(string)
	if username == "" {
		username, _ = claims["sub"].(string)
	}

	// Extract email
	email, _ := claims["email"].(string)

	// Extract roles
	roles := []string{}
	if rolesInterface, ok := claims[p.rolesClaim]; ok {
		_ = audit.Log("stdout", "system", "oidc_debug_roles_found", "oidc", map[string]interface{}{
			"value": rolesInterface,
			"type":  fmt.Sprintf("%T", rolesInterface),
		})
		switch v := rolesInterface.(type) {
		case []interface{}:
			for _, role := range v {
				if roleStr, ok := role.(string); ok {
					roles = append(roles, roleStr)
				}
			}
		case []string:
			roles = v
		case string:
			roles = []string{v}
		}
	} else {
		availableKeys := []string{}
		for k := range claims {
			availableKeys = append(availableKeys, k)
		}
		_ = audit.Log("stdout", "system", "oidc_debug_roles_not_found", "oidc", map[string]interface{}{
			"roles_claim":    p.rolesClaim,
			"available_keys": availableKeys,
		})
	}
	_ = audit.Log("stdout", "system", "oidc_debug_extracted_roles", "oidc", map[string]interface{}{
		"roles": roles,
	})

	sub, _ := claims["sub"].(string)

	return &UserInfo{
		Username: username,
		Email:    email,
		Roles:    roles,
		Metadata: map[string]string{
			"provider": p.name,
			"subject":  sub,
		},
	}, nil
}
