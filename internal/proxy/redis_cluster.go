package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/davidcohan/port-authorizing/internal/approval"
	"github.com/davidcohan/port-authorizing/internal/audit"
	"github.com/davidcohan/port-authorizing/internal/config"
)

// RedisClusterProxy handles Redis Cluster connections with slot-based routing
type RedisClusterProxy struct {
	config       *config.ConnectionConfig
	auditLogPath string
	username     string
	connectionID string
	whitelist    []string
	approvalMgr  *approval.Manager
	nodeConns    map[string]net.Conn // addr -> connection
	mu           sync.RWMutex
}

// NewRedisClusterProxy creates a new Redis Cluster proxy
func NewRedisClusterProxy(cfg *config.ConnectionConfig, auditLogPath, username, connectionID string, whitelist []string) *RedisClusterProxy {
	return &RedisClusterProxy{
		config:       cfg,
		auditLogPath: auditLogPath,
		username:     username,
		connectionID: connectionID,
		whitelist:    whitelist,
		nodeConns:    make(map[string]net.Conn),
	}
}

// SetApprovalManager sets the approval manager for this proxy
func (p *RedisClusterProxy) SetApprovalManager(mgr *approval.Manager) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.approvalMgr = mgr
}

// HandleConnection handles a Redis Cluster connection
func (p *RedisClusterProxy) HandleConnection(clientConn net.Conn) error {
	defer clientConn.Close()
	defer p.closeAllNodes()

	// Connect to initial cluster node
	initialAddr := fmt.Sprintf("%s:%d", p.config.Host, p.config.Port)
	initialConn, err := p.getOrCreateNodeConnection(initialAddr)
	if err != nil {
		audit.Log(p.auditLogPath, p.username, "redis_cluster_connect_failed", p.config.Name, map[string]interface{}{
			"connection_id": p.connectionID,
			"node":          initialAddr,
			"error":         err.Error(),
		})
		return fmt.Errorf("failed to connect to Redis cluster node: %w", err)
	}

	audit.Log(p.auditLogPath, p.username, "redis_cluster_connect", p.config.Name, map[string]interface{}{
		"connection_id": p.connectionID,
		"initial_node":  initialAddr,
	})

	// Start command interception loop
	parser := NewRESPParser(clientConn)

	for {
		// Parse command from client
		cmd, err := parser.ParseCommand()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("failed to parse command: %w", err)
		}

		// Validate command against whitelist
		if err := p.validateCommand(cmd); err != nil {
			audit.Log(p.auditLogPath, p.username, "redis_cluster_command_blocked", p.config.Name, map[string]interface{}{
				"connection_id": p.connectionID,
				"command":       cmd.String(),
				"reason":        err.Error(),
			})

			errorMsg := fmt.Sprintf("-ERR %s\r\n", err.Error())
			if _, writeErr := clientConn.Write([]byte(errorMsg)); writeErr != nil {
				return writeErr
			}
			continue
		}

		// Check approval if needed
		if err := p.checkApproval(cmd); err != nil {
			if _, writeErr := clientConn.Write([]byte(err.Error())); writeErr != nil {
				return writeErr
			}
			continue
		}

		// Log command
		audit.Log(p.auditLogPath, p.username, "redis_cluster_command", p.config.Name, map[string]interface{}{
			"connection_id": p.connectionID,
			"command":       cmd.String(),
		})

		// Execute command with cluster redirect handling
		if err := p.executeCommandWithRedirect(cmd, initialConn, clientConn); err != nil {
			return err
		}
	}
}

