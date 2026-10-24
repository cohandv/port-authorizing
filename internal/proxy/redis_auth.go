package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/davidcohan/port-authorizing/internal/approval"
	"github.com/davidcohan/port-authorizing/internal/audit"
	"github.com/davidcohan/port-authorizing/internal/config"
)

// RedisAuthProxy handles Redis with credential substitution, whitelist, and approval
type RedisAuthProxy struct {
	config       *config.ConnectionConfig
	auditLogPath string
	username     string
	connectionID string
	roles        []string
	apiConfig    *config.Config
	whitelist    []string
	approvalMgr  *approval.Manager
	resolver     *config.ConfigSourceResolver
}

// NewRedisAuthProxy creates a Redis proxy with auth handling
func NewRedisAuthProxy(cfg *config.ConnectionConfig, auditLogPath, username, connectionID string, roles []string, apiConfig *config.Config, whitelist []string, resolver *config.ConfigSourceResolver) *RedisAuthProxy {
	return &RedisAuthProxy{
		config:       cfg,
		auditLogPath: auditLogPath,
		username:     username,
		connectionID: connectionID,
		roles:        roles,
		apiConfig:    apiConfig,
		whitelist:    whitelist,
		approvalMgr:  nil, // Will be set later if approvals are enabled
		resolver:     resolver,
	}
}

// SetApprovalManager sets the approval manager for this proxy
func (p *RedisAuthProxy) SetApprovalManager(mgr *approval.Manager) {
	p.approvalMgr = mgr
}

// HandleConnection handles the full Redis connection with auth, whitelist, and approval
func (p *RedisAuthProxy) HandleConnection(clientConn net.Conn) error {
	defer func() { _ = clientConn.Close() }()

	// Resolve backend credentials dynamically (with caching)
	ctx := context.Background()

	// Resolve backend username (for Redis 6.0+ ACL)
	backendUsername := ""
	if p.config.BackendUsername.Value != "" || p.config.BackendUsername.Type != config.ConfigSourceTypePlain {
		backendUsername = p.config.BackendUsername.Value

		// Resolve username if it's from ConfigMap/Secret
		if p.resolver != nil && (p.config.BackendUsername.Type == config.ConfigSourceTypeConfigMap || p.config.BackendUsername.Type == config.ConfigSourceTypeSecret) {
			resolvedUsername, exists, err := p.resolver.ResolveConfigSource(ctx, p.config.BackendUsername)
			if err != nil || !exists {
				log.Printf("Warning: Failed to resolve backend username for connection %s: %v (using stored value)", p.config.Name, err)
			} else {
				backendUsername = resolvedUsername
				log.Printf("DEBUG: Resolved backend username for connection %s (type: %s)", p.config.Name, p.config.BackendUsername.Type)
			}
		}
	}

	// Resolve backend password
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

	// Connect to backend with BACKEND credentials (with optional TLS)
	backendAddr := fmt.Sprintf("%s:%d", p.config.Host, p.config.Port)
	backendConn, err := p.dialBackend(backendAddr)
	if err != nil {
		p.sendError(clientConn, "Backend connection failed")
		return fmt.Errorf("failed to connect to backend: %w", err)
	}
	defer func() { _ = backendConn.Close() }()

	// Authenticate to backend if password is configured
	if backendPassword != "" {
		if err := p.authenticateToBackend(backendConn, backendUsername, backendPassword); err != nil {
			p.sendError(clientConn, "Backend authentication failed")
			return fmt.Errorf("backend auth failed: %w", err)
		}
	}

	// Don't send +OK to client - we're a transparent proxy
	// The client will send commands and get responses naturally

	_ = audit.Log(p.auditLogPath, p.username, "redis_auth", p.config.Name, map[string]interface{}{
		"connection_id": p.connectionID,
		"status":        "authenticated",
	})

	// Now do transparent bidirectional forwarding with command logging and validation
	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Backend (with validation)
	go func() {
		defer wg.Done()
		defer func() { _ = backendConn.Close() }()
		p.forwardWithValidation(clientConn, backendConn, true)
	}()

	// Backend -> Client (no validation)
	go func() {
		defer wg.Done()
		defer func() { _ = clientConn.Close() }()
		p.forwardWithValidation(clientConn, backendConn, false)
	}()

	wg.Wait()
	return nil
}

