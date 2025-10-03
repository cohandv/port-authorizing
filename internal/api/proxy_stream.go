package api

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/davidcohan/port-authorizing/internal/audit"
	"github.com/gorilla/mux"
)

// handleProxyStream handles transparent TCP streaming to target service
// This doesn't parse any protocol - it's a pure TCP proxy
func (s *Server) handleProxyStream(w http.ResponseWriter, r *http.Request) {
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

	// Log audit event
	audit.Log(s.config.Logging.AuditLogPath, username, "proxy_stream", conn.Config.Name, map[string]interface{}{
		"connection_id": connectionID,
		"method":        r.Method,
	})

	// Connect to target service
	targetAddr := fmt.Sprintf("%s:%d", conn.Config.Host, conn.Config.Port)
	targetConn, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		respondError(w, http.StatusBadGateway, fmt.Sprintf("Failed to connect to target: %v", err))
		return
	}
	defer targetConn.Close()

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
	timeUntilExpiry := time.Until(conn.ExpiresAt)
	clientConn.SetDeadline(conn.ExpiresAt)
	targetConn.SetDeadline(conn.ExpiresAt)

	// Transparent bidirectional TCP proxy
	// Copy data in both directions without any parsing
	done := make(chan error, 2)

	// Client → Target
	go func() {
		_, err := io.Copy(targetConn, bufrw)
		done <- err
	}()

	// Target → Client
	go func() {
		_, err := io.Copy(bufrw, targetConn)
		if err == nil {
			bufrw.Flush()
		}
		done <- err
	}()

	// Wait for one direction to finish or timeout
	select {
	case <-done:
		// Normal completion
	case <-time.After(timeUntilExpiry):
		// Connection expired - server-enforced timeout
		fmt.Fprintf(bufrw, "\r\n\r\n[Connection timeout: %s expired]\r\n", conn.Config.Name)
		bufrw.Flush()
	}

	// Close both connections to stop the other direction
	targetConn.Close()
	clientConn.Close()

	// Log disconnection
	audit.Log(s.config.Logging.AuditLogPath, username, "proxy_disconnect", conn.Config.Name, map[string]interface{}{
		"connection_id": connectionID,
		"reason":        "timeout or completion",
	})
}
