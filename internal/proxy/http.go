package proxy

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/davidcohan/port-authorizing/internal/config"
)

// HTTPProxy handles HTTP/HTTPS proxying
type HTTPProxy struct {
	config *config.ConnectionConfig
	client *http.Client
}

// NewHTTPProxy creates a new HTTP proxy
func NewHTTPProxy(config *config.ConnectionConfig) *HTTPProxy {
	return &HTTPProxy{
		config: config,
		client: &http.Client{},
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

	// Copy response headers
	for key, values := range resp.Header {
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