// executeCommandWithRedirect executes a command, handling MOVED/ASK redirects
func (p *RedisClusterProxy) executeCommandWithRedirect(cmd *RedisCommand, nodeConn net.Conn, clientConn net.Conn) error {
	maxRedirects := 5
	currentConn := nodeConn

	for i := 0; i < maxRedirects; i++ {
		// Send command to current node
		if _, err := currentConn.Write(cmd.Raw); err != nil {
			return fmt.Errorf("failed to send command to cluster node: %w", err)
		}

		// Read response
		reader := bufio.NewReader(currentConn)
		firstByte, err := reader.ReadByte()
		if err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}

		// Check if this is an error response (MOVED or ASK)
		if firstByte == '-' {
			// Read the error line
			line, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read error line: %w", err)
			}
			errorMsg := strings.TrimSpace(line)

			// Handle MOVED redirect
			if strings.HasPrefix(errorMsg, "MOVED") {
				newAddr, err := p.parseRedirectAddress(errorMsg)
				if err != nil {
					// Not a redirect error, send to client
					response := fmt.Sprintf("-%s\r\n", errorMsg)
					_, _ = clientConn.Write([]byte(response))
					return nil
				}

				audit.Log(p.auditLogPath, p.username, "redis_cluster_moved", p.config.Name, map[string]interface{}{
					"connection_id": p.connectionID,
					"command":       cmd.String(),
					"new_node":      newAddr,
				})

				// Connect to new node
				newConn, err := p.getOrCreateNodeConnection(newAddr)
				if err != nil {
					return fmt.Errorf("failed to connect to redirected node %s: %w", newAddr, err)
				}
				currentConn = newConn
				continue
			}

			// Handle ASK redirect
			if strings.HasPrefix(errorMsg, "ASK") {
				newAddr, err := p.parseRedirectAddress(errorMsg)
				if err != nil {
					// Not a redirect error, send to client
					response := fmt.Sprintf("-%s\r\n", errorMsg)
					_, _ = clientConn.Write([]byte(response))
					return nil
				}

				audit.Log(p.auditLogPath, p.username, "redis_cluster_ask", p.config.Name, map[string]interface{}{
					"connection_id": p.connectionID,
					"command":       cmd.String(),
					"asking_node":   newAddr,
				})

				// Connect to asking node
				askConn, err := p.getOrCreateNodeConnection(newAddr)
				if err != nil {
					return fmt.Errorf("failed to connect to ASK node %s: %w", newAddr, err)
				}

				// Send ASKING command
				askingCmd := "*1\r\n$6\r\nASKING\r\n"
				if _, err := askConn.Write([]byte(askingCmd)); err != nil {
					return fmt.Errorf("failed to send ASKING command: %w", err)
				}

				// Read ASKING response (should be +OK)
				askReader := bufio.NewReader(askConn)
				if _, err := askReader.ReadString('\n'); err != nil {
					return fmt.Errorf("failed to read ASKING response: %w", err)
				}

				currentConn = askConn
				continue
			}

			// Other error, send to client
			response := fmt.Sprintf("-%s\r\n", errorMsg)
			_, _ = clientConn.Write([]byte(response))
			return nil
		}

		// Not an error, forward the response to client
		// We already read the first byte, write it back
		if _, err := clientConn.Write([]byte{firstByte}); err != nil {
			return err
		}

		// Copy the rest of the response
		// For simplicity, we'll read and forward the entire response
		// This works for most commands but may need refinement for large responses
		if _, err := io.Copy(clientConn, reader); err != nil && err != io.EOF {
			return err
		}

		return nil
	}

	return fmt.Errorf("exceeded maximum redirects (%d)", maxRedirects)
}

// parseRedirectAddress extracts the node address from MOVED/ASK error
// Format: "MOVED 3999 127.0.0.1:6381" or "ASK 3999 127.0.0.1:6381"
func (p *RedisClusterProxy) parseRedirectAddress(errorMsg string) (string, error) {
	parts := strings.Fields(errorMsg)
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid redirect message format")
	}
	return parts[2], nil
}

// getOrCreateNodeConnection gets or creates a connection to a cluster node
func (p *RedisClusterProxy) getOrCreateNodeConnection(addr string) (net.Conn, error) {
	p.mu.RLock()
	conn, exists := p.nodeConns[addr]
	p.mu.RUnlock()

	if exists && conn != nil {
		return conn, nil
	}

	// Create new connection
	newConn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, err
	}

	// Authenticate if password is configured
	if p.config.BackendPassword != "" {
		if err := p.authenticateNode(newConn); err != nil {
			newConn.Close()
			return nil, err
		}
	}

	p.mu.Lock()
	p.nodeConns[addr] = newConn
	p.mu.Unlock()

	return newConn, nil
}

