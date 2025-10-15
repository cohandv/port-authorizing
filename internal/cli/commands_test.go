package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunLogin_Success(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/login" {
			response := loginResponse{
				Token:     "test-token-12345",
				ExpiresAt: "2025-12-31T23:59:59Z",
			}
			response.User.Username = "admin"
			response.User.Roles = []string{"admin"}

			_ = json.NewEncoder(w).Encode(response)
		}
	}))
	defer server.Close()

	// Setup temp HOME
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	// Create root command with flags
	rootCmd := &cobra.Command{}
	rootCmd.PersistentFlags().String("api-url", server.URL, "")

	// Create login command
	loginCmd := &cobra.Command{
		Use:  "login",
		RunE: runLogin,
	}
	loginCmd.Flags().StringVarP(&username, "username", "u", "", "")
	loginCmd.Flags().StringVarP(&password, "password", "p", "", "")
	loginCmd.Flags().StringVar(&loginProvider, "provider", "", "")

	rootCmd.AddCommand(loginCmd)

	// Set flags
	username = "admin"
	password = "admin123"
	loginProvider = ""

	// Run command
	err := loginCmd.RunE(loginCmd, []string{})

	if err != nil {
		t.Errorf("runLogin() error = %v", err)
	}

	// Verify token was saved
	token, err := loadToken()
	if err != nil {
		t.Errorf("loadToken() error = %v", err)
	}

	if token != "test-token-12345" {
		t.Errorf("token = %s, want test-token-12345", token)
	}
}

func TestRunLogin_InvalidCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "Invalid credentials",
		})
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	rootCmd := &cobra.Command{}
	rootCmd.PersistentFlags().String("api-url", server.URL, "")

	loginCmd := &cobra.Command{
		Use:  "login",
		RunE: runLogin,
	}
	loginCmd.Flags().StringVarP(&username, "username", "u", "", "")
	loginCmd.Flags().StringVarP(&password, "password", "p", "", "")
	loginCmd.Flags().StringVar(&loginProvider, "provider", "", "")

	rootCmd.AddCommand(loginCmd)

	username = "admin"
	password = "wrong"
	loginProvider = ""

	err := loginCmd.RunE(loginCmd, []string{})

	if err == nil {
		t.Error("runLogin() should fail with invalid credentials")
	}
}

func TestRunLogin_MissingCredentials(t *testing.T) {
	// Test will try to use OIDC which won't work in tests, but we can test missing creds error
	rootCmd := &cobra.Command{}
	rootCmd.PersistentFlags().String("api-url", "http://localhost:8080", "")

	loginCmd := &cobra.Command{
		Use:  "login",
		RunE: runLogin,
	}
	loginCmd.Flags().StringVarP(&username, "username", "u", "", "")
	loginCmd.Flags().StringVarP(&password, "password", "p", "", "")
	loginCmd.Flags().StringVar(&loginProvider, "provider", "", "")

	rootCmd.AddCommand(loginCmd)

	// Only username, no password
	username = "admin"
	password = ""
	loginProvider = "local"

	err := loginCmd.RunE(loginCmd, []string{})

	if err == nil {
		t.Error("runLogin() should fail with missing password")
	}
}

func TestRunList_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/connections" {
			connections := []connectionInfo{
				{Name: "test-db", Type: "postgres"},
				{Name: "api-server", Type: "http"},
			}
			_ = json.NewEncoder(w).Encode(connections)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	// Create a context with token
	ctx := Context{
		Name:   "test",
		APIURL: server.URL,
		Token:  "valid-token",
	}
	_ = SaveContext(ctx, true)

	rootCmd := &cobra.Command{}
	rootCmd.PersistentFlags().String("api-url", server.URL, "")

	listCmd := &cobra.Command{
		Use:  "list",
		RunE: runList,
	}

	rootCmd.AddCommand(listCmd)

	// Capture output
	var buf bytes.Buffer
	listCmd.SetOut(&buf)

	err := listCmd.RunE(listCmd, []string{})

	if err != nil {
		t.Errorf("runList() error = %v", err)
	}
}

