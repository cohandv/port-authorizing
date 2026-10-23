package config

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config Source Type
type ConfigSourceType string

const (
	ConfigSourceTypePlain     ConfigSourceType = "plain"
	ConfigSourceTypeConfigMap ConfigSourceType = "configmap"
	ConfigSourceTypeSecret    ConfigSourceType = "secret"
)

// Config Source
type ConfigSource struct {
	Type    ConfigSourceType `yaml:"type" json:"type" default:"plain"`
	Value   string           `yaml:"value" json:"value" default:""`
	Ref     string           `yaml:"ref" json:"ref" default:""`
	RefName string           `yaml:"ref_name" json:"ref_name" default:""`
}

// UnmarshalYAML implements custom unmarshaling to support both plain strings and objects
func (cs *ConfigSource) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Try unmarshaling as a string first (backward compatibility)
	var str string
	if err := unmarshal(&str); err == nil {
		*cs = ConfigSource{
			Type:  ConfigSourceTypePlain,
			Value: str,
		}
		return nil
	}

	// If not a string, unmarshal as a struct
	type rawConfigSource ConfigSource
	var raw rawConfigSource
	if err := unmarshal(&raw); err != nil {
		return err
	}

	*cs = ConfigSource(raw)
	// Set default type if not specified
	if cs.Type == "" {
		cs.Type = ConfigSourceTypePlain
	}
	return nil
}

// MarshalYAML implements custom marshaling to support both plain strings and objects
func (cs ConfigSource) MarshalYAML() (interface{}, error) {
	// If it's a plain value with no refs, marshal as a simple string for readability
	if cs.Type == ConfigSourceTypePlain || cs.Type == "" {
		if cs.Ref == "" && cs.RefName == "" {
			return cs.Value, nil
		}
	}

	// For ConfigMap/Secret types, do NOT include the resolved value in the saved config
	// The value will be resolved at runtime from the ConfigMap/Secret
	value := cs.Value
	if cs.Type == ConfigSourceTypeConfigMap || cs.Type == ConfigSourceTypeSecret {
		value = "" // Don't persist resolved secrets/configmap values
	}

	// Otherwise, marshal as a full object
	return map[string]interface{}{
		"type":     cs.Type,
		"value":    value,
		"ref":      cs.Ref,
		"ref_name": cs.RefName,
	}, nil
}

// UnmarshalJSON implements custom JSON unmarshaling
func (cs *ConfigSource) UnmarshalJSON(data []byte) error {
	// Try unmarshaling as a string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*cs = ConfigSource{
			Type:  ConfigSourceTypePlain,
			Value: str,
		}
		return nil
	}

	// If not a string, unmarshal as a struct
	type rawConfigSource ConfigSource
	var raw rawConfigSource
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*cs = ConfigSource(raw)
	// Set default type if not specified
	if cs.Type == "" {
		cs.Type = ConfigSourceTypePlain
	}
	return nil
}

// MarshalJSON implements custom JSON marshaling
func (cs ConfigSource) MarshalJSON() ([]byte, error) {
	// For JSON, always use the full object format to preserve all fields
	// This is important for API communication
	type rawConfigSource ConfigSource
	raw := rawConfigSource(cs)

	// Set default type if empty
	if raw.Type == "" {
		raw.Type = ConfigSourceTypePlain
	}

	return json.Marshal(raw)
}

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
	Environment string             `yaml:"environment"`
}

// ServerConfig contains server settings
type ServerConfig struct {
	Port                  int           `yaml:"port"`
	MaxConnectionDuration time.Duration `yaml:"max_connection_duration"`
	BaseURL               string        `yaml:"base_url,omitempty"` // Base URL for callbacks (e.g., for Slack approval buttons)
}

// AuthConfig contains authentication settings
type AuthConfig struct {
	JWTSecret   ConfigSource         `yaml:"jwt_secret"`
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
	// ClientSecret can be used for OIDC client_secret from ConfigMap/Secret
	ClientSecret ConfigSource `yaml:"client_secret,omitempty" json:"client_secret,omitempty"`
}

// OIDC Config keys: issuer, client_id, client_secret, redirect_url
// SAML2 Config keys: idp_metadata_url, sp_entity_id, sp_acs_url, sp_cert, sp_key
// LDAP Config keys: url, bind_dn, bind_password, user_base_dn, user_filter, group_base_dn

