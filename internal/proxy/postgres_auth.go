package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
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
	"github.com/xdg-go/scram"
)

// PostgresAuthProxy handles postgres with credential substitution
type PostgresAuthProxy struct {
	config       *config.ConnectionConfig
	auditLogPath string
	username     string
	connectionID string
	apiConfig    *config.Config
	whitelist    []string
	approvalMgr  *approval.Manager
}

// NewPostgresAuthProxy creates a postgres proxy with auth handling
func NewPostgresAuthProxy(cfg *config.ConnectionConfig, auditLogPath, username, connectionID string, apiConfig *config.Config, whitelist []string) *PostgresAuthProxy {
	return &PostgresAuthProxy{
		config:       cfg,
		auditLogPath: auditLogPath,
		username:     username,
		connectionID: connectionID,
		apiConfig:    apiConfig,
		whitelist:    whitelist,
		approvalMgr:  nil, // Will be set later if approvals are enabled
	}
}

// SetApprovalManager sets the approval manager for this proxy
func (p *PostgresAuthProxy) SetApprovalManager(mgr *approval.Manager) {
	p.approvalMgr = mgr
}

// HandleConnection handles the full postgres connection with auth
func (p *PostgresAuthProxy) HandleConnection(clientConn net.Conn) error {
	defer func() { _ = clientConn.Close() }()

	// Read startup message from client (might be SSL request first)
	startupMsg, err := p.readStartupMessage(clientConn)
	if err != nil {
		return fmt.Errorf("failed to read startup message: %w", err)
	}

	// Check if this is an SSL request (protocol 80877103)
	if len(startupMsg) >= 8 {
		protocol := binary.BigEndian.Uint32(startupMsg[4:8])
		if protocol == 80877103 {
			// Reject SSL - send 'N'
			_, _ = clientConn.Write([]byte{'N'})

			// Now read the real startup message
			startupMsg, err = p.readStartupMessage(clientConn)
			if err != nil {
				return fmt.Errorf("failed to read startup message after SSL: %w", err)
			}
		}
	}

	// Parse parameters from startup
	params, database := p.parseStartupParams(startupMsg)
	clientUser := params["user"]

	// Request password from client (cleartext for simplicity)
	if err := p.sendAuthRequest(clientConn); err != nil {
		return err
	}

	// Read password from client (but don't validate it - JWT already authenticated the user)
	_, err = p.readPassword(clientConn)
	if err != nil {
		return err
	}

	// SECURITY: Enforce that psql username matches authenticated API username
	if clientUser != p.username {
		p.sendAuthError(clientConn, "Username mismatch: you must connect as your authenticated user")
		_ = audit.Log(p.auditLogPath, p.username, "postgres_auth_failed", p.config.Name, map[string]interface{}{
			"connection_id": p.connectionID,
			"client_user":   clientUser,
			"expected_user": p.username,
			"reason":        "username_mismatch",
		})
		return fmt.Errorf("username mismatch: client=%s, authenticated=%s", clientUser, p.username)
	}

	// NOTE: Password validation is SKIPPED because authentication already happened at the API/JWT level
	// The connection was established with a valid JWT token, so the user is already authenticated.
	// We accept any password here since the real authentication is the JWT token.
	// This allows OIDC/SAML users (who don't have local passwords) to connect.
	_ = audit.Log(p.auditLogPath, p.username, "postgres_client_auth", p.config.Name, map[string]interface{}{
		"connection_id": p.connectionID,
		"client_user":   clientUser,
		"note":          "password validation skipped - already authenticated via JWT",
	})

	// Connect to backend with BACKEND credentials
	backendAddr := fmt.Sprintf("%s:%d", p.config.Host, p.config.Port)
	backendConn, err := net.DialTimeout("tcp", backendAddr, 10*time.Second)
	if err != nil {
		p.sendAuthError(clientConn, "Backend connection failed")
		return fmt.Errorf("failed to connect to backend: %w", err)
	}
	defer func() { _ = backendConn.Close() }()

	// Send startup to backend with BACKEND username
	backendDB := p.config.BackendDatabase
	if backendDB == "" {
		backendDB = database
	}
	if err := p.sendBackendStartup(backendConn, p.config.BackendUsername, backendDB); err != nil {
		return err
	}

	// Handle backend authentication
	if err := p.handleBackendAuth(backendConn, p.config.BackendPassword); err != nil {
		p.sendAuthError(clientConn, "Backend authentication failed")
		return fmt.Errorf("backend auth failed: %w", err)
	}

	// Send success to client
	if err := p.sendAuthSuccess(clientConn); err != nil {
		return err
	}

	_ = audit.Log(p.auditLogPath, p.username, "postgres_auth", p.config.Name, map[string]interface{}{
		"connection_id": p.connectionID,
		"client_user":   clientUser,
		"database":      database,
		"status":        "authenticated",
	})

	// Now do transparent bidirectional forwarding with query logging
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		defer func() { _ = backendConn.Close() }()
		p.forwardWithLogging(clientConn, backendConn, true)
	}()

	go func() {
		defer wg.Done()
		defer func() { _ = clientConn.Close() }()
		p.forwardWithLogging(backendConn, clientConn, false)
	}()

	wg.Wait()
	return nil
}

