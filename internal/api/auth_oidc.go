package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/davidcohan/port-authorizing/internal/audit"
	"github.com/davidcohan/port-authorizing/internal/auth"
	"github.com/gorilla/websocket"
)

// oidcStateStore keeps track of OIDC state for CSRF protection
type oidcStateStore struct {
	mu     sync.RWMutex
	states map[string]*oidcState
}

type oidcState struct {
	cliCallback string
	ws          *websocket.Conn
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

func (s *oidcStateStore) set(state string, cliCallback string, ws *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[state] = &oidcState{
		cliCallback: cliCallback,
		ws:          ws,
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

// handleOIDCWebSocket handles WebSocket-based OIDC authentication
func (s *Server) handleOIDCWebSocket(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "Missing session_id parameter", http.StatusBadRequest)
		return
	}

	// Upgrade to WebSocket (reusing existing upgrader)
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "Failed to upgrade connection", http.StatusInternalServerError)
		return
	}
	defer ws.Close()

	// Find OIDC provider
	var oidcProvider *auth.OIDCProvider
	for _, provider := range s.authSvc.authManager.GetProviders() {
		if oidcProv, ok := provider.(*auth.OIDCProvider); ok && oidcProv.IsEnabled() {
			oidcProvider = oidcProv
			break
		}
	}

	if oidcProvider == nil {
		ws.WriteJSON(map[string]string{"error": "OIDC provider not configured"})
		return
	}

	// Build authorization URL
	redirectURL := s.config.Auth.Providers[0].Config["redirect_url"]
	authURL, err := oidcProvider.GetAuthorizationURL(sessionID, redirectURL)
	if err != nil {
		ws.WriteJSON(map[string]string{"error": fmt.Sprintf("Failed to generate auth URL: %v", err)})
		return
	}

	// Store WebSocket connection for this session
	stateStore.set(sessionID, "", ws)

	// Send auth URL to CLI
	if err := ws.WriteJSON(map[string]string{"auth_url": authURL}); err != nil {
		return
	}

	// Keep connection alive (CLI will wait for token)
	// The connection will be used by handleOIDCCallback to push the token
	for {
		if _, _, err := ws.ReadMessage(); err != nil {
			break // Connection closed
		}
	}
}

// handleOIDCLogin initiates the OIDC authentication flow (legacy browser-based)
func (s *Server) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	cliCallback := r.URL.Query().Get("cli_callback")

	if state == "" || cliCallback == "" {
		http.Error(w, "Missing state or cli_callback parameter", http.StatusBadRequest)
		return
	}

	// Store state for callback verification
	stateStore.set(state, cliCallback, nil)

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
	// DEBUG: Log that callback was hit
	_ = audit.Log(s.config.Logging.AuditLogPath, "system", "oidc_callback_start", "oidc", map[string]interface{}{
		"url": r.URL.String(),
	})

	// Get authorization code and state
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		_ = audit.Log(s.config.Logging.AuditLogPath, "system", "oidc_callback_missing_params", "oidc", map[string]interface{}{
			"has_code":  code != "",
			"has_state": state != "",
		})
		http.Error(w, "Missing code or state parameter", http.StatusBadRequest)
		return
	}

	_ = audit.Log(s.config.Logging.AuditLogPath, "system", "oidc_callback_has_params", "oidc", map[string]interface{}{
		"has_code":  true,
		"has_state": true,
	})

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
	_ = audit.Log(s.config.Logging.AuditLogPath, "system", "oidc_exchange_start", "oidc", map[string]interface{}{
		"redirect_url": redirectURL,
	})
	userInfo, err := oidcProvider.ExchangeCodeForToken(code, redirectURL)
	if err != nil {
		_ = audit.Log(s.config.Logging.AuditLogPath, "unknown", "oidc_token_exchange_failed", "oidc", map[string]interface{}{
			"error": err.Error(),
		})
		http.Error(w, fmt.Sprintf("Failed to exchange token: %v", err), http.StatusInternalServerError)
		return
	}

	// Generate our JWT token
	token, expiresAt, err := s.authSvc.generateToken(userInfo)
	if err != nil {
		_ = audit.Log(s.config.Logging.AuditLogPath, userInfo.Username, "jwt_generation_failed", "oidc", map[string]interface{}{
			"error": err.Error(),
		})
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	// Log successful OIDC login
	_ = audit.Log(s.config.Logging.AuditLogPath, userInfo.Username, "oidc_login_success", "oidc", map[string]interface{}{
		"email":       userInfo.Email,
		"roles":       userInfo.Roles,
		"roles_count": len(userInfo.Roles),
		"has_roles":   len(userInfo.Roles) > 0,
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

	// Check if this is a WebSocket-based authentication
	if stateData.ws != nil {
		// Push token through WebSocket
		if err := stateData.ws.WriteJSON(loginResp); err != nil {
			http.Error(w, "Failed to send token to CLI", http.StatusInternalServerError)
			return
		}

		// Show success page in browser
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `
<!DOCTYPE html>
<html>
<head>
    <title>Authentication Successful</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Arial, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            height: 100vh;
            margin: 0;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
        }
        .container {
            background: white;
            padding: 3rem;
            border-radius: 1rem;
            box-shadow: 0 10px 40px rgba(0,0,0,0.2);
            text-align: center;
        }
        h1 { color: #667eea; margin: 0 0 1rem 0; }
        .success { font-size: 4rem; color: #2ecc71; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Authentication Successful!</h1>
        <p>You can close this window and return to your terminal.</p>
    </div>
    <script>setTimeout(() => window.close(), 3000);</script>
</body>
</html>`)
		return
	}

	// Legacy: Redirect to CLI callback
	if stateData.cliCallback != "" {
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
}
