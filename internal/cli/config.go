package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// resolveContext returns the context to use for this invocation: the context named by
// an explicit --context flag if passed, otherwise the current context. Never changes
// what's persisted as current — that's what "context use" is for.
func resolveContext(cmd *cobra.Command) (*Context, error) {
	if cmd.Root().PersistentFlags().Changed("context") {
		name, _ := cmd.Root().PersistentFlags().GetString("context")
		return GetContext(name)
	}
	return GetCurrentContext()
}

// apiURLTemplate is the default API URL pattern; "{env}" is replaced with the context name.
const apiURLTemplate = "https://fwd.{env}.cohandv.io"

// resolveAPIURL determines the API URL for contextName.
// Precedence: an explicit --api-url flag (interpolating "{env}" with contextName if
// present, else used verbatim as a literal override), then the context's previously
// saved API URL, then the default template with contextName substituted in.
func resolveAPIURL(cmd *cobra.Command, contextName, savedAPIURL string) string {
	if cmd.Root().PersistentFlags().Changed("api-url") {
		raw, _ := cmd.Root().PersistentFlags().GetString("api-url")
		if strings.Contains(raw, "{env}") {
			return strings.ReplaceAll(raw, "{env}", contextName)
		}
		return raw
	}
	if savedAPIURL != "" {
		return savedAPIURL
	}
	return strings.ReplaceAll(apiURLTemplate, "{env}", contextName)
}

// Context represents an API server context
type Context struct {
	Name   string `json:"name"`
	APIURL string `json:"api_url"`
	Token  string `json:"token,omitempty"`
}

// Config represents the CLI configuration
type Config struct {
	Contexts       []Context                `json:"contexts"`
	CurrentContext string                   `json:"current_context"`
	ClientBindings map[string]ClientBinding `json:"client_bindings,omitempty"`
}

// ClientBinding configures a dedicated subcommand (keyed by name, e.g.
// "psql") that opens a tunnel and launches a local CLI client against it.
// Entries here override the built-in defaults field-by-field; unset fields
// keep the default.
type ClientBinding struct {
	Type string `json:"type,omitempty"` // match connections with this exact connection type, e.g. "postgres"
	Cli  string `json:"cli,omitempty"`  // binary name to exec, resolved via PATH unless Path is set
	Path string `json:"path,omitempty"` // explicit path to the binary, overrides PATH lookup
}

// SetConfigPath overrides the CLI config file path. Used by main binaries
// that define their own "--config" flag separately from this package's
// root command (and so can't bind it directly to configPath via
// cobra's StringVar) - they call this once the flag's been parsed instead.
func SetConfigPath(path string) {
	configPath = path
}

// GetConfigPath returns the path to the config file
func GetConfigPath() string {
	if configPath != "" && configPath != "$HOME/.port-auth/config.json" {
		return os.ExpandEnv(configPath)
	}
	return filepath.Join(os.Getenv("HOME"), ".port-auth", "config.json")
}

// LoadConfig loads the CLI configuration from disk
func LoadConfig() (*Config, error) {
	path := GetConfigPath()

	// If config doesn't exist, return empty config
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &Config{
			Contexts:       []Context{},
			CurrentContext: "",
		}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		// Try legacy format (single api_url + token)
		var legacy map[string]interface{}
		if err := json.Unmarshal(data, &legacy); err == nil {
			if apiURL, ok := legacy["api_url"].(string); ok {
				if token, ok := legacy["token"].(string); ok {
					// Convert legacy format to new format
					return &Config{
						Contexts: []Context{
							{
								Name:   "default",
								APIURL: apiURL,
								Token:  token,
							},
						},
						CurrentContext: "default",
					}, nil
				}
			}
		}
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

// SaveConfig saves the CLI configuration to disk
func SaveConfig(cfg *Config) error {
	path := GetConfigPath()
	configDir := filepath.Dir(path)

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetCurrentContext returns the current context
func GetCurrentContext() (*Context, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}

	if cfg.CurrentContext == "" {
		if len(cfg.Contexts) == 0 {
			return nil, fmt.Errorf("no contexts configured. Please run 'login' first")
		}
		// Default to first context if none set
		cfg.CurrentContext = cfg.Contexts[0].Name
	}

	for _, ctx := range cfg.Contexts {
		if ctx.Name == cfg.CurrentContext {
			return &ctx, nil
		}
	}

	return nil, fmt.Errorf("current context '%s' not found", cfg.CurrentContext)
}

// GetContext returns a context by name
func GetContext(name string) (*Context, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}

	for _, ctx := range cfg.Contexts {
		if ctx.Name == name {
			return &ctx, nil
		}
	}

	return nil, fmt.Errorf("context '%s' not found", name)
}

// SaveContext saves or updates a context
func SaveContext(ctx Context, setCurrent bool) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	// Update existing context or append new one
	found := false
	for i, existing := range cfg.Contexts {
		if existing.Name == ctx.Name {
			cfg.Contexts[i] = ctx
			found = true
			break
		}
	}

	if !found {
		cfg.Contexts = append(cfg.Contexts, ctx)
	}

	if setCurrent {
		cfg.CurrentContext = ctx.Name
	}

	return SaveConfig(cfg)
}

// DeleteContext removes a context
func DeleteContext(name string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	// Don't allow deleting the current context without switching
	if cfg.CurrentContext == name && len(cfg.Contexts) > 1 {
		return fmt.Errorf("cannot delete current context. Switch to another context first")
	}

	// Remove context
	newContexts := []Context{}
	found := false
	for _, ctx := range cfg.Contexts {
		if ctx.Name != name {
			newContexts = append(newContexts, ctx)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("context '%s' not found", name)
	}

	cfg.Contexts = newContexts

	// If we deleted the last context, clear current
	if len(cfg.Contexts) == 0 {
		cfg.CurrentContext = ""
	} else if cfg.CurrentContext == name {
		cfg.CurrentContext = cfg.Contexts[0].Name
	}

	return SaveConfig(cfg)
}

// SetCurrentContext switches to a different context
func SetCurrentContext(name string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	// Verify context exists
	found := false
	for _, ctx := range cfg.Contexts {
		if ctx.Name == name {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("context '%s' not found", name)
	}

	cfg.CurrentContext = name
	return SaveConfig(cfg)
}

// RenameContext renames an existing context
func RenameContext(oldName, newName string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	// Check if new name already exists
	for _, ctx := range cfg.Contexts {
		if ctx.Name == newName {
			return fmt.Errorf("context '%s' already exists", newName)
		}
	}

	// Rename context
	found := false
	for i, ctx := range cfg.Contexts {
		if ctx.Name == oldName {
			cfg.Contexts[i].Name = newName
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("context '%s' not found", oldName)
	}

	// Update current context if needed
	if cfg.CurrentContext == oldName {
		cfg.CurrentContext = newName
	}

	return SaveConfig(cfg)
}