// authenticateToBackend sends AUTH command to backend
// Supports both Redis 6.0+ ACL (username+password) and legacy (password-only) authentication
// Automatically falls back from ACL to legacy if username authentication fails
func (p *RedisAuthProxy) authenticateToBackend(conn net.Conn, username, password string) error {
	var authCmd string
	authMethod := "legacy (password-only)"

	// Try Redis 6.0+ ACL authentication (username + password) if username is provided
	if username != "" {
		authMethod = "ACL (username+password)"
		// AUTH <username> <password> - 3 arguments
		authCmd = fmt.Sprintf("*3\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n",
			len(username), username, len(password), password)

		log.Printf("DEBUG: Attempting Redis ACL authentication for connection %s (username: %s)", p.config.Name, username)

		if err := p.sendAuthCommand(conn, authCmd); err != nil {
			// Check if error indicates username/password auth not supported (Redis < 6.0)
			errStr := err.Error()
			if strings.Contains(errStr, "wrong number of arguments") ||
				strings.Contains(errStr, "ERR invalid password") ||
				strings.Contains(errStr, "WRONGPASS") {
				// Fall back to password-only authentication
				log.Printf("DEBUG: ACL auth failed for connection %s, falling back to legacy password-only auth: %v", p.config.Name, err)
				authMethod = "legacy (password-only, fallback)"
				authCmd = fmt.Sprintf("*2\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n", len(password), password)

				if err := p.sendAuthCommand(conn, authCmd); err != nil {
					return fmt.Errorf("both ACL and legacy authentication failed: %w", err)
				}
			} else {
				return fmt.Errorf("ACL authentication failed: %w", err)
			}
		}
	} else {
		// No username provided - use legacy password-only authentication (Redis < 6.0)
		authCmd = fmt.Sprintf("*2\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n", len(password), password)
		log.Printf("DEBUG: Using Redis legacy authentication for connection %s (no username configured)", p.config.Name)

		if err := p.sendAuthCommand(conn, authCmd); err != nil {
			return fmt.Errorf("legacy authentication failed: %w", err)
		}
	}

	log.Printf("DEBUG: Redis authentication successful for connection %s (method: %s)", p.config.Name, authMethod)
	return nil
}

// sendAuthCommand sends an AUTH command and reads the response
func (p *RedisAuthProxy) sendAuthCommand(conn net.Conn, authCmd string) error {
	// Send AUTH command using RESP protocol
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

// sendAuthSuccess sends authentication success to client
func (p *RedisAuthProxy) sendAuthSuccess(conn net.Conn) error {
	// Send +OK\r\n
	_, err := conn.Write([]byte("+OK\r\n"))
	return err
}

// sendError sends an error to client in RESP format
func (p *RedisAuthProxy) sendError(conn net.Conn, message string) {
	errMsg := fmt.Sprintf("-ERR %s\r\n", message)
	_, _ = conn.Write([]byte(errMsg))
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// forwardWithValidation forwards data and validates/logs commands
func (p *RedisAuthProxy) forwardWithValidation(clientConn, backendConn net.Conn, validateCommands bool) {
	buf := make([]byte, 32*1024)

	var src, dst net.Conn
	if validateCommands {
		// client -> backend
		src = clientConn
		dst = backendConn
	} else {
		// backend -> client
		src = backendConn
		dst = clientConn
	}

	reader := bufio.NewReader(src)

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			data := buf[:n]

			if validateCommands {
				// Validate commands against whitelist and check approval before forwarding
				if blocked, command := p.validateAndLogCommand(data); blocked {
					// Send error DIRECTLY to client (not through src)
					p.sendCommandBlockedError(clientConn, command)
					continue
				}
			}

			if _, err := dst.Write(data); err != nil {
				return
			}
		}

		if err != nil {
			return
		}
	}
}

