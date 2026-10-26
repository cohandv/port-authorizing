package api

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/davidcohan/port-authorizing/internal/approval"
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

	// For Redis connections:
	// - Use WebSocket-based Redis protocol handler
	if conn.Config.Type == "redis" {
		if isWebSocket {
			s.handleRedisWebSocket(w, r)
		} else {
			// Redis doesn't support non-WebSocket proxy mode (requires protocol-aware handling)
			respondError(w, http.StatusBadRequest, "Redis connections require WebSocket upgrade")
		}
		return
	}

	// K8s Services connections: proxy to a specific K8s service
	// Requires query params: namespace, service, port
	if conn.Config.Type == "k8s_services" {
		if isWebSocket {
			s.handleK8sServiceProxy(w, r)
		} else {
			respondError(w, http.StatusBadRequest, "K8s services connections require WebSocket upgrade")
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
	_ = audit.Log(s.config.Logging.AuditLogPath, username, "proxy_stream_websocket", conn.Config.Name, map[string]interface{}{
		"connection_id": connectionID,
		"method":        r.Method,
	})

	// Upgrade HTTP connection to WebSocket
	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		_ = audit.Log(s.config.Logging.AuditLogPath, username, "websocket_upgrade_failed", conn.Config.Name, map[string]interface{}{
			"connection_id": connectionID,
			"error":         err.Error(),
		})
		return
	}
	defer func() { _ = wsConn.Close() }()

	// Setup ping/pong to keep connection alive (send pong in response to ping from client)
	// Use connection expiry time as read deadline (typically 2 hours)
	// This prevents premature disconnections while still enforcing MaxConnectionDuration
	_ = wsConn.SetReadDeadline(conn.ExpiresAt)
	wsConn.SetPongHandler(func(string) error {
		_ = wsConn.SetReadDeadline(conn.ExpiresAt)
		return nil
	})

	// Connect to backend target service
	targetAddr := fmt.Sprintf("%s:%d", conn.Config.Host, conn.Config.Port)
	targetConn, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		_ = audit.Log(s.config.Logging.AuditLogPath, username, "backend_connect_failed", conn.Config.Name, map[string]interface{}{
			"connection_id": connectionID,
			"target":        targetAddr,
			"error":         err.Error(),
		})
		_ = wsConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "Failed to connect to backend"))
		return
	}
	defer func() { _ = targetConn.Close() }()

	// Set deadline based on connection expiry
	timeUntilExpiry := time.Until(conn.ExpiresAt)
	_ = targetConn.SetDeadline(conn.ExpiresAt)

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
		_ = targetConn.Close()
		_ = wsConn.Close()

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
		_ = targetConn.Close()
		_ = wsConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Connection expired"))
		_ = wsConn.Close()

		// Wait for both goroutines to finish
		<-done
		<-done
	}

	// Log session with captured traffic
	_ = audit.Log(s.config.Logging.AuditLogPath, username, "proxy_session_websocket", conn.Config.Name, map[string]interface{}{
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
	_ = audit.Log(s.config.Logging.AuditLogPath, username, "postgres_connect_websocket", conn.Config.Name, map[string]interface{}{
		"connection_id":   connectionID,
		"method":          r.Method,
		"roles":           roles,
		"whitelist_rules": len(whitelist),
	})

	// Upgrade HTTP connection to WebSocket
	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		_ = audit.Log(s.config.Logging.AuditLogPath, username, "websocket_upgrade_failed", conn.Config.Name, map[string]interface{}{
			"connection_id": connectionID,
			"error":         err.Error(),
		})
		return
	}
	defer func() { _ = wsConn.Close() }()

	// Setup ping/pong keepalive
	// Use connection expiry time as read deadline (typically 2 hours)
	_ = wsConn.SetReadDeadline(conn.ExpiresAt)
	wsConn.SetPongHandler(func(string) error {
		_ = wsConn.SetReadDeadline(conn.ExpiresAt)
		return nil
	})

	// Create Postgres proxy with protocol-aware query logging and security
	pgProxy := proxy.NewPostgresAuthProxy(
		conn.Config,
		s.config.Logging.AuditLogPath,
		username,
		connectionID,
		roles,
		s.config,
		whitelist,
		s.resolver,
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
		_ = wsNetConn.Close()
	}()

	// Handle the Postgres protocol connection through WebSocket
	if err := pgProxy.HandleConnection(wsNetConn); err != nil {
		if err != io.EOF {
			_ = audit.Log(s.config.Logging.AuditLogPath, username, "postgres_error", conn.Config.Name, map[string]interface{}{
				"connection_id": connectionID,
				"error":         err.Error(),
			})
		}
	}

	_ = audit.Log(s.config.Logging.AuditLogPath, username, "postgres_disconnect_websocket", conn.Config.Name, map[string]interface{}{
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
	})

	// Upgrade HTTP connection to WebSocket
	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		_ = audit.Log(s.config.Logging.AuditLogPath, username, "websocket_upgrade_failed", conn.Config.Name, map[string]interface{}{
			"connection_id": connectionID,
			"error":         err.Error(),
		})
		return
	}
	defer func() { _ = wsConn.Close() }()

	// Setup ping/pong keepalive
	// Use connection expiry time as read deadline (typically 2 hours)
	_ = wsConn.SetReadDeadline(conn.ExpiresAt)
	wsConn.SetPongHandler(func(string) error {
		_ = wsConn.SetReadDeadline(conn.ExpiresAt)
		return nil
	})

	// Create Redis proxy with protocol-aware command logging and security
	redisProxy := proxy.NewRedisAuthProxy(
		conn.Config,
		s.config.Logging.AuditLogPath,
		username,
		connectionID,
		roles,
		s.config,
		whitelist,
		s.resolver,
	)

	// Set approval manager if enabled
	if s.approvalMgr != nil {
		redisProxy.SetApprovalManager(s.approvalMgr)
	}

	// Create a virtual connection that wraps WebSocket
	// This allows the RedisAuthProxy to work with WebSocket instead of raw TCP
	wsNetConn := &websocketConn{
		ws:   wsConn,
		done: make(chan struct{}),
	}
	defer func() {
		// Safe close - won't panic if already closed
		_ = wsNetConn.Close()
	}()

	// Handle the Redis protocol connection through WebSocket
	if err := redisProxy.HandleConnection(wsNetConn); err != nil {
		if err != io.EOF {
			_ = audit.Log(s.config.Logging.AuditLogPath, username, "redis_error", conn.Config.Name, map[string]interface{}{
				"connection_id": connectionID,
				"error":         err.Error(),
			})
		}
	}

	_ = audit.Log(s.config.Logging.AuditLogPath, username, "redis_disconnect_websocket", conn.Config.Name, map[string]interface{}{
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
	_ = audit.Log(s.config.Logging.AuditLogPath, username, "http_connect_websocket", conn.Config.Name, map[string]interface{}{
		"connection_id":   connectionID,
		"method":          r.Method,
		"roles":           roles,
		"whitelist_rules": len(whitelist),
	})

	// Upgrade HTTP connection to WebSocket
	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		_ = audit.Log(s.config.Logging.AuditLogPath, username, "websocket_upgrade_failed", conn.Config.Name, map[string]interface{}{
			"connection_id": connectionID,
			"error":         err.Error(),
		})
		return
	}
	defer func() { _ = wsConn.Close() }()

	// Setup ping/pong keepalive
	// Use connection expiry time as read deadline (typically 2 hours)
	_ = wsConn.SetReadDeadline(conn.ExpiresAt)
	wsConn.SetPongHandler(func(string) error {
		_ = wsConn.SetReadDeadline(conn.ExpiresAt)
		return nil
	})

	// Create HTTP proxy with whitelist and approval support
	httpProxy := conn.Proxy
	if httpProxy == nil {
		_ = audit.Log(s.config.Logging.AuditLogPath, username, "http_error", conn.Config.Name, map[string]interface{}{
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
		_ = wsNetConn.Close()
	}()

	// Process HTTP requests from WebSocket stream
	// Similar to handleHTTPProxyStream but over WebSocket
	if err := s.handleHTTPOverWebSocket(wsNetConn, httpProxy, username, conn, connectionID); err != nil {
		if err != io.EOF {
			_ = audit.Log(s.config.Logging.AuditLogPath, username, "http_error", conn.Config.Name, map[string]interface{}{
				"connection_id": connectionID,
				"error":         err.Error(),
			})
		}
	}

	_ = audit.Log(s.config.Logging.AuditLogPath, username, "http_disconnect_websocket", conn.Config.Name, map[string]interface{}{
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

	for time.Now().Before(conn.ExpiresAt) {
		// Check if connection expired handled by loop condition

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
			_, _ = writer.WriteString("HTTP/1.1 400 Bad Request\r\n\r\nInvalid HTTP request\r\n")
			_ = writer.Flush()
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
		_ = bufrw.Flush()

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

// newWebsocketConn creates a new websocketConn adapter
func newWebsocketConn(ws *websocket.Conn) *websocketConn {
	return &websocketConn{
		ws:   ws,
		done: make(chan struct{}),
	}
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

// handleK8sServiceProxy handles WebSocket proxy to a specific K8s service
// Query params required: namespace, service, port
// Optional: port_name (for protocol detection)
func (s *Server) handleK8sServiceProxy(w http.ResponseWriter, r *http.Request) {
	username := r.Context().Value(ContextKeyUsername).(string)
	roles, _ := r.Context().Value(ContextKeyRoles).([]string)
	vars := mux.Vars(r)
	connectionID := vars["connectionID"]

	// Get connection
	conn, err := s.connMgr.GetConnection(connectionID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Connection not found or expired")
		return
	}

	// Parse required query params
	namespace := r.URL.Query().Get("namespace")
	service := r.URL.Query().Get("service")
	portStr := r.URL.Query().Get("port")
	portName := r.URL.Query().Get("port_name")

	if namespace == "" || service == "" || portStr == "" {
		respondError(w, http.StatusBadRequest, "Missing required params: namespace, service, port")
		return
	}

	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid port number")
		return
	}

	// Build target address using K8s service DNS
	targetHost := fmt.Sprintf("%s.%s.svc.cluster.local", service, namespace)
	targetAddr := fmt.Sprintf("%s:%d", targetHost, port)

	// Get service annotations for protocol detection
	var serviceAnnotations map[string]string
	discovery, err := proxy.NewK8sServiceDiscovery(
		conn.Config.K8sExcludeNamespaces,
		conn.Config.K8sIncludeNamespaces,
		conn.Config.K8sIgnoreServices,
	)
	if err == nil {
		if svc, err := discovery.GetService(r.Context(), namespace, service); err == nil {
			serviceAnnotations = svc.Annotations
		}
	}

	// Determine protocol
	protocol := determineK8sPortProtocol(conn.Config.K8sPortMappings, serviceAnnotations, portName, int32(port))

	// Get whitelist for HTTP/HTTPS
	var whitelist []string
	if protocol == "http" || protocol == "https" {
		whitelist = s.authz.GetWhitelistForConnection(roles, conn.Config.Name)
	}

	_ = audit.Log(s.config.Logging.AuditLogPath, username, "k8s_service_proxy_connect", conn.Config.Name, map[string]interface{}{
		"connection_id":   connectionID,
		"namespace":       namespace,
		"service":         service,
		"port":            port,
		"port_name":       portName,
		"target":          targetAddr,
		"protocol":        protocol,
		"whitelist_rules": len(whitelist),
	})

	// Upgrade to WebSocket
	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		_ = audit.Log(s.config.Logging.AuditLogPath, username, "k8s_service_websocket_upgrade_failed", conn.Config.Name, map[string]interface{}{
			"connection_id": connectionID,
			"error":         err.Error(),
		})
		return
	}
	defer wsConn.Close()

	// Connect to K8s service
	dialer := net.Dialer{Timeout: 10 * time.Second}
	backendConn, err := dialer.DialContext(r.Context(), "tcp", targetAddr)
	if err != nil {
		_ = audit.Log(s.config.Logging.AuditLogPath, username, "k8s_service_connect_error", conn.Config.Name, map[string]interface{}{
			"connection_id": connectionID,
			"target":        targetAddr,
			"error":         err.Error(),
		})
		_ = wsConn.WriteJSON(map[string]string{"error": fmt.Sprintf("Failed to connect to %s: %v", targetAddr, err)})
		return
	}
	defer backendConn.Close()

	_ = audit.Log(s.config.Logging.AuditLogPath, username, "k8s_service_connected", conn.Config.Name, map[string]interface{}{
		"connection_id": connectionID,
		"target":        targetAddr,
		"protocol":      protocol,
	})

	// Create websocketConn adapter
	clientConn := newWebsocketConn(wsConn)

	// Register connection
	conn.RegisterStream(backendConn)
	defer conn.UnregisterStream(backendConn)

	// Route to appropriate protocol handler
	switch protocol {
	case "http", "https":
		s.proxyK8sHTTP(clientConn, backendConn, conn, username, roles, whitelist, namespace, service)
	case "postgres":
		s.proxyK8sPostgres(clientConn, backendConn, conn, username)
	case "redis":
		s.proxyK8sRedis(clientConn, backendConn, conn, username)
	default:
		// TCP - raw bidirectional forwarding
		s.proxyK8sTCP(clientConn, backendConn, conn, username)
	}

	_ = audit.Log(s.config.Logging.AuditLogPath, username, "k8s_service_disconnected", conn.Config.Name, map[string]interface{}{
		"connection_id": connectionID,
		"target":        targetAddr,
	})
}

// proxyK8sHTTP handles HTTP proxying for K8s services with whitelist/approval
func (s *Server) proxyK8sHTTP(clientConn, backendConn net.Conn, conn *proxy.Connection, username string, roles, whitelist []string, namespace, service string) {
	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Backend (with whitelist/approval validation)
	go func() {
		defer wg.Done()
		s.proxyK8sHTTPWithWhitelist(clientConn, backendConn, conn, username, roles, whitelist, namespace, service)
	}()

	// Backend -> Client (pass through)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(clientConn, backendConn)
	}()

	wg.Wait()
}

// proxyK8sHTTPWithWhitelist validates each HTTP request against whitelist and approval
func (s *Server) proxyK8sHTTPWithWhitelist(clientConn, backendConn net.Conn, conn *proxy.Connection, username string, roles, whitelist []string, namespace, service string) {
	reader := bufio.NewReader(clientConn)

	for {
		// Read HTTP request line
		requestLine, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		// Parse request line
		parts := strings.Fields(strings.TrimSpace(requestLine))
		if len(parts) < 2 {
			// Not valid HTTP, forward as-is
			if _, err := backendConn.Write([]byte(requestLine)); err != nil {
				return
			}
			continue
		}

		method := parts[0]
		path := parts[1]
		requestPattern := method + " " + path

		// Check whitelist
		if len(whitelist) > 0 && !proxy.IsHTTPRequestAllowed(requestPattern, whitelist) {
			_ = audit.Log(s.config.Logging.AuditLogPath, username, "http_request_blocked", conn.Config.Name, map[string]interface{}{
				"connection_id": conn.ID,
				"method":        method,
				"path":          path,
				"service":       fmt.Sprintf("%s/%s", namespace, service),
				"reason":        "whitelist",
			})
			forbiddenResp := "HTTP/1.1 403 Forbidden\r\nContent-Type: application/json\r\nContent-Length: 44\r\nConnection: close\r\n\r\n{\"error\":\"Request blocked by whitelist policy\"}"
			_, _ = clientConn.Write([]byte(forbiddenResp))
			time.Sleep(100 * time.Millisecond)
			return
		}

		// Check approval
		if s.approvalMgr != nil {
			requiresApproval, timeout := s.approvalMgr.RequiresApproval(method, path, conn.Config.Tags, roles)
			if requiresApproval {
				_ = audit.Log(s.config.Logging.AuditLogPath, username, "http_approval_requested", conn.Config.Name, map[string]interface{}{
					"connection_id": conn.ID,
					"method":        method,
					"path":          path,
					"service":       fmt.Sprintf("%s/%s", namespace, service),
					"timeout":       timeout.String(),
				})

				approvalReq := &approval.Request{
					Username:       username,
					ConnectionID:   conn.ID,
					ConnectionType: "http",
					ConnectionName: conn.Config.Name,
					Method:         method,
					Path:           path,
					Metadata: map[string]string{
						"service":   fmt.Sprintf("%s/%s", namespace, service),
						"method":    method,
						"path":      path,
						"protocol":  "http",
					},
				}
				if s.config != nil {
					approvalReq.Environment = s.config.Environment
				}

				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				approvalResp, err := s.approvalMgr.RequestApproval(ctx, approvalReq, timeout)
				cancel()

				if err != nil || approvalResp.Decision != approval.DecisionApproved {
					reason := "timeout or error"
					if approvalResp != nil {
						reason = string(approvalResp.Decision) + ": " + approvalResp.Reason
					}
					if err != nil {
						reason = err.Error()
					}

					_ = audit.Log(s.config.Logging.AuditLogPath, username, "http_approval_rejected", conn.Config.Name, map[string]interface{}{
						"connection_id": conn.ID,
						"method":        method,
						"path":          path,
						"reason":        reason,
					})

					respBody := fmt.Sprintf("{\"error\":\"Request requires approval\",\"reason\":\"%s\"}", reason)
					forbiddenResp := fmt.Sprintf("HTTP/1.1 403 Forbidden\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(respBody), respBody)
					_, _ = clientConn.Write([]byte(forbiddenResp))
					time.Sleep(100 * time.Millisecond)
					return
				}

				_ = audit.Log(s.config.Logging.AuditLogPath, username, "http_approval_granted", conn.Config.Name, map[string]interface{}{
					"connection_id": conn.ID,
					"method":        method,
					"path":          path,
					"approved_by":   approvalResp.ApprovedBy,
				})
			}
		}

		// Log allowed request
		_ = audit.Log(s.config.Logging.AuditLogPath, username, "http_request", conn.Config.Name, map[string]interface{}{
			"connection_id": conn.ID,
			"method":        method,
			"path":          path,
			"service":       fmt.Sprintf("%s/%s", namespace, service),
			"allowed":       true,
		})

		// Forward request
		if _, err := backendConn.Write([]byte(requestLine)); err != nil {
			return
		}

		// Read and forward headers
		for {
			headerLine, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if _, err := backendConn.Write([]byte(headerLine)); err != nil {
				return
			}
			if headerLine == "\r\n" || headerLine == "\n" {
				break
			}
		}
	}
}

// proxyK8sPostgres handles Postgres proxying with query logging
func (s *Server) proxyK8sPostgres(clientConn, backendConn net.Conn, conn *proxy.Connection, username string) {
	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Backend (log queries)
	go func() {
		defer wg.Done()
		s.proxyPostgresWithLogging(clientConn, backendConn, conn, username, true)
	}()

	// Backend -> Client
	go func() {
		defer wg.Done()
		s.proxyPostgresWithLogging(backendConn, clientConn, conn, username, false)
	}()

	wg.Wait()
}

// proxyPostgresWithLogging proxies postgres data and optionally logs queries
func (s *Server) proxyPostgresWithLogging(src, dst net.Conn, conn *proxy.Connection, username string, logQueries bool) {
	reader := bufio.NewReader(src)
	buf := make([]byte, 32*1024)

	for {
		_ = src.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, err := reader.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return
		}

		if n > 0 {
			data := buf[:n]

			// Log queries from client->backend traffic
			if logQueries && s.config.Logging.AuditLogPath != "" {
				s.logPostgresQueries(data, conn, username)
			}

			if _, err := dst.Write(data); err != nil {
				return
			}
		}
	}
}

// logPostgresQueries extracts and logs SQL queries
func (s *Server) logPostgresQueries(data []byte, conn *proxy.Connection, username string) {
	for i := 0; i < len(data); i++ {
		if data[i] == 'Q' && i+5 < len(data) {
			length := int(data[i+1])<<24 | int(data[i+2])<<16 | int(data[i+3])<<8 | int(data[i+4])
			if i+1+length <= len(data) {
				queryStart := i + 5
				queryEnd := i + 1 + length
				if queryEnd <= len(data) {
					queryBytes := data[queryStart:queryEnd]
					query := string(bytes.TrimRight(queryBytes, "\x00"))
					if query != "" {
						_ = audit.Log(s.config.Logging.AuditLogPath, username, "postgres_query", conn.Config.Name, map[string]interface{}{
							"connection_id": conn.ID,
							"query":         query,
						})
					}
				}
				i += length
			}
		}
	}
}

// proxyK8sRedis handles Redis proxying with command logging
func (s *Server) proxyK8sRedis(clientConn, backendConn net.Conn, conn *proxy.Connection, username string) {
	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Backend (log commands)
	go func() {
		defer wg.Done()
		s.proxyRedisWithLogging(clientConn, backendConn, conn, username, true)
	}()

	// Backend -> Client
	go func() {
		defer wg.Done()
		s.proxyRedisWithLogging(backendConn, clientConn, conn, username, false)
	}()

	wg.Wait()
}

// proxyRedisWithLogging proxies redis data and optionally logs commands
func (s *Server) proxyRedisWithLogging(src, dst net.Conn, conn *proxy.Connection, username string, logCommands bool) {
	buf := make([]byte, 32*1024)
	reader := bufio.NewReader(src)

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			data := buf[:n]

			if logCommands && s.config.Logging.AuditLogPath != "" {
				s.logRedisCommands(data, conn, username)
			}

			if _, err := dst.Write(data); err != nil {
				return
			}
		}

		if err != nil {
			return
		}
	}
}

// logRedisCommands extracts and logs redis commands
func (s *Server) logRedisCommands(data []byte, conn *proxy.Connection, username string) {
	reader := bufio.NewReader(strings.NewReader(string(data)))

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		if strings.HasPrefix(line, "*") {
			cmd := s.parseRESPCommand(line, reader)
			if cmd != "" {
				_ = audit.Log(s.config.Logging.AuditLogPath, username, "redis_command", conn.Config.Name, map[string]interface{}{
					"connection_id": conn.ID,
					"command":       cmd,
				})
			}
		}
	}
}

// parseRESPCommand parses a RESP array command
func (s *Server) parseRESPCommand(firstLine string, reader *bufio.Reader) string {
	if !strings.HasPrefix(firstLine, "*") {
		return ""
	}

	var parts []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		if strings.HasPrefix(line, "$") {
			value, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			parts = append(parts, strings.TrimSpace(value))
		} else if strings.HasPrefix(line, "*") {
			break
		}
	}

	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	return ""
}

// proxyK8sTCP handles raw TCP bidirectional forwarding
func (s *Server) proxyK8sTCP(clientConn, backendConn net.Conn, conn *proxy.Connection, username string) {
	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Backend
	go func() {
		defer wg.Done()
		_, _ = io.Copy(backendConn, clientConn)
	}()

	// Backend -> Client
	go func() {
		defer wg.Done()
		_, _ = io.Copy(clientConn, backendConn)
	}()

	wg.Wait()
}
