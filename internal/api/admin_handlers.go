package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/davidcohan/port-authorizing/internal/audit"
	"github.com/davidcohan/port-authorizing/internal/config"
	"github.com/davidcohan/port-authorizing/internal/security"
	"github.com/gorilla/mux"
)

// Configuration Management Handlers

// handleGetConfig returns the current configuration
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.GetConfig()

	// Sanitize sensitive data
	sanitized := sanitizeConfig(cfg)
	respondJSON(w, http.StatusOK, sanitized)
}

// handleUpdateConfig updates the configuration and reloads the server
func (s *Server) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	var newCfg config.Config
	if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid configuration format")
		return
	}

	// Get comment from query parameter
	comment := r.URL.Query().Get("comment")
	if comment == "" {
		comment = "Configuration updated via admin UI"
	}

	// Add username to comment
	username := r.Context().Value(ContextKeyUsername).(string)
	comment = fmt.Sprintf("%s (by %s)", comment, username)

	// Save configuration
	if err := s.storageBackend.Save(r.Context(), &newCfg, comment); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save configuration: %v", err))
		return
	}

	// Reload configuration
	if err := s.ReloadConfig(&newCfg); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to reload configuration: %v", err))
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Configuration updated successfully",
		"comment": comment,
	})
}

// handleListConfigVersions lists available configuration versions
func (s *Server) handleListConfigVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := s.storageBackend.ListVersions(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to list versions: %v", err))
		return
	}

	respondJSON(w, http.StatusOK, versions)
}

// handleGetConfigVersion retrieves a specific configuration version
func (s *Server) handleGetConfigVersion(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	versionID := vars["id"]

	cfg, err := s.storageBackend.LoadVersion(r.Context(), versionID)
	if err != nil {
		respondError(w, http.StatusNotFound, fmt.Sprintf("Version not found: %v", err))
		return
	}

	sanitized := sanitizeConfig(cfg)
	respondJSON(w, http.StatusOK, sanitized)
}

// handleRollbackConfig rolls back to a specific configuration version
func (s *Server) handleRollbackConfig(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	versionID := vars["id"]

	_, err := s.storageBackend.Rollback(r.Context(), versionID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to rollback: %v", err))
		return
	}

	// Load the rolled back configuration
	cfg, err := s.storageBackend.Load(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to load configuration: %v", err))
		return
	}

	// Reload the server
	if err := s.ReloadConfig(cfg); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to reload configuration: %v", err))
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message":    "Configuration rolled back successfully",
		"version_id": versionID,
	})
}

// Connection Management Handlers

