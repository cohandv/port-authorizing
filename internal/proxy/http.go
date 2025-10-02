package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/url"

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
	// Build target URL
	scheme := p.config.Scheme
	if scheme == "" {
		scheme = "http"
	}

	targetURL := &url.URL{
		Scheme:   scheme,
		Host:     fmt.Sprintf("%s:%d", p.config.Host, p.config.Port),
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
	}

	// Create new request
	proxyReq, err := http.NewRequest(r.Method, targetURL.String(), r.Body)
	if err != nil {
		return fmt.Errorf("failed to create proxy request: %w", err)
	}

	// Copy headers
	for key, values := range r.Header {
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
