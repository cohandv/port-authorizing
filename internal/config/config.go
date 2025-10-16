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
	Policies    []RolePolicy       `yaml:"policies"`
	Security    SecurityConfig     `yaml:"security"`
	Logging     LoggingConfig      `yaml:"logging"`
	Approval    *ApprovalConfig    `yaml:"approval,omitempty"`
	Storage     *StorageConfig     `yaml:"storage,omitempty"`
}

// ServerConfig contains server settings
type ServerConfig struct {
	Port                  int           `yaml:"port"`
	MaxConnectionDuration time.Duration `yaml:"max_connection_duration"`
	BaseURL               string        `yaml:"base_url,omitempty"` // Base URL for callbacks (e.g., for Slack approval buttons)
}

// AuthConfig contains authentication settings
type AuthConfig struct {
	JWTSecret   string               `yaml:"jwt_secret"`
	TokenExpiry time.Duration        `yaml:"token_expiry"`
	Providers   []AuthProviderConfig `yaml:"providers"`
	// Legacy: local users (kept for backward compatibility)
	Users []User `yaml:"users,omitempty"`
}

// AuthProviderConfig defines an authentication provider
type AuthProviderConfig struct {
	Name    string            `yaml:"name"`    // Unique identifier
	Type    string            `yaml:"type"`    // local, oidc, saml2, ldap
	Enabled bool              `yaml:"enabled"` // Whether this provider is active
	Config  map[string]string `yaml:"config"`  // Provider-specific configuration
}

// OIDC Config keys: issuer, client_id, client_secret, redirect_url
// SAML2 Config keys: idp_metadata_url, sp_entity_id, sp_acs_url, sp_cert, sp_key
// LDAP Config keys: url, bind_dn, bind_password, user_base_dn, user_filter, group_base_dn

// User represents a user account
type User struct {
	Username string   `yaml:"username" json:"username"`
	Password string   `yaml:"password" json:"password"` // In production, use hashed passwords
	Roles    []string `yaml:"roles" json:"roles"`
}

// ConnectionConfig defines an available connection endpoint
type ConnectionConfig struct {
	Name     string            `yaml:"name" json:"name"`
	Type     string            `yaml:"type" json:"type"` // postgres, http, tcp, redis
	Host     string            `yaml:"host" json:"host"`
	Port     int               `yaml:"port" json:"port"`
	Scheme   string            `yaml:"scheme,omitempty" json:"scheme,omitempty"`     // for HTTP: http/https
	Duration time.Duration     `yaml:"duration,omitempty" json:"duration,omitempty"` // connection timeout duration
	Tags     []string          `yaml:"tags,omitempty" json:"tags,omitempty"`         // Tags for policy matching (env:prod, team:backend, etc.)
	Metadata map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	// Backend credentials (for protocols like Postgres/Redis where proxy re-authenticates)
	BackendUsername string `yaml:"backend_username,omitempty" json:"backend_username,omitempty"` // For Postgres
	BackendPassword string `yaml:"backend_password,omitempty" json:"backend_password,omitempty"` // For Postgres and Redis AUTH
	BackendDatabase string `yaml:"backend_database,omitempty" json:"backend_database,omitempty"` // For Postgres
	// Redis-specific options
	RedisCluster bool `yaml:"redis_cluster,omitempty" json:"redis_cluster,omitempty"` // Enable Redis Cluster mode
	// Deprecated: use policies instead
	Whitelist []string `yaml:"whitelist,omitempty" json:"whitelist,omitempty"` // DEPRECATED: regex patterns, use policies instead
}

// RolePolicy defines access policies for roles
type RolePolicy struct {
	Name      string            `yaml:"name" json:"name"`                               // Policy name
	Roles     []string          `yaml:"roles" json:"roles"`                             // Which roles this policy applies to
	Tags      []string          `yaml:"tags" json:"tags"`                               // Connection tags this policy applies to (e.g., "env:dev", "team:backend")
	TagMatch  string            `yaml:"tag_match,omitempty" json:"tag_match,omitempty"` // "all" (default) or "any"
	Whitelist []string          `yaml:"whitelist,omitempty" json:"whitelist,omitempty"` // Allowed patterns for matched connections
	Metadata  map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`   // Additional metadata
}

// SecurityConfig contains security settings
type SecurityConfig struct {
	EnableLLMAnalysis bool   `yaml:"enable_llm_analysis"`
	LLMProvider       string `yaml:"llm_provider,omitempty"`
	LLMAPIKey         string `yaml:"llm_api_key,omitempty"`
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	AuditLogPath  string `yaml:"audit_log_path"`
	LogLevel      string `yaml:"log_level"`
	AuditMemoryMB int    `yaml:"audit_memory_mb,omitempty"` // Max memory for in-memory audit buffer (0 to disable, default 1MB)
}

// ApprovalConfig contains approval workflow settings
type ApprovalConfig struct {
	Enabled  bool                    `yaml:"enabled"`
	Patterns []ApprovalPatternConfig `yaml:"patterns"`
	Webhook  *WebhookApprovalConfig  `yaml:"webhook,omitempty"`
	Slack    *SlackApprovalConfig    `yaml:"slack,omitempty"`
}

// ApprovalPatternConfig defines which requests require approval
type ApprovalPatternConfig struct {
	Pattern        string   `yaml:"pattern" json:"pattern"`                         // Regex pattern "^METHOD /path$"
	Tags           []string `yaml:"tags,omitempty" json:"tags,omitempty"`           // Connection tags (e.g., "env:prod", "team:backend")
	TagMatch       string   `yaml:"tag_match,omitempty" json:"tag_match,omitempty"` // "all" (default) or "any"
	TimeoutSeconds int      `yaml:"timeout_seconds" json:"timeout_seconds"`         // Approval timeout in seconds
}

// WebhookApprovalConfig configures generic webhook approvals
type WebhookApprovalConfig struct {
	URL string `yaml:"url" json:"url"` // Webhook endpoint URL
}

// SlackApprovalConfig configures Slack approvals
type SlackApprovalConfig struct {
	WebhookURL string `yaml:"webhook_url" json:"webhook_url"` // Slack incoming webhook URL
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
