package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/davidcohan/port-authorizing/internal/approval"
	"github.com/davidcohan/port-authorizing/internal/audit"
	"github.com/davidcohan/port-authorizing/internal/config"
)

// RedisProxy handles Redis protocol interception and proxying
type RedisProxy struct {
	config       *config.ConnectionConfig
	auditLogPath string
	username     string
	connectionID string
	whitelist    []string
	approvalMgr  *approval.Manager
	mu           sync.RWMutex
}

// NewRedisProxy creates a new Redis proxy
func NewRedisProxy(cfg *config.ConnectionConfig, auditLogPath, username, connectionID string, whitelist []string) *RedisProxy {
	return &RedisProxy{
		config:       cfg,
		auditLogPath: auditLogPath,
		username:     username,
		connectionID: connectionID,
		whitelist:    whitelist,
	}
}

// SetApprovalManager sets the approval manager for this proxy
func (p *RedisProxy) SetApprovalManager(mgr *approval.Manager) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.approvalMgr = mgr
}

// HandleConnection handles a Redis connection with protocol interception
func (p *RedisProxy) HandleConnection(clientConn net.Conn) error {
	defer clientConn.Close()

	// Connect to backend Redis
	backendAddr := fmt.Sprintf("%s:%d", p.config.Host, p.config.Port)
	backendConn, err := net.DialTimeout("tcp", backendAddr, 10*time.Second)
	if err != nil {
		audit.Log(p.auditLogPath, p.username, "redis_connect_failed", p.config.Name, map[string]interface{}{
			"connection_id": p.connectionID,
			"backend":       backendAddr,
			"error":         err.Error(),
		})
		return fmt.Errorf("failed to connect to Redis backend: %w", err)
	}
	defer backendConn.Close()

	audit.Log(p.auditLogPath, p.username, "redis_connect", p.config.Name, map[string]interface{}{
		"connection_id": p.connectionID,
		"backend":       backendAddr,
	})

	// Authenticate to backend if password is configured
	if p.config.BackendPassword != "" {
		if err := p.authenticateBackend(backendConn); err != nil {
			audit.Log(p.auditLogPath, p.username, "redis_auth_failed", p.config.Name, map[string]interface{}{
				"connection_id": p.connectionID,
				"error":         err.Error(),
			})
			return fmt.Errorf("failed to authenticate to Redis backend: %w", err)
		}

		audit.Log(p.auditLogPath, p.username, "redis_auth_success", p.config.Name, map[string]interface{}{
			"connection_id": p.connectionID,
		})
	}

	// Start bidirectional proxying with command interception
	errChan := make(chan error, 2)

	// Client -> Backend (with command interception)
	go func() {
		errChan <- p.interceptCommands(clientConn, backendConn)
	}()

	// Backend -> Client (passthrough)
	go func() {
		_, err := io.Copy(clientConn, backendConn)
		errChan <- err
	}()

	// Wait for either direction to complete
	err = <-errChan

	audit.Log(p.auditLogPath, p.username, "redis_disconnect", p.config.Name, map[string]interface{}{
		"connection_id": p.connectionID,
	})

	return err
}

// authenticateBackend sends AUTH command to backend Redis
func (p *RedisProxy) authenticateBackend(conn net.Conn) error {
	// Send AUTH command
	authCmd := fmt.Sprintf("*2\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n", len(p.config.BackendPassword), p.config.BackendPassword)
	if _, err := conn.Write([]byte(authCmd)); err != nil {
		return fmt.Errorf("failed to send AUTH command: %w", err)
	}

	// Read response
	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read AUTH response: %w", err)
	}

	// Check for error response
	if strings.HasPrefix(response, "-") {
		return fmt.Errorf("AUTH failed: %s", strings.TrimSpace(response[1:]))
	}

	// Expecting +OK\r\n
	if !strings.HasPrefix(response, "+OK") {
		return fmt.Errorf("unexpected AUTH response: %s", response)
	}

	return nil
}

