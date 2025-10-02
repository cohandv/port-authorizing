package proxy

import (
	"fmt"
	"sync"
	"time"

	"github.com/davidcohan/port-authorizing/internal/config"
	"github.com/google/uuid"
)

// Connection represents an active proxy connection
type Connection struct {
	ID        string
	Username  string
	Config    *config.ConnectionConfig
	Proxy     Protocol
	CreatedAt time.Time
	ExpiresAt time.Time
}

// ConnectionManager manages active proxy connections
type ConnectionManager struct {
	connections   map[string]*Connection
	mu            sync.RWMutex
	maxDuration   time.Duration
	cleanupTicker *time.Ticker
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(maxDuration time.Duration) *ConnectionManager {
	cm := &ConnectionManager{
		connections: make(map[string]*Connection),
		maxDuration: maxDuration,
	}

	// Start cleanup goroutine
	cm.cleanupTicker = time.NewTicker(30 * time.Second)
	go cm.cleanupExpired()

	return cm
}

// CreateConnection creates a new proxy connection
func (cm *ConnectionManager) CreateConnection(username string, connConfig *config.ConnectionConfig, duration time.Duration) (string, time.Time, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Create protocol-specific proxy
	proxy, err := NewProtocol(connConfig)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to create proxy: %w", err)
	}

	// Generate unique connection ID
	connectionID := uuid.New().String()
	expiresAt := time.Now().Add(duration)

	conn := &Connection{
		ID:        connectionID,
		Username:  username,
		Config:    connConfig,
		Proxy:     proxy,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}

	cm.connections[connectionID] = conn

	return connectionID, expiresAt, nil
}

// GetConnection retrieves a connection by ID
func (cm *ConnectionManager) GetConnection(connectionID string) (*Connection, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	conn, exists := cm.connections[connectionID]
	if !exists {
		return nil, fmt.Errorf("connection not found")
	}

	if time.Now().After(conn.ExpiresAt) {
		return nil, fmt.Errorf("connection expired")
	}

	return conn, nil
}

// CloseConnection closes a specific connection
func (cm *ConnectionManager) CloseConnection(connectionID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	conn, exists := cm.connections[connectionID]
	if !exists {
		return fmt.Errorf("connection not found")
	}

	conn.Proxy.Close()
	delete(cm.connections, connectionID)

	return nil
}

// CloseAll closes all active connections
func (cm *ConnectionManager) CloseAll() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for _, conn := range cm.connections {
		conn.Proxy.Close()
	}

	cm.connections = make(map[string]*Connection)
	cm.cleanupTicker.Stop()
}

// cleanupExpired removes expired connections
func (cm *ConnectionManager) cleanupExpired() {
	for range cm.cleanupTicker.C {
		cm.mu.Lock()
		now := time.Now()
		for id, conn := range cm.connections {
			if now.After(conn.ExpiresAt) {
				conn.Proxy.Close()
				delete(cm.connections, id)
			}
		}
		cm.mu.Unlock()
	}
}

// GetActiveConnections returns count of active connections
func (cm *ConnectionManager) GetActiveConnections() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return len(cm.connections)
}
