package proxy

import (
	"fmt"
	"net/http"

	"github.com/davidcohan/port-authorizing/internal/config"
)

// Protocol defines the interface for different protocol handlers
type Protocol interface {
	// HandleRequest handles an incoming request and proxies it
	HandleRequest(w http.ResponseWriter, r *http.Request) error
	// Close closes any resources held by the protocol handler
	Close() error
}

// NewProtocol creates a protocol handler based on connection type
func NewProtocol(connConfig *config.ConnectionConfig) (Protocol, error) {
	switch connConfig.Type {
	case "http", "https":
		return NewHTTPProxy(connConfig), nil
	case "postgres":
		return NewPostgresProxy(connConfig), nil
	case "tcp":
		return NewTCPProxy(connConfig), nil
	default:
		return nil, fmt.Errorf("unsupported protocol type: %s", connConfig.Type)
	}
}
