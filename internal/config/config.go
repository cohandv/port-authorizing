package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the main configuration structure
type Config struct {
	Server      ServerConfig       `yaml:"server"`
	Auth        AuthConfig         `yaml:"auth"`
	Connections []ConnectionConfig `yaml:"connections"`
	Security    SecurityConfig     `yaml:"security"`
	Logging     LoggingConfig      `yaml:"logging"`
}

// ServerConfig contains server settings
type ServerConfig struct {
	Port                  int           `yaml:"port"`
	MaxConnectionDuration time.Duration `yaml:"max_connection_duration"`
}

// AuthConfig contains authentication settings
type AuthConfig struct {
	JWTSecret   string        `yaml:"jwt_secret"`
	TokenExpiry time.Duration `yaml:"token_expiry"`
	Users       []User        `yaml:"users"`
}

// User represents a user account
type User struct {
	Username string   `yaml:"username"`
	Password string   `yaml:"password"` // In production, use hashed passwords
	Roles    []string `yaml:"roles"`
}

// ConnectionConfig defines an available connection endpoint
type ConnectionConfig struct {
	Name      string            `yaml:"name"`
	Type      string            `yaml:"type"` // postgres, http, tcp
	Host      string            `yaml:"host"`
	Port      int               `yaml:"port"`
	Scheme    string            `yaml:"scheme,omitempty"`    // for HTTP: http/https
	Whitelist []string          `yaml:"whitelist,omitempty"` // regex patterns
	Metadata  map[string]string `yaml:"metadata,omitempty"`
}

// SecurityConfig contains security settings
type SecurityConfig struct {
	EnableLLMAnalysis bool   `yaml:"enable_llm_analysis"`
	LLMProvider       string `yaml:"llm_provider,omitempty"`
	LLMAPIKey         string `yaml:"llm_api_key,omitempty"`
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	AuditLogPath string `yaml:"audit_log_path"`
	LogLevel     string `yaml:"log_level"`
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults
	if config.Server.Port == 0 {
		config.Server.Port = 8080
	}
	if config.Server.MaxConnectionDuration == 0 {
		config.Server.MaxConnectionDuration = 2 * time.Hour
	}
	if config.Auth.TokenExpiry == 0 {
		config.Auth.TokenExpiry = 24 * time.Hour
	}
	if config.Logging.LogLevel == "" {
		config.Logging.LogLevel = "info"
	}
	if config.Logging.AuditLogPath == "" {
		config.Logging.AuditLogPath = "audit.log"
	}

	return &config, nil
}
