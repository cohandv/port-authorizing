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
	Long:  "Authenticate with the API server and store the token locally",
	RunE:  runLogin,
}

var (
	username string
	password string
)

func init() {
	loginCmd.Flags().StringVarP(&username, "username", "u", "", "Username (required)")
	loginCmd.Flags().StringVarP(&password, "password", "p", "", "Password (required)")
	loginCmd.MarkFlagRequired("username")
	loginCmd.MarkFlagRequired("password")
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

func runLogin(cmd *cobra.Command, args []string) error {
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

	fmt.Printf("✓ Successfully logged in as %s\n", username)
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