// User represents a user account
type User struct {
	Username ConfigSource `yaml:"username" json:"username"`
	Password ConfigSource `yaml:"password" json:"password"` // In production, use hashed passwords
	Roles    []string     `yaml:"roles" json:"roles"`
}

// ConnectionConfig defines an available connection endpoint
type ConnectionConfig struct {
	Name     string            `yaml:"name" json:"name"`
	Type     string            `yaml:"type" json:"type"` // postgres, http, tcp
	Host     string            `yaml:"host" json:"host"`
	Port     int               `yaml:"port" json:"port"`
	Scheme   string            `yaml:"scheme,omitempty" json:"scheme,omitempty"`     // for HTTP: http/https
	Duration time.Duration     `yaml:"duration,omitempty" json:"duration,omitempty"` // connection timeout duration
	Tags     []string          `yaml:"tags,omitempty" json:"tags,omitempty"`         // Tags for policy matching (env:prod, team:backend, etc.)
	Metadata map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	// Backend credentials (for protocols like Postgres where proxy re-authenticates)
	BackendUsername ConfigSource `yaml:"backend_username,omitempty" json:"backend_username,omitempty"`
	BackendPassword ConfigSource `yaml:"backend_password,omitempty" json:"backend_password,omitempty"`
	BackendDatabase string       `yaml:"backend_database,omitempty" json:"backend_database,omitempty"`
	// Extra startup parameters for the backend (run-time settings, e.g. statement_timeout); clients cannot override them at connect
	BackendParams map[string]string `yaml:"backend_params,omitempty" json:"backend_params,omitempty"`
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
	EnableLLMAnalysis    bool          `yaml:"enable_llm_analysis"`
	LLMProvider          string        `yaml:"llm_provider,omitempty"`
	LLMAPIKey            string        `yaml:"llm_api_key,omitempty"`
	ConfigSourceCacheTTL time.Duration `yaml:"config_source_cache_ttl,omitempty"` // TTL for ConfigMap/Secret resolution cache (default: 1m)
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
	URL ConfigSource `yaml:"url" json:"url"` // Webhook endpoint URL - can be plain, configmap, or secret
}

// SlackApprovalConfig configures Slack approvals
type SlackApprovalConfig struct {
	WebhookURL ConfigSource `yaml:"webhook_url" json:"webhook_url"` // Can be plain, configmap, or secret
}

// LoadConfig loads configuration from a YAML file and resolves ConfigSources
func LoadConfig(path string) (*Config, error) {
	return LoadConfigWithResolver(path, "")
}

// LoadConfigWithResolver loads configuration and resolves ConfigSources
func LoadConfigWithResolver(path, namespace string) (*Config, error) {
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
	if config.Environment == "" {
		config.Environment = "local"
	}

	// Resolve ConfigSources from ConfigMaps/Secrets
	// Use namespace from Storage config if available
	resolverNamespace := namespace
	if resolverNamespace == "" && config.Storage != nil {
		resolverNamespace = config.Storage.Namespace
	}

	// Determine cache TTL (default 1 minute)
	cacheTTL := config.Security.ConfigSourceCacheTTL
	if cacheTTL == 0 {
		cacheTTL = 1 * time.Minute
	}

	resolver, err := NewConfigSourceResolverWithTTL(resolverNamespace, cacheTTL)
	if err != nil {
		return nil, fmt.Errorf("failed to create config resolver: %w", err)
	}

	ctx := context.Background()
	errors, err := resolver.ResolveConfig(ctx, &config)
	if err != nil {
		// Critical error - cannot continue
		return nil, fmt.Errorf("failed to resolve configuration: %w", err)
	}

	// Log non-critical warnings (connection credentials, etc.)
	if errors != nil {
		for _, cm := range errors.MissingConfigMaps {
			log.Printf("Warning: ConfigMap %s not found", cm)
		}
		for _, secret := range errors.MissingSecrets {
			log.Printf("Warning: Secret %s not found", secret)
		}
		for _, warning := range errors.Warnings {
			log.Printf("Warning: %s", warning)
		}
	}

	return &config, nil
}
