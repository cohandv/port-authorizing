package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Context represents an API server context
type Context struct {
	Name   string `json:"name"`
	APIURL string `json:"api_url"`
	Token  string `json:"token,omitempty"`
}

// Config represents the CLI configuration
type Config struct {
	Contexts       []Context `json:"contexts"`
	CurrentContext string    `json:"current_context"`
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
