package proxy

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/davidcohan/port-authorizing/internal/config"
)

func TestNewConnectionManager(t *testing.T) {
	maxDuration := 1 * time.Hour
	cm := NewConnectionManager(maxDuration)

	if cm == nil {
		t.Fatal("NewConnectionManager() returned nil")
	}

	if cm.maxDuration != maxDuration {
		t.Errorf("maxDuration = %v, want %v", cm.maxDuration, maxDuration)
	}

	if cm.connections == nil {
		t.Error("connections map should be initialized")
	}

	if cm.cleanupTicker == nil {
		t.Error("cleanupTicker should be initialized")
	}

	// Cleanup
	cm.CloseAll()
}

func TestConnectionManager_CreateConnection(t *testing.T) {
	cm := NewConnectionManager(1 * time.Hour)
	defer cm.CloseAll()

	tmpFile, _ := os.CreateTemp("", "audit-*.log")
	defer os.Remove(tmpFile.Name())

	connConfig := &config.ConnectionConfig{
		Name:   "test-http",
		Type:   "http",
		Host:   "localhost",
		Port:   8080,
		Scheme: "http",
	}

	whitelist := []string{"^GET /api/.*"}
	duration := 10 * time.Minute

	tests := []struct {
		name      string
		username  string
		config    *config.ConnectionConfig
		duration  time.Duration
		whitelist []string
		auditPath string
		wantErr   bool
	}{
		{
			name:      "create HTTP connection",
			username:  "testuser",
			config:    connConfig,
			duration:  duration,
			whitelist: whitelist,
			auditPath: tmpFile.Name(),
			wantErr:   false,
		},
		{
			name:      "create connection with empty whitelist",
			username:  "testuser2",
			config:    connConfig,
			duration:  duration,
			whitelist: []string{},
			auditPath: tmpFile.Name(),
			wantErr:   false,
		},
		{
			name:     "create postgres connection (no proxy created)",
			username: "testuser3",
			config: &config.ConnectionConfig{
				Name: "test-postgres",
				Type: "postgres",
				Host: "localhost",
				Port: 5432,
			},
			duration:  duration,
			whitelist: []string{},
			auditPath: tmpFile.Name(),
			wantErr:   false,
		},
		{
			name:     "create TCP connection",
			username: "testuser4",
			config: &config.ConnectionConfig{
				Name: "test-tcp",
				Type: "tcp",
				Host: "localhost",
				Port: 6379,
			},
			duration:  duration,
			whitelist: []string{},
			auditPath: tmpFile.Name(),
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
		connectionID, expiresAt, err := cm.CreateConnection(
			tt.username,
			tt.config,
			tt.duration,
			tt.whitelist,
			tt.auditPath,
			nil, // no approval manager for tests
		)

			if (err != nil) != tt.wantErr {
				t.Errorf("CreateConnection() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if connectionID == "" {
					t.Error("CreateConnection() returned empty connectionID")
				}

				if expiresAt.IsZero() {
					t.Error("CreateConnection() returned zero expiresAt")
				}

				if !expiresAt.After(time.Now()) {
					t.Error("expiresAt should be in the future")
				}

				// Verify connection was stored
				conn, err := cm.GetConnection(connectionID)
				if err != nil {
					t.Errorf("Failed to get created connection: %v", err)
				}

				if conn.Username != tt.username {
					t.Errorf("Username = %s, want %s", conn.Username, tt.username)
				}

				if conn.Config.Name != tt.config.Name {
					t.Errorf("Config.Name = %s, want %s", conn.Config.Name, tt.config.Name)
				}
			}
		})
	}
}

func TestConnectionManager_GetConnection(t *testing.T) {
	cm := NewConnectionManager(1 * time.Hour)
	defer cm.CloseAll()

	tmpFile, _ := os.CreateTemp("", "audit-*.log")
	defer os.Remove(tmpFile.Name())

	connConfig := &config.ConnectionConfig{
		Name:   "test-http",
		Type:   "http",
		Host:   "localhost",
		Port:   8080,
		Scheme: "http",
	}

	// Create a connection
	connectionID, _, err := cm.CreateConnection(
		"testuser",
		connConfig,
		10*time.Minute,
		[]string{},
		tmpFile.Name(),
		nil, // no approval manager for tests
	)
	if err != nil {
		t.Fatalf("Failed to create connection: %v", err)
	}

	tests := []struct {
		name         string
		connectionID string
		wantErr      bool
	}{
		{
			name:         "get existing connection",
			connectionID: connectionID,
			wantErr:      false,
		},
		{
			name:         "get non-existent connection",
			connectionID: "non-existent-id",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, err := cm.GetConnection(tt.connectionID)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetConnection() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if conn == nil {
					t.Error("GetConnection() returned nil connection")
				}
				if conn.ID != tt.connectionID {
					t.Errorf("Connection ID = %s, want %s", conn.ID, tt.connectionID)
				}
			}
		})
	}
}

