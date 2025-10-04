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
// Routes to appropriate protocol handler based on connection type
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

	// Route to appropriate handler based on connection type
	if conn.Config.Type == "postgres" {
		s.handlePostgresProxy(w, r)
		return
	}

	// For other types (http, tcp), use transparent TCP proxy

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

	// Create capture readers to record traffic (max 10KB per direction)
	maxCaptureSize := 10 * 1024
	requestCapture := newCaptureReader(bufrw, maxCaptureSize)
	responseCapture := newCaptureReader(targetConn, maxCaptureSize)

	// Transparent bidirectional TCP proxy with traffic capture
	done := make(chan error, 2)

	// Client → Target (read from client, write to target)
	go func() {
		_, err := io.Copy(targetConn, requestCapture)
		done <- err
	}()

	// Target → Client (read from target, write to client)
	go func() {
		_, err := io.Copy(clientConn, responseCapture)
		done <- err
	}()

	// Wait for one direction to finish, or timeout
	disconnectReason := "client_disconnect"

	select {
	case err1 := <-done:
		// One direction finished, close connections to signal EOF to both sides
		targetConn.Close()
		clientConn.Close()

		// Wait for the other goroutine to finish
		err2 := <-done

		// Log any errors (for debugging)
		if err1 != nil || err2 != nil {
			// Errors are expected when connections close
		}

	case <-time.After(timeUntilExpiry):
		// Connection expired - server-enforced timeout
		disconnectReason = "timeout"

		// Close connections to terminate goroutines
		targetConn.Close()
		clientConn.Close()

		// Wait for both goroutines to finish
		<-done
		<-done
	}

	// Get captured traffic
	requestData := requestCapture.GetData()
	responseData := responseCapture.GetData()

	// Log session with captured traffic
	audit.Log(s.config.Logging.AuditLogPath, username, "proxy_session", conn.Config.Name, map[string]interface{}{
		"connection_id":    connectionID,
		"reason":           disconnectReason,
		"request_size":     len(requestData),
		"response_size":    len(responseData),
		"request_preview":  truncateData(requestData, 500),
		"response_preview": truncateData(responseData, 500),
	})
}