// validateAndLogCommand extracts commands, validates against whitelist, checks approval, and logs
// Returns (blocked, command) where blocked=true if command should be blocked
func (p *RedisAuthProxy) validateAndLogCommand(data []byte) (bool, string) {
	// Try to extract command from RESP protocol first
	commands := p.extractRESPCommands(data)

	// If no RESP commands found, try inline command format (e.g., "GET key\n")
	if len(commands) == 0 {
		inlineCmd := p.extractInlineCommand(data)
		if inlineCmd != "" {
			commands = []string{inlineCmd}
		}
	}

	// No commands extracted - allow through (might be protocol data)
	if len(commands) == 0 {
		return false, ""
	}

	for _, command := range commands {
		if command == "" {
			continue
		}

		// Check whitelist first
		allowed := p.isCommandAllowed(command)

		// Log the command with whitelist result
		_ = audit.Log(p.auditLogPath, p.username, "redis_command", p.config.Name, map[string]interface{}{
			"connection_id": p.connectionID,
			"command":       command,
			"allowed":       allowed,
			"whitelist":     len(p.whitelist) > 0,
		})

		if !allowed {
			// Log blocked command
			_ = audit.Log(p.auditLogPath, p.username, "redis_command_blocked", p.config.Name, map[string]interface{}{
				"connection_id": p.connectionID,
				"command":       command,
				"reason":        "whitelist_violation",
			})
			return true, command
		}

		// Check if approval is required for this command
		if p.approvalMgr != nil {
			normalizedCommand := strings.TrimSpace(command)
			requiresApproval, timeout := p.approvalMgr.RequiresApproval(normalizedCommand, "", p.config.Tags, p.roles)
			if requiresApproval {
				// Request approval
				approvalReq := &approval.Request{
					Username:       p.username,
					ConnectionID:   p.connectionID,
					ConnectionType: p.config.Type,
					ConnectionName: p.config.Name,
					Command:        normalizedCommand,
					Environment:    p.apiConfig.Environment,
					Metadata: map[string]string{
						"connection_name": p.config.Name,
						"connection_type": p.config.Type,
					},
				}

				// Log approval request
				_ = audit.Log(p.auditLogPath, p.username, "redis_approval_requested", p.config.Name, map[string]interface{}{
					"connection_id": p.connectionID,
					"command":       command,
					"timeout":       timeout.String(),
				})

				// Wait for approval with timeout
				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				defer cancel()

				approvalResp, err := p.approvalMgr.RequestApproval(ctx, approvalReq, timeout)
				if err != nil {
					// Log approval error
					_ = audit.Log(p.auditLogPath, p.username, "redis_approval_error", p.config.Name, map[string]interface{}{
						"connection_id": p.connectionID,
						"command":       command,
						"error":         err.Error(),
					})
					return true, command
				}

				// Check approval decision
				if approvalResp.Decision != approval.DecisionApproved {
					// Log rejection/timeout
					_ = audit.Log(p.auditLogPath, p.username, "redis_approval_rejected", p.config.Name, map[string]interface{}{
						"connection_id": p.connectionID,
						"command":       command,
						"decision":      approvalResp.Decision,
						"reason":        approvalResp.Reason,
						"rejected_by":   approvalResp.ApprovedBy,
					})
					return true, command
				}

				// Log approval success
				_ = audit.Log(p.auditLogPath, p.username, "redis_approval_granted", p.config.Name, map[string]interface{}{
					"connection_id": p.connectionID,
					"command":       command,
					"approved_by":   approvalResp.ApprovedBy,
				})
			}
		}
	}

	return false, ""
}