func TestConnectionManager_CloseConnection(t *testing.T) {
	cm := NewConnectionManager(1 * time.Hour)
	defer cm.CloseAll()

	tmpFile, _ := os.CreateTemp("", "audit-*.log")
	defer os.Remove(tmpFile.Name())

	connConfig := &config.ConnectionConfig{
		Name:   "test-http",
		Type:   "http",
		Host:   "localhost",
		Port:   8080,
		Scheme: "http",
	}

	// Create a connection
	connectionID, _, err := cm.CreateConnection(
		"testuser",
		connConfig,
		10*time.Minute,
		[]string{},
		tmpFile.Name(),
		nil, // no approval manager for tests
	)
	if err != nil {
		t.Fatalf("Failed to create connection: %v", err)
	}

	// Verify it exists
	_, err = cm.GetConnection(connectionID)
	if err != nil {
		t.Fatalf("Connection should exist before close: %v", err)
	}

	// Close it
	err = cm.CloseConnection(connectionID)
	if err != nil {
		t.Fatalf("CloseConnection() error = %v", err)
	}

	// Verify it's gone
	_, err = cm.GetConnection(connectionID)
	if err == nil {
		t.Error("Connection should not exist after close")
	}

	// Try to close non-existent connection
	err = cm.CloseConnection("non-existent-id")
	if err == nil {
		t.Error("CloseConnection() should return error for non-existent connection")
	}
}

func TestConnectionManager_GetActiveConnections(t *testing.T) {
	cm := NewConnectionManager(1 * time.Hour)
	defer cm.CloseAll()

	tmpFile, _ := os.CreateTemp("", "audit-*.log")
	defer os.Remove(tmpFile.Name())

	connConfig := &config.ConnectionConfig{
		Name:   "test-http",
		Type:   "http",
		Host:   "localhost",
		Port:   8080,
		Scheme: "http",
	}

	// Initially should be 0
	if count := cm.GetActiveConnections(); count != 0 {
		t.Errorf("Initial active connections = %d, want 0", count)
	}

	// Create 3 connections
	for i := 0; i < 3; i++ {
		_, _, err := cm.CreateConnection(
			"testuser",
			connConfig,
			10*time.Minute,
			[]string{},
			tmpFile.Name(),
			nil, // no approval manager for tests
		)
		if err != nil {
			t.Fatalf("Failed to create connection: %v", err)
		}
	}

	// Should be 3
	if count := cm.GetActiveConnections(); count != 3 {
		t.Errorf("Active connections = %d, want 3", count)
	}

	// Close all
	cm.CloseAll()

	// Should be 0 again
	if count := cm.GetActiveConnections(); count != 0 {
		t.Errorf("Active connections after CloseAll = %d, want 0", count)
	}
}

func TestConnectionManager_CloseAll(t *testing.T) {
	cm := NewConnectionManager(1 * time.Hour)

	tmpFile, _ := os.CreateTemp("", "audit-*.log")
	defer os.Remove(tmpFile.Name())

	connConfig := &config.ConnectionConfig{
		Name:   "test-http",
		Type:   "http",
		Host:   "localhost",
		Port:   8080,
		Scheme: "http",
	}

	// Create multiple connections
	ids := make([]string, 5)
	for i := 0; i < 5; i++ {
		id, _, err := cm.CreateConnection(
			"testuser",
			connConfig,
			10*time.Minute,
			[]string{},
			tmpFile.Name(),
			nil, // no approval manager for tests
		)
		if err != nil {
			t.Fatalf("Failed to create connection: %v", err)
		}
		ids[i] = id
	}

	// Verify all exist
	for _, id := range ids {
		if _, err := cm.GetConnection(id); err != nil {
			t.Errorf("Connection %s should exist", id)
		}
	}

	// Close all
	cm.CloseAll()

	// Verify all are gone
	for _, id := range ids {
		if _, err := cm.GetConnection(id); err == nil {
			t.Errorf("Connection %s should not exist after CloseAll", id)
		}
	}

	// Verify count is 0
	if count := cm.GetActiveConnections(); count != 0 {
		t.Errorf("Active connections after CloseAll = %d, want 0", count)
	}
}