// readStartupMessage reads the postgres startup message
func (p *PostgresAuthProxy) readStartupMessage(conn net.Conn) ([]byte, error) {
	// First 4 bytes are length
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(lenBuf)
	if length < 8 || length > 10000 {
		return nil, fmt.Errorf("invalid startup message length: %d", length)
	}

	// Read the rest of the message (length includes itself)
	msgBuf := make([]byte, length-4)
	if _, err := io.ReadFull(conn, msgBuf); err != nil {
		return nil, err
	}

	// Combine length + message
	fullMsg := append(lenBuf, msgBuf...)
	return fullMsg, nil
}

// parseStartupParams extracts parameters from startup message
func (p *PostgresAuthProxy) parseStartupParams(msg []byte) (map[string]string, string) {
	params := make(map[string]string)
	database := ""

	// Skip length (4 bytes) and protocol version (4 bytes)
	data := msg[8:]

	// Parse null-terminated key-value pairs
	for len(data) > 0 {
		// Find null terminator
		nullIdx := bytes.IndexByte(data, 0)
		if nullIdx == -1 {
			break
		}
		key := string(data[:nullIdx])
		data = data[nullIdx+1:]

		if len(data) == 0 {
			break
		}

		nullIdx = bytes.IndexByte(data, 0)
		if nullIdx == -1 {
			break
		}
		value := string(data[:nullIdx])
		data = data[nullIdx+1:]

		params[key] = value
		if key == "database" {
			database = value
		}
	}

	return params, database
}

// sendAuthRequest sends cleartext password request
func (p *PostgresAuthProxy) sendAuthRequest(conn net.Conn) error {
	// AuthenticationCleartextPassword: 'R' + length(8) + type(3)
	msg := []byte{'R', 0, 0, 0, 8, 0, 0, 0, 3}
	_, err := conn.Write(msg)
	return err
}

// readPassword reads password message from client
func (p *PostgresAuthProxy) readPassword(conn net.Conn) (string, error) {
	// Read message type
	typeBuf := make([]byte, 1)
	if _, err := io.ReadFull(conn, typeBuf); err != nil {
		return "", err
	}

	if typeBuf[0] != 'p' {
		return "", fmt.Errorf("expected password message, got: %c", typeBuf[0])
	}

	// Read length
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return "", err
	}

	length := binary.BigEndian.Uint32(lenBuf)
	if length < 5 || length > 1000 {
		return "", fmt.Errorf("invalid password length: %d", length)
	}

	// Read password (null-terminated)
	pwdBuf := make([]byte, length-4)
	if _, err := io.ReadFull(conn, pwdBuf); err != nil {
		return "", err
	}

	password := string(bytes.TrimRight(pwdBuf, "\x00"))
	return password, nil
}

// sendBackendStartup sends startup message to backend with backend credentials
func (p *PostgresAuthProxy) sendBackendStartup(conn net.Conn, username, database string) error {
	// Build startup message
	var buf bytes.Buffer

	// Protocol version 3.0
	_ = binary.Write(&buf, binary.BigEndian, uint32(196608))

	// Parameters
	buf.WriteString("user")
	buf.WriteByte(0)
	buf.WriteString(username)
	buf.WriteByte(0)

	buf.WriteString("database")
	buf.WriteByte(0)
	buf.WriteString(database)
	buf.WriteByte(0)

	// End marker
	buf.WriteByte(0)

	// Prepend length
	msgLen := buf.Len() + 4
	fullMsg := make([]byte, 4)
	binary.BigEndian.PutUint32(fullMsg, uint32(msgLen))
	fullMsg = append(fullMsg, buf.Bytes()...)

	_, err := conn.Write(fullMsg)
	return err
}

