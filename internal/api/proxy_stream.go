package api

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/davidcohan/port-authorizing/internal/audit"
	"github.com/davidcohan/port-authorizing/internal/proxy"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// WebSocket upgrader with generous buffer sizes for throughput
var upgrader = websocket.Upgrader{
	ReadBufferSize:  32768, // 32KB
	WriteBufferSize: 32768, // 32KB
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins (auth is handled via JWT)
	},
}

// handleProxyStream handles WebSocket-based reverse tunneling to target service
// Routes to appropriate protocol handler based on connection type
func (s *Server) handleProxyStream(w http.ResponseWriter, r *http.Request) {
	username := r.Context().Value(ContextKeyUsername).(string)
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

	// Check if this is a WebSocket upgrade request (from CLI)
	isWebSocket := r.Header.Get("Upgrade") == "websocket" &&
		r.Header.Get("Connection") != "" &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")

	// Route to appropriate handler based on connection type
	if conn.Config.Type == "postgres" {
		// For PostgreSQL: use WebSocket if upgrade requested, otherwise use protocol-aware proxy
		if isWebSocket {
			s.handlePostgresWebSocket(w, r)
		} else {
			s.handlePostgresProxy(w, r)
		}
		return
	}

	// For HTTP/HTTPS connections:
	// - If WebSocket upgrade: use HTTP-aware WebSocket tunnel (for approval/whitelist)
	// - Otherwise: use HTTP-aware stream parser (for approval/whitelist)
	if conn.Config.Type == "http" || conn.Config.Type == "https" {
		if isWebSocket {
			s.handleHTTPWebSocket(w, r)
		} else {
			s.handleHTTPProxyStream(w, r)
		}
		return
	}

	// For WebSocket requests or TCP connections, use WebSocket-based reverse tunnel

	// Log audit event
	audit.Log(s.config.Logging.AuditLogPath, username, "proxy_stream_websocket", conn.Config.Name, map[string]interface{}{
		"connection_id": connectionID,
		"method":        r.Method,
	})

	// Upgrade HTTP connection to WebSocket
	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		audit.Log(s.config.Logging.AuditLogPath, username, "websocket_upgrade_failed", conn.Config.Name, map[string]interface{}{
			"connection_id": connectionID,
			"error":         err.Error(),
		})
		return
	}
	defer wsConn.Close()

	// Setup ping/pong to keep connection alive (send pong in response to ping from client)
	wsConn.SetReadDeadline(time.Now().Add(60 * time.Second))
	wsConn.SetPongHandler(func(string) error {
		wsConn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Connect to backend target service
	targetAddr := fmt.Sprintf("%s:%d", conn.Config.Host, conn.Config.Port)
	targetConn, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		audit.Log(s.config.Logging.AuditLogPath, username, "backend_connect_failed", conn.Config.Name, map[string]interface{}{
			"connection_id": connectionID,
			"target":        targetAddr,
			"error":         err.Error(),
		})
		wsConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "Failed to connect to backend"))
		return
	}
	defer targetConn.Close()

	// Set deadline based on connection expiry
	timeUntilExpiry := time.Until(conn.ExpiresAt)
	targetConn.SetDeadline(conn.ExpiresAt)

	// Create capture buffers to record traffic (max 10KB per direction)
	maxCaptureSize := 10 * 1024
	var requestData, responseData []byte
	var requestSize, responseSize int

	// Bidirectional forwarding with traffic capture
	done := make(chan error, 2)
	disconnectReason := "client_disconnect"

	// WebSocket → Backend (CLI sends data to backend)
	go func() {
		for {
			messageType, data, err := wsConn.ReadMessage()
			if err != nil {
				done <- err
				return
			}

			// Only process binary messages
			if messageType == websocket.BinaryMessage {
				// Capture traffic for audit
				requestSize += len(data)
				if len(requestData) < maxCaptureSize {
					requestData = append(requestData, data...)
					if len(requestData) > maxCaptureSize {
						requestData = requestData[:maxCaptureSize]
					}
				}

				// Forward to backend
				if _, err := targetConn.Write(data); err != nil {
					done <- err
					return
				}
			}
		}
	}()

	// Backend → WebSocket (backend sends data back to CLI)
	go func() {
		buf := make([]byte, 32768) // 32KB buffer
		for {
			n, err := targetConn.Read(buf)
			if err != nil {
				done <- err
				return
			}

			// Capture traffic for audit
			responseSize += n
			if len(responseData) < maxCaptureSize {
				responseData = append(responseData, buf[:n]...)
				if len(responseData) > maxCaptureSize {
					responseData = responseData[:maxCaptureSize]
				}
			}

			// Forward to CLI via WebSocket
			if err := wsConn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				done <- err
				return
			}
		}
	}()

	// Wait for one direction to finish, or timeout
	select {
	case err1 := <-done:
		// One direction finished, close connections
		targetConn.Close()
		wsConn.Close()

		// Wait for the other goroutine to finish
		<-done

		// Determine disconnect reason from error
		if err1 != nil && err1 != io.EOF {
			if websocket.IsUnexpectedCloseError(err1, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				disconnectReason = "websocket_error"
			} else {
				disconnectReason = "backend_error"
			}
		}

	case <-time.After(timeUntilExpiry):
		// Connection expired - server-enforced timeout
		disconnectReason = "timeout"

		// Close connections to terminate goroutines
		targetConn.Close()
		wsConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Connection expired"))
		wsConn.Close()

		// Wait for both goroutines to finish
		<-done
		<-done
	}

	// Log session with captured traffic
	audit.Log(s.config.Logging.AuditLogPath, username, "proxy_session_websocket", conn.Config.Name, map[string]interface{}{
		"connection_id":    connectionID,
		"reason":           disconnectReason,
		"request_size":     requestSize,
		"response_size":    responseSize,
		"request_preview":  truncateData(requestData, 500),
		"response_preview": truncateData(responseData, 500),
	})
}