func TestConnection_RegisterStream(t *testing.T) {
	conn := &Connection{
		ID:       "test-id",
		Username: "testuser",
	}

	// Create mock net.Conn (using a pipe)
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	// Register stream
	conn.RegisterStream(client)

	// Verify it's registered
	conn.streamsMu.Lock()
	if !conn.activeStreams[client] {
		t.Error("Stream should be registered")
	}
	conn.streamsMu.Unlock()
}

func TestConnection_UnregisterStream(t *testing.T) {
	conn := &Connection{
		ID:       "test-id",
		Username: "testuser",
	}

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	// Register then unregister
	conn.RegisterStream(client)
	conn.UnregisterStream(client)

	// Verify it's unregistered
	conn.streamsMu.Lock()
	if conn.activeStreams[client] {
		t.Error("Stream should be unregistered")
	}
	conn.streamsMu.Unlock()
}

func TestConnection_CloseAllStreams(t *testing.T) {
	conn := &Connection{
		ID:       "test-id",
		Username: "testuser",
	}

	// Create multiple streams
	streams := make([]net.Conn, 3)
	for i := 0; i < 3; i++ {
		client, server := net.Pipe()
		defer server.Close()
		streams[i] = client
		conn.RegisterStream(client)
	}

	// Close all streams
	conn.CloseAllStreams()

	// Verify all are closed and map is empty
	conn.streamsMu.Lock()
	if len(conn.activeStreams) != 0 {
		t.Errorf("Active streams = %d, want 0", len(conn.activeStreams))
	}
	conn.streamsMu.Unlock()
}

func TestConnectionManager_ExpiredConnections(t *testing.T) {
	// Use very short duration for testing
	cm := NewConnectionManager(1 * time.Hour)
	defer cm.CloseAll()

	tmpFile, _ := os.CreateTemp("", "audit-*.log")
	defer os.Remove(tmpFile.Name())

	connConfig := &config.ConnectionConfig{
		Name:   "test-http",
		Type:   "http",
		Host:   "localhost",
		Port:   8080,
		Scheme: "http",
	}

	// Create a connection with very short duration
	connectionID, _, err := cm.CreateConnection(
		"testuser",
		connConfig,
		1*time.Millisecond, // Very short duration
		[]string{},
		tmpFile.Name(),
		nil, // no approval manager for tests
	)
	if err != nil {
		t.Fatalf("Failed to create connection: %v", err)
	}

	// Wait for expiration
	time.Sleep(10 * time.Millisecond)

	// Try to get expired connection
	_, err = cm.GetConnection(connectionID)
	if err == nil {
		t.Error("GetConnection() should return error for expired connection")
	}
}

func BenchmarkConnectionManager_CreateConnection(b *testing.B) {
	cm := NewConnectionManager(1 * time.Hour)
	defer cm.CloseAll()

	tmpFile, _ := os.CreateTemp("", "audit-*.log")
	defer os.Remove(tmpFile.Name())

	connConfig := &config.ConnectionConfig{
		Name:   "test-http",
		Type:   "http",
		Host:   "localhost",
		Port:   8080,
		Scheme: "http",
	}

	whitelist := []string{"^GET /api/.*"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cm.CreateConnection("testuser", connConfig, 10*time.Minute, whitelist, tmpFile.Name(), nil)
	}
}

func BenchmarkConnectionManager_GetConnection(b *testing.B) {
	cm := NewConnectionManager(1 * time.Hour)
	defer cm.CloseAll()

	tmpFile, _ := os.CreateTemp("", "audit-*.log")
	defer os.Remove(tmpFile.Name())

	connConfig := &config.ConnectionConfig{
		Name:   "test-http",
		Type:   "http",
		Host:   "localhost",
		Port:   8080,
		Scheme: "http",
	}

	connectionID, _, _ := cm.CreateConnection("testuser", connConfig, 10*time.Minute, []string{}, tmpFile.Name(), nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cm.GetConnection(connectionID)
	}
}
