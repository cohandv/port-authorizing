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
	"time"

	"github.com/davidcohan/port-authorizing/internal/audit"
	"github.com/gorilla/mux"
)

// handleHTTPProxyStream handles HTTP connections through TCP stream with approval support
// This intercepts HTTP requests from the stream, parses them, checks approval, then forwards to backend
func (s *Server) handleHTTPProxyStream(w http.ResponseWriter, r *http.Request) {
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

	// Verify it's an HTTP/HTTPS connection
	if conn.Config.Type != "http" && conn.Config.Type != "https" {
		respondError(w, http.StatusBadRequest, "Not an HTTP/HTTPS connection")
		return
	}

	// Log audit event
	_ = audit.Log(s.config.Logging.AuditLogPath, username, "http_connect", conn.Config.Name, map[string]interface{}{
		"connection_id": connectionID,
		"method":        r.Method,
		"roles":         roles,
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

	// Use the HTTP proxy instance from connection (which has approval support)
	httpProxy := conn.Proxy
	if httpProxy == nil {
		_ = audit.Log(s.config.Logging.AuditLogPath, username, "http_error", conn.Config.Name, map[string]interface{}{
			"connection_id": connectionID,
			"error":         "HTTP proxy not initialized",
		})
		return
	}

	// Process HTTP requests in a loop
	reader := bufio.NewReader(bufrw)

	for time.Now().Before(conn.ExpiresAt) {
		// Check if connection expired handled by loop condition

		// Read HTTP request from client
		requestBytes, err := readHTTPRequest(reader)
		if err != nil {
			if err != io.EOF && !strings.Contains(err.Error(), "use of closed") {
				_ = audit.Log(s.config.Logging.AuditLogPath, username, "http_read_error", conn.Config.Name, map[string]interface{}{
					"connection_id": connectionID,
					"error":         err.Error(),
				})
			}
			break
		}

		// Parse HTTP request to get method and path for logging
		reqReader := bufio.NewReader(bytes.NewReader(requestBytes))
		httpReq, err := http.ReadRequest(reqReader)
		if err != nil {
			_ = audit.Log(s.config.Logging.AuditLogPath, username, "http_parse_error", conn.Config.Name, map[string]interface{}{
				"connection_id": connectionID,
				"error":         err.Error(),
			})
			// Send error response to client
			_, _ = fmt.Fprintf(bufrw, "HTTP/1.1 400 Bad Request\r\n\r\nInvalid HTTP request\r\n")
			_ = bufrw.Flush()
			break
		}

		// Create a synthetic HTTP request for the proxy handler
		// The proxy handler expects the request body to contain the raw HTTP request
		proxyReq := httptest.NewRequest("POST", "/", bytes.NewReader(requestBytes))
		proxyReq.Header.Set("Content-Type", "application/octet-stream")

		// Create response writer that writes back to the client
		respWriter := &streamResponseWriter{
			writer: bufrw,
			header: make(http.Header),
		}

		// Call the HTTP proxy's HandleRequest
		// This will check whitelist, approval, and forward to backend!
		err = httpProxy.HandleRequest(respWriter, proxyReq)

		// CRITICAL: Flush the response back to the client!
		_ = bufrw.Flush()

		if err != nil {
			// Error response was already sent by HandleRequest
			// Check if it was a 403 (blocked/rejected)
			if respWriter.statusCode == http.StatusForbidden {
				_ = audit.Log(s.config.Logging.AuditLogPath, username, "http_request_blocked", conn.Config.Name, map[string]interface{}{
					"connection_id": connectionID,
					"method":        httpReq.Method,
					"path":          httpReq.URL.Path,
					"reason":        "blocked by approval or whitelist",
				})
			}
			break
		}

		// If Connection: close, break the loop
		if strings.ToLower(httpReq.Header.Get("Connection")) == "close" {
			break
		}
	}

	_ = audit.Log(s.config.Logging.AuditLogPath, username, "http_disconnect", conn.Config.Name, map[string]interface{}{
		"connection_id": connectionID,
	})
}

// readHTTPRequest reads a complete HTTP request from the reader
func readHTTPRequest(reader *bufio.Reader) ([]byte, error) {
	var buffer bytes.Buffer

	// Peek to see if there's data available
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

		// Empty line indicates end of headers
		if line == "\r\n" || line == "\n" {
			break
		}
	}

	// Check if there's a Content-Length to read body
	requestStr := buffer.String()
	if strings.Contains(strings.ToLower(requestStr), "content-length:") {
		// Parse Content-Length
		lines := strings.Split(requestStr, "\n")
		for _, line := range lines {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "content-length:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					var contentLength int
					_, _ = fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &contentLength)

					if contentLength > 0 {
						// Read body
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

// streamResponseWriter writes HTTP responses directly to the client stream
type streamResponseWriter struct {
	writer      *bufio.ReadWriter
	header      http.Header
	statusCode  int
	wroteHeader bool
}

func (w *streamResponseWriter) Header() http.Header {
	return w.header
}

func (w *streamResponseWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.writer.Write(data)
}

func (w *streamResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.statusCode = statusCode

	// Write status line
	statusText := http.StatusText(statusCode)
	if statusText == "" {
		statusText = "Unknown"
	}
	_, _ = fmt.Fprintf(w.writer, "HTTP/1.1 %d %s\r\n", statusCode, statusText)

	// Write headers
	for key, values := range w.header {
		for _, value := range values {
			_, _ = fmt.Fprintf(w.writer, "%s: %s\r\n", key, value)
		}
	}

	// End headers
	_, _ = fmt.Fprint(w.writer, "\r\n")
	_ = w.writer.Flush()
}

// Implement http.Hijacker interface (needed by some handlers)
func (w *streamResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, fmt.Errorf("hijacking not supported")
}
