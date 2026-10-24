package proxy

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/davidcohan/port-authorizing/internal/approval"
	"github.com/davidcohan/port-authorizing/internal/config"
	"github.com/google/uuid"
)

// Connection represents an active proxy connection
type Connection struct {
	ID        string
	Username  string
	Roles     []string
	Config    *config.ConnectionConfig
	Proxy     Protocol
	CreatedAt time.Time
	ExpiresAt time.Time

	// Active TCP connections for this proxy connection
	activeStreams map[net.Conn]bool
	streamsMu     sync.Mutex
}

// RegisterStream registers an active TCP stream for this connection
func (c *Connection) RegisterStream(conn net.Conn) {
	c.streamsMu.Lock()
	defer c.streamsMu.Unlock()
	if c.activeStreams == nil {
		c.activeStreams = make(map[net.Conn]bool)
	}
	c.activeStreams[conn] = true
}

// UnregisterStream removes a TCP stream from active tracking
func (c *Connection) UnregisterStream(conn net.Conn) {
	c.streamsMu.Lock()
	defer c.streamsMu.Unlock()
	delete(c.activeStreams, conn)
}

// CloseAllStreams forcefully closes all active TCP streams
func (c *Connection) CloseAllStreams() {
	c.streamsMu.Lock()
	defer c.streamsMu.Unlock()
	for conn := range c.activeStreams {
		_ = conn.Close()
	}
	c.activeStreams = make(map[net.Conn]bool)
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
func (cm *ConnectionManager) CreateConnection(username string, roles []string, connConfig *config.ConnectionConfig, duration time.Duration, whitelist []string, auditLogPath string, approvalMgr *approval.Manager, apiConfig *config.Config) (string, time.Time, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Generate unique connection ID first (needed for proxy creation)
	connectionID := uuid.New().String()

	// Create protocol-specific proxy
	// Note: postgres and redis don't use the Protocol interface, they have dedicated handlers
	var proxy Protocol
	var err error

	if connConfig.Type != "postgres" && connConfig.Type != "redis" && connConfig.Type != "k8s_services" {
		if connConfig.Type == "http" || connConfig.Type == "https" {
			// Create HTTP proxy with whitelist support
			httpProxy := NewHTTPProxyWithWhitelist(connConfig, whitelist, auditLogPath, username, connectionID, roles, apiConfig)

			// Set approval manager if provided
			if approvalMgr != nil {
				httpProxy.SetApprovalManager(approvalMgr)
			}

			proxy = httpProxy
		} else {
			// Other protocols don't support whitelist yet
			proxy, err = NewProtocol(connConfig)
			if err != nil {
				return "", time.Time{}, fmt.Errorf("failed to create proxy: %w", err)
			}
		}
	}
	// For postgres, redis, and k8s_services, Proxy will be nil - they're handled by dedicated WebSocket handlers

	expiresAt := time.Now().Add(duration)

	conn := &Connection{
		ID:        connectionID,
		Username:  username,
		Roles:     roles,
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

	if conn.Proxy != nil {
		_ = conn.Proxy.Close()
	}
	delete(cm.connections, connectionID)

	return nil
}

// CloseAll closes all active connections
func (cm *ConnectionManager) CloseAll() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for _, conn := range cm.connections {
		if conn.Proxy != nil {
			_ = conn.Proxy.Close()
		}
	}

	cm.connections = make(map[string]*Connection)
	cm.cleanupTicker.Stop()
}

// cleanupExpired removes expired connections and forcefully closes active streams
func (cm *ConnectionManager) cleanupExpired() {
	for range cm.cleanupTicker.C {
		cm.mu.Lock()
		now := time.Now()
		for id, conn := range cm.connections {
			if now.After(conn.ExpiresAt) {
				// Forcefully close all active TCP streams for this connection
				conn.CloseAllStreams()

				// Close the protocol handler (if not postgres)
				if conn.Proxy != nil {
					_ = conn.Proxy.Close()
				}

				// Remove from tracking
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

// ActiveConnectionInfo represents info about an active connection
type ActiveConnectionInfo struct {
	ID            string    `json:"id"`
	Username      string    `json:"username"`
	ConnectionName string   `json:"connection_name"`
	ConnectionType string   `json:"connection_type"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
	ActiveStreams int       `json:"active_streams"`
}

// GetActiveConnectionsList returns detailed list of active connections
func (cm *ConnectionManager) GetActiveConnectionsList() []ActiveConnectionInfo {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	list := make([]ActiveConnectionInfo, 0, len(cm.connections))
	for _, conn := range cm.connections {
		conn.streamsMu.Lock()
		streamCount := len(conn.activeStreams)
		conn.streamsMu.Unlock()

		list = append(list, ActiveConnectionInfo{
			ID:             conn.ID,
			Username:       conn.Username,
			ConnectionName: conn.Config.Name,
			ConnectionType: conn.Config.Type,
			CreatedAt:      conn.CreatedAt,
			ExpiresAt:      conn.ExpiresAt,
			ActiveStreams:  streamCount,
		})
	}
	return list
}
