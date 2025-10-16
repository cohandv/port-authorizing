package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewStorageBackend(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *StorageConfig
		wantErr  bool
		wantType string
	}{
		{
			name:     "nil config defaults to file backend",
			cfg:      nil,
			wantErr:  false,
			wantType: "*config.FileBackend",
		},
		{
			name: "file backend",
			cfg: &StorageConfig{
				Type: "file",
				Path: "test.yaml",
			},
			wantErr:  false,
			wantType: "*config.FileBackend",
		},
		{
			name: "kubernetes backend without namespace",
			cfg: &StorageConfig{
				Type:         "kubernetes",
				ResourceName: "test",
			},
			wantErr: true,
		},
		{
			name: "kubernetes backend without resource name",
			cfg: &StorageConfig{
				Type:      "kubernetes",
				Namespace: "default",
			},
			wantErr: true,
		},
		{
			name: "unsupported type",
			cfg: &StorageConfig{
				Type: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend, err := NewStorageBackend(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewStorageBackend() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && backend == nil {
				t.Error("NewStorageBackend() returned nil backend")
			}
		})
	}
}

func TestFileBackend_LoadSaveRoundtrip(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	// Create file backend
	backend, err := NewFileBackend(configPath, 5)
	if err != nil {
		t.Fatalf("NewFileBackend() error = %v", err)
	}

	// Create test config
	testCfg := &Config{
		Server: ServerConfig{
			Port:                  8080,
			MaxConnectionDuration: 1 * time.Hour,
		},
		Auth: AuthConfig{
			JWTSecret:   "test-secret",
			TokenExpiry: 24 * time.Hour,
		},
	}

	ctx := context.Background()

	// Save config
	err = backend.Save(ctx, testCfg, "initial save")
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}

	// Load config
	loadedCfg, err := backend.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify loaded config matches
	if loadedCfg.Server.Port != testCfg.Server.Port {
		t.Errorf("Server.Port = %d, want %d", loadedCfg.Server.Port, testCfg.Server.Port)
	}
	if loadedCfg.Auth.JWTSecret != testCfg.Auth.JWTSecret {
		t.Errorf("Auth.JWTSecret = %s, want %s", loadedCfg.Auth.JWTSecret, testCfg.Auth.JWTSecret)
	}
}

func TestFileBackend_Versioning(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	// Create file backend with max 3 versions
	backend, err := NewFileBackend(configPath, 3)
	if err != nil {
		t.Fatalf("NewFileBackend() error = %v", err)
	}

	ctx := context.Background()

	// Create and save multiple versions
	for i := 1; i <= 5; i++ {
		cfg := &Config{
			Server: ServerConfig{
				Port: 8080 + i,
			},
		}
		comment := "version " + string(rune('0'+i))
		err := backend.Save(ctx, cfg, comment)
		if err != nil {
			t.Fatalf("Save() version %d error = %v", i, err)
		}
		time.Sleep(100 * time.Millisecond) // Ensure different timestamps
	}

	// List versions
	versions, err := backend.ListVersions(ctx)
	if err != nil {
		t.Fatalf("ListVersions() error = %v", err)
	}

	// Should have current + 3 backups = 4 total
	if len(versions) > 4 {
		t.Errorf("ListVersions() returned %d versions, want max 4 (current + 3 backups)", len(versions))
	}

	// Verify current version
	if versions[0].ID != "current" {
		t.Errorf("First version should be 'current', got %s", versions[0].ID)
	}

	// Load current version
	currentCfg, err := backend.LoadVersion(ctx, "current")
	if err != nil {
		t.Fatalf("LoadVersion(current) error = %v", err)
	}

	// Should be the last saved (port 8085)
	if currentCfg.Server.Port != 8085 {
		t.Errorf("Current version port = %d, want 8085", currentCfg.Server.Port)
	}
}

func TestFileBackend_Rollback(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	// Create file backend
	backend, err := NewFileBackend(configPath, 5)
	if err != nil {
		t.Fatalf("NewFileBackend() error = %v", err)
	}

	ctx := context.Background()

	// Save initial version
	v1Cfg := &Config{
		Server: ServerConfig{Port: 8080},
	}
	if err := backend.Save(ctx, v1Cfg, "version 1"); err != nil {
		t.Fatalf("Save() v1 error = %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Save second version
	v2Cfg := &Config{
		Server: ServerConfig{Port: 8081},
	}
	if err := backend.Save(ctx, v2Cfg, "version 2"); err != nil {
		t.Fatalf("Save() v2 error = %v", err)
	}

	// List versions to get the first backup ID
	versions, err := backend.ListVersions(ctx)
	if err != nil {
		t.Fatalf("ListVersions() error = %v", err)
	}

	if len(versions) < 2 {
		t.Fatal("Expected at least 2 versions (current + 1 backup)")
	}

	// Rollback to first version
	firstBackupID := versions[1].ID
	rolledCfg, err := backend.Rollback(ctx, firstBackupID)
	if err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}

	// Verify rolled back to port 8080
	if rolledCfg.Server.Port != 8080 {
		t.Errorf("Rolled back config port = %d, want 8080", rolledCfg.Server.Port)
	}

	// Verify current config is now 8080
	currentCfg, err := backend.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if currentCfg.Server.Port != 8080 {
		t.Errorf("Current config after rollback port = %d, want 8080", currentCfg.Server.Port)
	}
}

func BenchmarkFileBackend_Save(b *testing.B) {
	tmpDir := b.TempDir()
	configPath := filepath.Join(tmpDir, "bench-config.yaml")

	backend, err := NewFileBackend(configPath, 5)
	if err != nil {
		b.Fatalf("NewFileBackend() error = %v", err)
	}

	testCfg := &Config{
		Server: ServerConfig{
			Port:                  8080,
			MaxConnectionDuration: 1 * time.Hour,
		},
		Auth: AuthConfig{
			JWTSecret:   "test-secret",
			TokenExpiry: 24 * time.Hour,
		},
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := backend.Save(ctx, testCfg, "benchmark"); err != nil {
			b.Fatalf("Save() error = %v", err)
		}
	}
}

func BenchmarkFileBackend_Load(b *testing.B) {
	tmpDir := b.TempDir()
	configPath := filepath.Join(tmpDir, "bench-config.yaml")

	backend, err := NewFileBackend(configPath, 5)
	if err != nil {
		b.Fatalf("NewFileBackend() error = %v", err)
	}

	testCfg := &Config{
		Server: ServerConfig{
			Port:                  8080,
			MaxConnectionDuration: 1 * time.Hour,
		},
	}

	ctx := context.Background()

	// Save initial config
	if err := backend.Save(ctx, testCfg, "initial"); err != nil {
		b.Fatalf("Save() error = %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := backend.Load(ctx); err != nil {
			b.Fatalf("Load() error = %v", err)
		}
	}
}
