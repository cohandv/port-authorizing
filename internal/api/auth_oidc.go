package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/davidcohan/port-authorizing/internal/auth"
	"github.com/davidcohan/port-authorizing/internal/audit"
)

// oidcStateStore keeps track of OIDC state for CSRF protection
type oidcStateStore struct {
	mu     sync.RWMutex
	states map[string]*oidcState
}

type oidcState struct {
	cliCallback string
	createdAt   time.Time
}

var stateStore = &oidcStateStore{
	states: make(map[string]*oidcState),
}

// Clean up expired states periodically
func init() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			stateStore.cleanup()
		}
	}()
}

func (s *oidcStateStore) set(state string, cliCallback string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[state] = &oidcState{
		cliCallback: cliCallback,
		createdAt:   time.Now(),
	}
}

func (s *oidcStateStore) get(state string) (*oidcState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.states[state]
	return st, ok
}

func (s *oidcStateStore) delete(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.states, state)
}

func (s *oidcStateStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-10 * time.Minute)
	for state, st := range s.states {
		if st.createdAt.Before(cutoff) {
			delete(s.states, state)
		}
	}
}

// handleOIDCLogin initiates the OIDC authentication flow
func (s *Server) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	cliCallback := r.URL.Query().Get("cli_callback")

	if state == "" || cliCallback == "" {
		http.Error(w, "Missing state or cli_callback parameter", http.StatusBadRequest)
		return
	}

	// Store state for callback verification
	stateStore.set(state, cliCallback)

	// Find OIDC provider
	var oidcProvider *auth.OIDCProvider
	for _, provider := range s.authSvc.authManager.GetProviders() {
		if oidcProv, ok := provider.(*auth.OIDCProvider); ok && oidcProv.IsEnabled() {
			oidcProvider = oidcProv
			break
		}
	}

	if oidcProvider == nil {
		http.Error(w, "OIDC provider not configured", http.StatusInternalServerError)
		return
	}

	// Build authorization URL
	authURL, err := oidcProvider.GetAuthorizationURL(state, s.config.Auth.Providers[0].Config["redirect_url"])
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to generate auth URL: %v", err), http.StatusInternalServerError)
		return
	}

	// Redirect to Keycloak
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleOIDCCallback handles the OIDC callback from Keycloak
func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	// Get authorization code and state
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		http.Error(w, "Missing code or state parameter", http.StatusBadRequest)
		return
	}

	// Verify state and get CLI callback
	stateData, ok := stateStore.get(state)
	if !ok {
		http.Error(w, "Invalid or expired state", http.StatusBadRequest)
		return
	}
	defer stateStore.delete(state)

	// Find OIDC provider
	var oidcProvider *auth.OIDCProvider
	for _, provider := range s.authSvc.authManager.GetProviders() {
		if oidcProv, ok := provider.(*auth.OIDCProvider); ok && oidcProv.IsEnabled() {
			oidcProvider = oidcProv
			break
		}
	}

	if oidcProvider == nil {
		http.Error(w, "OIDC provider not configured", http.StatusInternalServerError)
		return
	}

	// Exchange authorization code for token
	redirectURL := s.config.Auth.Providers[0].Config["redirect_url"]
	userInfo, err := oidcProvider.ExchangeCodeForToken(code, redirectURL)
	if err != nil {
		audit.Log(s.config.Logging.AuditLogPath, "unknown", "oidc_token_exchange_failed", "oidc", map[string]interface{}{
			"error": err.Error(),
		})
		http.Error(w, fmt.Sprintf("Failed to exchange token: %v", err), http.StatusInternalServerError)
		return
	}

	// Generate our JWT token
	token, expiresAt, err := s.authSvc.generateToken(userInfo)
	if err != nil {
		audit.Log(s.config.Logging.AuditLogPath, userInfo.Username, "jwt_generation_failed", "oidc", map[string]interface{}{
			"error": err.Error(),
		})
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	// Log successful OIDC login
	audit.Log(s.config.Logging.AuditLogPath, userInfo.Username, "oidc_login_success", "oidc", map[string]interface{}{
		"email": userInfo.Email,
		"roles": userInfo.Roles,
	})

	// Build login response
	loginResp := LoginResponse{
		Token:     token,
		ExpiresAt: expiresAt,
		User: UserInfo{
			Username: userInfo.Username,
			Email:    userInfo.Email,
			Roles:    userInfo.Roles,
		},
	}

	// Encode as JSON
	jsonData, err := json.Marshal(loginResp)
	if err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}

	// Base64 encode for URL safety
	tokenData := base64.URLEncoding.EncodeToString(jsonData)

	// Redirect to CLI callback
	callbackURL := fmt.Sprintf("%s?state=%s&token_data=%s", stateData.cliCallback, state, tokenData)
	http.Redirect(w, r, callbackURL, http.StatusFound)
}