// handlePostgresWebSocket handles PostgreSQL connections via WebSocket with protocol-aware parsing
func (s *Server) handlePostgresWebSocket(w http.ResponseWriter, r *http.Request) {
	username := r.Context().Value(ContextKeyUsername).(string)
	roles, _ := r.Context().Value(ContextKeyRoles).([]string)
	vars := mux.Vars(r)
	connectionID := vars["connectionID"]

	// Get connection (already validated in parent function)
	conn, _ := s.connMgr.GetConnection(connectionID)

	// Get whitelist for this user's roles
	whitelist := s.authz.GetWhitelistForConnection(roles, conn.Config.Name)

	// Log audit event
	audit.Log(s.config.Logging.AuditLogPath, username, "postgres_connect_websocket", conn.Config.Name, map[string]interface{}{
		"connection_id":   connectionID,
		"method":          r.Method,
		"roles":           roles,
		"whitelist_rules": len(whitelist),
	})

	// Upgrade HTTP connection to WebSocket
	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		audit.Log(s.config.Logging.AuditLogPath, username, "websocket_upgrade_failed", conn.Config.Name, map[string]interface{}{
			"connection_id": connectionID,
			"error":         err.Error(),
		})
		return
	}
	defer wsConn.Close()

	// Setup ping/pong keepalive
	wsConn.SetReadDeadline(time.Now().Add(60 * time.Second))
	wsConn.SetPongHandler(func(string) error {
		wsConn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Create Postgres proxy with protocol-aware query logging and security
	pgProxy := proxy.NewPostgresAuthProxy(
		conn.Config,
		s.config.Logging.AuditLogPath,
		username,
		connectionID,
		s.config,
		whitelist,
	)

	// Set approval manager if enabled
	if s.approvalMgr != nil {
		pgProxy.SetApprovalManager(s.approvalMgr)
	}

	// Create a virtual connection that wraps WebSocket
	// This allows the PostgresAuthProxy to work with WebSocket instead of raw TCP
	wsNetConn := &websocketConn{
		ws:   wsConn,
		done: make(chan struct{}),
	}
	defer func() {
		// Safe close - won't panic if already closed
		wsNetConn.Close()
	}()

	// Handle the Postgres protocol connection through WebSocket
	if err := pgProxy.HandleConnection(wsNetConn); err != nil {
		if err != io.EOF {
			audit.Log(s.config.Logging.AuditLogPath, username, "postgres_error", conn.Config.Name, map[string]interface{}{
				"connection_id": connectionID,
				"error":         err.Error(),
			})
		}
	}

	audit.Log(s.config.Logging.AuditLogPath, username, "postgres_disconnect_websocket", conn.Config.Name, map[string]interface{}{
		"connection_id": connectionID,
	})
}

// handleHTTPWebSocket handles HTTP/HTTPS connections via WebSocket with HTTP-aware parsing
// This enables approval workflow and whitelist checking for HTTP traffic over WebSocket
func (s *Server) handleHTTPWebSocket(w http.ResponseWriter, r *http.Request) {
	username := r.Context().Value(ContextKeyUsername).(string)
	roles, _ := r.Context().Value(ContextKeyRoles).([]string)
	vars := mux.Vars(r)
	connectionID := vars["connectionID"]

	// Get connection (already validated in parent function)
	conn, _ := s.connMgr.GetConnection(connectionID)

	// Get whitelist for this user's roles
	whitelist := s.authz.GetWhitelistForConnection(roles, conn.Config.Name)

	// Log audit event
	audit.Log(s.config.Logging.AuditLogPath, username, "http_connect_websocket", conn.Config.Name, map[string]interface{}{
		"connection_id":   connectionID,
		"method":          r.Method,
		"roles":           roles,
		"whitelist_rules": len(whitelist),
	})

	// Upgrade HTTP connection to WebSocket
	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		audit.Log(s.config.Logging.AuditLogPath, username, "websocket_upgrade_failed", conn.Config.Name, map[string]interface{}{
			"connection_id": connectionID,
			"error":         err.Error(),
		})
		return
	}
	defer wsConn.Close()

	// Setup ping/pong keepalive
	wsConn.SetReadDeadline(time.Now().Add(60 * time.Second))
	wsConn.SetPongHandler(func(string) error {
		wsConn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Create HTTP proxy with whitelist and approval support
	httpProxy := conn.Proxy
	if httpProxy == nil {
		audit.Log(s.config.Logging.AuditLogPath, username, "http_error", conn.Config.Name, map[string]interface{}{
			"connection_id": connectionID,
			"error":         "HTTP proxy not initialized",
		})
		return
	}

	// Create a virtual connection that wraps WebSocket
	// This allows HTTP-aware parsing while maintaining WebSocket benefits
	wsNetConn := &websocketConn{
		ws:     wsConn,
		done:   make(chan struct{}),
		buffer: nil,
	}
	defer func() {
		// Safe close - won't panic if already closed
		wsNetConn.Close()
	}()

	// Process HTTP requests from WebSocket stream
	// Similar to handleHTTPProxyStream but over WebSocket
	if err := s.handleHTTPOverWebSocket(wsNetConn, httpProxy, username, conn, connectionID); err != nil {
		if err != io.EOF {
			audit.Log(s.config.Logging.AuditLogPath, username, "http_error", conn.Config.Name, map[string]interface{}{
				"connection_id": connectionID,
				"error":         err.Error(),
			})
		}
	}

	audit.Log(s.config.Logging.AuditLogPath, username, "http_disconnect_websocket", conn.Config.Name, map[string]interface{}{
		"connection_id": connectionID,
	})
}

// handleHTTPOverWebSocket processes HTTP requests from a WebSocket connection
// This enables approval and whitelist checks for HTTP traffic
func (s *Server) handleHTTPOverWebSocket(wsNetConn *websocketConn, httpProxy proxy.Protocol, username string, conn *proxy.Connection, connectionID string) error {
	// Create combined reader/writer for HTTP parsing
	reader := bufio.NewReader(wsNetConn)
	writer := bufio.NewWriter(wsNetConn)
	bufrw := bufio.NewReadWriter(reader, writer)

	for {
		// Check if connection expired
		if time.Now().After(conn.ExpiresAt) {
			break
		}

		// Read HTTP request from WebSocket
		requestBytes, err := readHTTPRequestFromStream(reader)
		if err != nil {
			if err != io.EOF && !strings.Contains(err.Error(), "use of closed") {
				return err
			}
			break
		}

		// Parse HTTP request
		reqReader := bufio.NewReader(bytes.NewReader(requestBytes))
		httpReq, err := http.ReadRequest(reqReader)
		if err != nil {
			// Send error response
			writer.WriteString("HTTP/1.1 400 Bad Request\r\n\r\nInvalid HTTP request\r\n")
			writer.Flush()
			break
		}

		// Create synthetic request for proxy handler
		proxyReq := httptest.NewRequest("POST", "/", bytes.NewReader(requestBytes))
		proxyReq.Header.Set("Content-Type", "application/octet-stream")

		// Create response writer that writes back to WebSocket
		respWriter := &streamResponseWriter{
			writer: bufrw,
			header: make(http.Header),
		}

		// Call HTTP proxy's HandleRequest (this checks approval + whitelist)
		err = httpProxy.HandleRequest(respWriter, proxyReq)
		bufrw.Flush()

		if err != nil {
			break
		}

		// If Connection: close, break the loop
		if strings.ToLower(httpReq.Header.Get("Connection")) == "close" {
			break
		}
	}

	return nil
}

// readHTTPRequestFromStream reads a complete HTTP request from a stream
func readHTTPRequestFromStream(reader *bufio.Reader) ([]byte, error) {
	var buffer bytes.Buffer

	// Peek to see if there's data
	_, err := reader.Peek(1)
	if err != nil {
		return nil, err
	}

	// Read request line and headers
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		buffer.WriteString(line)

		if line == "\r\n" || line == "\n" {
			break
		}
	}

	// Check for Content-Length
	requestStr := buffer.String()
	if strings.Contains(strings.ToLower(requestStr), "content-length:") {
		lines := strings.Split(requestStr, "\n")
		for _, line := range lines {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "content-length:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				var contentLength int
				_, _ = fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &contentLength)

					if contentLength > 0 {
						body := make([]byte, contentLength)
						_, err := io.ReadFull(reader, body)
						if err != nil {
							return nil, err
						}
						buffer.Write(body)
					}
				}
				break
			}
		}
	}

	return buffer.Bytes(), nil
}

