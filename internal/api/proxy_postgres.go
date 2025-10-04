package api

import (
	"fmt"
	"net/http"

	"github.com/davidcohan/port-authorizing/internal/audit"
	"github.com/davidcohan/port-authorizing/internal/proxy"
	"github.com/gorilla/mux"
)

// handlePostgresProxy handles Postgres protocol connections
// This creates a transparent TCP tunnel but with protocol-aware query logging
func (s *Server) handlePostgresProxy(w http.ResponseWriter, r *http.Request) {
	username := r.Context().Value("username").(string)
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

	// Verify it's a Postgres connection
	if conn.Config.Type != "postgres" {
		respondError(w, http.StatusBadRequest, "Not a Postgres connection")
		return
	}

	// Log audit event
	audit.Log(s.config.Logging.AuditLogPath, username, "postgres_connect", conn.Config.Name, map[string]interface{}{
		"connection_id": connectionID,
		"method":        r.Method,
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
	defer clientConn.Close()

	// Register this stream with the connection for timeout enforcement
	conn.RegisterStream(clientConn)
	defer conn.UnregisterStream(clientConn)

	// Send HTTP 200 response to indicate proxy is ready
	fmt.Fprintf(bufrw, "HTTP/1.1 200 Connection Established\r\n\r\n")
	bufrw.Flush()

	// Set deadline based on connection expiry
	clientConn.SetDeadline(conn.ExpiresAt)

	// Create Postgres proxy with credential substitution
	pgProxy := proxy.NewPostgresAuthProxy(
		conn.Config,
		s.config.Logging.AuditLogPath,
		username,
		connectionID,
		s.config,
	)

	// Handle the Postgres protocol connection
	// This will authenticate the client with API credentials,
	// log all queries, and forward to backend with backend credentials
	if err := pgProxy.HandleConnection(clientConn); err != nil {
		audit.Log(s.config.Logging.AuditLogPath, username, "postgres_error", conn.Config.Name, map[string]interface{}{
			"connection_id": connectionID,
			"error":         err.Error(),
		})
		return
	}

	audit.Log(s.config.Logging.AuditLogPath, username, "postgres_disconnect", conn.Config.Name, map[string]interface{}{
		"connection_id": connectionID,
	})
}
