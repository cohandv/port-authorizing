package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

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
)

func init() {
	loginCmd.Flags().StringVarP(&username, "username", "u", "", "Username (for local auth)")
	loginCmd.Flags().StringVarP(&password, "password", "p", "", "Password (for local auth)")
	loginCmd.Flags().StringVar(&loginProvider, "provider", "", "Authentication provider: local, oidc (auto-detects if not specified)")
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
	// Determine authentication method
	if loginProvider == "oidc" {
		return runOIDCLogin()
	}

	// If no username/password provided, default to OIDC flow
	if username == "" && password == "" && loginProvider == "" {
		fmt.Println("No credentials provided. Using browser-based OIDC authentication.")
		fmt.Println("(Use -u and -p flags for local username/password authentication)")
		fmt.Println("")
		return runOIDCLogin()
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

	// Save token to config file
	if err := saveToken(loginResp.Token); err != nil {
		return fmt.Errorf("failed to save token: %w", err)
	}

	fmt.Printf("✓ Successfully logged in as %s\n", loginResp.User.Username)
	if len(loginResp.User.Roles) > 0 {
		fmt.Printf("  Roles: %v\n", loginResp.User.Roles)
	}
	fmt.Printf("Token expires at: %s\n", loginResp.ExpiresAt)

	return nil
}

func saveToken(token string) error {
	// Expand config path
	configPath := os.ExpandEnv(configPath)
	configDir := filepath.Dir(configPath)

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Save token to file
	config := map[string]interface{}{
		"api_url": apiURL,
		"token":   token,
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
