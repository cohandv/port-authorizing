package config

import (
<<<<<<< HEAD
=======
	"context"
	"fmt"
>>>>>>> e6f31f6 ([temp] test)
	"time"
)

// StorageBackend defines the interface for configuration storage
type StorageBackend interface {
<<<<<<< HEAD
	// Load loads the current configuration
	Load() (*Config, error)

	// Save saves the configuration with a comment describing the change
	Save(cfg *Config, comment string) error

	// ListVersions returns a list of available configuration versions
	ListVersions() ([]Version, error)

	// LoadVersion loads a specific version of the configuration
	LoadVersion(id string) (*Config, error)

	// Rollback rolls back to a specific version
	Rollback(id string) error
}

// Version represents a configuration version
=======
	Load(ctx context.Context) (*Config, error)
	Save(ctx context.Context, cfg *Config, comment string) error
	ListVersions(ctx context.Context) ([]Version, error)
	LoadVersion(ctx context.Context, id string) (*Config, error)
	Rollback(ctx context.Context, id string) (*Config, error)
}

// Version represents a stored configuration version
>>>>>>> e6f31f6 ([temp] test)
type Version struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Comment   string    `json:"comment"`
<<<<<<< HEAD
	Author    string    `json:"author,omitempty"`
}

// StorageConfig defines the storage backend configuration
type StorageConfig struct {
	Type         string `yaml:"type"`          // file or kubernetes
	Path         string `yaml:"path"`          // for file backend
	Versions     int    `yaml:"versions"`      // number of versions to keep
	Namespace    string `yaml:"namespace"`     // for kubernetes backend
	ResourceType string `yaml:"resource_type"` // configmap or secret
	ResourceName string `yaml:"resource_name"` // name of configmap/secret
}

// NewStorageBackend creates a storage backend based on the configuration
func NewStorageBackend(cfg *StorageConfig) (StorageBackend, error) {
	if cfg == nil {
		// Default to file backend
		cfg = &StorageConfig{
			Type:     "file",
			Path:     "config.yaml",
			Versions: 5,
		}
	}

	switch cfg.Type {
	case "kubernetes", "k8s":
		return NewK8sBackend(cfg)
	case "file", "":
		return NewFileBackend(cfg)
	default:
		// Default to file backend
		return NewFileBackend(cfg)
=======
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
>>>>>>> e6f31f6 ([temp] test)
	}
}