// ConnectionResponse is a connection config with duration as string for JSON
type ConnectionResponse struct {
	Name            string            `json:"name"`
	Type            string            `json:"type"`
	Host            string            `json:"host"`
	Port            int               `json:"port"`
	Scheme          string            `json:"scheme,omitempty"`
	Duration        string            `json:"duration,omitempty"`
	Tags            []string          `json:"tags,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	BackendUsername string            `json:"backend_username,omitempty"`
	BackendPassword string            `json:"backend_password,omitempty"`
	BackendDatabase string            `json:"backend_database,omitempty"`
	Whitelist       []string          `json:"whitelist,omitempty"`
}

// toConnectionResponse converts ConnectionConfig to ConnectionResponse with duration as string
func toConnectionResponse(conn config.ConnectionConfig) ConnectionResponse {
	resp := ConnectionResponse{
		Name:            conn.Name,
		Type:            conn.Type,
		Host:            conn.Host,
		Port:            conn.Port,
		Scheme:          conn.Scheme,
		Tags:            conn.Tags,
		Metadata:        conn.Metadata,
		BackendUsername: conn.BackendUsername,
		BackendPassword: conn.BackendPassword,
		BackendDatabase: conn.BackendDatabase,
		Whitelist:       conn.Whitelist,
	}

	// Convert duration to string format
	if conn.Duration > 0 {
		resp.Duration = conn.Duration.String()
	}

	return resp
}

// handleListAllConnections lists all configured connections (admin view)
func (s *Server) handleListAllConnections(w http.ResponseWriter, r *http.Request) {
	cfg := s.GetConfig()

	// Convert connections to response format with duration as string
	connections := make([]ConnectionResponse, len(cfg.Connections))
	for i, conn := range cfg.Connections {
		connections[i] = toConnectionResponse(conn)
	}

	respondJSON(w, http.StatusOK, connections)
}

// handleCreateConnection creates a new connection
func (s *Server) handleCreateConnection(w http.ResponseWriter, r *http.Request) {
	// Decode into a generic map first to handle duration as string
	var rawConn map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&rawConn); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid connection format: %v", err))
		return
	}

	// Convert back to JSON and decode into ConnectionConfig, handling duration specially
	var conn config.ConnectionConfig
	jsonBytes, _ := json.Marshal(rawConn)

	// Parse duration if provided as string
	if durationStr, ok := rawConn["duration"].(string); ok && durationStr != "" {
		duration, err := time.ParseDuration(durationStr)
		if err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid duration format: %v. Use formats like 30m, 2h, 1h30m", err))
			return
		}
		delete(rawConn, "duration")
		jsonBytes, _ = json.Marshal(rawConn)
		if err := json.Unmarshal(jsonBytes, &conn); err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid connection format: %v", err))
			return
		}
		conn.Duration = duration
	} else {
		if err := json.Unmarshal(jsonBytes, &conn); err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid connection format: %v", err))
			return
		}
	}

	// Validate connection
	if conn.Name == "" || conn.Type == "" || conn.Host == "" || conn.Port == 0 {
		respondError(w, http.StatusBadRequest, "Missing required fields: name, type, host, port")
		return
	}

	// Validate metadata has description if metadata is provided
	if conn.Metadata != nil && len(conn.Metadata) > 0 {
		description, ok := conn.Metadata["description"]
		if !ok || description == "" {
			respondError(w, http.StatusBadRequest, "Metadata must include a 'description' field")
			return
		}
	}

	cfg := s.GetConfig()

	// Check if connection already exists
	for _, existing := range cfg.Connections {
		if existing.Name == conn.Name {
			respondError(w, http.StatusConflict, "Connection with this name already exists")
			return
		}
	}

	// Add connection
	cfg.Connections = append(cfg.Connections, conn)

	// Save and reload
	username := r.Context().Value(ContextKeyUsername).(string)
	comment := fmt.Sprintf("Added connection %s (by %s)", conn.Name, username)
	if err := s.storageBackend.Save(r.Context(), cfg, comment); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save: %v", err))
		return
	}

	if err := s.ReloadConfig(cfg); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to reload: %v", err))
		return
	}

	respondJSON(w, http.StatusCreated, toConnectionResponse(conn))
}

// handleUpdateConnection updates an existing connection
func (s *Server) handleUpdateConnection(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	// Decode into a generic map first to handle duration as string
	var rawConn map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&rawConn); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid connection format: %v", err))
		return
	}

	// Convert back to JSON and decode into ConnectionConfig, handling duration specially
	var updatedConn config.ConnectionConfig
	jsonBytes, _ := json.Marshal(rawConn)

	// Parse duration if provided as string
	if durationStr, ok := rawConn["duration"].(string); ok && durationStr != "" {
		duration, err := time.ParseDuration(durationStr)
		if err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid duration format: %v. Use formats like 30m, 2h, 1h30m", err))
			return
		}
		delete(rawConn, "duration")
		jsonBytes, _ = json.Marshal(rawConn)
		if err := json.Unmarshal(jsonBytes, &updatedConn); err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid connection format: %v", err))
			return
		}
		updatedConn.Duration = duration
	} else {
		if err := json.Unmarshal(jsonBytes, &updatedConn); err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid connection format: %v", err))
			return
		}
	}

	// Validate metadata has description if metadata is provided
	if updatedConn.Metadata != nil && len(updatedConn.Metadata) > 0 {
		description, ok := updatedConn.Metadata["description"]
		if !ok || description == "" {
			respondError(w, http.StatusBadRequest, "Metadata must include a 'description' field")
			return
		}
	}

	cfg := s.GetConfig()

	// Find and update connection
	found := false
	for i, conn := range cfg.Connections {
		if conn.Name == name {
			// Preserve the original name if not provided
			if updatedConn.Name == "" {
				updatedConn.Name = name
			}

			// Preserve backend_password if not provided (empty string means keep existing)
			if updatedConn.BackendPassword == "" && conn.BackendPassword != "" {
				updatedConn.BackendPassword = conn.BackendPassword
			}

			cfg.Connections[i] = updatedConn
			found = true
			break
		}
	}

	if !found {
		respondError(w, http.StatusNotFound, "Connection not found")
		return
	}

	// Save and reload
	username := r.Context().Value(ContextKeyUsername).(string)
	comment := fmt.Sprintf("Updated connection %s (by %s)", name, username)
	if err := s.storageBackend.Save(r.Context(), cfg, comment); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save: %v", err))
		return
	}

	if err := s.ReloadConfig(cfg); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to reload: %v", err))
		return
	}

	respondJSON(w, http.StatusOK, toConnectionResponse(updatedConn))
}

// handleDeleteConnection deletes a connection
func (s *Server) handleDeleteConnection(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	cfg := s.GetConfig()

	// Find and remove connection
	found := false
	newConnections := []config.ConnectionConfig{}
	for _, conn := range cfg.Connections {
		if conn.Name == name {
			found = true
			continue
		}
		newConnections = append(newConnections, conn)
	}

	if !found {
		respondError(w, http.StatusNotFound, "Connection not found")
		return
	}

	cfg.Connections = newConnections

	// Save and reload
	username := r.Context().Value(ContextKeyUsername).(string)
	comment := fmt.Sprintf("Deleted connection %s (by %s)", name, username)
	if err := s.storageBackend.Save(r.Context(), cfg, comment); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save: %v", err))
		return
	}

	if err := s.ReloadConfig(cfg); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to reload: %v", err))
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Connection deleted successfully"})
}

// User Management Handlers (Local Auth Only)

// handleListUsers lists all local users
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	cfg := s.GetConfig()

	// Sanitize passwords
	users := []map[string]interface{}{}
	for _, user := range cfg.Auth.Users {
		users = append(users, map[string]interface{}{
			"username": user.Username,
			"roles":    user.Roles,
		})
	}

	respondJSON(w, http.StatusOK, users)
}

// handleCreateUser creates a new local user
func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string   `json:"username"`
		Password string   `json:"password"`
		Roles    []string `json:"roles"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	if req.Username == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, "Username and password are required")
		return
	}

	cfg := s.GetConfig()

	// Check if user already exists
	for _, user := range cfg.Auth.Users {
		if user.Username == req.Username {
			respondError(w, http.StatusConflict, "User already exists")
			return
		}
	}

	// Add user
	// Note: Passwords are stored in plain text for operational requirements
	newUser := config.User{
		Username: req.Username,
		Password: req.Password,
		Roles:    req.Roles,
	}
	cfg.Auth.Users = append(cfg.Auth.Users, newUser)

	// Save and reload
	username := r.Context().Value(ContextKeyUsername).(string)
	comment := fmt.Sprintf("Added user %s (by %s)", req.Username, username)
	if err := s.storageBackend.Save(r.Context(), cfg, comment); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save: %v", err))
		return
	}

	if err := s.ReloadConfig(cfg); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to reload: %v", err))
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"username": req.Username,
		"roles":    req.Roles,
	})
}