// websocketConn wraps a WebSocket connection to implement net.Conn interface
// This allows existing protocol handlers to work with WebSocket
type websocketConn struct {
	ws        *websocket.Conn
	done      chan struct{}
	buffer    []byte    // Buffer for partial reads from WebSocket messages
	closeOnce sync.Once // Ensure Close is only executed once
}

func (c *websocketConn) Read(b []byte) (n int, err error) {
	// If we have buffered data, return that first
	if len(c.buffer) > 0 {
		n = copy(b, c.buffer)
		c.buffer = c.buffer[n:]
		return n, nil
	}

	// Read next WebSocket message
	for {
		messageType, data, err := c.ws.ReadMessage()
		if err != nil {
			return 0, err
		}

		// Only process binary messages (skip ping/pong/text)
		if messageType == websocket.BinaryMessage {
			// Copy what fits in the buffer
			n = copy(b, data)

			// Save remaining data for next read
			if n < len(data) {
				c.buffer = data[n:]
			}

			return n, nil
		}
	}
}

func (c *websocketConn) Write(b []byte) (n int, err error) {
	err = c.ws.WriteMessage(websocket.BinaryMessage, b)
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

func (c *websocketConn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.done)
		err = c.ws.Close()
	})
	return err
}

func (c *websocketConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
}

func (c *websocketConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
}

func (c *websocketConn) SetDeadline(t time.Time) error {
	return c.ws.SetReadDeadline(t)
}

func (c *websocketConn) SetReadDeadline(t time.Time) error {
	return c.ws.SetReadDeadline(t)
}

func (c *websocketConn) SetWriteDeadline(t time.Time) error {
	return c.ws.SetWriteDeadline(t)
}
