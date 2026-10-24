package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/davidcohan/port-authorizing/internal/audit"
	"github.com/davidcohan/port-authorizing/internal/config"
)

// RedisProxy handles Redis protocol proxying with command logging
type RedisProxy struct {
	config       *config.ConnectionConfig
	auditLogPath string
	username     string // API username (for audit logging)
	connectionID string
	apiConfig    *config.Config // Full API config for user validation
	resolver     *config.ConfigSourceResolver
}

// NewRedisProxy creates a new Redis protocol-aware proxy
func NewRedisProxy(cfg *config.ConnectionConfig, auditLogPath, username, connectionID string, apiConfig *config.Config, resolver *config.ConfigSourceResolver) *RedisProxy {
	return &RedisProxy{
		config:       cfg,
		auditLogPath: auditLogPath,
		username:     username,
		connectionID: connectionID,
		apiConfig:    apiConfig,
		resolver:     resolver,
	}
}

// HandleConnection handles a Redis protocol connection
// Client connects with API credentials, proxy intercepts commands and forwards to backend
func (p *RedisProxy) HandleConnection(clientConn net.Conn) error {
	defer func() { _ = clientConn.Close() }()

	// Resolve backend password dynamically (with caching) if configured
	ctx := context.Background()
	backendPassword := ""
	if p.config.BackendPassword.Value != "" || p.config.BackendPassword.Type != config.ConfigSourceTypePlain {
		backendPassword = p.config.BackendPassword.Value

		// Resolve password if it's from ConfigMap/Secret
		if p.resolver != nil && (p.config.BackendPassword.Type == config.ConfigSourceTypeConfigMap || p.config.BackendPassword.Type == config.ConfigSourceTypeSecret) {
			resolvedPassword, exists, err := p.resolver.ResolveConfigSource(ctx, p.config.BackendPassword)
			if err != nil || !exists {
				log.Printf("Warning: Failed to resolve backend password for connection %s: %v (using stored value)", p.config.Name, err)
			} else {
				backendPassword = resolvedPassword
				log.Printf("DEBUG: Resolved backend password for connection %s (type: %s)", p.config.Name, p.config.BackendPassword.Type)
			}
		}
	}

	// Connect to backend (with optional TLS)
	backendAddr := fmt.Sprintf("%s:%d", p.config.Host, p.config.Port)
	backendConn, err := p.dialBackend(backendAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to backend: %w", err)
	}
	defer func() { _ = backendConn.Close() }()

	// Authenticate to backend if password is configured
	if backendPassword != "" {
		if err := p.authenticateToBackend(backendConn, backendPassword); err != nil {
			return fmt.Errorf("backend authentication failed: %w", err)
		}
	}

	_ = audit.Log(p.auditLogPath, p.username, "redis_connected", p.config.Name, map[string]interface{}{
		"connection_id": p.connectionID,
		"status":        "connected",
	})

	// Now do raw TCP bidirectional forwarding with command logging
	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Backend (log commands)
	go func() {
		defer wg.Done()
		defer func() { _ = backendConn.Close() }()
		p.copyWithCommandLogging(clientConn, backendConn, true)
	}()

	// Backend -> Client (pass through)
	go func() {
		defer wg.Done()
		defer func() { _ = clientConn.Close() }()
		p.copyWithCommandLogging(backendConn, clientConn, false)
	}()

	wg.Wait()
	return nil
}

// authenticateToBackend sends AUTH command to backend
func (p *RedisProxy) authenticateToBackend(conn net.Conn, password string) error {
	// Send AUTH command using RESP protocol
	authCmd := fmt.Sprintf("*2\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n", len(password), password)
	if _, err := conn.Write([]byte(authCmd)); err != nil {
		return fmt.Errorf("failed to send AUTH command: %w", err)
	}

	// Read response byte-by-byte until we get a complete RESP message
	// AUTH response is "+OK\r\n" (5 bytes) or "-ERR message\r\n"
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer conn.SetReadDeadline(time.Time{}) // Clear deadline

	var responseBuf []byte
	buf := make([]byte, 1)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return fmt.Errorf("failed to read AUTH response: %w", err)
		}
		if n > 0 {
			responseBuf = append(responseBuf, buf[0])
			// Check if we have a complete response (ends with \r\n)
			if len(responseBuf) >= 2 && responseBuf[len(responseBuf)-2] == '\r' && responseBuf[len(responseBuf)-1] == '\n' {
				break
			}
			// Safety limit
			if len(responseBuf) > 1024 {
				return fmt.Errorf("AUTH response too long")
			}
		}
	}

	response := string(responseBuf)

	// Check for success (+OK)
	if !strings.HasPrefix(response, "+OK") {
		return fmt.Errorf("authentication failed: %s", strings.TrimSpace(response))
	}

	return nil
}

