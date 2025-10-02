package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/davidcohan/port-authorizing/internal/audit"
	"github.com/davidcohan/port-authorizing/internal/config"
	"github.com/gorilla/mux"
)

// ConnectionInfo represents connection information for the client
type ConnectionInfo struct {
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ConnectRequest represents a connection request
type ConnectRequest struct {
	Duration time.Duration `json:"duration"`
}

// ConnectResponse represents a connection response
type ConnectResponse struct {
	ConnectionID string    `json:"connection_id"`
	ExpiresAt    time.Time `json:"expires_at"`
	ProxyURL     string    `json:"proxy_url"`
}

// handleListConnections returns list of available connections
func (s *Server) handleListConnections(w http.ResponseWriter, r *http.Request) {
	username := r.Context().Value("username").(string)

	// Log audit event
	audit.Log(s.config.Logging.AuditLogPath, username, "list_connections", "", nil)

	connections := make([]ConnectionInfo, 0, len(s.config.Connections))
	for _, conn := range s.config.Connections {
		connections = append(connections, ConnectionInfo{
			Name:     conn.Name,
			Type:     conn.Type,
			Metadata: conn.Metadata,
		})
	}

	respondJSON(w, http.StatusOK, connections)
}

// handleConnect establishes a new proxy connection
func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	username := r.Context().Value("username").(string)
	vars := mux.Vars(r)
	connectionName := vars["name"]

	// Find connection config
	var connConfig *config.ConnectionConfig
	for i := range s.config.Connections {
		if s.config.Connections[i].Name == connectionName {
			connConfig = &s.config.Connections[i]
			break
		}
	}

	if connConfig == nil {
		respondError(w, http.StatusNotFound, "Connection not found")
		return
	}

	// Parse request
	var req ConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Duration = s.config.Server.MaxConnectionDuration
	}

	// Enforce max duration
	if req.Duration > s.config.Server.MaxConnectionDuration {
		req.Duration = s.config.Server.MaxConnectionDuration
	}

	// Create connection
	connectionID, expiresAt, err := s.connMgr.CreateConnection(username, connConfig, req.Duration)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create connection")
		return
	}

	// Log audit event
	audit.Log(s.config.Logging.AuditLogPath, username, "connect", connectionName, map[string]interface{}{
		"connection_id": connectionID,
		"duration":      req.Duration.String(),
	})

	respondJSON(w, http.StatusOK, ConnectResponse{
		ConnectionID: connectionID,
		ExpiresAt:    expiresAt,
		ProxyURL:     fmt.Sprintf("/api/proxy/%s", connectionID),
	})
}

// handleProxy handles proxying requests to the actual endpoint
func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	username := r.Context().Value("username").(string)
	vars := mux.Vars(r)
	connectionID := vars["connectionID"]

	// Get connection
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

	// Log audit event
	audit.Log(s.config.Logging.AuditLogPath, username, "proxy_request", conn.Config.Name, map[string]interface{}{
		"connection_id": connectionID,
		"method":        r.Method,
		"path":          r.URL.Path,
	})

	// Proxy the request based on protocol type
	if err := conn.Proxy.HandleRequest(w, r); err != nil {
		respondError(w, http.StatusBadGateway, fmt.Sprintf("Proxy error: %v", err))
		return
	}
}

// respondJSON sends a JSON response
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// respondError sends an error response
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}
