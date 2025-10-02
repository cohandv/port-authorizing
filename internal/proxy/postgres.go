package proxy

import (
	"fmt"
	"io"
	"net/http"

	"github.com/davidcohan/port-authorizing/internal/config"
	"github.com/davidcohan/port-authorizing/internal/security"
)

// PostgresProxy handles PostgreSQL protocol proxying
type PostgresProxy struct {
	config *config.ConnectionConfig
}

// NewPostgresProxy creates a new PostgreSQL proxy
func NewPostgresProxy(config *config.ConnectionConfig) *PostgresProxy {
	return &PostgresProxy{
		config: config,
	}
}

// HandleRequest handles PostgreSQL protocol requests
// Note: This is a simplified implementation. In production, you would:
// 1. Parse PostgreSQL wire protocol messages
// 2. Validate queries against whitelist
// 3. Forward to actual PostgreSQL server
// 4. Return responses in PostgreSQL wire protocol format
func (p *PostgresProxy) HandleRequest(w http.ResponseWriter, r *http.Request) error {
	// For HTTP-based PostgreSQL proxy, we expect JSON requests with SQL queries
	// In production, the CLI would establish a TCP connection that speaks PostgreSQL protocol

	// Read request body (expected to contain SQL query)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}

	// Validate against whitelist
	if err := p.validateQuery(string(body)); err != nil {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(fmt.Sprintf("Query blocked: %v", err)))
		return nil
	}

	// TODO: Forward to actual PostgreSQL server
	// This would involve:
	// 1. Establishing connection to PostgreSQL
	// 2. Executing the query
	// 3. Returning results

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "PostgreSQL proxy - implementation pending"}`))

	return nil
}

// validateQuery checks if the query matches whitelist patterns
func (p *PostgresProxy) validateQuery(query string) error {
	return security.ValidateQuery(p.config.Whitelist, query)
}

// Close closes the PostgreSQL proxy
func (p *PostgresProxy) Close() error {
	// Close any active PostgreSQL connections
	return nil
}
