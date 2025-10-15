package config

import (
	"time"
)

// StorageBackend defines the interface for configuration storage
type StorageBackend interface {
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
type Version struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Comment   string    `json:"comment"`
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
	}
}
