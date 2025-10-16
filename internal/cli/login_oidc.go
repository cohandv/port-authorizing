package cli

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/gorilla/websocket"
)

// runOIDCLogin performs browser-based OIDC authentication using WebSocket
func runOIDCLogin(apiURL, contextName string) error {
	fmt.Println("🔐 Starting browser-based OIDC authentication...")
	fmt.Println("")

	// Generate session ID
	sessionID, err := generateRandomString(32)
	if err != nil {
		return fmt.Errorf("failed to generate session ID: %w", err)
	}

	// Connect WebSocket for receiving token
	wsURL := convertHTTPToWS(apiURL) + "/api/auth/oidc/ws?session_id=" + sessionID
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer ws.Close()

	// Get auth URL from WebSocket
	var startMsg struct {
		AuthURL string `json:"auth_url"`
		Error   string `json:"error,omitempty"`
	}
	if err := ws.ReadJSON(&startMsg); err != nil {
		return fmt.Errorf("failed to receive auth URL: %w", err)
	}
	if startMsg.Error != "" {
		return fmt.Errorf("server error: %s", startMsg.Error)
	}

	// Open browser
	fmt.Println("Opening browser for authentication...")
	fmt.Printf("If browser doesn't open, visit: %s\n", startMsg.AuthURL)
	fmt.Println("")

	if err := openBrowser(startMsg.AuthURL); err != nil {
		fmt.Printf("⚠️  Failed to open browser: %v\n", err)
		fmt.Println("Please open the URL manually.")
	}

	fmt.Println("⏳ Waiting for authentication in browser...")
	fmt.Println("   (This will timeout after 5 minutes)")
	fmt.Println("")

	// Wait for token via WebSocket with timeout
	if err := ws.SetReadDeadline(time.Now().Add(5 * time.Minute)); err != nil {
		return fmt.Errorf("failed to set read deadline: %w", err)
	}

	var loginResp loginResponse
	if err := ws.ReadJSON(&loginResp); err != nil {
		return fmt.Errorf("failed to receive authentication response: %w", err)
	}

	// Save token and context
	ctx := Context{
		Name:   contextName,
		APIURL: apiURL,
		Token:  loginResp.Token,
	}
	if err := SaveContext(ctx, true); err != nil {
		return fmt.Errorf("failed to save context: %w", err)
	}

	fmt.Println("✓ Authentication successful!")
	if loginResp.User.Username != "" {
		fmt.Printf("  User: %s", loginResp.User.Username)
		if loginResp.User.Email != "" {
			fmt.Printf(" (%s)", loginResp.User.Email)
		}
		fmt.Println()
	}
	if len(loginResp.User.Roles) > 0 {
		fmt.Printf("  Roles: %v\n", loginResp.User.Roles)
	}
	if loginResp.ExpiresAt != "" {
		fmt.Printf("  Token expires: %s\n", loginResp.ExpiresAt)
	}

	return nil
}

// convertHTTPToWS converts http:// or https:// to ws:// or wss://
func convertHTTPToWS(httpURL string) string {
	if len(httpURL) >= 7 && httpURL[:7] == "http://" {
		return "ws://" + httpURL[7:]
	}
	if len(httpURL) >= 8 && httpURL[:8] == "https://" {
		return "wss://" + httpURL[8:]
	}
	return httpURL
}

func generateRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}

func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	default: // linux, freebsd, etc.
		cmd = "xdg-open"
		args = []string{url}
	}

	return exec.Command(cmd, args...).Start()
}