// authenticateNode sends AUTH command to a cluster node
func (p *RedisClusterProxy) authenticateNode(conn net.Conn) error {
	authCmd := fmt.Sprintf("*2\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n", len(p.config.BackendPassword), p.config.BackendPassword)
	if _, err := conn.Write([]byte(authCmd)); err != nil {
		return err
	}

	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		return err
	}

	if !strings.HasPrefix(response, "+OK") {
		return fmt.Errorf("AUTH failed: %s", response)
	}

	return nil
}

// closeAllNodes closes all cluster node connections
func (p *RedisClusterProxy) closeAllNodes() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for addr, conn := range p.nodeConns {
		if conn != nil {
			conn.Close()
			audit.Log(p.auditLogPath, p.username, "redis_cluster_node_disconnect", p.config.Name, map[string]interface{}{
				"connection_id": p.connectionID,
				"node":          addr,
			})
		}
	}
	p.nodeConns = make(map[string]net.Conn)
}

// validateCommand checks if a command is allowed by the whitelist
func (p *RedisClusterProxy) validateCommand(cmd *RedisCommand) error {
	if len(p.whitelist) == 0 {
		return nil
	}

	for _, pattern := range p.whitelist {
		if matchesRedisPattern(pattern, cmd) {
			return nil
		}
	}

	return fmt.Errorf("command not allowed by whitelist")
}

// checkApproval checks if approval is required and waits for it
func (p *RedisClusterProxy) checkApproval(cmd *RedisCommand) error {
	p.mu.RLock()
	approvalMgr := p.approvalMgr
	p.mu.RUnlock()

	if approvalMgr == nil {
		return nil
	}

	requiresApproval, timeout := approvalMgr.RequiresApproval(cmd.String(), "", p.config.Tags)
	if !requiresApproval {
		return nil
	}

	// Create approval request
	approvalReq := &approval.Request{
		Username:     p.username,
		ConnectionID: p.connectionID,
		Method:       cmd.String(), // Redis command as "method"
		Path:         "",           // No path for Redis commands
		Metadata: map[string]string{
			"connection_name": p.config.Name,
			"connection_type": p.config.Type,
			"cluster_mode":    "true",
		},
	}

	audit.Log(p.auditLogPath, p.username, "redis_cluster_command_awaiting_approval", p.config.Name, map[string]interface{}{
		"connection_id": p.connectionID,
		"command":       cmd.String(),
		"timeout":       timeout.String(),
	})

	// Wait for approval with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	approvalResp, err := approvalMgr.RequestApproval(ctx, approvalReq, timeout)
	if err != nil {
		audit.Log(p.auditLogPath, p.username, "redis_cluster_approval_error", p.config.Name, map[string]interface{}{
			"connection_id": p.connectionID,
			"command":       cmd.String(),
			"error":         err.Error(),
		})
		return fmt.Errorf("-ERR Approval request failed: %s\r\n", err.Error())
	}

	// Check approval decision
	if approvalResp.Decision != approval.DecisionApproved {
		audit.Log(p.auditLogPath, p.username, "redis_cluster_command_rejected", p.config.Name, map[string]interface{}{
			"connection_id": p.connectionID,
			"command":       cmd.String(),
			"request_id":    approvalResp.RequestID,
			"decision":      approvalResp.Decision,
		})
		return fmt.Errorf("-ERR Command rejected or timed out\r\n")
	}

	audit.Log(p.auditLogPath, p.username, "redis_cluster_command_approved", p.config.Name, map[string]interface{}{
		"connection_id": p.connectionID,
		"command":       cmd.String(),
		"request_id":    approvalResp.RequestID,
		"approver":      approvalResp.ApprovedBy,
	})

	return nil
}