func TestRunList_NoToken(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	rootCmd := &cobra.Command{}
	rootCmd.PersistentFlags().String("api-url", "http://localhost:8080", "")

	listCmd := &cobra.Command{
		Use:  "list",
		RunE: runList,
	}

	rootCmd.AddCommand(listCmd)

	err := listCmd.RunE(listCmd, []string{})

	if err == nil {
		t.Error("runList() should fail without token")
	}
}

func TestRunList_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "Unauthorized",
		})
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	// Create context with invalid token
	ctx := Context{
		Name:   "test",
		APIURL: server.URL,
		Token:  "invalid-token",
	}
	_ = SaveContext(ctx, true)

	rootCmd := &cobra.Command{}
	rootCmd.PersistentFlags().String("api-url", server.URL, "")

	listCmd := &cobra.Command{
		Use:  "list",
		RunE: runList,
	}

	rootCmd.AddCommand(listCmd)

	err := listCmd.RunE(listCmd, []string{})

	if err == nil {
		t.Error("runList() should fail with invalid token")
	}
}

func TestRunConnect_NoToken(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	rootCmd := &cobra.Command{}
	rootCmd.PersistentFlags().String("api-url", "http://localhost:8080", "")

	connectCmd := &cobra.Command{
		Use:  "connect",
		RunE: runConnect,
		Args: cobra.ExactArgs(1),
	}
	connectCmd.Flags().IntVarP(&localPort, "local-port", "l", 0, "")

	rootCmd.AddCommand(connectCmd)

	localPort = 8080

	err := connectCmd.RunE(connectCmd, []string{"test-db"})

	if err == nil {
		t.Error("runConnect() should fail without token")
	}
}

func TestRunConnect_InvalidToken(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	// Save invalid token
	_ = saveToken("invalid-token")

	rootCmd := &cobra.Command{}
	rootCmd.PersistentFlags().String("api-url", "http://localhost:8080", "")

	connectCmd := &cobra.Command{
		Use:  "connect",
		RunE: runConnect,
		Args: cobra.ExactArgs(1),
	}
	connectCmd.Flags().IntVarP(&localPort, "local-port", "l", 0, "")

	rootCmd.AddCommand(connectCmd)

	localPort = 8080

	err := connectCmd.RunE(connectCmd, []string{"test-db"})

	if err == nil {
		t.Error("runConnect() should fail with invalid token")
	}
}

func BenchmarkRunLogin(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := loginResponse{
			Token:     "test-token-12345",
			ExpiresAt: "2025-12-31T23:59:59Z",
		}
		response.User.Username = "admin"
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	tmpDir := b.TempDir()
	oldHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	rootCmd := &cobra.Command{}
	rootCmd.PersistentFlags().String("api-url", server.URL, "")

	loginCmd := &cobra.Command{
		Use:  "login",
		RunE: runLogin,
	}
	loginCmd.Flags().StringVarP(&username, "username", "u", "", "")
	loginCmd.Flags().StringVarP(&password, "password", "p", "", "")
	loginCmd.Flags().StringVar(&loginProvider, "provider", "", "")

	rootCmd.AddCommand(loginCmd)

	username = "admin"
	password = "admin123"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = loginCmd.RunE(loginCmd, []string{})
	}
}

func BenchmarkRunList(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connections := []connectionInfo{
			{Name: "test-db", Type: "postgres"},
		}
		_ = json.NewEncoder(w).Encode(connections)
	}))
	defer server.Close()

	tmpDir := b.TempDir()
	oldHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	_ = saveToken("valid-token")

	rootCmd := &cobra.Command{}
	rootCmd.PersistentFlags().String("api-url", server.URL, "")

	listCmd := &cobra.Command{
		Use:  "list",
		RunE: runList,
	}

	rootCmd.AddCommand(listCmd)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = listCmd.RunE(listCmd, []string{})
	}
}