// handleBackendAuth handles backend authentication flow
func (p *PostgresAuthProxy) handleBackendAuth(conn net.Conn, password string) error {
	reader := bufio.NewReader(conn)

	for {
		// Read message type
		msgType, err := reader.ReadByte()
		if err != nil {
			return err
		}

		// Read length
		lenBuf := make([]byte, 4)
		if _, err := io.ReadFull(reader, lenBuf); err != nil {
			return err
		}
		length := binary.BigEndian.Uint32(lenBuf)

		// Read message body
		body := make([]byte, length-4)
		if _, err := io.ReadFull(reader, body); err != nil {
			return err
		}

		switch msgType {
		case 'R': // Authentication request
			if len(body) >= 4 {
				authType := binary.BigEndian.Uint32(body[:4])
				switch authType {
				case 0:
					// Auth OK, continue
				case 3:
					// Cleartext password requested
					if err := p.sendBackendPassword(conn, password); err != nil {
						return err
					}
				case 5:
					// MD5 password requested
					if len(body) < 8 {
						return fmt.Errorf("invalid MD5 auth message")
					}
					salt := body[4:8]
					if err := p.sendBackendPasswordMD5(conn, password, p.config.BackendUsername, salt); err != nil {
						return err
					}
				case 10:
					// SCRAM-SHA-256 requested
					if err := p.handleSCRAMAuth(conn, reader, password, p.config.BackendUsername); err != nil {
						return err
					}
				default:
					return fmt.Errorf("unsupported auth type: %d", authType)
				}
			}

		case 'Z': // ReadyForQuery
			return nil // Backend ready

		case 'E': // ErrorResponse
			return fmt.Errorf("backend auth error: %s", string(body))

		case 'S', 'K': // ParameterStatus, BackendKeyData
			// Ignore these
			continue
		}
	}
}

// sendBackendPassword sends password to backend
func (p *PostgresAuthProxy) sendBackendPassword(conn net.Conn, password string) error {
	var buf bytes.Buffer
	buf.WriteByte('p')                                                // Message type
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(password)+5)) // Length
	buf.WriteString(password)
	buf.WriteByte(0)

	_, err := conn.Write(buf.Bytes())
	return err
}

// sendBackendPasswordMD5 sends MD5-hashed password to backend
func (p *PostgresAuthProxy) sendBackendPasswordMD5(conn net.Conn, password, username string, salt []byte) error {
	// PostgreSQL MD5 auth: "md5" + md5(md5(password + username) + salt)

	// First hash: md5(password + username)
	hasher := md5.New()
	hasher.Write([]byte(password))
	hasher.Write([]byte(username))
	hash1 := hex.EncodeToString(hasher.Sum(nil))

	// Second hash: md5(hash1 + salt)
	hasher.Reset()
	hasher.Write([]byte(hash1))
	hasher.Write(salt)
	hash2 := hex.EncodeToString(hasher.Sum(nil))

	// Final format: "md5" + hash2
	finalHash := "md5" + hash2

	var buf bytes.Buffer
	buf.WriteByte('p')                                                 // Message type
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(finalHash)+5)) // Length
	buf.WriteString(finalHash)
	buf.WriteByte(0)

	_, err := conn.Write(buf.Bytes())
	return err
}