// interceptCommands reads commands from client, validates them, and forwards to backend
func (p *RedisProxy) interceptCommands(clientConn, backendConn net.Conn) error {
	parser := NewRESPParser(clientConn)

	for {
		// Parse command
		cmd, err := parser.ParseCommand()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("failed to parse command: %w", err)
		}

		// Validate command against whitelist
		if err := p.validateCommand(cmd); err != nil {
			audit.Log(p.auditLogPath, p.username, "redis_command_blocked", p.config.Name, map[string]interface{}{
				"connection_id": p.connectionID,
				"command":       cmd.String(),
				"reason":        err.Error(),
			})

			// Send error response to client
			errorMsg := fmt.Sprintf("-ERR %s\r\n", err.Error())
			if _, writeErr := clientConn.Write([]byte(errorMsg)); writeErr != nil {
				return writeErr
			}
			continue
		}

		// Check if approval is required
		p.mu.RLock()
		approvalMgr := p.approvalMgr
		p.mu.RUnlock()

		if approvalMgr != nil {
			requiresApproval, timeout := approvalMgr.RequiresApproval(cmd.String(), "", p.config.Tags)
			if requiresApproval {
				// Create approval request
				approvalReq := &approval.Request{
					Username:     p.username,
					ConnectionID: p.connectionID,
					Method:       cmd.String(), // Redis command as "method"
					Path:         "",           // No path for Redis commands
					Metadata: map[string]string{
						"connection_name": p.config.Name,
						"connection_type": p.config.Type,
					},
				}

				audit.Log(p.auditLogPath, p.username, "redis_command_awaiting_approval", p.config.Name, map[string]interface{}{
					"connection_id": p.connectionID,
					"command":       cmd.String(),
					"timeout":       timeout.String(),
				})

				// Wait for approval with timeout
				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				defer cancel()

				approvalResp, err := approvalMgr.RequestApproval(ctx, approvalReq, timeout)
				if err != nil {
					audit.Log(p.auditLogPath, p.username, "redis_approval_error", p.config.Name, map[string]interface{}{
						"connection_id": p.connectionID,
						"command":       cmd.String(),
						"error":         err.Error(),
					})

					errorMsg := fmt.Sprintf("-ERR Approval request failed: %s\r\n", err.Error())
					if _, writeErr := clientConn.Write([]byte(errorMsg)); writeErr != nil {
						return writeErr
					}
					continue
				}

				// Check approval decision
				if approvalResp.Decision != approval.DecisionApproved {
					audit.Log(p.auditLogPath, p.username, "redis_command_rejected", p.config.Name, map[string]interface{}{
						"connection_id": p.connectionID,
						"command":       cmd.String(),
						"request_id":    approvalResp.RequestID,
						"decision":      approvalResp.Decision,
					})

					errorMsg := "-ERR Command rejected or timed out\r\n"
					if _, writeErr := clientConn.Write([]byte(errorMsg)); writeErr != nil {
						return writeErr
					}
					continue
				}

				audit.Log(p.auditLogPath, p.username, "redis_command_approved", p.config.Name, map[string]interface{}{
					"connection_id": p.connectionID,
					"command":       cmd.String(),
					"request_id":    approvalResp.RequestID,
					"approver":      approvalResp.ApprovedBy,
				})
			}
		}

		// Log command
		audit.Log(p.auditLogPath, p.username, "redis_command", p.config.Name, map[string]interface{}{
			"connection_id": p.connectionID,
			"command":       cmd.String(),
		})

		// Forward to backend
		if _, err := backendConn.Write(cmd.Raw); err != nil {
			return fmt.Errorf("failed to forward command to backend: %w", err)
		}
	}
}

// validateCommand checks if a command is allowed by the whitelist
func (p *RedisProxy) validateCommand(cmd *RedisCommand) error {
	if len(p.whitelist) == 0 {
		// No whitelist = allow all
		return nil
	}

	for _, pattern := range p.whitelist {
		if matchesRedisPattern(pattern, cmd) {
			return nil
		}
	}

	return fmt.Errorf("command not allowed by whitelist")
}

// matchesRedisPattern checks if a Redis command matches a whitelist pattern
// Pattern format: "COMMAND [arg_pattern...]"
// Examples:
//
//	"GET *" - allow all GET
//	"GET myapp:*" - allow GET on keys starting with "myapp:"
//	"SET myapp:*" - allow SET on keys starting with "myapp:"
//	"HGET * *" - allow all HGET (any hash, any field)
//	"DEL myapp:*" - allow DEL only for keys starting with "myapp:"
//	"KEYS" - allow KEYS command (no args = exact match)
func matchesRedisPattern(pattern string, cmd *RedisCommand) bool {
	parts := strings.Fields(pattern)
	if len(parts) == 0 {
		return false
	}

	// Command must match (case-insensitive)
	patternCmd := strings.ToUpper(parts[0])
	if patternCmd != cmd.Command {
		return false
	}

	// If no argument patterns specified, just match the command
	if len(parts) == 1 {
		return true
	}

	// Match argument patterns
	argPatterns := parts[1:]

	// If pattern has more args than command, no match
	if len(argPatterns) > len(cmd.Args) {
		return false
	}

	// Match each argument pattern
	for i, argPattern := range argPatterns {
		if !matchesGlobPattern(argPattern, cmd.Args[i]) {
			return false
		}
	}

	return true
}

// matchesGlobPattern matches a string against a glob pattern (* wildcard)
func matchesGlobPattern(pattern, s string) bool {
	if pattern == "*" {
		return true
	}

	// Convert glob pattern to regex
	regexPattern := "^" + regexp.QuoteMeta(pattern) + "$"
	regexPattern = strings.ReplaceAll(regexPattern, "\\*", ".*")

	matched, _ := regexp.MatchString(regexPattern, s)
	return matched
}
