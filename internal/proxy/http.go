package proxy

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/davidcohan/port-authorizing/internal/audit"
	"github.com/davidcohan/port-authorizing/internal/config"
)

// HTTPProxy handles HTTP/HTTPS proxying
type HTTPProxy struct {
	config       *config.ConnectionConfig
	client       *http.Client
	whitelist    []string
	auditLogPath string
	username     string
	connectionID string
}

// NewHTTPProxy creates a new HTTP proxy
func NewHTTPProxy(config *config.ConnectionConfig) *HTTPProxy {
	return &HTTPProxy{
		config: config,
		client: &http.Client{},
	}
}

// NewHTTPProxyWithWhitelist creates a new HTTP proxy with whitelist support
func NewHTTPProxyWithWhitelist(config *config.ConnectionConfig, whitelist []string, auditLogPath, username, connectionID string) *HTTPProxy {
	return &HTTPProxy{
		config:       config,
		client:       &http.Client{},
		whitelist:    whitelist,
		auditLogPath: auditLogPath,
		username:     username,
		connectionID: connectionID,
	}
}

// HandleRequest proxies HTTP requests
func (p *HTTPProxy) HandleRequest(w http.ResponseWriter, r *http.Request) error {
	// Read the raw HTTP request from the body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}

	// Parse the raw HTTP request
	// Expected format: "METHOD /path HTTP/1.1\r\nHeader: value\r\n\r\nbody"
	reader := bufio.NewReader(bytes.NewReader(body))

	// Read request line (e.g., "GET / HTTP/1.1")
	requestLine, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read request line: %w", err)
	}

	// Parse method and path
	parts := strings.Fields(requestLine)
	if len(parts) < 2 {
		return fmt.Errorf("invalid request line: %s", requestLine)
	}
	method := parts[0]
	path := parts[1]

	// Handle OPTIONS preflight requests
	if method == "OPTIONS" {
		// Add CORS headers for preflight
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Max-Age", "86400")
		w.WriteHeader(http.StatusOK)

		// Log OPTIONS request
		if p.auditLogPath != "" {
			audit.Log(p.auditLogPath, p.username, "http_preflight", p.config.Name, map[string]interface{}{
				"connection_id": p.connectionID,
				"method":        method,
				"path":          path,
			})
		}
		return nil
	}

	// Validate request against whitelist if configured
	if len(p.whitelist) > 0 {
		requestPattern := fmt.Sprintf("%s %s", method, path)
		if !p.isRequestAllowed(requestPattern) {
			// Log blocked request
			if p.auditLogPath != "" {
				audit.Log(p.auditLogPath, p.username, "http_request_blocked", p.config.Name, map[string]interface{}{
					"connection_id": p.connectionID,
					"method":        method,
					"path":          path,
					"reason":        "does not match whitelist",
				})
			}

			// Add CORS headers even for blocked requests
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin")

			// Return 403 Forbidden
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error":"Request blocked by security policy","message":"This HTTP request is not allowed by the whitelist"}`))
			return fmt.Errorf("request blocked by whitelist: %s %s", method, path)
		}

		// Log allowed request
		if p.auditLogPath != "" {
			audit.Log(p.auditLogPath, p.username, "http_request", p.config.Name, map[string]interface{}{
				"connection_id": p.connectionID,
				"method":        method,
				"path":          path,
				"allowed":       true,
			})
		}
	}

	// Build target URL
	scheme := p.config.Scheme
	if scheme == "" {
		scheme = "http"
	}

	targetURL := &url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%d", p.config.Host, p.config.Port),
		Path:   path,
	}

	// Read headers from raw request
	headers := make(http.Header)
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break // End of headers
		}

		// Parse header (e.g., "Host: localhost")
		headerParts := strings.SplitN(line, ":", 2)
		if len(headerParts) == 2 {
			key := strings.TrimSpace(headerParts[0])
			value := strings.TrimSpace(headerParts[1])
			headers.Add(key, value)
		}
	}

	// Read remaining body (if any)
	requestBody, _ := io.ReadAll(reader)

	// Create new request to target
	proxyReq, err := http.NewRequest(method, targetURL.String(), bytes.NewReader(requestBody))
	if err != nil {
		return fmt.Errorf("failed to create proxy request: %w", err)
	}

	// Copy parsed headers
	for key, values := range headers {
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}

	// Execute request
	resp, err := p.client.Do(proxyReq)
	if err != nil {
		return fmt.Errorf("failed to execute proxy request: %w", err)
	}
	defer resp.Body.Close()

	// Add CORS headers (allow all origins for proxied connections)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Type, Authorization")

	// Copy response headers from backend
	for key, values := range resp.Header {
		// Don't override CORS headers we just set
		if strings.HasPrefix(strings.ToLower(key), "access-control-") {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Write status code
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("failed to copy response body: %w", err)
	}

	return nil
}

// Close closes the HTTP proxy
func (p *HTTPProxy) Close() error {
	p.client.CloseIdleConnections()
	return nil
}

// isRequestAllowed checks if an HTTP request matches the whitelist
// Pattern format: "METHOD /path/pattern"
// Examples: "GET /api/.*", "POST /api/users", "GET /api/users/[0-9]+"
func (p *HTTPProxy) isRequestAllowed(request string) bool {
	if len(p.whitelist) == 0 {
		return true // No whitelist means everything is allowed
	}

	for _, pattern := range p.whitelist {
		// Make pattern case-insensitive for the HTTP method part
		re, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			// Log error and skip this pattern
			if p.auditLogPath != "" {
				audit.Log(p.auditLogPath, p.username, "http_whitelist_error", p.config.Name, map[string]interface{}{
					"pattern": pattern,
					"error":   err.Error(),
				})
			}
			continue
		}

		if re.MatchString(request) {
			return true
		}
	}

	return false
}
