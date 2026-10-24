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
// Note: Postgres and Redis use different handlers (handlePostgresProxy, handleRedisWebSocket) and don't use this interface
func NewProtocol(connConfig *config.ConnectionConfig) (Protocol, error) {
	switch connConfig.Type {
	case "http", "https":
		return NewHTTPProxy(connConfig), nil
	case "postgres":
		// Postgres handled separately via handlePostgresProxy in API
		return nil, fmt.Errorf("postgres protocol uses dedicated handler, not this interface")
	case "redis":
		// Redis handled separately via handleRedisWebSocket in API
		return nil, fmt.Errorf("redis protocol uses dedicated handler, not this interface")
	case "tcp":
		return NewTCPProxy(connConfig), nil
	case "k8s_services":
		// K8s services handled separately via dedicated WebSocket handler
		return nil, fmt.Errorf("k8s_services protocol uses dedicated handler, not this interface")
	default:
		return nil, fmt.Errorf("unsupported protocol type: %s", connConfig.Type)
	}
}