// handleSCRAMAuth handles SCRAM-SHA-256 authentication
func (p *PostgresAuthProxy) handleSCRAMAuth(conn net.Conn, reader *bufio.Reader, password, username string) error {
	// Create SCRAM client
	client, err := scram.SHA256.NewClient(username, password, "")
	if err != nil {
		return fmt.Errorf("failed to create SCRAM client: %w", err)
	}

	conv := client.NewConversation()

	// Generate initial client message
	clientFirst, err := conv.Step("")
	if err != nil {
		return fmt.Errorf("SCRAM step 1 failed: %w", err)
	}

	// Send SASL Initial Response
	var buf bytes.Buffer
	buf.WriteByte('p') // PasswordMessage type

	// Calculate total message length
	mechanism := "SCRAM-SHA-256"
	// Length = 4 (length itself) + len(mechanism) + 1 (null) + 4 (clientFirst length) + len(clientFirst)
	totalLen := 4 + len(mechanism) + 1 + 4 + len(clientFirst)

	_ = binary.Write(&buf, binary.BigEndian, int32(totalLen))
	buf.WriteString(mechanism)
	buf.WriteByte(0) // Null terminator
	_ = binary.Write(&buf, binary.BigEndian, int32(len(clientFirst)))
	buf.WriteString(clientFirst)

	if _, err := conn.Write(buf.Bytes()); err != nil {
		return err
	}

	// Read server-first-message
	msgType, err := reader.ReadByte()
	if err != nil {
		return err
	}

	if msgType == 'E' {
		// Error response
		return fmt.Errorf("SCRAM auth error from server")
	}

	if msgType != 'R' {
		return fmt.Errorf("unexpected message type during SCRAM: %c", msgType)
	}

	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(reader, lenBuf); err != nil {
		return err
	}
	length := binary.BigEndian.Uint32(lenBuf)

	body := make([]byte, length-4)
	if _, err := io.ReadFull(reader, body); err != nil {
		return err
	}

	// Check if this is AuthenticationSASLContinue (type 11)
	if len(body) < 4 || binary.BigEndian.Uint32(body[:4]) != 11 {
		return fmt.Errorf("expected SASL Continue")
	}

	serverFirst := string(body[4:])

	// Process server-first and generate client-final
	clientFinal, err := conv.Step(serverFirst)
	if err != nil {
		return fmt.Errorf("SCRAM step 2 failed: %w", err)
	}

	// Send client-final-message
	buf.Reset()
	buf.WriteByte('p')
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(clientFinal)+4))
	buf.WriteString(clientFinal)

	if _, err := conn.Write(buf.Bytes()); err != nil {
		return err
	}

	// Read server-final-message
	msgType, err = reader.ReadByte()
	if err != nil {
		return err
	}

	if msgType == 'E' {
		return fmt.Errorf("SCRAM auth failed")
	}

	if msgType != 'R' {
		return fmt.Errorf("unexpected message type during SCRAM final: %c", msgType)
	}

	if _, err := io.ReadFull(reader, lenBuf); err != nil {
		return err
	}
	length = binary.BigEndian.Uint32(lenBuf)

	body = make([]byte, length-4)
	if _, err := io.ReadFull(reader, body); err != nil {
		return err
	}

	// Check if this is AuthenticationSASLFinal (type 12)
	if len(body) < 4 || binary.BigEndian.Uint32(body[:4]) != 12 {
		return fmt.Errorf("expected SASL Final")
	}

	serverFinal := string(body[4:])

	// Validate server signature
	if _, err := conv.Step(serverFinal); err != nil {
		return fmt.Errorf("SCRAM validation failed: %w", err)
	}

	return nil
}

// sendAuthSuccess sends authentication success to client
func (p *PostgresAuthProxy) sendAuthSuccess(conn net.Conn) error {
	var buf bytes.Buffer

	// AuthenticationOk
	buf.WriteByte('R')
	_ = binary.Write(&buf, binary.BigEndian, int32(8))
	_ = binary.Write(&buf, binary.BigEndian, int32(0))

	// ParameterStatus messages
	params := map[string]string{
		"server_version":  "14.0",
		"server_encoding": "UTF8",
		"client_encoding": "UTF8",
		"DateStyle":       "ISO, MDY",
		"TimeZone":        "UTC",
	}

	for key, value := range params {
		buf.WriteByte('S')
		paramData := key + "\x00" + value + "\x00"
		_ = binary.Write(&buf, binary.BigEndian, int32(len(paramData)+4))
		buf.WriteString(paramData)
	}

	// BackendKeyData (dummy)
	buf.WriteByte('K')
	_ = binary.Write(&buf, binary.BigEndian, int32(12))
	_ = binary.Write(&buf, binary.BigEndian, int32(12345)) // process ID
	_ = binary.Write(&buf, binary.BigEndian, int32(67890)) // secret key

	// ReadyForQuery
	buf.WriteByte('Z')
	_ = binary.Write(&buf, binary.BigEndian, int32(5))
	buf.WriteByte('I') // Idle status

	_, err := conn.Write(buf.Bytes())
	return err
}

