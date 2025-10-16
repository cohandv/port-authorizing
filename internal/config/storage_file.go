package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// FileBackend implements StorageBackend using local file system with versioning
type FileBackend struct {
	path        string
	maxVersions int
}

// NewFileBackend creates a new file-based storage backend
func NewFileBackend(path string, maxVersions int) (*FileBackend, error) {
	if path == "" {
		return nil, fmt.Errorf("file path cannot be empty")
	}
	if maxVersions < 0 {
		maxVersions = 5
	}

	return &FileBackend{
		path:        path,
		maxVersions: maxVersions,
	}, nil
}

// Load reads the configuration from the file
func (f *FileBackend) Load(ctx context.Context) (*Config, error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
}

// Save writes the configuration to file and creates a versioned backup
func (f *FileBackend) Save(ctx context.Context, cfg *Config, comment string) error {
	// Marshal config to YAML
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Create backup of current file if it exists
	if _, err := os.Stat(f.path); err == nil {
		if err := f.createBackup(comment); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// Write new config
	if err := os.WriteFile(f.path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	// Rotate old versions
	if err := f.rotateVersions(); err != nil {
		return fmt.Errorf("failed to rotate versions: %w", err)
	}

	return nil
}

// createBackup creates a versioned backup of the current config
func (f *FileBackend) createBackup(comment string) error {
	timestamp := time.Now().Format("20060102-150405")
	backupPath := fmt.Sprintf("%s.%s", f.path, timestamp)

	// Copy current file to backup
	data, err := os.ReadFile(f.path)
	if err != nil {
		return err
	}

	// Add metadata comment to backup
	metadata := fmt.Sprintf("# Backup created: %s\n# Comment: %s\n\n", time.Now().Format(time.RFC3339), comment)
	backupData := append([]byte(metadata), data...)

	return os.WriteFile(backupPath, backupData, 0644)
}

// rotateVersions removes old backups keeping only maxVersions
func (f *FileBackend) rotateVersions() error {
	if f.maxVersions <= 0 {
		return nil // Keep all versions
	}

	backups, err := f.findBackups()
	if err != nil {
		return err
	}

	// Sort by timestamp (newest first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i] > backups[j]
	})

	// Remove old backups
	for i := f.maxVersions; i < len(backups); i++ {
		if err := os.Remove(backups[i]); err != nil {
			return fmt.Errorf("failed to remove old backup %s: %w", backups[i], err)
		}
	}

	return nil
}

// findBackups returns list of backup files sorted by name
func (f *FileBackend) findBackups() ([]string, error) {
	dir := filepath.Dir(f.path)
	baseName := filepath.Base(f.path)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var backups []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Match pattern: config.yaml.20240115-143025
		if strings.HasPrefix(name, baseName+".") && len(name) > len(baseName)+1 {
			backups = append(backups, filepath.Join(dir, name))
		}
	}

	return backups, nil
}

// ListVersions returns list of available configuration versions
func (f *FileBackend) ListVersions(ctx context.Context) ([]Version, error) {
	backups, err := f.findBackups()
	if err != nil {
		return nil, err
	}

	// Sort by timestamp (newest first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i] > backups[j]
	})

	versions := []Version{
		{
			ID:        "current",
			Timestamp: time.Now(),
			Comment:   "Current configuration",
		},
	}

	// Parse backup files for metadata
	for _, backup := range backups {
		info, err := os.Stat(backup)
		if err != nil {
			continue
		}

		// Extract timestamp from filename: config.yaml.20240115-143025
		parts := strings.Split(filepath.Base(backup), ".")
		if len(parts) < 3 {
			continue
		}
		timestampStr := parts[len(parts)-1]

		// Parse timestamp
		timestamp, err := time.Parse("20060102-150405", timestampStr)
		if err != nil {
			timestamp = info.ModTime()
		}

		// Extract comment from backup file
		comment := ""
		data, err := os.ReadFile(backup)
		if err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "# Comment:") {
					comment = strings.TrimPrefix(line, "# Comment:")
					comment = strings.TrimSpace(comment)
					break
				}
			}
		}

		versions = append(versions, Version{
			ID:        filepath.Base(backup),
			Timestamp: timestamp,
			Comment:   comment,
		})
	}

	return versions, nil
}

// LoadVersion loads a specific configuration version
func (f *FileBackend) LoadVersion(ctx context.Context, id string) (*Config, error) {
	var data []byte
	var err error

	if id == "current" {
		data, err = os.ReadFile(f.path)
	} else {
		// Construct path from ID
		versionPath := filepath.Join(filepath.Dir(f.path), id)
		data, err = os.ReadFile(versionPath)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read version %s: %w", id, err)
	}

	// Remove metadata comments before parsing
	lines := strings.Split(string(data), "\n")
	var cleanLines []string
	for _, line := range lines {
		if !strings.HasPrefix(line, "# Backup") && !strings.HasPrefix(line, "# Comment:") {
			cleanLines = append(cleanLines, line)
		}
	}
	cleanData := []byte(strings.Join(cleanLines, "\n"))

	var cfg Config
	if err := yaml.Unmarshal(cleanData, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
}

// Rollback restores a previous configuration version
func (f *FileBackend) Rollback(ctx context.Context, id string) (*Config, error) {
	// Load the version
	cfg, err := f.LoadVersion(ctx, id)
	if err != nil {
		return nil, err
	}

	// Save as current (this will create a backup of current config)
	comment := fmt.Sprintf("Rolled back to version %s", id)
	if err := f.Save(ctx, cfg, comment); err != nil {
		return nil, err
	}

	return cfg, nil
}
