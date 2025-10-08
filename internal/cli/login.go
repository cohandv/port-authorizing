package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to the API server",
	Long:  "Authenticate with the API server using local credentials or OIDC browser flow",
	RunE:  runLogin,
}

var (
	username      string
	password      string
	loginProvider string
	contextName   string
)

func init() {
	loginCmd.Flags().StringVarP(&username, "username", "u", "", "Username (for local auth)")
	loginCmd.Flags().StringVarP(&password, "password", "p", "", "Password (for local auth)")
	loginCmd.Flags().StringVar(&loginProvider, "provider", "", "Authentication provider: local, oidc (auto-detects if not specified)")
	loginCmd.Flags().StringVarP(&contextName, "context", "c", "", "Context name (default: use current or create 'default')")
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
	User      struct {
		Username string   `json:"username"`
		Email    string   `json:"email"`
		Roles    []string `json:"roles"`
	} `json:"user"`
}

func runLogin(cmd *cobra.Command, args []string) error {
	// Determine context name
	if contextName == "" {
		cfg, _ := LoadConfig()
		if cfg != nil && cfg.CurrentContext != "" {
			contextName = cfg.CurrentContext
		} else {
			contextName = "default"
		}
	}

	// Get API URL from parent command flags
	apiURL, _ := cmd.Root().PersistentFlags().GetString("api-url")
	if apiURL == "" {
		// Try to get from existing context
		if ctx, err := GetContext(contextName); err == nil && ctx.APIURL != "" {
			apiURL = ctx.APIURL
		} else {
			apiURL = "http://localhost:8080"
		}
	}

	// Determine authentication method
	if loginProvider == "oidc" {
		return runOIDCLoginWithContext(apiURL, contextName)
	}

	// If no username/password provided, default to OIDC flow
	if username == "" && password == "" && loginProvider == "" {
		fmt.Println("No credentials provided. Using browser-based OIDC authentication.")
		fmt.Println("(Use -u and -p flags for local username/password authentication)")
		fmt.Println("")
		return runOIDCLoginWithContext(apiURL, contextName)
	}

	// Local username/password authentication
	if username == "" || password == "" {
		return fmt.Errorf("username and password are required for local authentication")
	}

	// Prepare login request
	reqBody := loginRequest{
		Username: username,
		Password: password,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send login request
	resp, err := http.Post(fmt.Sprintf("%s/api/login", apiURL), "application/json", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to send login request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login failed: %s", string(body))
	}

	// Parse response
	var loginResp loginResponse
	if err := json.Unmarshal(body, &loginResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Save token to context
	ctx := Context{
		Name:   contextName,
		APIURL: apiURL,
		Token:  loginResp.Token,
	}
	if err := SaveContext(ctx, true); err != nil {
		return fmt.Errorf("failed to save context: %w", err)
	}

	fmt.Printf("✓ Successfully logged in as %s\n", loginResp.User.Username)
	if len(loginResp.User.Roles) > 0 {
		fmt.Printf("  Roles: %v\n", loginResp.User.Roles)
	}
	fmt.Printf("  Context: %s\n", contextName)
	fmt.Printf("  API URL: %s\n", apiURL)
	fmt.Printf("  Token expires at: %s\n", loginResp.ExpiresAt)

	return nil
}

// runOIDCLoginWithContext wraps OIDC login with context saving
func runOIDCLoginWithContext(apiURL, contextName string) error {
	// TODO: Implement OIDC with context support
	return runOIDCLogin(apiURL)
}

// Legacy saveToken - keeping for OIDC backward compatibility
func saveToken(token string) error {
	// This is now handled by SaveContext
	// Keeping for OIDC login compatibility
	return nil
}

func loadToken() (string, error) {
	ctx, err := GetCurrentContext()
	if err != nil {
		return "", fmt.Errorf("not logged in: %w. Please run 'login' first", err)
	}

	if ctx.Token == "" {
		return "", fmt.Errorf("no token found for context '%s'. Please run 'login'", ctx.Name)
	}

	return ctx.Token, nil
}