// handleUpdateUser updates an existing user
func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]

	var req struct {
		Password *string  `json:"password,omitempty"`
		Roles    []string `json:"roles"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	cfg := s.GetConfig()

	// Find and update user
	found := false
	for i, user := range cfg.Auth.Users {
		if user.Username == username {
			// Update roles
			cfg.Auth.Users[i].Roles = req.Roles

			// Update password if provided
			// Note: Passwords are stored in plain text for operational requirements
			if req.Password != nil && *req.Password != "" {
				cfg.Auth.Users[i].Password = *req.Password
			}

			found = true
			break
		}
	}

	if !found {
		respondError(w, http.StatusNotFound, "User not found")
		return
	}

	// Save and reload
	adminUsername := r.Context().Value(ContextKeyUsername).(string)
	comment := fmt.Sprintf("Updated user %s (by %s)", username, adminUsername)
	if err := s.storageBackend.Save(r.Context(), cfg, comment); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save: %v", err))
		return
	}

	if err := s.ReloadConfig(cfg); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to reload: %v", err))
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"username": username,
		"roles":    req.Roles,
	})
}

// handleDeleteUser deletes a user
func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]

	cfg := s.GetConfig()

	// Find and remove user
	found := false
	newUsers := []config.User{}
	for _, user := range cfg.Auth.Users {
		if user.Username == username {
			found = true
			continue
		}
		newUsers = append(newUsers, user)
	}

	if !found {
		respondError(w, http.StatusNotFound, "User not found")
		return
	}

	cfg.Auth.Users = newUsers

	// Save and reload
	adminUsername := r.Context().Value(ContextKeyUsername).(string)
	comment := fmt.Sprintf("Deleted user %s (by %s)", username, adminUsername)
	if err := s.storageBackend.Save(r.Context(), cfg, comment); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save: %v", err))
		return
	}

	if err := s.ReloadConfig(cfg); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to reload: %v", err))
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "User deleted successfully"})
}

// Policy Management Handlers

// handleListPolicies lists all policies
func (s *Server) handleListPolicies(w http.ResponseWriter, r *http.Request) {
	cfg := s.GetConfig()
	respondJSON(w, http.StatusOK, cfg.Policies)
}

// handleCreatePolicy creates a new policy
func (s *Server) handleCreatePolicy(w http.ResponseWriter, r *http.Request) {
	var policy config.RolePolicy
	if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid policy format")
		return
	}

	if policy.Name == "" {
		respondError(w, http.StatusBadRequest, "Policy name is required")
		return
	}

	cfg := s.GetConfig()

	// Check if policy already exists
	for _, existing := range cfg.Policies {
		if existing.Name == policy.Name {
			respondError(w, http.StatusConflict, "Policy with this name already exists")
			return
		}
	}

	// Add policy
	cfg.Policies = append(cfg.Policies, policy)

	// Save and reload
	username := r.Context().Value(ContextKeyUsername).(string)
	comment := fmt.Sprintf("Added policy %s (by %s)", policy.Name, username)
	if err := s.storageBackend.Save(r.Context(), cfg, comment); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save: %v", err))
		return
	}

	if err := s.ReloadConfig(cfg); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to reload: %v", err))
		return
	}

	respondJSON(w, http.StatusCreated, policy)
}

// handleUpdatePolicy updates an existing policy
func (s *Server) handleUpdatePolicy(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	var updatedPolicy config.RolePolicy
	if err := json.NewDecoder(r.Body).Decode(&updatedPolicy); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid policy format")
		return
	}

	cfg := s.GetConfig()

	// Find and update policy
	found := false
	for i, policy := range cfg.Policies {
		if policy.Name == name {
			// Preserve the original name if not provided
			if updatedPolicy.Name == "" {
				updatedPolicy.Name = name
			}
			cfg.Policies[i] = updatedPolicy
			found = true
			break
		}
	}

	if !found {
		respondError(w, http.StatusNotFound, "Policy not found")
		return
	}

	// Save and reload
	username := r.Context().Value(ContextKeyUsername).(string)
	comment := fmt.Sprintf("Updated policy %s (by %s)", name, username)
	if err := s.storageBackend.Save(r.Context(), cfg, comment); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save: %v", err))
		return
	}

	if err := s.ReloadConfig(cfg); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to reload: %v", err))
		return
	}

	respondJSON(w, http.StatusOK, updatedPolicy)
}

// handleDeletePolicy deletes a policy
func (s *Server) handleDeletePolicy(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	cfg := s.GetConfig()

	// Find and remove policy
	found := false
	newPolicies := []config.RolePolicy{}
	for _, policy := range cfg.Policies {
		if policy.Name == name {
			found = true
			continue
		}
		newPolicies = append(newPolicies, policy)
	}

	if !found {
		respondError(w, http.StatusNotFound, "Policy not found")
		return
	}

	cfg.Policies = newPolicies

	// Save and reload
	username := r.Context().Value(ContextKeyUsername).(string)
	comment := fmt.Sprintf("Deleted policy %s (by %s)", name, username)
	if err := s.storageBackend.Save(r.Context(), cfg, comment); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save: %v", err))
		return
	}

	if err := s.ReloadConfig(cfg); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to reload: %v", err))
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Policy deleted successfully"})
}

// Audit Log Handlers

// handleGetAuditLogs returns audit logs with filtering and pagination
func (s *Server) handleGetAuditLogs(w http.ResponseWriter, r *http.Request) {
	cfg := s.GetConfig()
	auditLogPath := cfg.Logging.AuditLogPath

	// Parse query parameters for filtering
	username := r.URL.Query().Get("username")
	action := r.URL.Query().Get("action")
	connection := r.URL.Query().Get("connection")

	var logs []string
	var totalCount int

	// If audit goes to stdout or memory, use in-memory buffer
	if auditLogPath == "stdout" || auditLogPath == "-" {
		// Use in-memory buffer
		recentLogs := audit.GetRecentLogs(1000)
		for _, entry := range recentLogs {
			// Filter
			if username != "" && entry.Username != username {
				continue
			}
			if action != "" && entry.Action != action {
				continue
			}
			if connection != "" && entry.Resource != connection {
				continue
			}

			// Format as JSON string for display
			logJSON, _ := json.Marshal(entry)
			logs = append(logs, string(logJSON))
		}
		totalCount = len(logs)
	} else {
		// Read from file
		data, err := os.ReadFile(auditLogPath)
		if err != nil {
			// Fallback to memory if file not readable
			recentLogs := audit.GetRecentLogs(1000)
			for _, entry := range recentLogs {
				if username != "" && entry.Username != username {
					continue
				}
				if action != "" && entry.Action != action {
					continue
				}
				if connection != "" && entry.Resource != connection {
					continue
				}
				logJSON, _ := json.Marshal(entry)
				logs = append(logs, string(logJSON))
			}
			totalCount = len(logs)
		} else {
			// Parse file contents
			lines := strings.Split(string(data), "\n")

			for _, line := range lines {
				if line == "" {
					continue
				}

				// Simple filtering
				if username != "" && !strings.Contains(line, username) {
					continue
				}
				if action != "" && !strings.Contains(line, action) {
					continue
				}
				if connection != "" && !strings.Contains(line, connection) {
					continue
				}

				logs = append(logs, line)
			}
			totalCount = len(logs)
		}
	}

	// Return last 100 entries (pagination can be added)
	start := 0
	if len(logs) > 100 {
		start = len(logs) - 100
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"logs":  logs[start:],
		"total": totalCount,
	})
}

// handleGetAuditStats returns audit log statistics
func (s *Server) handleGetAuditStats(w http.ResponseWriter, r *http.Request) {
	cfg := s.GetConfig()
	auditLogPath := cfg.Logging.AuditLogPath

	var totalEvents int

	// Get memory buffer stats
	currentMB, maxMB, entryCount, memEnabled := audit.GetMemoryStats()

	// If audit goes to stdout or memory, use in-memory buffer
	if auditLogPath == "stdout" || auditLogPath == "-" {
		recentLogs := audit.GetRecentLogs(0) // Get all
		totalEvents = len(recentLogs)
	} else {
		// Read audit log file
		data, err := os.ReadFile(auditLogPath)
		if err != nil {
			// Fallback to memory
			recentLogs := audit.GetRecentLogs(0)
			totalEvents = len(recentLogs)
		} else {
			// Count lines (simple stats)
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if line != "" {
					totalEvents++
				}
			}
		}
	}

	response := map[string]interface{}{
		"total_events": totalEvents,
		"log_path":     auditLogPath,
		"source":       getAuditSource(auditLogPath, memEnabled),
		"memory": map[string]interface{}{
			"enabled":       memEnabled,
			"current_mb":    fmt.Sprintf("%.2f", currentMB),
			"max_mb":        fmt.Sprintf("%.2f", maxMB),
			"entry_count":   entryCount,
			"configured_mb": cfg.Logging.AuditMemoryMB,
		},
	}

	respondJSON(w, http.StatusOK, response)
}

// getAuditSource returns a user-friendly description of where audit logs are stored
func getAuditSource(logPath string, memEnabled bool) string {
	if !memEnabled {
		return "file: " + logPath + " (memory buffer disabled)"
	}
	if logPath == "stdout" || logPath == "-" {
		return "in-memory buffer"
	}
	return "file: " + logPath + " (with memory buffer)"
}

// System Status Handler

// handleGetSystemStatus returns system status and active connections
func (s *Server) handleGetSystemStatus(w http.ResponseWriter, r *http.Request) {
	cfg := s.GetConfig()

	// Get active connections from connection manager
	activeConnections := s.connMgr.GetActiveConnections()

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":                 "running",
		"active_connections":     activeConnections,
		"configured_connections": len(cfg.Connections),
		"policies":               len(cfg.Policies),
		"users":                  len(cfg.Auth.Users),
		"uptime":                 time.Since(time.Now()).String(), // Placeholder
	})
}

// Helper Functions

// sanitizeConfig removes sensitive data from configuration
func sanitizeConfig(cfg *config.Config) map[string]interface{} {
	sanitized := map[string]interface{}{
		"server":      cfg.Server,
		"connections": cfg.Connections,
		"policies":    cfg.Policies,
		"security":    cfg.Security,
		"logging":     cfg.Logging,
		"approval":    cfg.Approval,
		"storage":     cfg.Storage,
	}

	// Sanitize auth config
	authSanitized := map[string]interface{}{
		"token_expiry": cfg.Auth.TokenExpiry,
		"providers":    cfg.Auth.Providers,
	}

	// Don't include JWT secret
	// Sanitize user passwords
	users := []map[string]interface{}{}
	for _, user := range cfg.Auth.Users {
		users = append(users, map[string]interface{}{
			"username": user.Username,
			"roles":    user.Roles,
		})
	}
	authSanitized["users"] = users

	sanitized["auth"] = authSanitized

	// Sanitize connection backend credentials
	connections := []map[string]interface{}{}
	for _, conn := range cfg.Connections {
		connMap := map[string]interface{}{
			"name":     conn.Name,
			"type":     conn.Type,
			"host":     conn.Host,
			"port":     conn.Port,
			"scheme":   conn.Scheme,
			"duration": conn.Duration,
			"tags":     conn.Tags,
			"metadata": conn.Metadata,
		}
		// Include backend username but not password
		if conn.BackendUsername != "" {
			connMap["backend_username"] = conn.BackendUsername
			connMap["backend_password"] = "********"
		}
		if conn.BackendDatabase != "" {
			connMap["backend_database"] = conn.BackendDatabase
		}
		connections = append(connections, connMap)
	}
	sanitized["connections"] = connections

	return sanitized
}

// Policy Tester Handler

// handlePolicyTest tests which policies apply to a specific connection and role combination
func (s *Server) handlePolicyTest(w http.ResponseWriter, r *http.Request) {
	var testData struct {
		Connection string `json:"connection"`
		Role       string `json:"role"`
		QueryType  string `json:"query_type"` // "http" or "database"
		Method     string `json:"method"`     // for HTTP requests
		Path       string `json:"path"`       // for HTTP requests
		Query      string `json:"query"`      // for database queries
	}

	if err := json.NewDecoder(r.Body).Decode(&testData); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid test data format")
		return
	}

	if testData.Connection == "" || testData.Role == "" {
		respondError(w, http.StatusBadRequest, "Connection and role are required")
		return
	}

	cfg := s.GetConfig()

	// Find the connection
	var connection *config.ConnectionConfig
	for _, conn := range cfg.Connections {
		if conn.Name == testData.Connection {
			connection = &conn
			break
		}
	}

	if connection == nil {
		respondError(w, http.StatusNotFound, "Connection not found")
		return
	}

	// Find matching policies
	var matchingPolicies []map[string]interface{}
	hasAccess := false

	// Auto-detect query type if not specified
	queryType := testData.QueryType
	if queryType == "" {
		// Auto-detect based on connection type
		if connection.Type == "postgres" || connection.Type == "oracle" || connection.Type == "mysql" {
			queryType = "database"
		} else if connection.Type == "http" || connection.Type == "https" {
			queryType = "http"
		} else {
			// Default to database for TCP and unknown types
			queryType = "database"
		}
	}

	for _, policy := range cfg.Policies {
		// Check if role matches
		roleMatches := false
		for _, policyRole := range policy.Roles {
			if policyRole == testData.Role {
				roleMatches = true
				break
			}
		}

		if !roleMatches {
			continue
		}

		// Check if tags match (if connection has tags)
		tagMatch := "no tags"
		if len(connection.Tags) > 0 && len(policy.Tags) > 0 {
			// Check if any connection tag matches any policy tag
			for _, connTag := range connection.Tags {
				for _, policyTag := range policy.Tags {
					if connTag == policyTag {
						tagMatch = "matched"
						break
					}
				}
				if tagMatch == "matched" {
					break
				}
			}
			if tagMatch != "matched" {
				tagMatch = "no match"
			}
		} else if len(connection.Tags) == 0 && len(policy.Tags) == 0 {
			tagMatch = "no tags required"
		} else if len(connection.Tags) == 0 {
			tagMatch = "connection has no tags"
		} else {
			tagMatch = "policy has no tags"
		}

		// If tags don't match, skip this policy
		if len(connection.Tags) > 0 && len(policy.Tags) > 0 && tagMatch != "matched" {
			continue
		}

		// This policy matches - add it to results
		policyResult := map[string]interface{}{
			"name":      policy.Name,
			"roles":     policy.Roles,
			"tags":      policy.Tags,
			"tagMatch":  tagMatch,
			"whitelist": policy.Whitelist,
		}
		matchingPolicies = append(matchingPolicies, policyResult)

		// Check if this policy would allow access for the given query
		var queryToTest string
		var hasSpecificQuery bool

		if queryType == "database" && testData.Query != "" {
			// Database query - validate subqueries for PL/SQL scripts
			queryToTest = testData.Query
			hasSpecificQuery = true
		} else if queryType == "http" && testData.Method != "" && testData.Path != "" {
			// HTTP request
			queryToTest = fmt.Sprintf("%s %s", testData.Method, testData.Path)
			hasSpecificQuery = true
		}

		if hasSpecificQuery {
			// Use the authorization logic to check if access would be granted
			whitelist := s.authz.GetWhitelistForConnection([]string{testData.Role}, testData.Connection)
			if len(whitelist) > 0 {
				// Check if the query matches any whitelist rule
				if err := s.authz.ValidatePattern(queryToTest, whitelist); err == nil {
					hasAccess = true
				}
			} else {
				// No whitelist rules means access is allowed
				hasAccess = true
			}
		} else {
			// If no specific query provided, just having a matching policy means potential access
			hasAccess = true
		}
	}

	// If no specific query provided, having any matching policies means access
	if (testData.QueryType == "http" && (testData.Method == "" || testData.Path == "")) ||
		(testData.QueryType == "database" && testData.Query == "") {
		hasAccess = len(matchingPolicies) > 0
	}

	result := map[string]interface{}{
		"hasAccess":        hasAccess,
		"connection":       testData.Connection,
		"role":             testData.Role,
		"query_type":       queryType, // Use the determined query type (auto-detected or specified)
		"method":           testData.Method,
		"path":             testData.Path,
		"query":            testData.Query,
		"matchingPolicies": matchingPolicies,
		"connectionTags":   connection.Tags,
		"connectionType":   connection.Type, // Include connection type for reference
	}

	// Add subquery validation for database queries
	if queryType == "database" && testData.Query != "" {
		validator := security.NewSubqueryValidator()
		whitelist := s.authz.GetWhitelistForConnection([]string{testData.Role}, testData.Connection)
		validationResult := validator.ValidateScript(testData.Query, whitelist)
		result["subquery_validation"] = validationResult
	}

	// Check if approval is required
	if s.approvalMgr != nil {
		var requiresApproval bool
		var approvalTimeout time.Duration

		if queryType == "http" && testData.Method != "" && testData.Path != "" {
			requiresApproval, approvalTimeout = s.approvalMgr.RequiresApproval(testData.Method, testData.Path, connection.Tags)
		} else if queryType == "database" && testData.Query != "" {
			requiresApproval, approvalTimeout = s.approvalMgr.RequiresApproval(testData.Query, "", connection.Tags)
		}

		result["requiresApproval"] = requiresApproval
		if requiresApproval {
			result["approvalTimeout"] = approvalTimeout.String()
		}
	}

	respondJSON(w, http.StatusOK, result)
}

// Approval Management Handlers

// ApprovalConfigResponse is the response for approval configuration
type ApprovalConfigResponse struct {
	Enabled  bool                          `json:"enabled"`
	Patterns []ApprovalPatternResponse     `json:"patterns"`
	Webhook  *config.WebhookApprovalConfig `json:"webhook,omitempty"`
	Slack    *config.SlackApprovalConfig   `json:"slack,omitempty"`
}

// ApprovalPatternResponse represents an approval pattern with index
type ApprovalPatternResponse struct {
	Index          int      `json:"index"`
	Pattern        string   `json:"pattern"`
	Tags           []string `json:"tags,omitempty"`
	TagMatch       string   `json:"tag_match,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds"`
}

// handleGetApprovalConfig returns the approval configuration
func (s *Server) handleGetApprovalConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.GetConfig()

	patterns := make([]ApprovalPatternResponse, len(cfg.Approval.Patterns))
	for i, p := range cfg.Approval.Patterns {
		patterns[i] = ApprovalPatternResponse{
			Index:          i,
			Pattern:        p.Pattern,
			Tags:           p.Tags,
			TagMatch:       p.TagMatch,
			TimeoutSeconds: p.TimeoutSeconds,
		}
	}

	resp := ApprovalConfigResponse{
		Enabled:  cfg.Approval.Enabled,
		Patterns: patterns,
		Webhook:  cfg.Approval.Webhook,
		Slack:    cfg.Approval.Slack,
	}

	respondJSON(w, http.StatusOK, resp)
}

// handleUpdateApprovalEnabled updates the approval enabled status
func (s *Server) handleUpdateApprovalEnabled(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	cfg := s.GetConfig()
	cfg.Approval.Enabled = req.Enabled

	comment := fmt.Sprintf("Updated approval enabled status to %v", req.Enabled)
	if err := s.storageBackend.Save(r.Context(), cfg, comment); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save: %v", err))
		return
	}

	if err := s.ReloadConfig(cfg); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to reload: %v", err))
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Approval status updated successfully",
		"enabled": req.Enabled,
	})
}

// handleCreateApprovalPattern creates a new approval pattern
func (s *Server) handleCreateApprovalPattern(w http.ResponseWriter, r *http.Request) {
	var pattern config.ApprovalPatternConfig
	if err := json.NewDecoder(r.Body).Decode(&pattern); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid pattern: %v", err))
		return
	}

	// Validate pattern
	if pattern.Pattern == "" {
		respondError(w, http.StatusBadRequest, "Pattern is required")
		return
	}

	if pattern.TimeoutSeconds <= 0 {
		pattern.TimeoutSeconds = 300 // Default 5 minutes
	}

	cfg := s.GetConfig()
	cfg.Approval.Patterns = append(cfg.Approval.Patterns, pattern)

	comment := fmt.Sprintf("Added approval pattern: %s", pattern.Pattern)
	if err := s.storageBackend.Save(r.Context(), cfg, comment); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save: %v", err))
		return
	}

	if err := s.ReloadConfig(cfg); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to reload: %v", err))
		return
	}

	resp := ApprovalPatternResponse{
		Index:          len(cfg.Approval.Patterns) - 1,
		Pattern:        pattern.Pattern,
		Tags:           pattern.Tags,
		TagMatch:       pattern.TagMatch,
		TimeoutSeconds: pattern.TimeoutSeconds,
	}

	respondJSON(w, http.StatusCreated, resp)
}

// handleUpdateApprovalPattern updates an existing approval pattern
func (s *Server) handleUpdateApprovalPattern(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	indexStr := vars["index"]

	var index int
	if _, err := fmt.Sscanf(indexStr, "%d", &index); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid index")
		return
	}

	var pattern config.ApprovalPatternConfig
	if err := json.NewDecoder(r.Body).Decode(&pattern); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid pattern: %v", err))
		return
	}

	// Validate pattern
	if pattern.Pattern == "" {
		respondError(w, http.StatusBadRequest, "Pattern is required")
		return
	}

	if pattern.TimeoutSeconds <= 0 {
		pattern.TimeoutSeconds = 300
	}

	cfg := s.GetConfig()
	if index < 0 || index >= len(cfg.Approval.Patterns) {
		respondError(w, http.StatusNotFound, "Pattern not found")
		return
	}

	oldPattern := cfg.Approval.Patterns[index].Pattern
	cfg.Approval.Patterns[index] = pattern

	comment := fmt.Sprintf("Updated approval pattern from '%s' to '%s'", oldPattern, pattern.Pattern)
	if err := s.storageBackend.Save(r.Context(), cfg, comment); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save: %v", err))
		return
	}

	if err := s.ReloadConfig(cfg); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to reload: %v", err))
		return
	}

	resp := ApprovalPatternResponse{
		Index:          index,
		Pattern:        pattern.Pattern,
		Tags:           pattern.Tags,
		TagMatch:       pattern.TagMatch,
		TimeoutSeconds: pattern.TimeoutSeconds,
	}

	respondJSON(w, http.StatusOK, resp)
}

// handleDeleteApprovalPattern deletes an approval pattern
func (s *Server) handleDeleteApprovalPattern(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	indexStr := vars["index"]

	var index int
	if _, err := fmt.Sscanf(indexStr, "%d", &index); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid index")
		return
	}

	cfg := s.GetConfig()
	if index < 0 || index >= len(cfg.Approval.Patterns) {
		respondError(w, http.StatusNotFound, "Pattern not found")
		return
	}

	patternName := cfg.Approval.Patterns[index].Pattern
	cfg.Approval.Patterns = append(cfg.Approval.Patterns[:index], cfg.Approval.Patterns[index+1:]...)

	comment := fmt.Sprintf("Deleted approval pattern: %s", patternName)
	if err := s.storageBackend.Save(r.Context(), cfg, comment); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save: %v", err))
		return
	}

	if err := s.ReloadConfig(cfg); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to reload: %v", err))
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Approval pattern deleted successfully",
	})
}

// handleUpdateApprovalProviders updates webhook and Slack approval providers
func (s *Server) handleUpdateApprovalProviders(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Webhook *config.WebhookApprovalConfig `json:"webhook,omitempty"`
		Slack   *config.SlackApprovalConfig   `json:"slack,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	cfg := s.GetConfig()

	// Update providers
	if req.Webhook != nil {
		if cfg.Approval.Webhook == nil {
			cfg.Approval.Webhook = &config.WebhookApprovalConfig{}
		}
		cfg.Approval.Webhook.URL = req.Webhook.URL
	}

	if req.Slack != nil {
		if cfg.Approval.Slack == nil {
			cfg.Approval.Slack = &config.SlackApprovalConfig{}
		}
		cfg.Approval.Slack.WebhookURL = req.Slack.WebhookURL
	}

	comment := "Updated approval providers configuration"
	if err := s.storageBackend.Save(r.Context(), cfg, comment); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save: %v", err))
		return
	}

	if err := s.ReloadConfig(cfg); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to reload: %v", err))
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Approval providers updated successfully",
	})
}
