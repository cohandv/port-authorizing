package api

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/davidcohan/port-authorizing/internal/audit"
	"github.com/davidcohan/port-authorizing/internal/proxy"
	"github.com/gorilla/mux"
)

// handleRedisProxy handles Redis protocol connections via HTTP hijacking
// This creates a transparent TCP tunnel with protocol-aware command logging and whitelisting
func (s *Server) handleRedisProxy(w http.ResponseWriter, r *http.Request) {
	username := r.Context().Value(ContextKeyUsername).(string)
	roles, _ := r.Context().Value(ContextKeyRoles).([]string)
	vars := mux.Vars(r)
	connectionID := vars["connectionID"]

	// Validate connection exists and hasn't expired
	conn, err := s.connMgr.GetConnection(connectionID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Connection not found or expired")
		return
	}

	// Verify ownership
	if conn.Username != username {
		respondError(w, http.StatusForbidden, "Access denied")
		return
	}

	// Verify it's a Redis connection
	if conn.Config.Type != "redis" {
		respondError(w, http.StatusBadRequest, "Not a Redis connection")
		return
	}

	// Get whitelist for this user's roles and connection
	whitelist := s.authz.GetWhitelistForConnection(roles, conn.Config.Name)

	// Log audit event
	_ = audit.Log(s.config.Logging.AuditLogPath, username, "redis_connect", conn.Config.Name, map[string]interface{}{
		"connection_id":   connectionID,
		"method":          r.Method,
		"roles":           roles,
		"whitelist_rules": len(whitelist),
		"cluster_mode":    conn.Config.RedisCluster,
	})

	// Hijack HTTP connection to get raw TCP socket
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		respondError(w, http.StatusInternalServerError, "HTTP hijacking not supported")
		return
	}

	clientConn, bufrw, err := hijacker.Hijack()
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to hijack connection: %v", err))
		return
	}
	defer func() { _ = clientConn.Close() }()

	// Register this stream with the connection for timeout enforcement
	conn.RegisterStream(clientConn)
	defer conn.UnregisterStream(clientConn)

	// Send HTTP 200 response to indicate proxy is ready
	_, _ = fmt.Fprintf(bufrw, "HTTP/1.1 200 Connection Established\r\n\r\n")
	_ = bufrw.Flush()

	// Set deadline based on connection expiry
	_ = clientConn.SetDeadline(conn.ExpiresAt)

	// Create Redis proxy with command interception and whitelist
	if conn.Config.RedisCluster {
		// Use cluster-aware proxy
		clusterProxy := proxy.NewRedisClusterProxy(
			conn.Config,
			s.config.Logging.AuditLogPath,
			username,
			connectionID,
			whitelist,
		)
		if s.approvalMgr != nil {
			clusterProxy.SetApprovalManager(s.approvalMgr)
		}

		// Handle the Redis Cluster protocol connection
		err = clusterProxy.HandleConnection(clientConn)
	} else {
		// Use standalone proxy
		standaloneProxy := proxy.NewRedisProxy(
			conn.Config,
			s.config.Logging.AuditLogPath,
			username,
			connectionID,
			whitelist,
		)
		if s.approvalMgr != nil {
			standaloneProxy.SetApprovalManager(s.approvalMgr)
		}

		// Handle the Redis protocol connection
		err = standaloneProxy.HandleConnection(clientConn)
	}

	if err != nil {
		_ = audit.Log(s.config.Logging.AuditLogPath, username, "redis_error", conn.Config.Name, map[string]interface{}{
			"connection_id": connectionID,
			"error":         err.Error(),
		})
		return
	}

	_ = audit.Log(s.config.Logging.AuditLogPath, username, "redis_disconnect", conn.Config.Name, map[string]interface{}{
		"connection_id": connectionID,
	})
}

// handleRedisWebSocket handles Redis connections via WebSocket with protocol-aware parsing
func (s *Server) handleRedisWebSocket(w http.ResponseWriter, r *http.Request) {
	username := r.Context().Value(ContextKeyUsername).(string)
	roles, _ := r.Context().Value(ContextKeyRoles).([]string)
	vars := mux.Vars(r)
	connectionID := vars["connectionID"]

	// Get connection (already validated in parent function)
	conn, _ := s.connMgr.GetConnection(connectionID)

	// Get whitelist for this user's roles
	whitelist := s.authz.GetWhitelistForConnection(roles, conn.Config.Name)

	// Log audit event
	_ = audit.Log(s.config.Logging.AuditLogPath, username, "redis_connect_websocket", conn.Config.Name, map[string]interface{}{
		"connection_id":   connectionID,
		"method":          r.Method,
		"roles":           roles,
		"whitelist_rules": len(whitelist),
		"cluster_mode":    conn.Config.RedisCluster,
	})

	// Upgrade HTTP connection to WebSocket
	wsConn, wsErr := upgrader.Upgrade(w, r, nil)
	if wsErr != nil {
		_ = audit.Log(s.config.Logging.AuditLogPath, username, "websocket_upgrade_failed", conn.Config.Name, map[string]interface{}{
			"connection_id": connectionID,
			"error":         wsErr.Error(),
		})
		return
	}
	defer func() { _ = wsConn.Close() }()

	// Setup ping/pong keepalive
	_ = wsConn.SetReadDeadline(time.Now().Add(60 * time.Second))
	wsConn.SetPongHandler(func(string) error {
		_ = wsConn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Create a virtual connection that wraps WebSocket
	wsNetConn := &websocketConn{
		ws:   wsConn,
		done: make(chan struct{}),
	}
	defer func() {
		_ = wsNetConn.Close()
	}()

	// Create Redis proxy based on cluster mode and handle connection
	var connErr error

	if conn.Config.RedisCluster {
		clusterProxy := proxy.NewRedisClusterProxy(
			conn.Config,
			s.config.Logging.AuditLogPath,
			username,
			connectionID,
			whitelist,
		)
		if s.approvalMgr != nil {
			clusterProxy.SetApprovalManager(s.approvalMgr)
		}

		// Handle the Redis Cluster protocol connection through WebSocket
		connErr = clusterProxy.HandleConnection(wsNetConn)
	} else {
		standaloneProxy := proxy.NewRedisProxy(
			conn.Config,
			s.config.Logging.AuditLogPath,
			username,
			connectionID,
			whitelist,
		)
		if s.approvalMgr != nil {
			standaloneProxy.SetApprovalManager(s.approvalMgr)
		}

		// Handle the Redis protocol connection through WebSocket
		connErr = standaloneProxy.HandleConnection(wsNetConn)
	}

	if connErr != nil {
		if connErr != io.EOF {
			_ = audit.Log(s.config.Logging.AuditLogPath, username, "redis_error", conn.Config.Name, map[string]interface{}{
				"connection_id": connectionID,
				"error":         connErr.Error(),
			})
		}
	}

	_ = audit.Log(s.config.Logging.AuditLogPath, username, "redis_disconnect_websocket", conn.Config.Name, map[string]interface{}{
		"connection_id": connectionID,
	})
}
