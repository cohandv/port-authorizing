package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// FileBackend implements storage backed by local files with versioning
type FileBackend struct {
	path         string
	maxVersions  int
	versionCache []Version
}

// NewFileBackend creates a new file-based storage backend
func NewFileBackend(cfg *StorageConfig) (*FileBackend, error) {
	if cfg.Path == "" {
		cfg.Path = "config.yaml"
	}
	if cfg.Versions == 0 {
		cfg.Versions = 5
	}

	return &FileBackend{
		path:        cfg.Path,
		maxVersions: cfg.Versions,
	}, nil
}

// Load loads the current configuration from the file
func (fb *FileBackend) Load() (*Config, error) {
	return LoadConfig(fb.path)
}

// Save saves the configuration and creates a versioned backup
func (fb *FileBackend) Save(cfg *Config, comment string) error {
	// First, rotate existing versions
	if err := fb.rotateVersions(); err != nil {
		return fmt.Errorf("failed to rotate versions: %w", err)
	}

	// Marshal configuration to YAML
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Add a comment header with metadata
	header := fmt.Sprintf("# Configuration saved at %s\n# %s\n\n",
		time.Now().Format(time.RFC3339), comment)
	data = append([]byte(header), data...)

	// Create a backup of the current file if it exists
	if _, err := os.Stat(fb.path); err == nil {
		backupPath := fb.getVersionPath(1)
		if err := fb.copyFile(fb.path, backupPath); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}

		// Create metadata file for the backup
		metaPath := backupPath + ".meta"
		meta := Version{
			ID:        filepath.Base(backupPath),
			Timestamp: time.Now(),
			Comment:   comment,
		}
		metaData, _ := yaml.Marshal(meta)
		_ = os.WriteFile(metaPath, metaData, 0600)
	}

	// Write the new configuration
	if err := os.WriteFile(fb.path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// Clear version cache
	fb.versionCache = nil

	return nil
}

// ListVersions returns a list of available configuration versions
func (fb *FileBackend) ListVersions() ([]Version, error) {
	if fb.versionCache != nil {
		return fb.versionCache, nil
	}

	versions := []Version{}

	// Add current version
	if stat, err := os.Stat(fb.path); err == nil {
		versions = append(versions, Version{
			ID:        "current",
			Timestamp: stat.ModTime(),
			Comment:   "Current configuration",
		})
	}

	// List versioned files
	for i := 1; i <= fb.maxVersions; i++ {
		versionPath := fb.getVersionPath(i)
		metaPath := versionPath + ".meta"

		// Check if version file exists
		stat, err := os.Stat(versionPath)
		if err != nil {
			continue
		}

		// Try to load metadata
		version := Version{
			ID:        fmt.Sprintf("v%d", i),
			Timestamp: stat.ModTime(),
		}

		if metaData, err := os.ReadFile(metaPath); err == nil {
			var meta Version
			if err := yaml.Unmarshal(metaData, &meta); err == nil {
				version.Comment = meta.Comment
				version.Author = meta.Author
			}
		}

		versions = append(versions, version)
	}

	// Sort by timestamp descending
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Timestamp.After(versions[j].Timestamp)
	})

	fb.versionCache = versions
	return versions, nil
}

// LoadVersion loads a specific version of the configuration
func (fb *FileBackend) LoadVersion(id string) (*Config, error) {
	var path string

	if id == "current" {
		path = fb.path
	} else if strings.HasPrefix(id, "v") {
		// Extract version number
		var versionNum int
		if _, err := fmt.Sscanf(id, "v%d", &versionNum); err != nil {
			return nil, fmt.Errorf("invalid version ID: %s", id)
		}
		path = fb.getVersionPath(versionNum)
	} else {
		return nil, fmt.Errorf("invalid version ID: %s", id)
	}

	return LoadConfig(path)
}

// Rollback rolls back to a specific version
func (fb *FileBackend) Rollback(id string) error {
	// Load the specified version
	cfg, err := fb.LoadVersion(id)
	if err != nil {
		return fmt.Errorf("failed to load version %s: %w", id, err)
	}

	// Save it as the current configuration
	comment := fmt.Sprintf("Rolled back to version %s", id)
	return fb.Save(cfg, comment)
}

// rotateVersions shifts existing versions (v1 -> v2, v2 -> v3, etc.)
func (fb *FileBackend) rotateVersions() error {
	// Start from the oldest and move backwards
	for i := fb.maxVersions; i > 1; i-- {
		srcPath := fb.getVersionPath(i - 1)
		dstPath := fb.getVersionPath(i)
		srcMeta := srcPath + ".meta"
		dstMeta := dstPath + ".meta"

		// Only rotate if source exists
		if _, err := os.Stat(srcPath); err == nil {
			// Remove destination if it exists
			_ = os.Remove(dstPath)
			_ = os.Remove(dstMeta)

			// Rename source to destination
			if err := os.Rename(srcPath, dstPath); err != nil {
				return err
			}
			// Rename metadata too
			_ = os.Rename(srcMeta, dstMeta)
		}
	}

	return nil
}

// getVersionPath returns the path for a specific version number
func (fb *FileBackend) getVersionPath(version int) string {
	ext := filepath.Ext(fb.path)
	base := strings.TrimSuffix(fb.path, ext)
	return fmt.Sprintf("%s.v%d%s", base, version, ext)
}

// copyFile copies a file from src to dst
func (fb *FileBackend) copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0600)
}