// extractInlineCommand extracts commands from inline format (e.g., "GET key\n", "SET key value\n")
func (p *RedisAuthProxy) extractInlineCommand(data []byte) string {
	// Inline commands are newline-terminated
	dataStr := string(data)
	dataStr = strings.TrimSpace(dataStr)

	if dataStr == "" {
		return ""
	}

	// Return the entire line as the command
	// This will be in format like "GET key" or "SET key value"
	return dataStr
}

// extractRESPCommands parses RESP protocol to extract Redis commands
func (p *RedisAuthProxy) extractRESPCommands(data []byte) []string {
	var commands []string
	str := string(data)
	pos := 0

	for pos < len(str) {
		// Look for array marker
		if str[pos] == '*' {
			// Find end of this command and parse it
			endPos := pos

			// Read array count line
			lineEnd := strings.Index(str[pos:], "\r\n")
			if lineEnd == -1 {
				break
			}
			endPos = pos + lineEnd + 2

			// Extract just this command's data
			cmdStart := pos

			// Count expected bulk strings
			countStr := strings.TrimSpace(str[pos+1 : pos+lineEnd])
			var count int
			fmt.Sscanf(countStr, "%d", &count)

			// Read each bulk string
			for i := 0; i < count && endPos < len(str); i++ {
				// Find '$' marker
				if endPos >= len(str) || str[endPos] != '$' {
					break
				}

				// Read length line
				lineEnd = strings.Index(str[endPos:], "\r\n")
				if lineEnd == -1 {
					break
				}
				endPos += lineEnd + 2

				// Read value line
				lineEnd = strings.Index(str[endPos:], "\r\n")
				if lineEnd == -1 {
					break
				}
				endPos += lineEnd + 2
			}

			// Parse this command
			cmdData := str[cmdStart:endPos]
			cmd := p.parseRESPCommandFromString(cmdData)
			if cmd != "" {
				commands = append(commands, cmd)
			}

			pos = endPos
		} else {
			pos++
		}
	}

	return commands
}

// parseRESPCommandFromString parses a complete RESP command string
func (p *RedisAuthProxy) parseRESPCommandFromString(cmdStr string) string {
	reader := bufio.NewReader(strings.NewReader(cmdStr))
	firstLine, err := reader.ReadString('\n')
	if err != nil {
		return ""
	}
	return p.parseRESPCommand(firstLine, reader)
}

// parseRESPCommand parses a RESP array command
func (p *RedisAuthProxy) parseRESPCommand(firstLine string, reader *bufio.Reader) string {
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

// isCommandAllowed checks if a command matches the whitelist patterns (case-insensitive)
func (p *RedisAuthProxy) isCommandAllowed(command string) bool {
	// If no whitelist, allow everything (backward compatibility)
	if len(p.whitelist) == 0 {
		return true
	}

	// Check against whitelist patterns
	for _, pattern := range p.whitelist {
		// Compile with case-insensitive flag
		re, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			// Log bad pattern but don't block
			_ = audit.Log(p.auditLogPath, p.username, "whitelist_error", p.config.Name, map[string]interface{}{
				"connection_id": p.connectionID,
				"pattern":       pattern,
				"error":         err.Error(),
			})
			continue
		}
		if re.MatchString(command) {
			return true
		}
	}

	return false
}

// sendCommandBlockedError sends an error response to the client for blocked commands
func (p *RedisAuthProxy) sendCommandBlockedError(conn net.Conn, command string) {
	// Truncate command if too long
	displayCommand := command
	if len(displayCommand) > 100 {
		displayCommand = displayCommand[:100] + "..."
	}

	// Send Redis error response in RESP format
	errMsg := fmt.Sprintf("-ERR Command blocked by whitelist policy: %s\r\n", displayCommand)
	_, _ = conn.Write([]byte(errMsg))
}

// dialBackend creates a connection to the Redis backend with optional TLS support
func (p *RedisAuthProxy) dialBackend(addr string) (net.Conn, error) {
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
