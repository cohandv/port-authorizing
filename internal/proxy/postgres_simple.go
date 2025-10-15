package proxy

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/davidcohan/port-authorizing/internal/audit"
	"github.com/davidcohan/port-authorizing/internal/config"
)

// SimplePostgresProxy is a simpler postgres proxy that focuses on query logging
type SimplePostgresProxy struct {
	config       *config.ConnectionConfig
	auditLogPath string
	username     string
	connectionID string
}

// NewSimplePostgresProxy creates a simplified postgres proxy
func NewSimplePostgresProxy(cfg *config.ConnectionConfig, auditLogPath, username, connectionID string) *SimplePostgresProxy {
	return &SimplePostgresProxy{
		config:       cfg,
		auditLogPath: auditLogPath,
		username:     username,
		connectionID: connectionID,
	}
}

// HandleConnection handles a postgres connection with simple pass-through and query logging
func (p *SimplePostgresProxy) HandleConnection(clientConn net.Conn) error {
	defer clientConn.Close()

	// Connect to backend immediately
	backendAddr := fmt.Sprintf("%s:%d", p.config.Host, p.config.Port)
	backendConn, err := net.DialTimeout("tcp", backendAddr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to backend: %w", err)
	}
	defer backendConn.Close()

	audit.Log(p.auditLogPath, p.username, "postgres_connect", p.config.Name, map[string]interface{}{
		"connection_id": p.connectionID,
		"backend":       backendAddr,
	})

	// Do bidirectional proxying with query interception
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Backend (intercept and log queries)
	go func() {
		defer wg.Done()
		defer backendConn.Close()
		p.proxyWithQueryLogging(ctx, clientConn, backendConn, true)
	}()

	// Backend -> Client (pass through)
	go func() {
		defer wg.Done()
		defer clientConn.Close()
		p.proxyWithQueryLogging(ctx, backendConn, clientConn, false)
	}()

	wg.Wait()
	return nil
}

// proxyWithQueryLogging proxies data and optionally logs queries
func (p *SimplePostgresProxy) proxyWithQueryLogging(ctx context.Context, src, dst net.Conn, logQueries bool) {
	reader := bufio.NewReader(src)
	writer := bufio.NewWriter(dst)

	buf := make([]byte, 32*1024)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Set read deadline to allow context cancellation
		_ = src.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

		n, err := reader.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // timeout, check context and try again
			}
			if err != io.EOF {
				audit.Log(p.auditLogPath, p.username, "postgres_error", p.config.Name, map[string]interface{}{
					"connection_id": p.connectionID,
					"error":         err.Error(),
					"log_queries":   logQueries,
				})
			}
			return
		}

		if n > 0 {
			data := buf[:n]

			// Try to extract and log queries if this is client->backend traffic
			if logQueries {
				p.tryLogQuery(data)
			}

			// Forward the data
			if _, err := writer.Write(data); err != nil {
				return
			}
			if err := writer.Flush(); err != nil {
				return
			}
		}
	}
}

// tryLogQuery attempts to extract SQL queries from postgres protocol messages
func (p *SimplePostgresProxy) tryLogQuery(data []byte) {
	// Postgres simple query protocol: 'Q' followed by 4-byte length, then SQL string
	for i := 0; i < len(data); i++ {
		if data[i] == 'Q' && i+5 < len(data) {
			// Read length (4 bytes, big-endian)
			length := int(data[i+1])<<24 | int(data[i+2])<<16 | int(data[i+3])<<8 | int(data[i+4])

			// Check if we have the full message
			if i+1+length <= len(data) {
				// Extract query (skip 'Q' and 4-byte length)
				queryStart := i + 5
				queryEnd := i + 1 + length

				if queryEnd <= len(data) {
					queryBytes := data[queryStart:queryEnd]
					// Query is null-terminated
					query := string(bytes.TrimRight(queryBytes, "\x00"))

					if query != "" {
						audit.Log(p.auditLogPath, p.username, "postgres_query", p.config.Name, map[string]interface{}{
							"connection_id": p.connectionID,
							"query":         query,
							"database":      p.config.BackendDatabase,
						})
					}
				}

				// Move past this message
				i += length
			}
		}
	}
}