// copyWithCommandLogging copies data between connections and optionally logs commands
func (p *RedisProxy) copyWithCommandLogging(src, dst net.Conn, logCommands bool) {
	buf := make([]byte, 32*1024)
	reader := bufio.NewReader(src)

	for {
		// Read data from source
		n, err := reader.Read(buf)
		if n > 0 {
			data := buf[:n]

			// Try to log commands if this is client->backend traffic
			if logCommands {
				p.tryExtractCommand(data)
			}

			// Forward the data
			if _, err := dst.Write(data); err != nil {
				return
			}
		}

		if err != nil {
			if err != io.EOF {
				_ = audit.Log(p.auditLogPath, p.username, "redis_error", p.config.Name, map[string]interface{}{
					"connection_id": p.connectionID,
					"error":         err.Error(),
					"log_commands":  logCommands,
				})
			}
			return
		}
	}
}

// tryExtractCommand attempts to extract Redis commands from RESP protocol messages
func (p *RedisProxy) tryExtractCommand(data []byte) {
	// Redis RESP protocol: commands start with '*' (array)
	// Example: *2\r\n$3\r\nGET\r\n$3\r\nkey\r\n
	reader := bufio.NewReader(strings.NewReader(string(data)))

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		// Check if this is an array (command)
		if strings.HasPrefix(line, "*") {
			// Parse the command
			cmd := p.parseRESPCommand(line, reader)
			if cmd != "" {
				p.logCommand(cmd)
			}
		}
	}
}

// parseRESPCommand parses a RESP array command
func (p *RedisProxy) parseRESPCommand(firstLine string, reader *bufio.Reader) string {
	// Skip if not array marker
	if !strings.HasPrefix(firstLine, "*") {
		return ""
	}

	// Read bulk strings to build command
	var parts []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		// Bulk string marker '$'
		if strings.HasPrefix(line, "$") {
			// Read the actual string value
			value, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			parts = append(parts, strings.TrimSpace(value))
		} else if strings.HasPrefix(line, "*") {
			// Another command started, stop here
			break
		}
	}

	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	return ""
}

// logCommand logs a Redis command
func (p *RedisProxy) logCommand(command string) {
	if command == "" {
		return
	}

	_ = audit.Log(p.auditLogPath, p.username, "redis_command", p.config.Name, map[string]interface{}{
		"connection_id": p.connectionID,
		"command":       command,
	})
}

// validateAPICredentials validates username/password against API users
func (p *RedisProxy) validateAPICredentials(username, password string) bool {
	if p.apiConfig == nil {
		return false
	}

	for _, user := range p.apiConfig.Auth.Users {
		if user.Username.Value == username && user.Password.Value == password {
			return true
		}
	}

	return false
}

// dialBackend creates a connection to the Redis backend with optional TLS support
func (p *RedisProxy) dialBackend(addr string) (net.Conn, error) {
	timeout := 10 * time.Second

	// If TLS is not enabled, use plain TCP connection
	if p.config.TLS == nil || !p.config.TLS.Enabled {
		return net.DialTimeout("tcp", addr, timeout)
	}

	// Build TLS configuration
	tlsConfig := &tls.Config{
		ServerName:         p.config.TLS.ServerName,
		InsecureSkipVerify: p.config.TLS.SkipVerify,
	}

	// If server name is not set, use the host from the address
	if tlsConfig.ServerName == "" {
		tlsConfig.ServerName = p.config.Host
	}

	// Load CA certificate if provided
	if p.config.TLS.CACert != "" {
		caCert, err := os.ReadFile(p.config.TLS.CACert)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	// Load client certificate and key if provided
	if p.config.TLS.ClientCert != "" && p.config.TLS.ClientKey != "" {
		clientCert, err := tls.LoadX509KeyPair(p.config.TLS.ClientCert, p.config.TLS.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{clientCert}
	}

	// Create TLS connection
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("TLS connection failed: %w", err)
	}

	log.Printf("DEBUG: Established TLS connection to Redis %s (server=%s, skip_verify=%v)",
		addr, tlsConfig.ServerName, tlsConfig.InsecureSkipVerify)

	return conn, nil
}
