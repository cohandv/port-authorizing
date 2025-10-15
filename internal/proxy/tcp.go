package proxy

import (
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/davidcohan/port-authorizing/internal/config"
)

// TCPProxy handles raw TCP proxying
type TCPProxy struct {
	config *config.ConnectionConfig
}

// NewTCPProxy creates a new TCP proxy
func NewTCPProxy(config *config.ConnectionConfig) *TCPProxy {
	return &TCPProxy{
		config: config,
	}
}

// HandleRequest handles TCP proxy requests
// Note: This is simplified for HTTP-based API. In production:
// 1. The CLI would establish a TCP connection
// 2. This would proxy raw TCP data bidirectionally
func (p *TCPProxy) HandleRequest(w http.ResponseWriter, r *http.Request) error {
	// Connect to target
	targetAddr := fmt.Sprintf("%s:%d", p.config.Host, p.config.Port)
	conn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to target: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}

	// Send to target
	if _, err := conn.Write(body); err != nil {
		return fmt.Errorf("failed to write to target: %w", err)
	}

	// Read response
	response, err := io.ReadAll(conn)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read from target: %w", err)
	}

	// Send response
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(response)

	return nil
}

// Close closes the TCP proxy
func (p *TCPProxy) Close() error {
	// Close any active TCP connections
	return nil
}