// sendAuthError sends authentication error to client
func (p *PostgresAuthProxy) sendAuthError(conn net.Conn, message string) {
	// ErrorResponse message
	var buf bytes.Buffer
	buf.WriteByte('E')

	// Build error fields
	fields := fmt.Sprintf("SFATAL\x00C28P01\x00M%s\x00\x00", message)
	length := len(fields) + 4

	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(length))

	buf.Write(lenBuf)
	buf.WriteString(fields)

	_, _ = conn.Write(buf.Bytes())
}

// validateAPICredentials checks API username/password
//
//nolint:unused // Reserved for future API credential validation
func (p *PostgresAuthProxy) validateAPICredentials(username, password string) bool {
	if p.apiConfig == nil {
		return false
	}

	for _, user := range p.apiConfig.Auth.Users {
		if user.Username == username && user.Password == password {
			return true
		}
	}
	return false
}

// forwardWithLogging forwards data and logs/validates queries
func (p *PostgresAuthProxy) forwardWithLogging(src, dst net.Conn, logQueries bool) {
	buf := make([]byte, 32*1024)

	for {
		n, err := src.Read(buf)
		if n > 0 {
			data := buf[:n]

			if logQueries {
				// Validate queries against whitelist before forwarding
				if blocked, query := p.validateAndLogQuery(data); blocked {
					// Send error to client and don't forward to backend
					p.sendQueryBlockedError(src, query)
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

// validateAndLogQuery extracts queries, validates against whitelist, checks approval, and logs
// Returns (blocked, query) where blocked=true if query should be blocked
func (p *PostgresAuthProxy) validateAndLogQuery(data []byte) (bool, string) {
	for i := 0; i < len(data); i++ {
		// Check for both Simple Query ('Q') and Extended Query Parse ('P') messages
		msgType := data[i]
		if (msgType == 'Q' || msgType == 'P') && i+5 < len(data) {
			length := int(binary.BigEndian.Uint32(data[i+1 : i+5]))

			if i+1+length <= len(data) && length > 4 {
				var query string

				switch msgType {
				case 'Q':
					// Simple Query: query string starts at i+5
					queryBytes := data[i+5 : i+1+length]
					query = string(bytes.TrimRight(queryBytes, "\x00"))
				case 'P':
					// Parse message: format is statement_name (null-terminated) + query (null-terminated)
					// Skip statement name to get to query
					nameStart := i + 5
					nameEnd := nameStart
					for nameEnd < i+1+length && data[nameEnd] != 0 {
						nameEnd++
					}
					if nameEnd < i+1+length {
						nameEnd++ // Skip null terminator
						queryBytes := data[nameEnd : i+1+length]
						query = string(bytes.TrimRight(queryBytes, "\x00"))
					}
				}

				if query != "" {
					// Check whitelist first
					allowed := p.isQueryAllowed(query)

					// Log the query with whitelist result
					_ = audit.Log(p.auditLogPath, p.username, "postgres_query", p.config.Name, map[string]interface{}{
						"connection_id": p.connectionID,
						"query":         query,
						"database":      p.config.BackendDatabase,
						"allowed":       allowed,
						"whitelist":     len(p.whitelist) > 0,
						"message_type":  string(msgType),
					})

					if !allowed {
						// Log blocked query
						_ = audit.Log(p.auditLogPath, p.username, "postgres_query_blocked", p.config.Name, map[string]interface{}{
							"connection_id": p.connectionID,
							"query":         query,
							"reason":        "whitelist_violation",
						})
						return true, query
					}

					// Check if approval is required for this query
					if p.approvalMgr != nil {
						normalizedQuery := strings.TrimSpace(query)
						requiresApproval, timeout := p.approvalMgr.RequiresApproval(normalizedQuery, "", p.config.Tags)
						if requiresApproval {
							// Request approval
							approvalReq := &approval.Request{
								Username:     p.username,
								ConnectionID: p.connectionID,
								Method:       normalizedQuery, // For postgres, query is the "method"
								Path:         "",              // No path for SQL queries
								Metadata: map[string]string{
									"connection_name": p.config.Name,
									"connection_type": p.config.Type,
									"database":        p.config.BackendDatabase,
								},
							}

							// Log approval request
							_ = audit.Log(p.auditLogPath, p.username, "postgres_approval_requested", p.config.Name, map[string]interface{}{
								"connection_id": p.connectionID,
								"query":         query,
								"database":      p.config.BackendDatabase,
								"timeout":       timeout.String(),
							})

							// Wait for approval with timeout
							ctx, cancel := context.WithTimeout(context.Background(), timeout)
							defer cancel()

							approvalResp, err := p.approvalMgr.RequestApproval(ctx, approvalReq, timeout)
							if err != nil {
								// Log approval error
								_ = audit.Log(p.auditLogPath, p.username, "postgres_approval_error", p.config.Name, map[string]interface{}{
									"connection_id": p.connectionID,
									"query":         query,
									"error":         err.Error(),
								})
								return true, query
							}

							// Check approval decision
							if approvalResp.Decision != approval.DecisionApproved {
								// Log rejection/timeout
								_ = audit.Log(p.auditLogPath, p.username, "postgres_approval_rejected", p.config.Name, map[string]interface{}{
									"connection_id": p.connectionID,
									"query":         query,
									"decision":      approvalResp.Decision,
									"reason":        approvalResp.Reason,
									"rejected_by":   approvalResp.ApprovedBy,
								})
								return true, query
							}

							// Log approval success
							_ = audit.Log(p.auditLogPath, p.username, "postgres_approval_granted", p.config.Name, map[string]interface{}{
								"connection_id": p.connectionID,
								"query":         query,
								"database":      p.config.BackendDatabase,
								"approved_by":   approvalResp.ApprovedBy,
							})
						}
					}
				}

				i += length
			}
		}
	}
	return false, ""
}

// isQueryAllowed checks if a query matches the whitelist patterns (case-insensitive)
func (p *PostgresAuthProxy) isQueryAllowed(query string) bool {
	// If no whitelist, allow everything (backward compatibility)
	if len(p.whitelist) == 0 {
		return true
	}

	// Check each whitelist pattern (case-insensitive)
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
		if re.MatchString(query) {
			return true
		}
	}

	return false
}

// sendQueryBlockedError sends a proper PostgreSQL error response to the client for blocked queries
func (p *PostgresAuthProxy) sendQueryBlockedError(conn net.Conn, query string) {
	// Truncate query if too long
	displayQuery := query
	if len(displayQuery) > 100 {
		displayQuery = displayQuery[:100] + "..."
	}

	// Build PostgreSQL ErrorResponse message
	var buf bytes.Buffer

	// Message type 'E' for ErrorResponse
	buf.WriteByte('E')

	// Build error fields according to PostgreSQL protocol
	// S = Severity, C = SQLSTATE code, M = Message
	var fields bytes.Buffer
	fields.WriteString("SERROR\x00") // Severity: ERROR
	fields.WriteString("C42501\x00") // SQLSTATE: insufficient_privilege
	fields.WriteString(fmt.Sprintf("MQuery blocked by whitelist policy: %s\x00", displayQuery))
	fields.WriteString("HCheck your role's whitelist patterns in the configuration.\x00") // Hint
	fields.WriteByte(0)                                                                   // Null terminator for fields

	// Write message length (includes the length field itself)
	msgLength := uint32(4 + fields.Len())
	_ = binary.Write(&buf, binary.BigEndian, msgLength)

	// Write fields
	buf.Write(fields.Bytes())

	// Send complete error message to client
	_, _ = conn.Write(buf.Bytes())

	// Now send ReadyForQuery to indicate we're ready for next command
	// This prevents client from hanging
	var readyBuf bytes.Buffer
	readyBuf.WriteByte('Z')                                 // ReadyForQuery message type
	_ = binary.Write(&readyBuf, binary.BigEndian, int32(5)) // Length
	readyBuf.WriteByte('I')                                 // Transaction status: Idle

	_, _ = conn.Write(readyBuf.Bytes())
}
