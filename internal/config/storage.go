package config

import (
	"context"
	"fmt"
	"time"
)

// StorageBackend defines the interface for configuration storage
type StorageBackend interface {
	Load(ctx context.Context) (*Config, error)
	Save(ctx context.Context, cfg *Config, comment string) error
	ListVersions(ctx context.Context) ([]Version, error)
	LoadVersion(ctx context.Context, id string) (*Config, error)
	Rollback(ctx context.Context, id string) (*Config, error)
}

// Version represents a stored configuration version
type Version struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Comment   string    `json:"comment"`
	Author    string    `json:"author,omitempty"` // User who made the change
}

// StorageConfig defines the configuration for the storage backend
type StorageConfig struct {
	Type         string `yaml:"type"`                    // file, kubernetes
	Path         string `yaml:"path,omitempty"`          // For file backend
	Versions     int    `yaml:"versions,omitempty"`      // Number of versions to keep (default: 5)
	Namespace    string `yaml:"namespace,omitempty"`     // For Kubernetes backend
	ResourceType string `yaml:"resource_type,omitempty"` // configmap or secret
	ResourceName string `yaml:"resource_name,omitempty"` // Name of configmap/secret
}

// NewStorageBackend creates a new storage backend based on config
func NewStorageBackend(cfg *StorageConfig) (StorageBackend, error) {
	if cfg == nil {
		// Default to file backend with current config
		return NewFileBackend("config.yaml", 5)
	}

	switch cfg.Type {
	case "file", "":
		path := cfg.Path
		if path == "" {
			path = "config.yaml"
		}
		versions := cfg.Versions
		if versions <= 0 {
			versions = 5
		}
		return NewFileBackend(path, versions)

	case "kubernetes":
		if cfg.Namespace == "" {
			return nil, fmt.Errorf("kubernetes backend requires namespace")
		}
		if cfg.ResourceName == "" {
			return nil, fmt.Errorf("kubernetes backend requires resource_name")
		}
		resourceType := cfg.ResourceType
		if resourceType == "" {
			resourceType = "configmap"
		}
		versions := cfg.Versions
		if versions <= 0 {
			versions = 5
		}
		return NewK8sBackend(cfg.Namespace, resourceType, cfg.ResourceName, versions)

	default:
		return nil, fmt.Errorf("unsupported storage type: %s", cfg.Type)
	}
}
