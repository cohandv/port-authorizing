package config

import (
<<<<<<< HEAD
=======
	"context"
>>>>>>> e6f31f6 ([temp] test)
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

<<<<<<< HEAD
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
=======
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
>>>>>>> e6f31f6 ([temp] test)
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

<<<<<<< HEAD
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
=======
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
>>>>>>> e6f31f6 ([temp] test)

	return nil
}

<<<<<<< HEAD
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
=======
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
>>>>>>> e6f31f6 ([temp] test)
		if err != nil {
			continue
		}

<<<<<<< HEAD
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
=======
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
>>>>>>> e6f31f6 ([temp] test)
}
